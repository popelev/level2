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

	"github.com/gorilla/websocket"
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

	readSample core.Sample
	readErr    error
	readCalls  int
	// readDelay simulates PLC lag before readback matches written value.
	readDelayMatches int
	readMismatch     core.Sample
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
func (m *mockWriter) ReadValue(_ context.Context, tag core.Tag) (core.Sample, error) {
	m.readCalls++
	if m.readErr != nil {
		return core.Sample{}, m.readErr
	}
	if m.readDelayMatches > 0 && m.readCalls <= m.readDelayMatches {
		if m.readMismatch.TagID != "" {
			return m.readMismatch, nil
		}
		n := 0.0
		return core.Sample{TagID: tag.ID, Quality: core.QualityGood, ValueNum: &n}, nil
	}
	if m.readSample.TagID != "" {
		return m.readSample, nil
	}
	// Default: echo last written float/bool as Good sample.
	s := core.Sample{TagID: tag.ID, Quality: core.QualityGood, Time: time.Now().UTC()}
	switch v := m.lastVal.(type) {
	case float64:
		s.ValueNum = &v
	case bool:
		s.ValueBool = &v
	case int64:
		n := float64(v)
		s.ValueNum = &n
	case uint32:
		n := float64(v)
		s.ValueNum = &n
	case string:
		s.ValueText = &v
	}
	return s, nil
}

func writeTestServer(t *testing.T, enabled bool, w *mockWriter) (*Server, *http.ServeMux) {
	t.Helper()
	live := store.NewLive()
	tag := core.Tag{
		ID: "Motor1.SpeedSP", NodeID: "ns=4;i=4209",
		DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000, Writable: true,
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
	if !strings.Contains(body, "openapi:") || !strings.Contains(body, "/api/v1/tags/{id}/value") || !strings.Contains(body, "/api/v1/tags/values") {
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

func TestHandleWriteTagValue_NotWritable(t *testing.T) {
	live := store.NewLive()
	tag := core.Tag{
		ID: "Motor1.SpeedSP", NodeID: "ns=4;i=4209",
		DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000, Writable: false,
	}
	dev := core.Device{ID: "s7_1500", Endpoint: "opc.tcp://x", Tags: []core.Tag{tag}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := devruntime.NewHub(log, false)
	w := &mockWriter{connected: true}
	hub.InjectDriver(dev, w, nil)
	s := &Server{
		Live: live,
		Tags: func() []core.Tag { return []core.Tag{tag} },
		Devices: func() []core.Device {
			return []core.Device{dev}
		},
		Hub:             NewHub(),
		OPCWriteEnabled: func() bool { return true },
		DevHub:          hub,
	}
	mux := http.NewServeMux()
	s.Mount(mux)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":1,"device_id":"s7_1500"}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d body %s", rr.Code, rr.Body.String())
	}
	if w.calls != 0 {
		t.Fatalf("writer should not be called")
	}
}

func TestHandleBatchWriteTagValues(t *testing.T) {
	live := store.NewLive()
	tags := []core.Tag{
		{ID: "A", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000, Writable: true},
		{ID: "B", NodeID: "ns=4;i=2", DataType: core.ValueBool, Enabled: true, IntervalMs: 1000, Writable: true},
		{ID: "C", NodeID: "ns=4;i=3", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000, Writable: false},
	}
	dev := core.Device{ID: "s7_1500", Endpoint: "opc.tcp://x", Tags: tags}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := devruntime.NewHub(log, false)
	w := &mockWriter{connected: true}
	hub.InjectDriver(dev, w, nil)
	s := &Server{
		Live:            live,
		Tags:            func() []core.Tag { return tags },
		Devices:         func() []core.Device { return []core.Device{dev} },
		Hub:             NewHub(),
		OPCWriteEnabled: func() bool { return true },
		DevHub:          hub,
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	body := `{"writes":[
		{"tag_id":"A","value":1.5,"device_id":"s7_1500"},
		{"tag_id":"B","value":true,"device_id":"s7_1500"},
		{"tag_id":"C","value":9,"device_id":"s7_1500"},
		{"tag_id":"missing","value":1}
	]}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags/values", bytes.NewReader([]byte(body)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body %s", rr.Code, rr.Body.String())
	}
	var resp batchWriteResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.OKCount != 2 || resp.FailCount != 2 || len(resp.Results) != 4 {
		t.Fatalf("%#v", resp)
	}
	if !resp.Results[0].OK || !resp.Results[1].OK {
		t.Fatalf("first two should ok: %#v", resp.Results)
	}
	if resp.Results[2].OK || resp.Results[2].HTTP != http.StatusForbidden {
		t.Fatalf("writable=false: %#v", resp.Results[2])
	}
	if resp.Results[3].OK || resp.Results[3].HTTP != http.StatusNotFound {
		t.Fatalf("missing: %#v", resp.Results[3])
	}
	if w.calls != 2 {
		t.Fatalf("calls=%d", w.calls)
	}
}

func TestHandleBatchWrite_Disabled(t *testing.T) {
	_, mux := writeTestServer(t, false, &mockWriter{connected: true})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags/values",
		bytes.NewReader([]byte(`{"writes":[{"tag_id":"Motor1.SpeedSP","value":1}]}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rr.Code)
	}
}

func TestAPIAuth_TokenGate(t *testing.T) {
	w := &mockWriter{connected: true}
	s, mux := writeTestServer(t, true, w)
	s.APIToken = func() string { return "secret-token" }
	handler := s.APIAuth(mux)

	// missing token → 401
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":1,"device_id":"s7_1500"}`)))
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rr.Code)
	}

	// wrong token → 401
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":1,"device_id":"s7_1500"}`)))
	req.Header.Set("Authorization", "Bearer wrong")
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rr.Code)
	}

	// Bearer ok
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":42,"device_id":"s7_1500"}`)))
	req.Header.Set("Authorization", "Bearer secret-token")
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body %s", rr.Code, rr.Body.String())
	}

	// X-API-Token ok on batch
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tags/values",
		bytes.NewReader([]byte(`{"writes":[{"tag_id":"Motor1.SpeedSP","value":3,"device_id":"s7_1500"}]}`)))
	req.Header.Set("X-API-Token", "secret-token")
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("batch want 200 got %d body %s", rr.Code, rr.Body.String())
	}

	// GET stays open
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET want 200 got %d", rr.Code)
	}
}

func TestParseWSTagFilter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws/stream?tag_id=a&tag_id=b&tag_ids=c,d", nil)
	f := parseWSTagFilter(req)
	if f == nil || len(f) != 4 {
		t.Fatalf("%#v", f)
	}
	for _, id := range []string{"a", "b", "c", "d"} {
		if _, ok := f[id]; !ok {
			t.Fatalf("missing %s", id)
		}
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ws/stream", nil)
	if parseWSTagFilter(req) != nil {
		t.Fatal("expected nil filter")
	}
}

func TestHubBroadcast_Filter(t *testing.T) {
	hub := NewHub()
	wsUp := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := wsUp.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		filter := parseWSTagFilter(r)
		hub.add(c, filter)
		defer hub.remove(c)
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?tag_id=keep"
	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	time.Sleep(50 * time.Millisecond)

	hub.Broadcast(core.Sample{TagID: "skip", Quality: core.QualityGood})
	hub.Broadcast(core.Sample{TagID: "keep", Quality: core.QualityGood, ValueNum: floatPtr(1)})

	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var dto sampleDTOType
	if err := json.Unmarshal(msg, &dto); err != nil {
		t.Fatal(err)
	}
	if dto.TagID != "keep" {
		t.Fatalf("got %#v", dto)
	}
	_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("expected no further messages")
	}
}

func floatPtr(v float64) *float64 { return &v }

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

func TestHandleWriteTagValue_VerifyOK(t *testing.T) {
	w := &mockWriter{connected: true}
	_, mux := writeTestServer(t, true, w)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value?verify=true",
		bytes.NewReader([]byte(`{"value":42.5,"device_id":"s7_1500"}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body %s", rr.Code, rr.Body.String())
	}
	var resp writeValueResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Verified || resp.Observed == nil || resp.Observed.ValueNum == nil || *resp.Observed.ValueNum != 42.5 {
		t.Fatalf("%#v", resp)
	}
	if w.readCalls < 1 {
		t.Fatalf("expected verify read, calls=%d", w.readCalls)
	}
}

func TestHandleWriteTagValue_VerifyMismatch(t *testing.T) {
	bad := 99.0
	w := &mockWriter{
		connected: true,
		readSample: core.Sample{
			TagID: "Motor1.SpeedSP", Quality: core.QualityGood, ValueNum: &bad,
		},
	}
	_, mux := writeTestServer(t, true, w)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":42.5,"device_id":"s7_1500","verify":true,"verify_timeout_ms":80}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409 got %d body %s", rr.Code, rr.Body.String())
	}
	var resp writeValueResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Verified || resp.Written.ValueNum == nil || *resp.Written.ValueNum != 42.5 {
		t.Fatalf("written %#v", resp)
	}
	if resp.Observed == nil || resp.Observed.ValueNum == nil || *resp.Observed.ValueNum != 99 {
		t.Fatalf("observed %#v", resp)
	}
	if resp.Error != "verify_failed" {
		t.Fatalf("error %#v", resp)
	}
}

func TestHandleWriteTagValue_VerifyBodyFlag(t *testing.T) {
	w := &mockWriter{connected: true}
	_, mux := writeTestServer(t, true, w)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":1.25,"device_id":"s7_1500","verify":true}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body %s", rr.Code, rr.Body.String())
	}
	var resp writeValueResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Verified {
		t.Fatalf("%#v", resp)
	}
}

func TestHandleWriteTagValue_OptimisticFalse(t *testing.T) {
	w := &mockWriter{connected: true}
	s, mux := writeTestServer(t, true, w)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value",
		bytes.NewReader([]byte(`{"value":7,"device_id":"s7_1500","optimistic":false}`)))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body %s", rr.Code, rr.Body.String())
	}
	sample, ok := s.Live.Get("Motor1.SpeedSP")
	if !ok || sample.ValueNum == nil || *sample.ValueNum != 7 {
		t.Fatalf("live should update after write even when optimistic=false without verify: %#v ok=%v", sample, ok)
	}
}

func TestWriteVerifyMatch(t *testing.T) {
	n := 42.5
	exp := core.Sample{Quality: core.QualityGood, ValueNum: &n}
	obs := core.Sample{Quality: core.QualityGood, ValueNum: floatPtr(42.5 + 1e-9)}
	if !writeVerifyMatch(exp, obs, core.ValueFloat64) {
		t.Fatal("expected float epsilon match")
	}
	obs2 := core.Sample{Quality: core.QualityGood, ValueNum: floatPtr(43)}
	if writeVerifyMatch(exp, obs2, core.ValueFloat64) {
		t.Fatal("expected mismatch")
	}
	b := true
	if !writeVerifyMatch(
		core.Sample{Quality: core.QualityGood, ValueBool: &b},
		core.Sample{Quality: core.QualityGood, ValueBool: &b},
		core.ValueBool,
	) {
		t.Fatal("bool match")
	}
}

func TestNormalizeVerifyTimeoutMs(t *testing.T) {
	if normalizeVerifyTimeoutMs(0) != 2000*time.Millisecond {
		t.Fatal("default")
	}
	if normalizeVerifyTimeoutMs(100) != 100*time.Millisecond {
		t.Fatal("custom")
	}
	if normalizeVerifyTimeoutMs(999999) != 30000*time.Millisecond {
		t.Fatal("cap")
	}
}

