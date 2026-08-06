package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
	"github.com/popelev/level2/internal/core"
)

func (s *Server) mountTagBulk(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/devices/{id}/tags/sync", s.handleSyncTags)
	mux.HandleFunc("DELETE /api/v1/devices/{id}/tags", s.handleDeleteAllTags)
}

func (s *Server) handleSyncTags(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	deviceID := r.PathValue("id")
	tags, err := s.Cfg.DeviceTags(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var body struct {
		TagIDs []string `json:"tag_ids"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	syncOnly := map[string]struct{}{}
	for _, id := range body.TagIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			syncOnly[id] = struct{}{}
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	updated := 0
	var errors []string
	drv := s.opcDriver(deviceID)

	var toSync []core.Tag
	var syncIdx []int
	for i := range tags {
		if len(syncOnly) > 0 {
			if _, ok := syncOnly[tags[i].ID]; !ok {
				continue
			}
		}
		toSync = append(toSync, tags[i])
		syncIdx = append(syncIdx, i)
	}
	if len(toSync) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"device_id": deviceID,
			"total":     0,
			"updated":   0,
			"errors":    []string{},
		})
		return
	}

	oldTypes := make([]core.ValueType, len(toSync))
	for i := range toSync {
		oldTypes[i] = toSync[i].DataType
	}
	if drv != nil {
		opcuaDriver.ApplyDataTypesFromOPC(ctx, drv, toSync)
	} else {
		for i := range toSync {
			toSync[i].DataType = opcuaDriver.GuessDataType(toSync[i].ID)
		}
	}
	for j := range toSync {
		if err := validateTagFields(&toSync[j]); err != nil {
			errors = append(errors, toSync[j].ID+": "+err.Error())
			toSync[j].DataType = oldTypes[j]
			continue
		}
		if toSync[j].DataType != oldTypes[j] {
			updated++
		}
		tags[syncIdx[j]] = toSync[j]
	}
	if err := s.Cfg.SetDeviceTags(deviceID, tags); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.OnDeviceChanged != nil {
		s.OnDeviceChanged(deviceID, false)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id": deviceID,
		"total":     len(toSync),
		"updated":   updated,
		"errors":    errors,
	})
}

func (s *Server) handleDeleteAllTags(w http.ResponseWriter, r *http.Request) {
	if s.Cfg == nil {
		http.Error(w, "config store unavailable", http.StatusServiceUnavailable)
		return
	}
	deviceID := r.PathValue("id")
	removed, err := s.Cfg.ClearDeviceTags(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if s.OnDeviceChanged != nil {
		s.OnDeviceChanged(deviceID, false)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_id": deviceID,
		"removed":   removed,
	})
}

func (s *Server) opcDriver(deviceID string) *opcuaDriver.Driver {
	if s.DevHub == nil {
		return nil
	}
	ent, ok := s.DevHub.Entry(deviceID)
	if !ok || !ent.Connected {
		return nil
	}
	drv, ok := ent.Driver.(*opcuaDriver.Driver)
	if !ok {
		return nil
	}
	return drv
}

func validateTagFields(t *core.Tag) error {
	if t.IntervalMs <= 0 {
		t.IntervalMs = 1000
	}
	t.DataType = core.NormalizeValueType(t.DataType)
	if t.DataType == "" {
		t.DataType = core.ValueFloat64
	}
	if !core.ValidValueType(t.DataType) {
		return errUnsupportedDataType(t.DataType)
	}
	return nil
}

type errUnsupportedDataType core.ValueType

func (e errUnsupportedDataType) Error() string {
	return "unsupported datatype " + string(e)
}
