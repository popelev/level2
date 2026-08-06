package api

import (
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/metrics"
)

func (s *Server) mountDatabase(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/database/status", s.handleDatabaseStatus)
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
	out["eta_seconds"] = cap.ETASeconds
	out["window_seconds"] = cap.WindowSeconds
	writeJSON(w, http.StatusOK, out)
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
