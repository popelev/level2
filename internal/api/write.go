package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
)

const writeTimeout = 5 * time.Second

// Max batch size (per-item OPC writes; same order of magnitude as Siemens maxNodesPerWrite).
const maxBatchWrites = 100

type writeValueBody struct {
	Value           any      `json:"value"`
	ValueNum        *float64 `json:"value_num"`
	ValueText       *string  `json:"value_text"`
	ValueBool       *bool    `json:"value_bool"`
	DeviceID        string   `json:"device_id"`
	Verify          *bool    `json:"verify"`
	VerifyTimeoutMs *int     `json:"verify_timeout_ms"`
	Optimistic      *bool    `json:"optimistic"`
}

type writeValueResponse struct {
	TagID    string         `json:"tag_id"`
	DeviceID string         `json:"device_id"`
	NodeID   string         `json:"node_id"`
	Status   string         `json:"status"`
	Written  sampleDTOType  `json:"written"`
	Verified bool           `json:"verified"`
	Observed *sampleDTOType `json:"observed,omitempty"`
	Error    string         `json:"error,omitempty"`
	Message  string         `json:"message,omitempty"`
}

type batchWriteBody struct {
	Writes          []batchWriteItem `json:"writes"`
	Verify          *bool            `json:"verify"`
	VerifyTimeoutMs *int             `json:"verify_timeout_ms"`
	Optimistic      *bool            `json:"optimistic"`
}

type batchWriteItem struct {
	TagID           string   `json:"tag_id"`
	DeviceID        string   `json:"device_id"`
	Value           any      `json:"value"`
	ValueNum        *float64 `json:"value_num"`
	ValueText       *string  `json:"value_text"`
	ValueBool       *bool    `json:"value_bool"`
	Verify          *bool    `json:"verify"`
	VerifyTimeoutMs *int     `json:"verify_timeout_ms"`
	Optimistic      *bool    `json:"optimistic"`
}

type batchWriteResult struct {
	TagID    string         `json:"tag_id"`
	DeviceID string         `json:"device_id,omitempty"`
	NodeID   string         `json:"node_id,omitempty"`
	OK       bool           `json:"ok"`
	Status   string         `json:"status,omitempty"`
	Written  *sampleDTOType `json:"written,omitempty"`
	Verified bool           `json:"verified,omitempty"`
	Observed *sampleDTOType `json:"observed,omitempty"`
	Error    string         `json:"error,omitempty"`
	HTTP     int            `json:"http_status"`
}

type batchWriteResponse struct {
	Results   []batchWriteResult `json:"results"`
	OKCount   int                `json:"ok_count"`
	FailCount int                `json:"fail_count"`
}

type writeOptions struct {
	verify          bool
	verifyTimeoutMs int
	optimistic      bool
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
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "failed to read request body")
		return
	}
	s.withIdempotency(w, r, bodyBytes, func(w http.ResponseWriter) {
		s.handleWriteTagValueInner(w, r, bodyBytes)
	})
}

func (s *Server) handleWriteTagValueInner(w http.ResponseWriter, r *http.Request, bodyBytes []byte) {
	if !s.opcWriteEnabled() {
		diag.OPCWrite(diag.LevelWarn, "", r.PathValue("id"), "opc write denied", "opc_write_enabled=false")
		writeAPIError(w, http.StatusForbidden, "opc_write_disabled", "OPC write is disabled (opc_write_enabled=false)")
		return
	}
	if s.simBrowserActive() {
		diag.OPCWrite(diag.LevelWarn, "", r.PathValue("id"), "opc write denied", "sim_browser active")
		writeAPIError(w, http.StatusConflict, "opc_write_blocked_sim_browser", "OPC write blocked while sim browser is active (no writes to real PLC)")
		return
	}
	if s.tagSimulationActive() {
		diag.OPCWrite(diag.LevelWarn, "", r.PathValue("id"), "opc write denied", "tag_simulation active")
		writeAPIError(w, http.StatusConflict, "opc_write_blocked_tag_simulation", "OPC write blocked while legacy global tag_simulation is active (no writes to real PLC)")
		return
	}

	tagID := r.PathValue("id")
	if tagID == "" {
		writeAPIError(w, http.StatusBadRequest, "tag_id_required", "tag id required")
		return
	}

	var body writeValueBody
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}

	raw, err := pickWriteRawValue(body)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	opts := resolveWriteOptions(body.Verify, body.VerifyTimeoutMs, body.Optimistic, r)
	res := s.executeWrite(r.Context(), tagID, body.DeviceID, raw, opts)
	if !res.OK {
		if res.Written != nil && (res.HTTP == http.StatusConflict || strings.Contains(res.Error, "verify")) {
			writeJSON(w, res.HTTP, writeValueResponse{
				TagID:    res.TagID,
				DeviceID: res.DeviceID,
				NodeID:   res.NodeID,
				Status:   res.Status,
				Written:  *res.Written,
				Verified: false,
				Observed: res.Observed,
				Error:    "verify_failed",
				Message:  res.Error,
			})
			return
		}
		writeFailureAPIError(w, res)
		return
	}
	writeJSON(w, http.StatusOK, writeValueResponse{
		TagID:    res.TagID,
		DeviceID: res.DeviceID,
		NodeID:   res.NodeID,
		Status:   res.Status,
		Written:  *res.Written,
		Verified: res.Verified,
		Observed: res.Observed,
	})
}

func (s *Server) handleBatchWriteTagValues(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_body", "failed to read request body")
		return
	}
	s.withIdempotency(w, r, bodyBytes, func(w http.ResponseWriter) {
		s.handleBatchWriteTagValuesInner(w, r, bodyBytes)
	})
}

func (s *Server) handleBatchWriteTagValuesInner(w http.ResponseWriter, r *http.Request, bodyBytes []byte) {
	if !s.opcWriteEnabled() {
		diag.OPCWrite(diag.LevelWarn, "", "", "opc batch write denied", "opc_write_enabled=false")
		writeAPIError(w, http.StatusForbidden, "opc_write_disabled", "OPC write is disabled (opc_write_enabled=false)")
		return
	}
	if s.simBrowserActive() {
		diag.OPCWrite(diag.LevelWarn, "", "", "opc batch write denied", "sim_browser active")
		writeAPIError(w, http.StatusConflict, "opc_write_blocked_sim_browser", "OPC write blocked while sim browser is active (no writes to real PLC)")
		return
	}
	if s.tagSimulationActive() {
		diag.OPCWrite(diag.LevelWarn, "", "", "opc batch write denied", "tag_simulation active")
		writeAPIError(w, http.StatusConflict, "opc_write_blocked_tag_simulation", "OPC write blocked while legacy global tag_simulation is active (no writes to real PLC)")
		return
	}

	var body batchWriteBody
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}
	if len(body.Writes) == 0 {
		writeAPIError(w, http.StatusBadRequest, "writes_required", "writes array is required and must be non-empty")
		return
	}
	if len(body.Writes) > maxBatchWrites {
		writeAPIError(w, http.StatusBadRequest, "writes_too_many", fmt.Sprintf("writes exceeds max %d items", maxBatchWrites))
		return
	}

	out := batchWriteResponse{Results: make([]batchWriteResult, 0, len(body.Writes))}
	for _, item := range body.Writes {
		raw, err := pickWriteRawValue(writeValueBody{
			Value: item.Value, ValueNum: item.ValueNum, ValueText: item.ValueText, ValueBool: item.ValueBool,
		})
		if err != nil {
			out.Results = append(out.Results, batchWriteResult{
				TagID: item.TagID, DeviceID: item.DeviceID, OK: false, Error: err.Error(), HTTP: http.StatusBadRequest,
			})
			out.FailCount++
			continue
		}
		if item.TagID == "" {
			out.Results = append(out.Results, batchWriteResult{
				TagID: item.TagID, DeviceID: item.DeviceID, OK: false, Error: "tag_id required", HTTP: http.StatusBadRequest,
			})
			out.FailCount++
			continue
		}
		verify := body.Verify
		if item.Verify != nil {
			verify = item.Verify
		}
		timeout := body.VerifyTimeoutMs
		if item.VerifyTimeoutMs != nil {
			timeout = item.VerifyTimeoutMs
		}
		optimistic := body.Optimistic
		if item.Optimistic != nil {
			optimistic = item.Optimistic
		}
		opts := resolveWriteOptions(verify, timeout, optimistic, r)
		res := s.executeWrite(r.Context(), item.TagID, item.DeviceID, raw, opts)
		out.Results = append(out.Results, res)
		if res.OK {
			out.OKCount++
		} else {
			out.FailCount++
		}
	}
	writeJSON(w, http.StatusOK, out)
}
func resolveWriteOptions(verify *bool, timeoutMs *int, optimistic *bool, r *http.Request) writeOptions {
	opts := writeOptions{optimistic: true}
	if verify != nil {
		opts.verify = *verify
	}
	if q := r.URL.Query().Get("verify"); q != "" {
		opts.verify = parseBoolQuery(q)
	}
	if timeoutMs != nil {
		opts.verifyTimeoutMs = *timeoutMs
	}
	if q := r.URL.Query().Get("verify_timeout_ms"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			opts.verifyTimeoutMs = n
		}
	}
	if optimistic != nil {
		opts.optimistic = *optimistic
	}
	if q := r.URL.Query().Get("optimistic"); q != "" {
		opts.optimistic = parseBoolQuery(q)
	}
	return opts
}

func parseBoolQuery(q string) bool {
	switch strings.ToLower(strings.TrimSpace(q)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *Server) executeWrite(parent context.Context, tagID, deviceIDHint string, raw any, opts writeOptions) batchWriteResult {
	deviceID, tag, err := s.resolveWritableTag(tagID, deviceIDHint)
	if err != nil {
		code := http.StatusNotFound
		if errors.Is(err, errAmbiguousTag) {
			code = http.StatusBadRequest
		}
		return batchWriteResult{TagID: tagID, DeviceID: deviceIDHint, OK: false, Error: err.Error(), HTTP: code}
	}
	if !tag.Writable {
		diag.OPCWrite(diag.LevelWarn, deviceID, tag.ID, "opc write denied", "tag writable=false")
		return batchWriteResult{
			TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
			OK: false, Error: "tag is not writable (writable=false)", HTTP: http.StatusForbidden,
		}
	}
	if tag.Simulate {
		diag.OPCWrite(diag.LevelWarn, deviceID, tag.ID, "opc write denied", "tag simulate=true")
		return batchWriteResult{
			TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
			OK: false, Error: "tag is simulated (simulate=true); no writes to real PLC", HTTP: http.StatusConflict,
		}
	}

	coerced, err := core.CoerceWriteValue(tag.DataType, raw)
	if err != nil {
		return batchWriteResult{
			TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
			OK: false, Error: err.Error(), HTTP: http.StatusBadRequest,
		}
	}

	if s.DevHub == nil {
		return batchWriteResult{
			TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
			OK: false, Error: "device hub unavailable", HTTP: http.StatusServiceUnavailable,
		}
	}
	writer, err := s.DevHub.ValueWriter(deviceID)
	if err != nil {
		return batchWriteResult{
			TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
			OK: false, Error: err.Error(), HTTP: http.StatusConflict,
		}
	}
	ent, ok := s.DevHub.Entry(deviceID)
	if !ok || ent.Driver == nil || !ent.Driver.Connected() {
		return batchWriteResult{
			TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
			OK: false, Error: fmt.Sprintf("device %q not connected", deviceID), HTTP: http.StatusConflict,
		}
	}

	ctx, cancel := context.WithTimeout(parent, writeTimeout)
	defer cancel()
	if err := writer.WriteValue(ctx, tag, coerced); err != nil {
		var stErr *opcuaDriver.WriteStatusError
		if errors.As(err, &stErr) {
			return batchWriteResult{
				TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
				OK: false, Error: stErr.Error(), HTTP: http.StatusBadGateway,
			}
		}
		if err.Error() == "not connected" {
			return batchWriteResult{
				TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
				OK: false, Error: fmt.Sprintf("device %q not connected", deviceID), HTTP: http.StatusConflict,
			}
		}
		diag.OPCWrite(diag.LevelError, deviceID, tag.ID, "opc write failed", err.Error())
		return batchWriteResult{
			TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
			OK: false, Error: err.Error(), HTTP: http.StatusBadGateway,
		}
	}

	written := sampleFromCoerced(tag.ID, tag.DataType, coerced)
	dto := sampleDTO(written)

	publishLive := func(sample core.Sample) {
		if s.Live != nil {
			s.Live.Update(sample)
		}
		if s.Hub != nil {
			s.Hub.Broadcast(sample)
		}
	}

	if opts.optimistic {
		publishLive(written)
	}

	if !opts.verify {
		if !opts.optimistic {
			publishLive(written)
		}
		return batchWriteResult{
			TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
			OK: true, Status: "Good", Written: &dto, Verified: false, HTTP: http.StatusOK,
		}
	}

	reader, err := s.DevHub.ValueReader(deviceID)
	if err != nil {
		diag.OPCWrite(diag.LevelWarn, deviceID, tag.ID, "opc verify unavailable", err.Error())
		return batchWriteResult{
			TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
			OK: false, Status: "Good", Written: &dto,
			Error: fmt.Sprintf("write ok but verify unavailable: %v (non-transactional)", err),
			HTTP:  http.StatusConflict,
		}
	}

	verifyTimeout := normalizeVerifyTimeoutMs(opts.verifyTimeoutMs)
	vctx, vcancel := context.WithTimeout(parent, verifyTimeout)
	defer vcancel()

	observed, match, verr := s.verifyWriteReadback(vctx, reader, tag, written)
	var obsDTO *sampleDTOType
	if observed != nil {
		d := sampleDTO(*observed)
		obsDTO = &d
	}
	if verr != nil {
		diag.OPCWrite(diag.LevelError, deviceID, tag.ID, "opc verify read failed", verr.Error())
		return batchWriteResult{
			TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
			OK: false, Status: "Good", Written: &dto, Observed: obsDTO,
			Error: fmt.Sprintf("write ok but verify read failed: %v (non-transactional)", verr),
			HTTP:  http.StatusBadGateway,
		}
	}
	if !match {
		msg := "write ok but readback mismatch (non-transactional)"
		diag.OPCWrite(diag.LevelWarn, deviceID, tag.ID, "opc verify mismatch", msg)
		return batchWriteResult{
			TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
			OK: false, Status: "Good", Written: &dto, Observed: obsDTO, Verified: false,
			Error: msg, HTTP: http.StatusConflict,
		}
	}

	if observed != nil {
		if !opts.optimistic {
			publishLive(*observed)
		} else {
			// Refresh Live with authoritative readback timestamps/values.
			publishLive(*observed)
		}
	} else if !opts.optimistic {
		publishLive(written)
	}

	diag.OPCWrite(diag.LevelInfo, deviceID, tag.ID, "opc write verified", fmt.Sprintf("node=%s", tag.NodeID))
	return batchWriteResult{
		TagID: tag.ID, DeviceID: deviceID, NodeID: tag.NodeID,
		OK: true, Status: "Good", Written: &dto, Observed: obsDTO, Verified: true, HTTP: http.StatusOK,
	}
}

func (s *Server) verifyWriteReadback(ctx context.Context, reader core.ValueReader, tag core.Tag, expected core.Sample) (*core.Sample, bool, error) {
	var last *core.Sample
	var lastErr error
	for {
		sample, err := reader.ReadValue(ctx, tag)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				if last != nil {
					return last, false, nil
				}
				return nil, false, lastErr
			}
		} else {
			cp := sample
			last = &cp
			if writeVerifyMatch(expected, sample, tag.DataType) {
				return last, true, nil
			}
		}

		// Wait briefly then retry until verify timeout (context). Do not hold OPC client locks across sleep.
		timer := time.NewTimer(verifyPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if last != nil {
				return last, false, nil
			}
			if lastErr != nil {
				return nil, false, lastErr
			}
			return nil, false, ctx.Err()
		case <-timer.C:
		}
	}
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
