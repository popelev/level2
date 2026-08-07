package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/popelev/level2/internal/core"
	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
	devruntime "github.com/popelev/level2/internal/runtime"
	"github.com/popelev/level2/internal/store"
)

type writeOnlyDriver struct {
	connected bool
	err       error
	calls     int
}

func (d *writeOnlyDriver) Connect(context.Context) error    { return nil }
func (d *writeOnlyDriver) Disconnect(context.Context) error { return nil }
func (d *writeOnlyDriver) Connected() bool                  { return d.connected }
func (d *writeOnlyDriver) Subscribe(context.Context, []core.Tag, chan<- core.Sample) error {
	return fmt.Errorf("no subscribe")
}
func (d *writeOnlyDriver) WriteValue(_ context.Context, _ core.Tag, _ any) error {
	d.calls++
	return d.err
}

func TestWriteGuards_SimBrowserAndTagSimulation(t *testing.T) {
	s, mux := writeTestServer(t, true, &mockWriter{connected: true})
	s.SimBrowserActive = func() bool { return true }

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":1,"device_id":"s7_1500"}`))))
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "sim browser") {
		t.Fatalf("sim browser write: %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/values",
		bytes.NewReader([]byte(`{"writes":[{"tag_id":"Motor1.SpeedSP","value":1}]}`))))
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "sim browser") {
		t.Fatalf("sim browser batch: %d %s", rr.Code, rr.Body.String())
	}

	s.SimBrowserActive = nil
	s.TagSimulationActive = func() bool { return true }
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":1,"device_id":"s7_1500"}`))))
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "tag_simulation") {
		t.Fatalf("tag sim write: %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/values",
		bytes.NewReader([]byte(`{"writes":[{"tag_id":"Motor1.SpeedSP","value":1}]}`))))
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "tag_simulation") {
		t.Fatalf("tag sim batch: %d %s", rr.Code, rr.Body.String())
	}
}

func TestWrite_InvalidJSONAndBatchValidation(t *testing.T) {
	_, mux := writeTestServer(t, true, &mockWriter{connected: true})

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{bad`))))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json: %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/values",
		bytes.NewReader([]byte(`{bad`))))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("batch bad json: %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/values",
		bytes.NewReader([]byte(`{"writes":[]}`))))
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "non-empty") {
		t.Fatalf("empty writes: %d %s", rr.Code, rr.Body.String())
	}

	items := make([]string, maxBatchWrites+1)
	for i := range items {
		items[i] = `{"tag_id":"Motor1.SpeedSP","value":1,"device_id":"s7_1500"}`
	}
	body := `{"writes":[` + strings.Join(items, ",") + `]}`
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/values", bytes.NewReader([]byte(body))))
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "max") {
		t.Fatalf("too many writes: %d %s", rr.Code, rr.Body.String())
	}
}

func TestBatchWrite_PerItemErrors(t *testing.T) {
	_, mux := writeTestServer(t, true, &mockWriter{connected: true})
	body := `{"writes":[
		{"tag_id":"Motor1.SpeedSP"},
		{"value":1,"device_id":"s7_1500"},
		{"tag_id":"Motor1.SpeedSP","value":1,"device_id":"s7_1500","verify":true,"verify_timeout_ms":50,"optimistic":false}
	]}`
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/values", bytes.NewReader([]byte(body))))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d %s", rr.Code, rr.Body.String())
	}
	var resp batchWriteResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.FailCount < 2 || len(resp.Results) != 3 {
		t.Fatalf("%#v", resp)
	}
	if resp.Results[0].OK || !strings.Contains(resp.Results[0].Error, "value") {
		t.Fatalf("missing value: %#v", resp.Results[0])
	}
	if resp.Results[1].OK || resp.Results[1].Error != "tag_id required" {
		t.Fatalf("missing tag_id: %#v", resp.Results[1])
	}
	if !resp.Results[2].OK || !resp.Results[2].Verified {
		t.Fatalf("ok verified item: %#v", resp.Results[2])
	}
}

func TestExecuteWrite_AmbiguousSimulateNoHub(t *testing.T) {
	tag := core.Tag{
		ID: "dup", NodeID: "ns=4;i=1", DataType: core.ValueFloat64,
		Enabled: true, Writable: true,
	}
	sim := core.Tag{
		ID: "sim", NodeID: "ns=4;i=2", DataType: core.ValueFloat64,
		Enabled: true, Writable: true, Simulate: true,
	}
	s := &Server{
		OPCWriteEnabled: func() bool { return true },
		Devices: func() []core.Device {
			return []core.Device{
				{ID: "a", Tags: []core.Tag{tag, sim}},
				{ID: "b", Tags: []core.Tag{tag}},
			}
		},
	}

	res := s.executeWrite(context.Background(), "dup", "", 1.0, writeOptions{})
	if res.OK || res.HTTP != http.StatusBadRequest || !strings.Contains(res.Error, "ambiguous") {
		t.Fatalf("ambiguous: %#v", res)
	}

	res = s.executeWrite(context.Background(), "sim", "a", 1.0, writeOptions{})
	if res.OK || res.HTTP != http.StatusConflict || !strings.Contains(res.Error, "simulated") {
		t.Fatalf("simulate: %#v", res)
	}

	res = s.executeWrite(context.Background(), "dup", "a", 1.0, writeOptions{})
	if res.OK || res.HTTP != http.StatusServiceUnavailable || !strings.Contains(res.Error, "hub") {
		t.Fatalf("no hub: %#v", res)
	}

	_, tagHit, err := s.resolveWritableTag("dup", "missing")
	if err == nil || tagHit.ID != "" {
		t.Fatalf("wrong device: %#v %v", tagHit, err)
	}
}

func TestExecuteWrite_StatusErrorAndNotConnectedString(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	tag := core.Tag{
		ID: "t1", NodeID: "ns=4;i=1", DataType: core.ValueFloat64,
		Enabled: true, Writable: true,
	}
	dev := core.Device{ID: "plc", Tags: []core.Tag{tag}}

	w := &writeOnlyDriver{connected: true, err: &opcuaDriver.WriteStatusError{Status: ua.StatusBad}}
	hub := devruntime.NewHub(log, false)
	hub.InjectDriver(dev, w, nil)
	s := &Server{
		OPCWriteEnabled: func() bool { return true },
		Devices:         func() []core.Device { return []core.Device{dev} },
		DevHub:          hub,
	}
	res := s.executeWrite(context.Background(), "t1", "plc", 1.0, writeOptions{})
	if res.OK || res.HTTP != http.StatusBadGateway {
		t.Fatalf("status err: %#v", res)
	}

	w.err = fmt.Errorf("not connected")
	res = s.executeWrite(context.Background(), "t1", "plc", 2.0, writeOptions{})
	if res.OK || res.HTTP != http.StatusConflict || !strings.Contains(res.Error, "not connected") {
		t.Fatalf("not connected string: %#v", res)
	}

	w.err = fmt.Errorf("plc busy")
	res = s.executeWrite(context.Background(), "t1", "plc", 3.0, writeOptions{})
	if res.OK || res.HTTP != http.StatusBadGateway || !strings.Contains(res.Error, "plc busy") {
		t.Fatalf("generic write err: %#v", res)
	}
}

func TestExecuteWrite_VerifyUnavailableAndReadFail(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tag := core.Tag{
		ID: "t1", NodeID: "ns=4;i=1", DataType: core.ValueFloat64,
		Enabled: true, Writable: true,
	}
	dev := core.Device{ID: "plc", Tags: []core.Tag{tag}}

	wo := &writeOnlyDriver{connected: true}
	hub := devruntime.NewHub(log, false)
	hub.InjectDriver(dev, wo, nil)
	s := &Server{
		Live:            store.NewLive(),
		Hub:             NewHub(),
		OPCWriteEnabled: func() bool { return true },
		Devices:         func() []core.Device { return []core.Device{dev} },
		DevHub:          hub,
	}
	res := s.executeWrite(context.Background(), "t1", "plc", 1.5, writeOptions{verify: true, verifyTimeoutMs: 50})
	if res.OK || res.HTTP != http.StatusConflict || !strings.Contains(res.Error, "verify unavailable") {
		t.Fatalf("verify unavailable: %#v", res)
	}
	if wo.calls != 1 {
		t.Fatalf("calls=%d", wo.calls)
	}

	mw := &mockWriter{connected: true, readErr: fmt.Errorf("read timeout")}
	hub.InjectDriver(dev, mw, nil)
	res = s.executeWrite(context.Background(), "t1", "plc", 2.5, writeOptions{verify: true, verifyTimeoutMs: 80, optimistic: false})
	if res.OK || res.HTTP != http.StatusBadGateway || !strings.Contains(res.Error, "verify read failed") {
		t.Fatalf("verify read fail: %#v", res)
	}
}

func TestSampleFromCoerced_AllTypes(t *testing.T) {
	s := sampleFromCoerced("t", core.ValueString, "hello")
	if s.ValueText == nil || *s.ValueText != "hello" {
		t.Fatalf("string %#v", s)
	}
	s = sampleFromCoerced("t", core.ValueInt64, int64(-7))
	if s.ValueNum == nil || *s.ValueNum != -7 {
		t.Fatalf("int64 %#v", s)
	}
	s = sampleFromCoerced("t", core.ValueUint, uint32(9))
	if s.ValueNum == nil || *s.ValueNum != 9 {
		t.Fatalf("uint %#v", s)
	}
	s = sampleFromCoerced("t", core.ValueFloat64, 1.25)
	if s.ValueNum == nil || *s.ValueNum != 1.25 {
		t.Fatalf("float %#v", s)
	}
	s = sampleFromCoerced("t", "boolean", true)
	if s.ValueBool == nil || !*s.ValueBool {
		t.Fatalf("alias bool %#v", s)
	}
	ts := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	s = sampleFromCoerced("t", core.ValueDateTime, ts)
	if s.ValueText == nil || !strings.HasPrefix(*s.ValueText, "2026-03-04") {
		t.Fatalf("datetime %#v", s)
	}
	if s.TagID != "t" || s.Quality != core.QualityGood {
		t.Fatalf("meta %#v", s)
	}
}

func TestPickWriteRawValue_TextAndBool(t *testing.T) {
	txt := "hi"
	v, err := pickWriteRawValue(writeValueBody{ValueText: &txt})
	if err != nil || v != "hi" {
		t.Fatalf("%v %v", v, err)
	}
	b := false
	v, err = pickWriteRawValue(writeValueBody{ValueBool: &b})
	if err != nil || v != false {
		t.Fatalf("%v %v", v, err)
	}
}

func TestVerifyWriteReadback_MismatchUntilTimeout(t *testing.T) {
	bad := 0.0
	w := &mockWriter{
		connected: true,
		readSample: core.Sample{
			TagID: "t", Quality: core.QualityGood, ValueNum: &bad,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	want := 1.0
	obs, match, err := (&Server{}).verifyWriteReadback(ctx, w, core.Tag{ID: "t", DataType: core.ValueFloat64},
		core.Sample{TagID: "t", Quality: core.QualityGood, ValueNum: &want})
	if err != nil {
		t.Fatal(err)
	}
	if match || obs == nil || obs.ValueNum == nil || *obs.ValueNum != 0 {
		t.Fatalf("match=%v obs=%#v", match, obs)
	}
}
