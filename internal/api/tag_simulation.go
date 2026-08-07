package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) mountTagSimulation(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/tag-simulation", s.handleGetTagSimulation)
	mux.HandleFunc("PUT /api/v1/tag-simulation", s.handlePutTagSimulation)
}

func (s *Server) tagSimulationActive() bool {
	if s.TagSimulationActive != nil {
		return s.TagSimulationActive()
	}
	if s.Cfg != nil {
		return s.Cfg.TagSimulation()
	}
	return false
}

func (s *Server) simBrowserActive() bool {
	if s.SimBrowserActive != nil {
		return s.SimBrowserActive()
	}
	return false
}

type tagSimulationDTO struct {
	// Enabled is the persisted config preference (tag_simulation / LEVEL2_TAG_SIMULATION).
	Enabled bool `json:"enabled"`
	// Active is what this process is actually doing (set at collector start).
	Active bool `json:"active"`
	// SimBrowser is LEVEL2_SIM_BROWSER (full browse+samples sim).
	SimBrowser bool `json:"sim_browser"`
	// RestartRequired is true when config preference ≠ process active (recreate collector).
	RestartRequired bool   `json:"restart_required"`
	Note            string `json:"note,omitempty"`
}

func (s *Server) tagSimulationDTO() tagSimulationDTO {
	cfgOn := s.Cfg != nil && s.Cfg.TagSimulation()
	active := s.tagSimulationActive()
	simBr := s.simBrowserActive()
	out := tagSimulationDTO{
		Enabled:         cfgOn,
		Active:          active,
		SimBrowser:      simBr,
		RestartRequired: cfgOn != active && !simBr,
	}
	if simBr {
		out.Note = "LEVEL2_SIM_BROWSER is on: full sim browse + synthetic samples. Tag simulation flag is redundant until sim browser is off."
	} else if out.RestartRequired {
		out.Note = "Config saved; recreate collector (docker compose up -d --force-recreate collector) to apply."
	} else if active {
		out.Note = "Synthetic samples from mock.NewDemo. Writes to real PLC are blocked. Not live OPC."
	} else {
		out.Note = "Default off. Never auto-enabled on OPC disconnect. See docs/tag-simulation.md."
	}
	return out
}

func (s *Server) handleGetTagSimulation(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.tagSimulationDTO())
}

func (s *Server) handlePutTagSimulation(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store not available", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.Cfg.SetTagSimulation(body.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, s.tagSimulationDTO())
}
