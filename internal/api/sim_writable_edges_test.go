package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
	devruntime "github.com/popelev/level2/internal/runtime"
)

func TestTagEffectivelySimulatedAndCount(t *testing.T) {
	s := &Server{}
	tag := core.Tag{ID: "a", Simulate: false, Enabled: true}
	if s.tagEffectivelySimulated(tag) {
		t.Fatal("default off")
	}
	tag.Simulate = true
	if !s.tagEffectivelySimulated(tag) {
		t.Fatal("per-tag simulate")
	}

	s.TagSimulationActive = func() bool { return true }
	tag.Simulate = false
	if !s.tagEffectivelySimulated(tag) {
		t.Fatal("global sim")
	}
	s.TagSimulationActive = nil
	s.SimBrowserActive = func() bool { return true }
	if !s.tagEffectivelySimulated(tag) || !s.simBrowserActive() {
		t.Fatal("sim browser")
	}

	s = &Server{
		SimBrowserActive: func() bool { return true },
		Tags: func() []core.Tag {
			return []core.Tag{
				{ID: "a", Enabled: true},
				{ID: "b", Enabled: false},
				{ID: "c", Enabled: true},
			}
		},
	}
	if n := s.countTagsSimulated(); n != 2 {
		t.Fatalf("sim browser count=%d", n)
	}

	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x", Security: "None",
		Tags: []core.Tag{
			{ID: "t1", NodeID: "ns=1;i=1", DataType: core.ValueFloat64, Enabled: true, Simulate: true, IntervalMs: 1000},
			{ID: "t2", NodeID: "ns=1;i=2", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
		},
	})
	s = &Server{Cfg: cfg}
	if n := s.countTagsSimulated(); n != 1 {
		t.Fatalf("cfg count=%d", n)
	}

	s = &Server{
		Tags: func() []core.Tag {
			return []core.Tag{{ID: "x", Simulate: true}, {ID: "y", Simulate: false}}
		},
	}
	if n := s.countTagsSimulated(); n != 1 {
		t.Fatalf("tags-only count=%d", n)
	}
}

func TestBulkWritableTags(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{
			{ID: "a", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			{ID: "b", NodeID: "ns=4;i=2", DataType: core.ValueBool, Enabled: true, IntervalMs: 1000},
		},
	})
	s := &Server{Cfg: cfg}
	mux := http.NewServeMux()
	s.mountTagSimulation(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/writable",
		strings.NewReader(`{"writable":true,"all":true}`)))
	if rr.Code != 200 {
		t.Fatalf("all %d %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if int(out["updated"].(float64)) != 2 {
		t.Fatalf("%v", out)
	}
	tags, _ := cfg.DeviceTags("plc")
	for _, tg := range tags {
		if !tg.Writable {
			t.Fatalf("%#v", tg)
		}
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/writable",
		strings.NewReader(`{"writable":false,"tag_ids":["a"]}`)))
	if rr.Code != 200 {
		t.Fatalf("ids %d %s", rr.Code, rr.Body.String())
	}
	var partial map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &partial); err != nil {
		t.Fatal(err)
	}
	if partial["writable"] != false || int(partial["updated"].(float64)) != 1 {
		t.Fatalf("partial clear: %v", partial)
	}
	tags, _ = cfg.DeviceTags("plc")
	byID := map[string]core.Tag{}
	for _, tg := range tags {
		byID[tg.ID] = tg
	}
	if byID["a"].Writable || !byID["b"].Writable {
		t.Fatalf("only a cleared: %#v", byID)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/writable",
		strings.NewReader(`{"writable":true}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want tag_ids required, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "tag_ids") {
		t.Fatalf("body=%s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/writable",
		strings.NewReader(`{`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/missing/tags/writable",
		strings.NewReader(`{"writable":true,"all":true}`)))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing device %d", rr.Code)
	}

	s2 := &Server{}
	mux2 := http.NewServeMux()
	s2.mountTagSimulation(mux2)
	rr = httptest.NewRecorder()
	mux2.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/writable",
		strings.NewReader(`{"writable":true,"all":true}`)))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("no cfg %d", rr.Code)
	}
}

func TestOPCDriverAndWriteEnabledViaCfg(t *testing.T) {
	s := &Server{}
	if s.opcDriver("x") != nil {
		t.Fatal("no hub")
	}
	if s.opcWriteEnabled() {
		t.Fatal("default off")
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := devruntime.NewHub(log, false)
	hub.InjectDriver(core.Device{ID: "plc"}, offlineDriver{}, nil)
	s.DevHub = hub
	if s.opcDriver("plc") != nil {
		t.Fatal("non-opcua driver")
	}
	if s.opcDriver("missing") != nil {
		t.Fatal("missing")
	}

	drv := opcuaDriver.New(core.Device{ID: "opc"}, log)
	hub.InjectDriver(core.Device{ID: "opc"}, drv, nil)
	// Injected entry Connected follows Driver.Connected(); New driver is offline.
	if s.opcDriver("opc") != nil {
		t.Fatal("disconnected opcua driver")
	}

	dir := t.TempDir()
	cfgOn := config.NewStore(dir+"/c.yaml", &config.File{
		Listen: ":0", SpoolDir: dir, UIDir: dir,
		Database:        config.Database{URL: "postgres://u:p@localhost/db", CapacityPercent: 90, FullPolicy: config.FullPolicyStop},
		OPCWriteEnabled: true,
	})
	s.Cfg = cfgOn
	if !s.opcWriteEnabled() {
		t.Fatal("cfg opc write")
	}
}

func TestHubSetFilterAndTagIDSet(t *testing.T) {
	hub := NewHub()
	// setFilter on unknown/nil conn is a no-op (must not panic).
	hub.setFilter(nil, map[string]struct{}{"a": {}})

	got := tagIDSet([]string{"a", "", "b", "a"})
	if len(got) != 2 {
		t.Fatalf("%#v", got)
	}
	if _, ok := got["a"]; !ok {
		t.Fatal("missing a")
	}
	if _, ok := got["b"]; !ok {
		t.Fatal("missing b")
	}
	if _, ok := got[""]; ok {
		t.Fatal("empty id must be dropped")
	}
	if empty := tagIDSet(nil); len(empty) != 0 {
		t.Fatalf("nil input: %#v", empty)
	}
	if empty := tagIDSet([]string{"", ""}); len(empty) != 0 {
		t.Fatalf("only empties: %#v", empty)
	}
}

func TestPatchTag_WritableAndEnabled(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x", Security: "None",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=1;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	})
	s := &Server{Cfg: cfg}
	mux := http.NewServeMux()
	s.mountTagSimulation(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/api/v1/devices/plc/tags/t1",
		strings.NewReader(`{"writable":true,"enabled":false}`)))
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var tag core.Tag
	if err := json.Unmarshal(rr.Body.Bytes(), &tag); err != nil {
		t.Fatal(err)
	}
	if !tag.Writable || tag.Enabled {
		t.Fatalf("%#v", tag)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/api/v1/devices/plc/tags/t1",
		strings.NewReader(`{}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty patch %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/api/v1/devices/plc/tags/missing",
		strings.NewReader(`{"simulate":true}`)))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing tag %d", rr.Code)
	}
}
