package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/popelev/level2/internal/core"
)

func (s *Server) mountTagSimulation(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/tag-simulation", s.handleGetTagSimulation)
	mux.HandleFunc("PUT /api/v1/tag-simulation", s.handlePutTagSimulation)
	mux.HandleFunc("POST /api/v1/devices/{id}/tags/simulate", s.handleBulkSimulateTags)
	mux.HandleFunc("POST /api/v1/tags/simulate", s.handleBulkSimulateTagsAll)
	mux.HandleFunc("PATCH /api/v1/devices/{id}/tags/{tagId}", s.handlePatchTag)
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

// tagEffectivelySimulated reports whether this tag is fed by mock samples now.
func (s *Server) tagEffectivelySimulated(t core.Tag) bool {
	if s.simBrowserActive() || s.tagSimulationActive() {
		return true
	}
	return t.Simulate
}

type tagSimulationDTO struct {
	// Enabled is the persisted legacy global master (tag_simulation / LEVEL2_TAG_SIMULATION).
	Enabled bool `json:"enabled"`
	// Active is what this process is actually doing for the global master / sim browser.
	Active bool `json:"active"`
	// SimBrowser is LEVEL2_SIM_BROWSER (full browse+samples sim).
	SimBrowser bool `json:"sim_browser"`
	// TagsSimulated is how many tags have simulate=true (or all enabled under global/sim browser).
	TagsSimulated int `json:"tags_simulated"`
	// RestartRequired is true when config preference ≠ process active (recreate collector).
	RestartRequired bool   `json:"restart_required"`
	Note            string `json:"note,omitempty"`
}

func (s *Server) countTagsSimulated() int {
	if s.simBrowserActive() || s.tagSimulationActive() {
		n := 0
		var tags []core.Tag
		if s.Tags != nil {
			tags = s.Tags()
		} else if s.Cfg != nil {
			tags = s.Cfg.AllTags()
		}
		for _, t := range tags {
			if t.Enabled {
				n++
			}
		}
		return n
	}
	if s.Cfg != nil {
		return s.Cfg.CountSimulatedTags()
	}
	n := 0
	if s.Tags != nil {
		for _, t := range s.Tags() {
			if t.Simulate {
				n++
			}
		}
	}
	return n
}

func (s *Server) tagSimulationDTO() tagSimulationDTO {
	cfgOn := s.Cfg != nil && s.Cfg.TagSimulation()
	active := s.tagSimulationActive()
	simBr := s.simBrowserActive()
	out := tagSimulationDTO{
		Enabled:         cfgOn,
		Active:          active,
		SimBrowser:      simBr,
		TagsSimulated:   s.countTagsSimulated(),
		RestartRequired: cfgOn != active && !simBr,
	}
	if simBr {
		out.Note = "LEVEL2_SIM_BROWSER is on: full sim browse + synthetic samples for all enabled tags."
	} else if out.RestartRequired {
		out.Note = "Legacy global master saved; recreate collector (docker compose up -d --force-recreate collector) to apply."
	} else if active {
		out.Note = "Legacy global tag_simulation active: all enabled tags simulated; OPC collect paused. Prefer per-tag simulate."
	} else {
		out.Note = "Per-tag simulate is the source of truth (default false). Global master is opt-in legacy. Never auto-enabled on OPC disconnect. See docs/tag-simulation.md."
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

type bulkSimulateBody struct {
	Simulate bool     `json:"simulate"`
	TagIDs   []string `json:"tag_ids"`
	All      bool     `json:"all"`
	DeviceID string   `json:"device_id"`
}

func (s *Server) handleBulkSimulateTags(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store not available", http.StatusServiceUnavailable)
		return
	}
	deviceID := r.PathValue("id")
	var body bulkSimulateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	tagIDs := body.TagIDs
	if body.All {
		tagIDs = nil
	}
	updated, err := s.Cfg.SetTagsSimulate(deviceID, tagIDs, body.Simulate)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no matching") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id":       deviceID,
		"simulate":        body.Simulate,
		"updated":         updated,
		"tags_simulated":  s.countTagsSimulated(),
	})
}

// handleBulkSimulateTagsAll applies simulate across devices.
// Body: {simulate, tag_ids?, all?, device_id?} — empty tag_ids + all=true updates every tag (optionally scoped by device_id).
func (s *Server) handleBulkSimulateTagsAll(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store not available", http.StatusServiceUnavailable)
		return
	}
	var body bulkSimulateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	devs := s.Cfg.Devices()
	if body.DeviceID != "" {
		filtered := make([]core.Device, 0, 1)
		for _, d := range devs {
			if d.ID == body.DeviceID {
				filtered = append(filtered, d)
				break
			}
		}
		if len(filtered) == 0 {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		devs = filtered
	}
	tagFilter := map[string]struct{}{}
	for _, id := range body.TagIDs {
		if id != "" {
			tagFilter[id] = struct{}{}
		}
	}
	useFilter := len(tagFilter) > 0
	if !body.All && !useFilter {
		http.Error(w, "tag_ids required unless all=true", http.StatusBadRequest)
		return
	}
	total := 0
	for _, d := range devs {
		var ids []string
		if useFilter {
			for _, t := range d.Tags {
				if _, ok := tagFilter[t.ID]; ok {
					ids = append(ids, t.ID)
				}
			}
			if len(ids) == 0 {
				continue
			}
		} else {
			ids = nil // all tags on device
		}
		n, err := s.Cfg.SetTagsSimulate(d.ID, ids, body.Simulate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		total += n
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"simulate":       body.Simulate,
		"updated":        total,
		"tags_simulated": s.countTagsSimulated(),
	})
}

func (s *Server) handlePatchTag(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store not available", http.StatusServiceUnavailable)
		return
	}
	deviceID := r.PathValue("id")
	tagID := r.PathValue("tagId")
	var body struct {
		Simulate *bool `json:"simulate"`
		Writable *bool `json:"writable"`
		Enabled  *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Simulate == nil && body.Writable == nil && body.Enabled == nil {
		http.Error(w, "no patch fields (simulate, writable, enabled)", http.StatusBadRequest)
		return
	}
	tags, err := s.Cfg.DeviceTags(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	found := -1
	for i := range tags {
		if tags[i].ID == tagID {
			found = i
			break
		}
	}
	if found < 0 {
		http.Error(w, "tag not found", http.StatusNotFound)
		return
	}
	t := tags[found]
	if body.Simulate != nil {
		t.Simulate = *body.Simulate
	}
	if body.Writable != nil {
		t.Writable = *body.Writable
	}
	if body.Enabled != nil {
		t.Enabled = *body.Enabled
	}
	if err := s.Cfg.UpsertTag(deviceID, t); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, t)
}
