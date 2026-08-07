package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/popelev/level2/internal/api"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/driver/mock"
	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
	devruntime "github.com/popelev/level2/internal/runtime"
	"github.com/popelev/level2/internal/store"
)

func TestWireHTTP_HealthReadyMetricsAndUI(t *testing.T) {
	log := testLog()
	cfg := testConfig(t)
	live := store.NewLive()
	devHub := devruntime.NewHub(log, true)
	if err := devHub.Upsert(context.Background(), testDevice("plc")); err != nil {
		t.Fatal(err)
	}
	apiSrv := &api.Server{
		Log: log, Live: live, Hub: api.NewHub(), Cfg: cfg,
		Tags: cfg.AllTags, Devices: cfg.Devices, DevHub: devHub,
	}
	ui := t.TempDir()
	if err := os.WriteFile(filepath.Join(ui, "index.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := wireHTTP(log, ui, apiSrv, false, devHub)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != 200 || rr.Body.String() != "ok" {
		t.Fatalf("healthz %d %q", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != 200 {
		t.Fatalf("readyz want 200 got %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rr.Code != 200 {
		t.Fatalf("metrics %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != 200 || rr.Body.Len() == 0 {
		t.Fatalf("ui %d len=%d", rr.Code, rr.Body.Len())
	}
}

func TestWireHTTP_ReadyzNotConnected(t *testing.T) {
	log := testLog()
	cfg := testConfig(t)
	devHub := devruntime.NewHub(log, false)
	apiSrv := &api.Server{
		Log: log, Live: store.NewLive(), Hub: api.NewHub(), Cfg: cfg,
		Tags: cfg.AllTags, Devices: cfg.Devices, DevHub: devHub,
	}
	h := wireHTTP(log, t.TempDir()+"/missing-ui", apiSrv, false, devHub)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 got %d", rr.Code)
	}

	h2 := wireHTTP(log, "", apiSrv, true, devHub)
	rr = httptest.NewRecorder()
	h2.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != 200 {
		t.Fatalf("sim ready: %d", rr.Code)
	}
}

func TestStartDeviceCollect_Guards(t *testing.T) {
	log := testLog()
	cfg := testConfig(t, testDevice("plc", testTag("t", "")))
	hub := devruntime.NewHub(log, false)
	var once sync.Map
	samples := make(chan core.Sample, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startDeviceCollect(ctx, log, cfg, hub, &once, true, samples, "plc")
	if _, ok := once.Load("plc"); ok {
		t.Fatal("full sim must not arm collectOnce")
	}

	startDeviceCollect(ctx, log, cfg, hub, &once, false, samples, "plc")
	if _, ok := once.Load("plc"); !ok {
		t.Fatal("expected collectOnce armed")
	}
	startDeviceCollect(ctx, log, cfg, hub, &once, false, samples, "plc")

	once2 := sync.Map{}
	hub.InjectDriver(testDevice("x"), &stubDriver{on: true}, nil)
	startDeviceCollect(ctx, log, cfg, hub, &once2, false, samples, "x")
}

func TestStartDeviceCollect_LaunchesOPC(t *testing.T) {
	log := testLog()
	dev := testDevice("plc", testTag("t", "ns=4;i=1"))
	cfg := testConfig(t, dev)
	hub := devruntime.NewHub(log, false)
	drv := opcuaDriver.New(dev, log)
	hub.InjectDriver(dev, drv, drv)
	var once sync.Map
	samples := make(chan core.Sample, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startDeviceCollect(ctx, log, cfg, hub, &once, false, samples, "plc")
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestRunDemo_EmptyThenTag(t *testing.T) {
	log := testLog()
	cfg := testConfig(t)
	demo := mock.NewDemo(20 * time.Millisecond)
	if err := demo.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	samples := make(chan core.Sample, 8)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runDemo(ctx, log, demo, cfg, samples, false)
		close(done)
	}()
	if err := cfg.UpsertDevice(testDevice("plc", core.Tag{
		ID: "sim1", NodeID: "ns=1;i=1", DataType: core.ValueFloat64,
		Enabled: true, Simulate: true, IntervalMs: 50,
	})); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	got := false
	for time.Now().Before(deadline) {
		select {
		case <-samples:
			got = true
		default:
			time.Sleep(20 * time.Millisecond)
		}
		if got {
			break
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runDemo did not exit")
	}
	if !got {
		t.Fatal("expected at least one demo sample")
	}
}
