package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/importexcel"
	"github.com/popelev/level2/internal/projectxlsx"
	"github.com/popelev/level2/internal/store"
)

func TestTagSimulation_ErrorEdges(t *testing.T) {
	muxNoCfg := http.NewServeMux()
	(&Server{}).mountTagSimulation(muxNoCfg)

	rr := httptest.NewRecorder()
	muxNoCfg.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/v1/tag-simulation",
		strings.NewReader(`{"enabled":true}`)))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("put no cfg %d", rr.Code)
	}

	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x", Security: "None",
		Tags: []core.Tag{
			{ID: "t1", NodeID: "ns=1;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			{ID: "t2", NodeID: "ns=1;i=2", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
		},
	})
	s := &Server{Cfg: cfg, Tags: cfg.AllTags, Devices: cfg.Devices}
	mux := http.NewServeMux()
	s.mountTagSimulation(mux)

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/v1/tag-simulation",
		strings.NewReader(`{`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json put %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/simulate",
		strings.NewReader(`{`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json bulk %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/missing/tags/simulate",
		strings.NewReader(`{"simulate":true,"all":true}`)))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing device %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/simulate",
		strings.NewReader(`{"simulate":true,"tag_ids":["t1"]}`)))
	if rr.Code != 200 {
		t.Fatalf("ids bulk %d %s", rr.Code, rr.Body.String())
	}
	var bulk map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &bulk); err != nil {
		t.Fatal(err)
	}
	if bulk["device_id"] != "plc" || bulk["simulate"] != true || int(bulk["updated"].(float64)) != 1 {
		t.Fatalf("ids bulk body: %v", bulk)
	}
	if int(bulk["tags_simulated"].(float64)) != 1 {
		t.Fatalf("tags_simulated after t1: %v", bulk)
	}
	tags, err := cfg.DeviceTags("plc")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]core.Tag{}
	for _, tg := range tags {
		byID[tg.ID] = tg
	}
	if !byID["t1"].Simulate || byID["t2"].Simulate {
		t.Fatalf("only t1 should be simulated: %#v", byID)
	}

	rr = httptest.NewRecorder()
	muxNoCfg.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/simulate",
		strings.NewReader(`{"simulate":true,"all":true}`)))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("bulk no cfg %d", rr.Code)
	}

	// cross-device simulate: all + device filter + missing device + bad filter
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/simulate",
		strings.NewReader(`{`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("all bad json %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/simulate",
		strings.NewReader(`{"simulate":true}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("need tag_ids or all %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/simulate",
		strings.NewReader(`{"simulate":true,"all":true,"device_id":"nope"}`)))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("device filter missing %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/simulate",
		strings.NewReader(`{"simulate":false,"all":true,"device_id":"plc"}`)))
	if rr.Code != 200 {
		t.Fatalf("all scoped %d %s", rr.Code, rr.Body.String())
	}
	var scoped map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &scoped); err != nil {
		t.Fatal(err)
	}
	if scoped["simulate"] != false || int(scoped["updated"].(float64)) < 1 {
		t.Fatalf("scoped clear: %v", scoped)
	}
	if int(scoped["tags_simulated"].(float64)) != 0 {
		t.Fatalf("want tags_simulated=0 after clear: %v", scoped)
	}
	tags, err = cfg.DeviceTags("plc")
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range tags {
		if tg.Simulate {
			t.Fatalf("simulate still set after clear: %#v", tg)
		}
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/simulate",
		strings.NewReader(`{"simulate":true,"tag_ids":["no-such"]}`)))
	if rr.Code != 200 {
		t.Fatalf("no matching ids still 200 updated=0: %d %s", rr.Code, rr.Body.String())
	}
	var miss map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &miss); err != nil {
		t.Fatal(err)
	}
	if int(miss["updated"].(float64)) != 0 || miss["simulate"] != true {
		t.Fatalf("no-such ids: %v", miss)
	}

	rr = httptest.NewRecorder()
	muxNoCfg.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/tags/simulate",
		strings.NewReader(`{"simulate":true,"all":true}`)))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("all no cfg %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/api/v1/devices/plc/tags/t1",
		strings.NewReader(`{`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("patch bad json %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/api/v1/devices/missing/tags/t1",
		strings.NewReader(`{"simulate":true}`)))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("patch missing device %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	muxNoCfg.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/api/v1/devices/plc/tags/t1",
		strings.NewReader(`{"simulate":true}`)))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("patch no cfg %d", rr.Code)
	}
}

func TestTagSimulationDTO_Notes(t *testing.T) {
	cfg := testAPIConfig(t)
	_ = cfg.SetTagSimulation(true)
	s := &Server{
		Cfg:                 cfg,
		TagSimulationActive: func() bool { return false },
		SimBrowserActive:    func() bool { return false },
	}
	dto := s.tagSimulationDTO()
	if !dto.RestartRequired || dto.Note == "" {
		t.Fatalf("%#v", dto)
	}

	s.SimBrowserActive = func() bool { return true }
	dto = s.tagSimulationDTO()
	if !dto.SimBrowser || !strings.Contains(dto.Note, "SIM_BROWSER") {
		t.Fatalf("%#v", dto)
	}

	s.SimBrowserActive = nil
	s.TagSimulationActive = func() bool { return true }
	dto = s.tagSimulationDTO()
	if !dto.Active || !strings.Contains(dto.Note, "global") {
		t.Fatalf("%#v", dto)
	}
}

func TestProjectImportReplaceAndErrors(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{
		ID: "old", Endpoint: "opc.tcp://old", Security: "None",
		Tags: []core.Tag{{ID: "x", NodeID: "ns=1;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	})
	changed := []string{}
	s := &Server{
		Cfg: cfg, Devices: cfg.Devices, Hub: NewHub(), Live: store.NewLive(),
		OnDeviceChanged: func(id string, _ bool) { changed = append(changed, id) },
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	xlsx, err := projectxlsx.Write([]core.Device{{
		ID: "plc", Endpoint: "opc.tcp://new:4840", Security: "None",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "p.xlsx")
	_, _ = fw.Write(xlsx)
	_ = w.Close()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/project/import?mode=replace", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("replace %d %s", rr.Code, rr.Body.String())
	}
	devs := cfg.Devices()
	if len(devs) != 1 || devs[0].ID != "plc" {
		t.Fatalf("replace devices: %+v", devs)
	}
	if len(changed) != 1 || changed[0] != "plc" {
		t.Fatalf("changed=%v", changed)
	}

	// preview/import missing file
	rr = httptest.NewRecorder()
	var empty bytes.Buffer
	w2 := multipart.NewWriter(&empty)
	_ = w2.WriteField("x", "1")
	_ = w2.Close()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/project/preview", &empty)
	req.Header.Set("Content-Type", w2.FormDataContentType())
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("preview no file %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/project/import", strings.NewReader("not-multipart")))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("import bad multipart %d", rr.Code)
	}

	// compare without file side
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/project/compare?a=live&b=file", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("compare missing file %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/project/compare.xlsx?a=live&b=file", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("compare.xlsx missing file %d", rr.Code)
	}

	// compare live unavailable
	s2 := &Server{Hub: NewHub(), Live: store.NewLive()}
	mux2 := http.NewServeMux()
	s2.Mount(mux2)
	rr = httptest.NewRecorder()
	mux2.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/project/compare?a=live&b=live", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("live unavailable %d", rr.Code)
	}
}

func TestProjectValidate_TypeMismatchAndUnreachable(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x", Security: "None",
		Tags: []core.Tag{
			{ID: "boolish", NodeID: "ns=4;i=4208", DataType: core.ValueBool, Enabled: true, IntervalMs: 1000},
			{ID: "textish", NodeID: "ns=4;i=4208", DataType: core.ValueString, Enabled: true, IntervalMs: 1000},
		},
	})
	live := store.NewLive()
	n := 3.14
	live.Update(core.Sample{TagID: "boolish", ValueNum: &n, Quality: core.QualityGood, Time: time.Now().UTC()})
	live.Update(core.Sample{TagID: "textish", ValueNum: &n, Quality: core.QualityGood, Time: time.Now().UTC()})

	// Browser that does not implement nodeProber → unreachable/probe not supported
	s := &Server{
		Cfg: cfg, Devices: cfg.Devices, Live: live, Hub: NewHub(),
		Browser: stubBrowser{},
	}
	mux := http.NewServeMux()
	s.Mount(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/project/validate",
		strings.NewReader(`{"device_id":"plc"}`)))
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var out struct {
		Rows []struct {
			TagID  string `json:"id"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, r := range out.Rows {
		if r.Status != "unreachable" {
			t.Fatalf("want unreachable for stub browser, got %#v", r)
		}
	}
}

func TestProjectImport_RejectsLegacy(t *testing.T) {
	cfg := testAPIConfig(t)
	s := &Server{Cfg: cfg, Devices: cfg.Devices, Hub: NewHub(), Live: store.NewLive()}
	mux := http.NewServeMux()
	s.Mount(mux)

	// Minimal legacy plant sheet (importexcel headers).
	xlsx, err := importexcel.Write([]core.Tag{
		{ID: "SIG_1", NodeID: "ns=4;i=100", DataType: core.ValueFloat64},
	})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "plant.xlsx")
	_, _ = fw.Write(xlsx)
	_ = w.Close()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/project/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "legacy") {
		t.Fatalf("legacy import: %d %s", rr.Code, rr.Body.String())
	}
}

func TestStatusSummary_FakeDBPing(t *testing.T) {
	s := &Server{
		DB:       &fakeDB{pingErr: nil},
		Devices:  func() []core.Device { return nil },
		Tags:     func() []core.Tag { return nil },
		ReadyCheck: func() bool { return false },
	}
	out := s.buildStatusSummary(httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	if !out.DatabaseConnected {
		t.Fatal("expected ping ok")
	}
	s.DB = &fakeDB{pingErr: errors.New("down")}
	out = s.buildStatusSummary(httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	if out.DatabaseConnected {
		t.Fatal("expected ping fail")
	}
}

// stubBrowser satisfies core.Browser but not nodeProber.
type stubBrowser struct{}

func (stubBrowser) BrowseChildren(context.Context, string) ([]core.BrowseNode, error) {
	return nil, nil
}
func (stubBrowser) ExpandStructure(context.Context, string, string, int) ([]core.ExpandedTag, error) {
	return nil, nil
}
