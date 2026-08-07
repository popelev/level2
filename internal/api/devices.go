package api

import (
	"encoding/json"
	"net/http"

	"github.com/popelev/level2/internal/core"
)

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	devs := s.Devices()
	status := map[string]bool{}
	if s.DevHub != nil {
		status = s.DevHub.Status()
	}
	type dto struct {
		ID              string `json:"id"`
		Endpoint        string `json:"endpoint"`
		Security        string `json:"security"`
		Username        string `json:"username,omitempty"`
		PollConcurrency int    `json:"poll_concurrency"`
		TagCount        int    `json:"tag_count"`
		Connected       bool   `json:"connected"`
	}
	out := make([]dto, 0, len(devs))
	for _, d := range devs {
		out = append(out, dto{
			ID: d.ID, Endpoint: d.Endpoint, Security: d.Security,
			Username: d.Username, TagCount: len(d.Tags),
			PollConcurrency: core.NormalizePollConcurrency(d.PollConcurrency),
			Connected:       status[d.ID],
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateDevice(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	var body core.Device
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	for _, d := range s.Cfg.Devices() {
		if d.ID == body.ID {
			http.Error(w, "device already exists", http.StatusConflict)
			return
		}
	}
	if err := s.Cfg.UpsertDevice(body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.OnDeviceChanged != nil {
		s.OnDeviceChanged(body.ID, false)
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": body.ID})
}

func (s *Server) handleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	var body core.Device
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	body.ID = id
	if err := s.Cfg.UpsertDevice(body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.OnDeviceChanged != nil {
		s.OnDeviceChanged(id, false)
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if err := s.Cfg.DeleteDevice(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if s.OnDeviceChanged != nil {
		s.OnDeviceChanged(id, true)
	}
	w.WriteHeader(http.StatusNoContent)
}
