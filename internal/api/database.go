package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/metrics"
	"github.com/popelev/level2/internal/store"
)

func (s *Server) mountDatabase(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/database/status", s.handleDatabaseStatus)
	mux.HandleFunc("GET /api/v1/database/capacity-policy", s.handleGetCapacityPolicy)
	mux.HandleFunc("PUT /api/v1/database/capacity-policy", s.handlePutCapacityPolicy)
	mux.HandleFunc("POST /api/v1/database/wipe-samples", s.handleWipeSamples)
}

func (s *Server) handleDatabaseStatus(w http.ResponseWriter, r *http.Request) {
	dbURL := ""
	if s.Cfg != nil {
		snap := s.Cfg.Snapshot()
		dbURL = snap.Database.URL
	}
	if s.DB == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"connected":  false,
			"ping_ok":    false,
			"ready":      false,
			"ping_error": "historian not configured",
			"url_masked": timescale.MaskDatabaseURL(dbURL),
		})
		return
	}
	st := s.DB.Status(r.Context(), dbURL)
	writeJSON(w, http.StatusOK, map[string]any{
		"connected":           st.Connected,
		"ping_ok":             st.PingOK,
		"ready":               st.PingOK,
		"ping_error":          st.PingError,
		"url_masked":          st.URLMasked,
		"host":                st.Host,
		"port":                st.Port,
		"database":            st.Database,
		"user":                st.User,
		"sslmode":             st.SSLMode,
		"server_version":      st.ServerVersion,
		"timescale_version":   st.TimescaleVer,
		"pool_max_conns":      st.PoolMaxConns,
		"pool_total_conns":    st.PoolTotalConns,
		"pool_idle_conns":     st.PoolIdleConns,
		"pool_acquired_conns": st.PoolAcquired,
	})
}

func (s *Server) handleCapacity(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	tagCount := 0
	if s.Tags != nil {
		tagCount = len(s.Tags())
	}

	out := map[string]any{
		"tag_count": tagCount,
		"metrics": map[string]float64{
			"samples_written_total": counterValue(metrics.SamplesWritten),
			"samples_spooled_total": counterValue(metrics.SamplesSpooled),
			"write_errors_total":    counterValue(metrics.WriteErrors),
			"spool_depth":           gaugeValue(metrics.SpoolDepth),
			"capacity_halts_total":  counterValue(metrics.CapacityHalts),
			"capacity_drops_total":  counterValue(metrics.CapacityDrops),
		},
		"collector_memory": map[string]any{
			"alloc_bytes":      mem.Alloc,
			"sys_bytes":        mem.Sys,
			"heap_alloc_bytes": mem.HeapAlloc,
			"num_gc":           mem.NumGC,
		},
	}

	if s.DB == nil {
		out["error"] = "historian not configured"
		writeJSON(w, http.StatusOK, out)
		return
	}

	cap, err := s.DB.Capacity(r.Context())
	if err != nil {
		out["error"] = err.Error()
		writeJSON(w, http.StatusOK, out)
		return
	}

	// Prefer process counter rate when DB window is empty (cold start / sparse writes).
	if rate := samplesWrittenRate(); rate > 0 && cap.SamplesPerSec <= 0 {
		cap.SamplesPerSec = rate
		cap.GrowthBytesPerSec = rate * cap.AvgSampleBytes
		if cap.FreeBytes != nil && cap.GrowthBytesPerSec > 0 {
			eta := float64(*cap.FreeBytes) / cap.GrowthBytesPerSec
			cap.ETASeconds = &eta
		}
	}

	out["database_size_bytes"] = cap.DatabaseSizeBytes
	out["samples_size_bytes"] = cap.SamplesSizeBytes
	out["samples_approx_rows"] = cap.SamplesApproxRows
	out["samples_last_5min"] = cap.SamplesLast5Min
	out["samples_per_sec"] = cap.SamplesPerSec
	out["avg_sample_bytes"] = cap.AvgSampleBytes
	out["growth_bytes_per_sec"] = cap.GrowthBytesPerSec
	out["free_bytes"] = cap.FreeBytes
	out["free_bytes_source"] = cap.FreeBytesSource
	out["capacity_bytes"] = cap.CapacityBytes
	out["disk_path"] = cap.DiskPath
	out["disk_avail_bytes"] = cap.DiskAvailBytes
	out["disk_total_bytes"] = cap.DiskTotalBytes
	out["limit_bytes"] = cap.LimitBytes
	out["capacity_percent"] = cap.CapacityPercent
	out["full_policy"] = cap.FullPolicy
	out["used_over_limit"] = cap.UsedOverLimit
	out["eta_seconds"] = cap.ETASeconds
	out["window_seconds"] = cap.WindowSeconds
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetCapacityPolicy(w http.ResponseWriter, r *http.Request) {
	percent := 90
	policy := "stop"
	if s.Cfg != nil {
		snap := s.Cfg.Snapshot()
		if snap.Database.CapacityPercent > 0 {
			percent = snap.Database.CapacityPercent
		}
		if snap.Database.FullPolicy != "" {
			policy = snap.Database.FullPolicy
		}
	}
	if s.DB != nil {
		live := s.DB.CapacityPolicy()
		percent = live.Percent
		policy = live.Policy
	}
	out := map[string]any{
		"capacity_percent": percent,
		"full_policy":      policy,
	}
	if s.DB != nil {
		if cap, err := s.DB.Capacity(r.Context()); err == nil {
			out["disk_path"] = cap.DiskPath
			out["disk_avail_bytes"] = cap.DiskAvailBytes
			out["disk_total_bytes"] = cap.DiskTotalBytes
			out["limit_bytes"] = cap.LimitBytes
			out["database_size_bytes"] = cap.DatabaseSizeBytes
			out["used_over_limit"] = cap.UsedOverLimit
			out["free_bytes"] = cap.FreeBytes
			out["free_bytes_source"] = cap.FreeBytesSource
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type capacityPolicyBody struct {
	CapacityPercent int    `json:"capacity_percent"`
	FullPolicy      string `json:"full_policy"`
}

func (s *Server) handlePutCapacityPolicy(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	var body capacityPolicyBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := s.Cfg.SetCapacityPolicy(body.CapacityPercent, body.FullPolicy); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	snap := s.Cfg.Snapshot()
	if s.DB != nil {
		s.DB.SetCapacityPolicy(snap.Database.CapacityPercent, snap.Database.FullPolicy)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"capacity_percent": snap.Database.CapacityPercent,
		"full_policy":      snap.Database.FullPolicy,
		"status":           "saved",
	})
}

type wipeSamplesBody struct {
	ClearTags bool `json:"clear_tags"`
}

// handleWipeSamples truncates historian time-series (collector.samples).
// Requires ?confirm=wipe. Optional JSON body {"clear_tags":true} also clears
// monitored tags on every device (config only — same as Projects → Clear tags).
// After a successful wipe: snapshot Live → clear Live → WriteBatch the snapshot
// so Timescale is re-seeded immediately and FanIn will not suppress the next poll.
func (s *Server) handleWipeSamples(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("confirm") != "wipe" {
		http.Error(w, `missing confirm=wipe query parameter`, http.StatusBadRequest)
		return
	}
	if s.DB == nil {
		http.Error(w, "historian not configured", http.StatusServiceUnavailable)
		return
	}

	var body wipeSamplesBody
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
	}

	result, err := s.DB.WipeSamples(r.Context())
	if err != nil {
		diag.DBWrite(diag.LevelError, "historian wipe samples failed", err.Error(), 0)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tagsRemoved := 0
	devicesCleared := 0
	if body.ClearTags {
		if s.Cfg == nil {
			http.Error(w, "samples wiped but config store unavailable for clear_tags", http.StatusInternalServerError)
			return
		}
		for _, d := range s.Cfg.Devices() {
			n, err := s.Cfg.ClearDeviceTags(d.ID)
			if err != nil {
				diag.DBWrite(diag.LevelError, "historian wipe clear_tags failed", err.Error(), tagsRemoved)
				http.Error(w, fmt.Sprintf("samples wiped (%s) but clear_tags failed on %s: %v", result.Method, d.ID, err), http.StatusInternalServerError)
				return
			}
			tagsRemoved += n
			devicesCleared++
			if s.OnDeviceChanged != nil {
				s.OnDeviceChanged(d.ID, false)
			}
		}
	}

	liveCleared, reseeded, reseedErr := reseedAfterWipe(r.Context(), s.Live, s.DB.WriteBatch)
	if reseedErr != nil {
		diag.DBWrite(diag.LevelError, "historian wipe reseed failed", reseedErr.Error(), liveCleared)
	}

	detail := fmt.Sprintf("method=%s approx_rows_before=%d clear_tags=%v tags_removed=%d live_cleared=%d reseeded=%d",
		result.Method, result.ApproxRows, body.ClearTags, tagsRemoved, liveCleared, reseeded)
	diag.DBWrite(diag.LevelWarn, "historian samples wiped", detail, int(result.ApproxRows))

	out := map[string]any{
		"status":             "wiped",
		"method":             result.Method,
		"approx_rows_before": result.ApproxRows,
		"clear_tags":         body.ClearTags,
		"tags_removed":       tagsRemoved,
		"devices_cleared":    devicesCleared,
		"live_cleared":       liveCleared,
		"reseeded":           reseeded,
	}
	if reseedErr != nil {
		out["reseed_error"] = reseedErr.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

// reseedAfterWipe snapshots Live, clears it (so FanIn has no prev for suppress),
// then writes the snapshot as a fresh WriteBatch. Clear happens even when the
// batch is empty or writeBatch is nil; write errors leave Live cleared so the
// next poll still refills the historian.
func reseedAfterWipe(ctx context.Context, live *store.Live, writeBatch func(context.Context, []core.Sample) error) (cleared, written int, err error) {
	if live == nil {
		return 0, 0, nil
	}
	snap := live.All()
	cleared = live.Clear()
	if len(snap) == 0 || writeBatch == nil {
		return cleared, 0, nil
	}
	now := time.Now().UTC()
	for i := range snap {
		snap[i].Time = now
	}
	if err := writeBatch(ctx, snap); err != nil {
		return cleared, 0, err
	}
	return cleared, len(snap), nil
}

var (
	rateMu       sync.Mutex
	ratePrev     float64
	ratePrevTime time.Time
)

// samplesWrittenRate estimates samples/sec from the process counter between calls.
func samplesWrittenRate() float64 {
	now := time.Now()
	cur := counterValue(metrics.SamplesWritten)
	rateMu.Lock()
	defer rateMu.Unlock()
	if ratePrevTime.IsZero() {
		ratePrev = cur
		ratePrevTime = now
		return 0
	}
	dt := now.Sub(ratePrevTime).Seconds()
	if dt < 1 {
		return 0
	}
	delta := cur - ratePrev
	ratePrev = cur
	ratePrevTime = now
	if delta < 0 {
		return 0
	}
	return delta / dt
}
