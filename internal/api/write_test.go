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

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	devruntime "github.com/popelev/level2/internal/runtime"
	"github.com/popelev/level2/internal/store"
)

type mockWriter struct {
	connected bool
	lastTag   core.Tag
	lastVal   any
	err       error
	calls     int
}

func (m *mockWriter) Connect(context.Context) error    { return nil }
func (m *mockWriter) Disconnect(context.Context) error { return nil }
func (m *mockWriter) Connected() bool                  { return m.connected }
func (m *mockWriter) Subscribe(context.Context, []core.Tag, chan<- core.Sample) error {
	return fmt.Errorf("no subscribe")
}
func (m *mockWriter) WriteValue(_ context.Context, tag core.Tag, value any) error {
	m.calls++
	m.lastTag = tag
	m.lastVal = value
	return m.err
}

func writeTestServer(t *testing.T, enabled bool, w *mockWriter) (*Server, *http.ServeMux) {
	t.Helper()
	live := store.NewLive()
	tag := core.Tag{
		ID: "Motor1.SpeedSP", NodeID: "ns=4;i=4209",
		DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000,
	}
	dev := core.Device{ID: "s7_1500", Endpoint: "opc.tcp://x", Tags: []core.Tag{tag}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := devruntime.NewHub(log, false)
	hub.InjectDriver(dev, w, nil)
	s := &Server{
		Live: live,
		Tags: func() []core.Tag { return []core.Tag{tag} },
		Devices: func() []core.Device {
			return []core.Device{dev}
		},
		Hub:             NewHub(),
		OPCWriteEnabled: func() bool { return enabled },
		DevHub:          hub,
	}
	mux := http.NewServeMux()
	s.Mount(mux)
	return s, mux
}

func TestHandleWriteTagValue_Disabled(t *testing.T) {
	buf := diag.NewBuffer(50)
	diag.SetDefault(buf)
	t.Cleanup(func() { diag.SetDefault(nil) })

	_, mux := writeTestServer(t, false, &mockWriter{connected: true})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":42.5,"device_id":"s7_1500"}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d body %s", rr.Code, rr.Body.String())
	}
}

func TestHandleWriteTagValue_CoercionBad(t *testing.T) {
	_, mux := writeTestServer(t, true, &mockWriter{connected: true})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":"not-a-float","device_id":"s7_1500"}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d body %s", rr.Code, rr.Body.String())
	}
}

func TestHandleWriteTagValue_UnknownTag(t *testing.T) {
	_, mux := writeTestServer(t, true, &mockWriter{connected: true})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/missing/value",
		bytes.NewReader([]byte(`{"value":1}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", rr.Code)
	}
}

func TestHandleWriteTagValue_HappyPath(t *testing.T) {
	buf := diag.NewBuffer(50)
	diag.SetDefault(buf)
	t.Cleanup(func() { diag.SetDefault(nil) })

	w := &mockWriter{connected: true}
	s, mux := writeTestServer(t, true, w)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":42.5,"device_id":"s7_1500"}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body %s", rr.Code, rr.Body.String())
	}
	if w.calls != 1 {
		t.Fatalf("calls=%d", w.calls)
	}
	if w.lastVal != float64(42.5) {
		t.Fatalf("wrote %#v", w.lastVal)
	}
	var resp writeValueResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "Good" || resp.Verified || resp.Written.ValueNum == nil || *resp.Written.ValueNum != 42.5 {
		t.Fatalf("%#v", resp)
	}
	sample, ok := s.Live.Get("Motor1.SpeedSP")
	if !ok || sample.ValueNum == nil || *sample.ValueNum != 42.5 {
		t.Fatalf("live %#v ok=%v", sample, ok)
	}
}

func TestHandleWriteTagValue_NotConnected(t *testing.T) {
	_, mux := writeTestServer(t, true, &mockWriter{connected: false})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":1,"device_id":"s7_1500"}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409 got %d body %s", rr.Code, rr.Body.String())
	}
}

func TestOpenAPIServe(t *testing.T) {
	s := &Server{Live: store.NewLive(), Devices: func() []core.Device { return nil }, Tags: func() []core.Tag { return nil }, Hub: NewHub()}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil))
	if rr.Code != 200 {
		t.Fatalf("openapi %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "openapi:") || !strings.Contains(body, "/api/v1/tags/{id}/value") {
		prefix := body
		if len(prefix) > 120 {
			prefix = prefix[:120]
		}
		t.Fatalf("unexpected openapi body prefix: %s", prefix)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "yaml") {
		t.Fatalf("ct=%q", ct)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if rr.Code != 200 {
		t.Fatalf("docs %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "swagger-ui") {
		t.Fatalf("docs html missing swagger-ui")
	}
}

func TestPickWriteRawValue(t *testing.T) {
	v, err := pickWriteRawValue(writeValueBody{Value: 1.0})
	if err != nil || v != 1.0 {
		t.Fatalf("%v %v", v, err)
	}
	n := 2.0
	v, err = pickWriteRawValue(writeValueBody{ValueNum: &n})
	if err != nil || v != 2.0 {
		t.Fatalf("%v %v", v, err)
	}
	if _, err := pickWriteRawValue(writeValueBody{}); err == nil {
		t.Fatal("expected error")
	}
	b := true
	if _, err := pickWriteRawValue(writeValueBody{Value: true, ValueBool: &b}); err == nil {
		t.Fatal("expected multi-field error")
	}
}

func TestSampleFromCoerced(t *testing.T) {
	s := sampleFromCoerced("t", core.ValueBool, true)
	if s.ValueBool == nil || !*s.ValueBool {
		t.Fatalf("%#v", s)
	}
	s = sampleFromCoerced("t", core.ValueDateTime, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if s.ValueText == nil || !strings.HasPrefix(*s.ValueText, "2026-01-02") {
		t.Fatalf("%#v", s)
	}
}
