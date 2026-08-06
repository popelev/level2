package api

import (
	"net/http"
	"strconv"

	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/metrics"
	dto "github.com/prometheus/client_model/go"
)

func (s *Server) mountDiagnostics(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/diagnostics/logs", s.handleDiagLogs)
	mux.HandleFunc("DELETE /api/v1/diagnostics/logs", s.handleDiagClear)
	mux.HandleFunc("POST /api/v1/diagnostics/reset", s.handleDiagReset)
	mux.HandleFunc("GET /api/v1/diagnostics/capacity", s.handleCapacity)
}

func (s *Server) handleDiagLogs(w http.ResponseWriter, r *http.Request) {
	if s.Diag == nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []diag.Entry{}, "metrics": diagMetrics()})
		return
	}
	q := r.URL.Query()
	category := q.Get("category")
	if category == "" {
		category = "all"
	}
	errorsOnly := q.Get("errors_only") == "1" || q.Get("errors_only") == "true"
	limit := parseIntDefault(q.Get("limit"), 300)
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": s.Diag.Query(category, errorsOnly, limit),
		"metrics": diagMetrics(),
	})
}

func (s *Server) handleDiagClear(w http.ResponseWriter, r *http.Request) {
	if s.Diag != nil {
		s.Diag.Clear()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

// handleDiagReset clears the diagnostics ring log and last-hour incident counters
// used by Overview (Recent errors + “drops / last hour” pills).
func (s *Server) handleDiagReset(w http.ResponseWriter, r *http.Request) {
	if s.Diag != nil {
		s.Diag.Clear()
	}
	inc := s.Incidents
	if inc == nil {
		inc = diag.DefaultIncidents()
	}
	if inc != nil {
		inc.Clear()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func diagMetrics() map[string]float64 {
	return map[string]float64{
		"samples_written_total": counterValue(metrics.SamplesWritten),
		"samples_spooled_total": counterValue(metrics.SamplesSpooled),
		"write_errors_total":    counterValue(metrics.WriteErrors),
		"spool_depth":           gaugeValue(metrics.SpoolDepth),
	}
}

func counterValue(c interface{ Write(*dto.Metric) error }) float64 {
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

func gaugeValue(g interface{ Write(*dto.Metric) error }) float64 {
	var m dto.Metric
	if err := g.Write(&m); err != nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

func parseIntDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
