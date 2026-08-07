package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
)

const writeTimeout = 5 * time.Second

type writeValueBody struct {
	Value     any     `json:"value"`
	ValueNum  *float64 `json:"value_num"`
	ValueText *string  `json:"value_text"`
	ValueBool *bool    `json:"value_bool"`
	DeviceID  string   `json:"device_id"`
}

type writeValueResponse struct {
	TagID    string         `json:"tag_id"`
	DeviceID string         `json:"device_id"`
	NodeID   string         `json:"node_id"`
	Status   string         `json:"status"`
	Written  sampleDTOType  `json:"written"`
	Verified bool           `json:"verified"`
}

func (s *Server) opcWriteEnabled() bool {
	if s.OPCWriteEnabled != nil {
		return s.OPCWriteEnabled()
	}
	if s.Cfg != nil {
		return s.Cfg.OPCWriteEnabled()
	}
	return false
}

func (s *Server) handleWriteTagValue(w http.ResponseWriter, r *http.Request) {
	if !s.opcWriteEnabled() {
		diag.OPCWrite(diag.LevelWarn, "", r.PathValue("id"), "opc write denied", "opc_write_enabled=false")
		http.Error(w, "OPC write is disabled (opc_write_enabled=false)", http.StatusForbidden)
		return
	}

	tagID := r.PathValue("id")
	if tagID == "" {
		http.Error(w, "tag id required", http.StatusBadRequest)
		return
	}

	var body writeValueBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	raw, err := pickWriteRawValue(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	deviceID, tag, err := s.resolveWritableTag(tagID, body.DeviceID)
	if err != nil {
		code := http.StatusNotFound
		if errors.Is(err, errAmbiguousTag) {
			code = http.StatusBadRequest
		}
		http.Error(w, err.Error(), code)
		return
	}

	coerced, err := opcuaDriver.CoerceWriteValue(tag.DataType, raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.DevHub == nil {
		http.Error(w, "device hub unavailable", http.StatusServiceUnavailable)
		return
	}
	writer, err := s.DevHub.ValueWriter(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	ent, ok := s.DevHub.Entry(deviceID)
	if !ok || ent.Driver == nil || !ent.Driver.Connected() {
		http.Error(w, fmt.Sprintf("device %q not connected", deviceID), http.StatusConflict)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), writeTimeout)
	defer cancel()
	if err := writer.WriteValue(ctx, tag, coerced); err != nil {
		var stErr *opcuaDriver.WriteStatusError
		if errors.As(err, &stErr) {
			http.Error(w, stErr.Error(), http.StatusBadGateway)
			return
		}
		if err.Error() == "not connected" {
			http.Error(w, fmt.Sprintf("device %q not connected", deviceID), http.StatusConflict)
			return
		}
		diag.OPCWrite(diag.LevelError, deviceID, tag.ID, "opc write failed", err.Error())
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	written := sampleFromCoerced(tag.ID, tag.DataType, coerced)
	if s.Live != nil {
		s.Live.Update(written)
	}
	if s.Hub != nil {
		s.Hub.Broadcast(written)
	}

	writeJSON(w, http.StatusOK, writeValueResponse{
		TagID:    tag.ID,
		DeviceID: deviceID,
		NodeID:   tag.NodeID,
		Status:   "Good",
		Written:  sampleDTO(written),
		Verified: false,
	})
}

var errAmbiguousTag = errors.New("ambiguous tag_id; provide device_id")

func pickWriteRawValue(body writeValueBody) (any, error) {
	set := 0
	var raw any
	if body.Value != nil {
		raw = body.Value
		set++
	}
	if body.ValueNum != nil {
		raw = *body.ValueNum
		set++
	}
	if body.ValueText != nil {
		raw = *body.ValueText
		set++
	}
	if body.ValueBool != nil {
		raw = *body.ValueBool
		set++
	}
	if set == 0 {
		return nil, fmt.Errorf("value is required")
	}
	if set > 1 {
		return nil, fmt.Errorf("provide only one of value, value_num, value_text, value_bool")
	}
	return raw, nil
}

func (s *Server) resolveWritableTag(tagID, deviceID string) (string, core.Tag, error) {
	devs := s.Devices()
	type hit struct {
		deviceID string
		tag      core.Tag
	}
	var hits []hit
	for _, d := range devs {
		if deviceID != "" && d.ID != deviceID {
			continue
		}
		for _, t := range d.Tags {
			if t.ID == tagID {
				hits = append(hits, hit{deviceID: d.ID, tag: t})
			}
		}
	}
	if len(hits) == 0 {
		if deviceID != "" {
			return "", core.Tag{}, fmt.Errorf("tag %q not found on device %q", tagID, deviceID)
		}
		return "", core.Tag{}, fmt.Errorf("tag %q not found", tagID)
	}
	if len(hits) > 1 {
		return "", core.Tag{}, errAmbiguousTag
	}
	return hits[0].deviceID, hits[0].tag, nil
}

func sampleFromCoerced(tagID string, dt core.ValueType, v any) core.Sample {
	now := time.Now().UTC()
	s := core.Sample{Time: now, TagID: tagID, Quality: core.QualityGood}
	switch core.NormalizeValueType(dt) {
	case core.ValueBool:
		b := v.(bool)
		s.ValueBool = &b
	case core.ValueString:
		str := v.(string)
		s.ValueText = &str
	case core.ValueDateTime:
		tm := v.(time.Time)
		str := tm.UTC().Format(time.RFC3339Nano)
		s.ValueText = &str
	case core.ValueInt64:
		n := float64(v.(int64))
		s.ValueNum = &n
	case core.ValueUint:
		n := float64(v.(uint32))
		s.ValueNum = &n
	case core.ValueFloat64:
		n := v.(float64)
		s.ValueNum = &n
	}
	return s
}
