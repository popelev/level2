package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	devruntime "github.com/popelev/level2/internal/runtime"
	"github.com/popelev/level2/internal/store"
)

func TestHandleTags_DisconnectedRemapsStaleGood(t *testing.T) {
	live := store.NewLive()
	n := 90.5
	txt := "demo"
	b := true
	live.Update(core.Sample{TagID: "t_bool", Time: time.Now().UTC(), ValueBool: &b, Quality: core.QualityGood})
	live.Update(core.Sample{TagID: "t_num", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	live.Update(core.Sample{TagID: "t_sim", Time: time.Now().UTC(), ValueText: &txt, Quality: core.QualityGood})

	hub := devruntime.NewHub(nil, false)
	hub.InjectDriver(core.Device{ID: "dev1"}, offlineDriver{}, nil)

	s := &Server{
		Live:   live,
		DevHub: hub,
		Devices: func() []core.Device {
			return []core.Device{{
				ID: "dev1",
				Tags: []core.Tag{
					{ID: "t_bool", Enabled: true},
					{ID: "t_num", Enabled: true},
					{ID: "t_sim", Enabled: true, Simulate: true},
					{ID: "t_empty", Enabled: true},
				},
			}}
		},
		Hub: NewHub(),
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags?device_id=dev1", nil))
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var list []store.TagValue
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	byID := map[string]store.TagValue{}
	for _, tv := range list {
		byID[tv.Tag.ID] = tv
	}
	if byID["t_bool"].Sample == nil || byID["t_bool"].Sample.Quality != core.QualityBad {
		t.Fatalf("stale non-sim bool want Bad, got %#v", byID["t_bool"].Sample)
	}
	if byID["t_bool"].Sample.ValueBool == nil || !*byID["t_bool"].Sample.ValueBool {
		t.Fatal("value must be kept on stale remap")
	}
	if byID["t_num"].Sample == nil || byID["t_num"].Sample.Quality != core.QualityBad {
		t.Fatalf("stale non-sim num want Bad, got %#v", byID["t_num"].Sample)
	}
	if byID["t_sim"].Sample == nil || byID["t_sim"].Sample.Quality != core.QualityGood {
		t.Fatalf("simulated tag must keep Good, got %#v", byID["t_sim"].Sample)
	}
	if byID["t_empty"].Sample != nil {
		t.Fatalf("empty tag: %#v", byID["t_empty"].Sample)
	}

	// Live store itself must also flip to Bad after reconcile.
	if s, ok := live.Get("t_num"); !ok || s.Quality != core.QualityBad {
		t.Fatalf("Live should reconcile to Bad after GET /tags, got %#v ok=%v", s, ok)
	}
	if s, ok := live.Get("t_sim"); !ok || s.Quality != core.QualityGood {
		t.Fatalf("simulated Live must stay Good, got %#v ok=%v", s, ok)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/t_num/value", nil))
	if rr.Code != 200 {
		t.Fatalf("value status %d", rr.Code)
	}
	var dto sampleDTOType
	if err := json.Unmarshal(rr.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.Quality != int(core.QualityBad) {
		t.Fatalf("single value want Bad, got %#v", dto)
	}
}

func TestHandleTags_ConnectedKeepsGood(t *testing.T) {
	live := store.NewLive()
	n := 1.0
	live.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})

	hub := devruntime.NewHub(nil, false)
	hub.InjectDriver(core.Device{ID: "dev1"}, onlineDriver{}, nil)

	s := &Server{
		Live:   live,
		DevHub: hub,
		Devices: func() []core.Device {
			return []core.Device{{
				ID:   "dev1",
				Tags: []core.Tag{{ID: "t1", Enabled: true}},
			}}
		},
		Hub: NewHub(),
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags?device_id=dev1", nil))
	var list []store.TagValue
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Sample == nil || list[0].Sample.Quality != core.QualityGood {
		t.Fatalf("connected must keep Good: %#v", list)
	}
}

type onlineDriver struct{}

func (onlineDriver) Connect(context.Context) error    { return nil }
func (onlineDriver) Disconnect(context.Context) error { return nil }
func (onlineDriver) Connected() bool                  { return true }
func (onlineDriver) Subscribe(context.Context, []core.Tag, chan<- core.Sample) error {
	return nil
}
