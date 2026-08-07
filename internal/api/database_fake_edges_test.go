package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/metrics"
	"github.com/popelev/level2/internal/store"
)

// fakeDB implements dbBackend for API handler tests (no live Postgres).
type fakeDB struct {
	status       timescale.ConnectionStatus
	cap          timescale.CapacityStats
	capErr       error
	policy       timescale.CapacityPolicySettings
	setPolicyPct int
	setPolicyStr string
	wipe         timescale.WipeResult
	wipeErr      error
	writeErr     error
	writeCalls   int
	pingErr      error
}

func (f *fakeDB) Status(context.Context, string) timescale.ConnectionStatus { return f.status }
func (f *fakeDB) Capacity(context.Context) (timescale.CapacityStats, error) {
	return f.cap, f.capErr
}
func (f *fakeDB) CapacityPolicy() timescale.CapacityPolicySettings { return f.policy }
func (f *fakeDB) SetCapacityPolicy(percent int, policy string) {
	f.setPolicyPct = percent
	f.setPolicyStr = policy
}
func (f *fakeDB) WipeSamples(context.Context) (timescale.WipeResult, error) {
	return f.wipe, f.wipeErr
}
func (f *fakeDB) WriteBatch(_ context.Context, samples []core.Sample) error {
	f.writeCalls++
	_ = samples
	return f.writeErr
}
func (f *fakeDB) Ping(context.Context) error { return f.pingErr }

func TestDatabaseStatus_WithFakeDB(t *testing.T) {
	cfg := testAPIConfig(t)
	db := &fakeDB{status: timescale.ConnectionStatus{
		Connected: true, PingOK: true, Host: "db", Port: "5432",
		Database: "level2", User: "u", SSLMode: "disable",
		ServerVersion: "16", TimescaleVer: "2.14",
		PoolMaxConns: 4, PoolTotalConns: 2, PoolIdleConns: 1, PoolAcquired: 1,
	}}
	s := &Server{Cfg: cfg, DB: db}
	mux := http.NewServeMux()
	s.mountDatabase(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/database/status", nil))
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["connected"] != true || out["ready"] != true || out["host"] != "db" {
		t.Fatalf("%v", out)
	}
}

func TestHandleCapacity_WithFakeDBAndRateFallback(t *testing.T) {
	free := int64(1_000_000)
	db := &fakeDB{cap: timescale.CapacityStats{
		DatabaseSizeBytes: 100, SamplesSizeBytes: 50, SamplesApproxRows: 10,
		SamplesLast5Min: 0, SamplesPerSec: 0, AvgSampleBytes: 100,
		FreeBytes: &free, FreeBytesSource: "limit", WindowSeconds: 300,
		CapacityPercent: 90, FullPolicy: "stop",
	}}
	s := &Server{
		DB:   db,
		Tags: func() []core.Tag { return []core.Tag{{ID: "a"}, {ID: "b"}, {ID: "c"}} },
	}
	mux := http.NewServeMux()
	s.mountDiagnostics(mux)

	rateMu.Lock()
	ratePrev = counterValue(metrics.SamplesWritten) - 20
	ratePrevTime = time.Now().Add(-2 * time.Second)
	rateMu.Unlock()

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/capacity", nil))
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["error"] != nil {
		t.Fatalf("unexpected error: %v", out)
	}
	if out["tag_count"].(float64) != 3 {
		t.Fatalf("tag_count=%v", out["tag_count"])
	}
	if out["samples_per_sec"].(float64) <= 0 {
		t.Fatalf("expected rate fallback samples_per_sec, got %v", out["samples_per_sec"])
	}
	if out["eta_seconds"] == nil {
		t.Fatalf("expected eta from free_bytes/rate: %v", out)
	}
}

func TestHandleCapacity_FakeDBError(t *testing.T) {
	db := &fakeDB{capErr: errors.New("capacity query failed")}
	s := &Server{DB: db}
	mux := http.NewServeMux()
	s.mountDiagnostics(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/capacity", nil))
	if rr.Code != 200 {
		t.Fatalf("%d", rr.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["error"] != "capacity query failed" {
		t.Fatalf("%v", out)
	}
}

func TestCapacityPolicy_WithFakeDB(t *testing.T) {
	cfg := testAPIConfig(t)
	free := int64(5000)
	db := &fakeDB{
		policy: timescale.CapacityPolicySettings{Percent: 85, Policy: "drop_oldest"},
		cap: timescale.CapacityStats{
			DiskPath: "/data", DiskAvailBytes: &free, DiskTotalBytes: &free,
			LimitBytes: &free, DatabaseSizeBytes: 100, FreeBytes: &free,
			FreeBytesSource: "disk", UsedOverLimit: false,
		},
	}
	s := &Server{Cfg: cfg, DB: db}
	mux := http.NewServeMux()
	s.mountDatabase(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/database/capacity-policy", nil))
	if rr.Code != 200 {
		t.Fatalf("%d", rr.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["capacity_percent"].(float64) != 85 || got["full_policy"] != "drop_oldest" {
		t.Fatalf("live policy: %v", got)
	}
	if got["disk_path"] != "/data" {
		t.Fatalf("disk fields: %v", got)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/database/capacity-policy",
		strings.NewReader(`{"capacity_percent":70,"full_policy":"stop"}`))
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("put %d %s", rr.Code, rr.Body.String())
	}
	if db.setPolicyPct != 70 || db.setPolicyStr != "stop" {
		t.Fatalf("SetCapacityPolicy not applied: pct=%d policy=%q", db.setPolicyPct, db.setPolicyStr)
	}
}

func TestWipeSamples_FakeSuccessClearTagsAndReseed(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x", Security: "None",
		Tags: []core.Tag{
			{ID: "t1", NodeID: "ns=1;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			{ID: "t2", NodeID: "ns=1;i=2", DataType: core.ValueBool, Enabled: true, IntervalMs: 1000},
		},
	})
	live := store.NewLive()
	n := 1.0
	live.Update(core.Sample{TagID: "t1", ValueNum: &n, Quality: core.QualityGood, Time: time.Now().UTC()})
	changed := 0
	db := &fakeDB{wipe: timescale.WipeResult{Method: "truncate", ApproxRows: 42}}
	s := &Server{
		Cfg: cfg, DB: db, Live: live,
		OnDeviceChanged: func(string, bool) { changed++ },
	}
	mux := http.NewServeMux()
	s.mountDatabase(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/database/wipe-samples?confirm=wipe",
		strings.NewReader(`{"clear_tags":true}`))
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "wiped" || out["method"] != "truncate" {
		t.Fatalf("%v", out)
	}
	if int(out["tags_removed"].(float64)) != 2 || int(out["devices_cleared"].(float64)) != 1 {
		t.Fatalf("clear_tags: %v", out)
	}
	if int(out["live_cleared"].(float64)) != 1 || int(out["reseeded"].(float64)) != 1 {
		t.Fatalf("reseed: %v", out)
	}
	if db.writeCalls != 1 {
		t.Fatalf("writeCalls=%d", db.writeCalls)
	}
	if changed != 1 {
		t.Fatalf("OnDeviceChanged calls=%d", changed)
	}
	tags, _ := cfg.DeviceTags("plc")
	if len(tags) != 0 {
		t.Fatalf("tags not cleared: %d", len(tags))
	}
}

func TestWipeSamples_FakeReseedErrorAndWipeError(t *testing.T) {
	live := store.NewLive()
	n := 2.0
	live.Update(core.Sample{TagID: "a", ValueNum: &n, Quality: core.QualityGood, Time: time.Now().UTC()})
	db := &fakeDB{
		wipe:     timescale.WipeResult{Method: "delete", ApproxRows: 1},
		writeErr: errors.New("reseed failed"),
	}
	s := &Server{DB: db, Live: live}
	mux := http.NewServeMux()
	s.mountDatabase(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/database/wipe-samples?confirm=wipe", nil))
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["reseed_error"] == nil || !strings.Contains(out["reseed_error"].(string), "reseed failed") {
		t.Fatalf("%v", out)
	}
	if out["status"] != "wiped" || out["method"] != "delete" {
		t.Fatalf("wipe status: %v", out)
	}

	db2 := &fakeDB{wipeErr: errors.New("wipe boom")}
	s2 := &Server{DB: db2}
	mux2 := http.NewServeMux()
	s2.mountDatabase(mux2)
	rr = httptest.NewRecorder()
	mux2.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/database/wipe-samples?confirm=wipe", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("wipe err want 500 got %d", rr.Code)
	}
}

func TestWipeSamples_ClearTagsWithoutConfig(t *testing.T) {
	db := &fakeDB{wipe: timescale.WipeResult{Method: "truncate", ApproxRows: 0}}
	s := &Server{DB: db, Live: store.NewLive()}
	mux := http.NewServeMux()
	s.mountDatabase(mux)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/database/wipe-samples?confirm=wipe",
		strings.NewReader(`{"clear_tags":true}`))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "config store unavailable") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}
