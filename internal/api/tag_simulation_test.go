package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	devruntime "github.com/popelev/level2/internal/runtime"
	"github.com/popelev/level2/internal/store"
)

func TestTagSimulation_GetPut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	yaml := `
listen: ":8080"
database:
  url: postgres://u:p@localhost:5432/db
devices:
  - id: plc
    endpoint: opc.tcp://plc:4840
    tags:
      - id: t1
        node_id: ns=4;i=1
        datatype: float64
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfgStore := config.NewStore(path, f)
	s := &Server{
		Cfg:                 cfgStore,
		TagSimulationActive: func() bool { return false },
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tag-simulation", nil))
	if rr.Code != 200 {
		t.Fatalf("get %d %s", rr.Code, rr.Body.String())
	}
	var got tagSimulationDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled || got.Active || got.RestartRequired || got.TagsSimulated != 0 {
		t.Fatalf("default off: %#v", got)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tag-simulation",
		strings.NewReader(`{"enabled":true}`))
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req)
	if rr2.Code != 200 {
		t.Fatalf("put %d %s", rr2.Code, rr2.Body.String())
	}
	var after tagSimulationDTO
	if err := json.Unmarshal(rr2.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if !after.Enabled || after.Active || !after.RestartRequired {
		t.Fatalf("want enabled+restart: %#v", after)
	}
	if !cfgStore.TagSimulation() {
		t.Fatal("store not persisted")
	}
}

func TestPerTagSimulate_PatchAndBulk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	yaml := `
listen: ":8080"
database:
  url: postgres://u:p@localhost:5432/db
devices:
  - id: plc
    endpoint: opc.tcp://plc:4840
    tags:
      - id: t1
        node_id: ns=4;i=1
        datatype: float64
        enabled: true
      - id: t2
        node_id: ns=4;i=2
        datatype: float64
        enabled: true
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfgStore := config.NewStore(path, f)
	s := &Server{Cfg: cfgStore, Tags: cfgStore.AllTags, Devices: cfgStore.Devices}
	mux := http.NewServeMux()
	s.Mount(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/devices/plc/tags/t1",
		strings.NewReader(`{"simulate":true}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("patch %d %s", rr.Code, rr.Body.String())
	}
	var tag core.Tag
	if err := json.Unmarshal(rr.Body.Bytes(), &tag); err != nil {
		t.Fatal(err)
	}
	if !tag.Simulate {
		t.Fatalf("want simulate: %#v", tag)
	}
	if cfgStore.CountSimulatedTags() != 1 {
		t.Fatalf("count=%d", cfgStore.CountSimulatedTags())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/simulate",
		strings.NewReader(`{"simulate":true,"all":true}`))
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Code != 200 {
		t.Fatalf("bulk %d %s", rr2.Code, rr2.Body.String())
	}
	var bulk map[string]any
	if err := json.Unmarshal(rr2.Body.Bytes(), &bulk); err != nil {
		t.Fatal(err)
	}
	if int(bulk["updated"].(float64)) != 2 {
		t.Fatalf("bulk %#v", bulk)
	}
	if cfgStore.CountSimulatedTags() != 2 {
		t.Fatalf("after all count=%d", cfgStore.CountSimulatedTags())
	}

	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/tags/simulate",
		strings.NewReader(`{"simulate":false,"tag_ids":["t1"]}`))
	rr3 := httptest.NewRecorder()
	mux.ServeHTTP(rr3, req3)
	if rr3.Code != 200 {
		t.Fatalf("cross %d %s", rr3.Code, rr3.Body.String())
	}
	if cfgStore.CountSimulatedTags() != 1 {
		t.Fatalf("after unsim t1 count=%d", cfgStore.CountSimulatedTags())
	}
}

func TestStatusSummary_TagsSimulatedAndPerTagStale(t *testing.T) {
	live := store.NewLive()
	n := 1.0
	now := time.Now().UTC()
	live.Update(core.Sample{TagID: "t1", Time: now, ValueNum: &n, Quality: core.QualityGood})
	live.Update(core.Sample{TagID: "t2", Time: now, ValueNum: &n, Quality: core.QualityGood})

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
					{ID: "t1", Enabled: true, Simulate: true},
					{ID: "t2", Enabled: true, Simulate: false},
				},
			}}
		},
	}
	out := s.buildStatusSummary(httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	if out.TagsSimulated != 1 {
		t.Fatalf("tags_simulated=%d want 1", out.TagsSimulated)
	}
	// t1 simulated keeps Good; t2 disconnected → Bad
	if out.QualityGood != 1 || out.QualityBad != 1 {
		t.Fatalf("quality good=%d bad=%d", out.QualityGood, out.QualityBad)
	}
}
