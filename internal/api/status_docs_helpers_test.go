package api

import (
	"context"
	"encoding/json"
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

func TestOpenAPIDocs_TrailingSlashAndHeaders(t *testing.T) {
	s := &Server{
		Live:    store.NewLive(),
		Devices: func() []core.Device { return nil },
		Tags:    func() []core.Tag { return nil },
		Hub:     NewHub(),
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("docs/ %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "swagger-ui") || !strings.Contains(body, "/api/v1/openapi.yaml") {
		t.Fatalf("docs html missing markers")
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("ct=%q", ct)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil))
	if rr.Code != 200 {
		t.Fatalf("openapi %d", rr.Code)
	}
	if cc := rr.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=60") {
		t.Fatalf("cache=%q", cc)
	}
	if !strings.Contains(rr.Body.String(), "openapi:") {
		t.Fatal("yaml body")
	}
}

func TestBuildStatusSummary_TagsFallbackAndSimBrowser(t *testing.T) {
	s := &Server{
		ReadyCheck:      func() bool { return true },
		SimBrowserActive: func() bool { return true },
		Devices: func() []core.Device {
			return []core.Device{{ID: "d1"}}
		},
		Tags: func() []core.Tag {
			return []core.Tag{
				{ID: "a", Enabled: true, Simulate: true},
				{ID: "b", Enabled: true},
				{ID: "c", Enabled: false},
			}
		},
	}
	out := s.buildStatusSummary(httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	if !out.SimBrowser || out.ReadyDetail != "sim browser" {
		t.Fatalf("ready %#v", out)
	}
	if out.TagsTotal != 3 || out.TagsEnabled != 2 {
		t.Fatalf("tags %#v", out)
	}
	// a Simulate + b via sim browser + enabled → 2 simulated (c disabled)
	if out.TagsSimulated != 2 {
		t.Fatalf("simulated=%d", out.TagsSimulated)
	}
	if out.DevicesTotal != 1 || out.DevicesConnected != 0 {
		t.Fatalf("devices %#v", out)
	}
}

func TestBuildStatusSummary_LivePollAvgAndNilSample(t *testing.T) {
	live := store.NewLive()
	n := 3.0
	live.Update(core.Sample{TagID: "good", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	avg := int64(12)
	// SnapshotDevices fills PollAvgMs from live history; seed a second update so avg can exist.
	live.Update(core.Sample{TagID: "good", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	_ = avg

	hub := devruntime.NewHub(nil, false)
	hub.InjectDriver(core.Device{ID: "dev1"}, statusOnlineDriver{}, nil)

	s := &Server{
		Live:   live,
		DevHub: hub,
		ReadyCheck: func() bool { return true },
		Devices: func() []core.Device {
			return []core.Device{{
				ID: "dev1",
				Tags: []core.Tag{
					{ID: "good", Enabled: true},
					{ID: "empty", Enabled: true},
					{ID: "sim", Enabled: true, Simulate: true},
				},
			}}
		},
	}
	out := s.buildStatusSummary(httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	if !out.CollectorReady || out.ReadyDetail != "ready" {
		t.Fatalf("ready %#v", out)
	}
	if out.DevicesConnected != 1 || out.DevicesDisconnected != 0 {
		t.Fatalf("conn %#v", out)
	}
	if out.QualityGood < 1 {
		t.Fatalf("quality %#v", out)
	}
	// empty enabled + connected → unknown (not stale-bad)
	if out.QualityUnknown < 1 {
		t.Fatalf("want unknown for empty sample: %#v", out)
	}
	if out.TagsSimulated < 1 {
		t.Fatalf("per-tag simulate count %#v", out)
	}
}

type statusOnlineDriver struct{}

func (statusOnlineDriver) Connect(context.Context) error    { return nil }
func (statusOnlineDriver) Disconnect(context.Context) error { return nil }
func (statusOnlineDriver) Connected() bool                  { return true }
func (statusOnlineDriver) Subscribe(context.Context, []core.Tag, chan<- core.Sample) error {
	return nil
}

func TestBuildStatusSummary_DevHubReadyAndCountSimulated(t *testing.T) {
	hub := devruntime.NewHub(nil, false)
	hub.InjectDriver(core.Device{ID: "dev1"}, statusOnlineDriver{}, nil)

	inc := diag.NewIncidentTracker(10, time.Hour)
	inc.Record(diag.IncidentOPCDisconnect, "dev1")

	s := &Server{
		DevHub:    hub,
		Incidents: inc,
		Devices: func() []core.Device {
			return []core.Device{{ID: "dev1", Tags: []core.Tag{
				{ID: "x", Enabled: true, Simulate: true},
			}}}
		},
		Tags: func() []core.Tag {
			return []core.Tag{{ID: "x", Enabled: true, Simulate: true}}
		},
	}
	out := s.buildStatusSummary(httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	if !out.CollectorReady || out.ReadyDetail != "ready" {
		t.Fatalf("devhub ready %#v", out)
	}
	if out.TagsSimulated != 1 {
		t.Fatalf("countTagsSimulated=%d", out.TagsSimulated)
	}
	if out.OPCDisconnectsLastHour != 1 || out.OPCDisconnectsByDevice["dev1"] != 1 {
		t.Fatalf("incidents %#v", out)
	}
}

func TestBuildStatusSummary_JSONRoundTrip(t *testing.T) {
	s := &Server{
		ReadyCheck: func() bool { return false },
		Devices:    func() []core.Device { return nil },
		Tags:       func() []core.Tag { return nil },
	}
	mux := http.NewServeMux()
	s.mountStatus(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	if rr.Code != 200 {
		t.Fatalf("%d", rr.Code)
	}
	var out statusSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.APIOK || out.CollectorReady || out.ReadyDetail != "not connected" {
		t.Fatalf("%#v", out)
	}
}
