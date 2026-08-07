package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	devruntime "github.com/popelev/level2/internal/runtime"
	"github.com/popelev/level2/internal/store"
)

func TestHandleStatusSummary(t *testing.T) {
	live := store.NewLive()
	n := 1.0
	live.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	live.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	live.Update(core.Sample{TagID: "t2", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityBad})

	buf := diag.NewBuffer(50)
	buf.Add(diag.Entry{Level: diag.LevelError, Category: diag.CategoryOPCRead, Message: "read fail"})

	s := &Server{
		Live: live,
		Diag: buf,
		ReadyCheck: func() bool { return true },
		Devices: func() []core.Device {
			return []core.Device{{
				ID: "dev1",
				Tags: []core.Tag{
					{ID: "t1", Enabled: true, IntervalMs: 1000},
					{ID: "t2", Enabled: true, IntervalMs: 1000},
					{ID: "t3", Enabled: false, IntervalMs: 1000},
				},
			}}
		},
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var out statusSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.APIOK || !out.CollectorReady || out.ReadyDetail != "ready" {
		t.Fatalf("ready fields %#v", out)
	}
	if out.DevicesTotal != 1 || out.DevicesDisconnected != 1 {
		t.Fatalf("devices %#v", out)
	}
	if out.TagsTotal != 3 || out.TagsEnabled != 2 {
		t.Fatalf("tags %#v", out)
	}
	if out.QualityGood != 1 || out.QualityBad != 1 {
		t.Fatalf("quality %#v", out)
	}
	if out.QualityGoodPct == nil || *out.QualityGoodPct != 50 {
		t.Fatalf("pct %#v", out.QualityGoodPct)
	}
	if len(out.RecentErrors) != 1 {
		t.Fatalf("recent_errors %#v", out.RecentErrors)
	}

	inc := diag.NewIncidentTracker(100, time.Hour)
	inc.Record(diag.IncidentOPCDisconnect, "dev1")
	inc.Record(diag.IncidentOPCDisconnect, "dev1")
	inc.Record(diag.IncidentCollectorDown, "")
	inc.Record(diag.IncidentDBWriteError, "")
	s.Incidents = inc

	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	var out2 statusSummary
	if err := json.Unmarshal(rr2.Body.Bytes(), &out2); err != nil {
		t.Fatal(err)
	}
	if out2.OPCDisconnectsLastHour != 2 || out2.CollectorDownLastHour != 1 || out2.DBWriteErrorsLastHour != 1 {
		t.Fatalf("incident counts %#v", out2)
	}
	if out2.OPCDisconnectsByDevice["dev1"] != 2 {
		t.Fatalf("by device %#v", out2.OPCDisconnectsByDevice)
	}
}

type offlineDriver struct{}

func (offlineDriver) Connect(context.Context) error    { return nil }
func (offlineDriver) Disconnect(context.Context) error { return nil }
func (offlineDriver) Connected() bool                  { return false }
func (offlineDriver) Subscribe(context.Context, []core.Tag, chan<- core.Sample) error {
	return nil
}

func TestStatusSummary_DisconnectedCountsStaleGoodAsBad(t *testing.T) {
	live := store.NewLive()
	n := 1.0
	live.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	live.Update(core.Sample{TagID: "t2", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})

	hub := devruntime.NewHub(nil, false)
	hub.InjectDriver(core.Device{ID: "dev1"}, offlineDriver{}, nil)

	s := &Server{
		Live:   live,
		DevHub: hub,
		ReadyCheck: func() bool { return false },
		Devices: func() []core.Device {
			return []core.Device{{
				ID: "dev1",
				Tags: []core.Tag{
					{ID: "t1", Enabled: true},
					{ID: "t2", Enabled: true},
					{ID: "t3", Enabled: true}, // no live sample
				},
			}}
		},
	}
	out := s.buildStatusSummary(httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	if out.CollectorReady || out.DevicesConnected != 0 || out.DevicesDisconnected != 1 {
		t.Fatalf("connectivity %#v", out)
	}
	if out.QualityGood != 0 || out.QualityBad != 3 {
		t.Fatalf("want all bad while disconnected, got good=%d bad=%d unknown=%d", out.QualityGood, out.QualityBad, out.QualityUnknown)
	}
	if out.QualityGoodPct == nil || *out.QualityGoodPct != 0 {
		t.Fatalf("pct %#v", out.QualityGoodPct)
	}
}

func TestStatusSummary_TagSimulationSkipsStaleOverride(t *testing.T) {
	live := store.NewLive()
	n := 1.0
	live.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})

	hub := devruntime.NewHub(nil, false)
	hub.InjectDriver(core.Device{ID: "dev1"}, offlineDriver{}, nil)

	s := &Server{
		Live:   live,
		DevHub: hub,
		ReadyCheck: func() bool { return true },
		TagSimulationActive: func() bool { return true },
		Devices: func() []core.Device {
			return []core.Device{{
				ID:   "dev1",
				Tags: []core.Tag{{ID: "t1", Enabled: true}},
			}}
		},
	}
	out := s.buildStatusSummary(httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	if !out.TagSimulation || out.ReadyDetail != "tag simulation" {
		t.Fatalf("%#v", out)
	}
	if out.QualityGood != 1 || out.QualityBad != 0 {
		t.Fatalf("sim must keep Live Good: good=%d bad=%d", out.QualityGood, out.QualityBad)
	}
}
