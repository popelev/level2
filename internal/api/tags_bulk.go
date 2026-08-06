package api

import (
	"context"
	"net/http"
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
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	updated := 0
	var errors []string
	drv := s.opcDriver(deviceID)
	oldTypes := make([]core.ValueType, len(tags))
	for i := range tags {
		oldTypes[i] = tags[i].DataType
	}
	if drv != nil {
		opcuaDriver.ApplyDataTypesFromOPC(ctx, drv, tags)
	} else {
		for i := range tags {
			tags[i].DataType = opcuaDriver.GuessDataType(tags[i].ID)
		}
	}
	for i := range tags {
		if err := validateTagFields(&tags[i]); err != nil {
			errors = append(errors, tags[i].ID+": "+err.Error())
			tags[i].DataType = oldTypes[i]
			continue
		}
		if tags[i].DataType != oldTypes[i] {
			updated++
		}
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
		"total":     len(tags),
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
	switch t.DataType {
	case core.ValueBool, core.ValueInt64, core.ValueFloat64, core.ValueString:
		return nil
	case "":
		t.DataType = core.ValueFloat64
		return nil
	default:
		return errUnsupportedDataType(t.DataType)
	}
}

type errUnsupportedDataType core.ValueType

func (e errUnsupportedDataType) Error() string {
	return "unsupported datatype " + string(e)
}
