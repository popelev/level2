package api

import (
	"net/http"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/metrics"
)

func (s *Server) mountStatus(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/status/summary", s.handleStatusSummary)
}

// statusSummary is a lightweight console header / overview payload (no secrets).
type statusSummary struct {
	APIOK               bool         `json:"api_ok"`
	CollectorReady      bool         `json:"collector_ready"`
	ReadyDetail         string       `json:"ready_detail"`
	DevicesTotal        int          `json:"devices_total"`
	DevicesConnected    int          `json:"devices_connected"`
	DevicesDisconnected int          `json:"devices_disconnected"`
	TagsTotal           int          `json:"tags_total"`
	TagsEnabled         int          `json:"tags_enabled"`
	QualityGood         int          `json:"quality_good"`
	QualityBad          int          `json:"quality_bad"`
	QualityUnknown      int          `json:"quality_unknown"`
	QualityGoodPct      *float64     `json:"quality_good_pct,omitempty"`
	PollAvgMs           *int64       `json:"poll_avg_ms,omitempty"`
	SamplesWritten      float64      `json:"samples_written_total"`
	SamplesPerSec       float64      `json:"samples_per_sec"`
	WriteErrors         float64      `json:"write_errors_total"`
	SpoolDepth          float64      `json:"spool_depth"`
	DatabaseConnected   bool         `json:"database_connected"`
	RecentErrors        []diag.Entry `json:"recent_errors"`
}

func (s *Server) handleStatusSummary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.buildStatusSummary(r))
}

func (s *Server) buildStatusSummary(r *http.Request) statusSummary {
	out := statusSummary{
		APIOK:          true,
		ReadyDetail:    "not connected",
		SamplesWritten: counterValue(metrics.SamplesWritten),
		WriteErrors:    counterValue(metrics.WriteErrors),
		SpoolDepth:     gaugeValue(metrics.SpoolDepth),
		SamplesPerSec:  samplesWrittenRate(),
		RecentErrors:   []diag.Entry{},
	}

	devs := []core.Device{}
	if s.Devices != nil {
		devs = s.Devices()
	}
	out.DevicesTotal = len(devs)

	conn := map[string]bool{}
	if s.DevHub != nil {
		conn = s.DevHub.Status()
	}
	for _, d := range devs {
		if conn[d.ID] {
			out.DevicesConnected++
		}
	}
	out.DevicesDisconnected = out.DevicesTotal - out.DevicesConnected

	switch {
	case s.ReadyCheck != nil:
		out.CollectorReady = s.ReadyCheck()
	case s.DevHub != nil:
		out.CollectorReady = s.DevHub.AnyConnected()
	}
	if out.CollectorReady {
		out.ReadyDetail = "ready"
	}

	if s.Live != nil && len(devs) > 0 {
		tvs := s.Live.SnapshotDevices(devs)
		var pollSum int64
		var pollN int64
		for _, tv := range tvs {
			out.TagsTotal++
			if tv.Tag.Enabled {
				out.TagsEnabled++
			}
			if tv.Sample == nil {
				if tv.Tag.Enabled {
					out.QualityUnknown++
				}
				continue
			}
			switch tv.Sample.Quality {
			case core.QualityGood:
				out.QualityGood++
			default:
				out.QualityBad++
			}
			if tv.PollAvgMs != nil && *tv.PollAvgMs > 0 {
				pollSum += *tv.PollAvgMs
				pollN++
			}
		}
		sampled := out.QualityGood + out.QualityBad
		if sampled > 0 {
			pct := 100 * float64(out.QualityGood) / float64(sampled)
			out.QualityGoodPct = &pct
		}
		if pollN > 0 {
			avg := pollSum / pollN
			out.PollAvgMs = &avg
		}
	} else if s.Tags != nil {
		for _, t := range s.Tags() {
			out.TagsTotal++
			if t.Enabled {
				out.TagsEnabled++
			}
		}
	}

	if s.DB != nil {
		out.DatabaseConnected = s.DB.Ping(r.Context()) == nil
	}

	if s.Diag != nil {
		out.RecentErrors = s.Diag.Query("all", true, 8)
	}

	return out
}
