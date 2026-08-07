package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/driver/simbrowser"
	"github.com/popelev/level2/internal/projectxlsx"
	devruntime "github.com/popelev/level2/internal/runtime"
	"github.com/popelev/level2/internal/store"
)

type mockHistory struct {
	rows []core.Sample
	err  error
}

func (m mockHistory) QueryHistory(ctx context.Context, tagID string, from, to time.Time, limit int) ([]core.Sample, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.rows, nil
}

func TestBrowseAndExpand_SimBrowser(t *testing.T) {
	br := simbrowser.NewDemo()
	s := &Server{
		Browser: br,
		Devices: func() []core.Device { return nil },
		Hub:     NewHub(),
		Live:    store.NewLive(),
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/browse", nil))
	if rr.Code != 200 {
		t.Fatalf("browse root %d %s", rr.Code, rr.Body.String())
	}
	var nodes []core.BrowseNode
	if err := json.Unmarshal(rr.Body.Bytes(), &nodes); err != nil {
		t.Fatal(err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected root children")
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/browse?node_id="+url.QueryEscape("ns=4;i=9999"), nil))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("unknown node: %d body=%s", rr.Code, rr.Body.String())
	}

	body := `{"node_id":"ns=4;i=4207","parent_tag_id":"mp","max_depth":2}`
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/expand", strings.NewReader(body)))
	if rr.Code != 200 {
		t.Fatalf("expand %d %s", rr.Code, rr.Body.String())
	}
	var tags []core.ExpandedTag
	if err := json.Unmarshal(rr.Body.Bytes(), &tags); err != nil {
		t.Fatal(err)
	}
	if len(tags) < 2 {
		t.Fatalf("expand tags=%d", len(tags))
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/expand", strings.NewReader(`{`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json: %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/expand", strings.NewReader(`{"node_id":""}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty node: %d", rr.Code)
	}
}

func TestExpandStream_SimBrowser(t *testing.T) {
	br := simbrowser.NewDemo()
	s := &Server{Browser: br, Hub: NewHub(), Live: store.NewLive(), Devices: func() []core.Device { return nil }}
	mux := http.NewServeMux()
	s.Mount(mux)

	body := `{"node_id":"ns=4;i=4207","parent_tag_id":"mp","stream":true}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/expand", strings.NewReader(body))
	req.Header.Set("Accept", "application/x-ndjson")
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("stream %d %s", rr.Code, rr.Body.String())
	}
	bodyOut := rr.Body.String()
	if !strings.Contains(bodyOut, `"type":"result"`) {
		t.Fatalf("ndjson missing result: %s", bodyOut)
	}
	var sawResult bool
	var tagCount int
	for _, line := range strings.Split(bodyOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			t.Fatalf("ndjson line %q: %v", line, err)
		}
		switch evt["type"] {
		case "progress":
			if evt["phase"] == nil {
				t.Fatalf("progress without phase: %v", evt)
			}
		case "result":
			sawResult = true
			tags, _ := evt["tags"].([]any)
			tagCount = len(tags)
		case "error":
			t.Fatalf("unexpected error event: %v", evt)
		}
	}
	if !sawResult || tagCount < 2 {
		t.Fatalf("want result with tags>=2, got count=%d body=%s", tagCount, bodyOut)
	}
}

func TestBrowseUnavailable(t *testing.T) {
	s := &Server{Hub: NewHub(), Live: store.NewLive(), Devices: func() []core.Device { return nil }}
	mux := http.NewServeMux()
	s.Mount(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/browse", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestHistory_OKAndError(t *testing.T) {
	n := 1.5
	s := &Server{
		Hub:     NewHub(),
		Live:    store.NewLive(),
		Devices: func() []core.Device { return nil },
		History: mockHistory{rows: []core.Sample{{TagID: "t", ValueNum: &n, Quality: core.QualityGood, Time: time.Unix(1, 0).UTC()}}},
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/t/history?from=2020-01-01T00:00:00Z&to=2030-01-01T00:00:00Z&limit=10", nil))
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var rows []sampleDTOType
	if err := json.Unmarshal(rr.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ValueNum == nil || *rows[0].ValueNum != 1.5 {
		t.Fatalf("%#v", rows)
	}

	s.History = mockHistory{err: context.DeadlineExceeded}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/t/history", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestProjectValidate_WithProbe(t *testing.T) {
	br := simbrowser.NewDemo()
	b := true
	live := store.NewLive()
	live.Update(core.Sample{TagID: "ok_leaf", ValueBool: &b, Quality: core.QualityGood, Time: time.Now().UTC()})
	live.Update(core.Sample{TagID: "type_mismatch", ValueNum: ptrFloat(1), Quality: core.QualityGood, Time: time.Now().UTC()})

	s := &Server{
		Browser: br,
		Live:    live,
		Hub:     NewHub(),
		Devices: func() []core.Device {
			return []core.Device{{
				ID: "plc",
				Tags: []core.Tag{
					{ID: "disabled", NodeID: "ns=4;i=4208", DataType: core.ValueFloat64, Enabled: false},
					{ID: "bad_node", NodeID: "not-a-node", DataType: core.ValueFloat64, Enabled: true},
					{ID: "ok_leaf", NodeID: "ns=4;i=4208", DataType: core.ValueBool, Enabled: true},
					{ID: "type_mismatch", NodeID: "ns=4;i=4209", DataType: core.ValueBool, Enabled: true},
					{ID: "folder", NodeID: "ns=4;i=4207", DataType: core.ValueFloat64, Enabled: true},
					{ID: "missing", NodeID: "ns=9;i=1", DataType: core.ValueFloat64, Enabled: true},
				},
			}}
		},
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/project/validate", strings.NewReader(`{"device_id":"plc"}`)))
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var out struct {
		Counts map[string]int `json:"counts"`
		Rows   []validateRow  `json:"rows"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"disabled_skip", "bad_node_id", "ok", "type_mismatch", "not_variable", "missing"} {
		if out.Counts[want] < 1 {
			t.Fatalf("missing count %q in %#v rows=%#v", want, out.Counts, out.Rows)
		}
	}
}

func TestProjectExportAndPreview(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	})
	s := &Server{Cfg: cfg, Devices: cfg.Devices, Hub: NewHub(), Live: store.NewLive()}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/project.xlsx", nil))
	if rr.Code != 200 {
		t.Fatalf("export %d", rr.Code)
	}
	xlsx := rr.Body.Bytes()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "project.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(xlsx); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/project/preview", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("preview %d %s", rr.Code, rr.Body.String())
	}
	var preview map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview["legacy"] != false {
		t.Fatalf("preview legacy: %v", preview)
	}
	if int(preview["servers"].(float64)) != 1 || int(preview["tags"].(float64)) != 1 {
		t.Fatalf("preview summary: %v", preview)
	}
	ids, _ := preview["device_ids"].([]any)
	if len(ids) != 1 || ids[0] != "plc" {
		t.Fatalf("device_ids: %v", preview["device_ids"])
	}

	// Import merge
	var buf2 bytes.Buffer
	w2 := multipart.NewWriter(&buf2)
	fw2, _ := w2.CreateFormFile("file", "project.xlsx")
	_, _ = fw2.Write(xlsx)
	_ = w2.Close()
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/project/import?mode=merge", &buf2)
	req.Header.Set("Content-Type", w2.FormDataContentType())
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("import %d %s", rr.Code, rr.Body.String())
	}
	var imported map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	if imported["mode"] != "merge" || int(imported["servers"].(float64)) != 1 || int(imported["tags"].(float64)) != 1 {
		t.Fatalf("import body: %v", imported)
	}
	tags, err := cfg.DeviceTags("plc")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].ID != "t1" {
		t.Fatalf("merged tags: %#v", tags)
	}
}

func TestProjectCompare_LiveVsFile(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	})
	xlsx, err := projectxlsx.Write([]core.Device{{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=4;i=2", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Cfg: cfg, Devices: cfg.Devices, Hub: NewHub(), Live: store.NewLive()}
	mux := http.NewServeMux()
	s.Mount(mux)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("b", "other.xlsx")
	_, _ = fw.Write(xlsx)
	_ = w.Close()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/project/compare?a=live&b=file", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("compare %d %s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if int(out["count"].(float64)) < 1 {
		t.Fatalf("expected diff rows, got %v", out)
	}
	rows, _ := out["rows"].([]any)
	if len(rows) < 1 {
		t.Fatalf("rows missing: %v", out)
	}
}

func TestDiagLogsAndClear(t *testing.T) {
	buf := diag.NewBuffer(20)
	buf.Add(diag.Entry{Level: diag.LevelError, Category: diag.CategoryOPCRead, Message: "boom"})
	buf.Add(diag.Entry{Level: diag.LevelInfo, Category: diag.CategoryDBWrite, Message: "ok"})
	s := &Server{Diag: buf, Hub: NewHub(), Live: store.NewLive(), Devices: func() []core.Device { return nil }}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/logs?category=opc&errors_only=1&limit=5", nil))
	if rr.Code != 200 {
		t.Fatalf("%d", rr.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	entries, _ := out["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries=%v", out["entries"])
	}
	entry, _ := entries[0].(map[string]any)
	if entry["message"] != "boom" {
		t.Fatalf("filtered entry: %v", entry)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/v1/diagnostics/logs", nil))
	if rr.Code != 200 {
		t.Fatalf("clear %d", rr.Code)
	}
	if len(buf.Query("all", false, 10)) != 0 {
		t.Fatal("expected cleared")
	}

	// nil diag
	s2 := &Server{Hub: NewHub(), Live: store.NewLive(), Devices: func() []core.Device { return nil }}
	mux2 := http.NewServeMux()
	s2.Mount(mux2)
	rr = httptest.NewRecorder()
	mux2.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/logs", nil))
	if rr.Code != 200 {
		t.Fatalf("nil diag %d", rr.Code)
	}
	var empty map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if ents, _ := empty["entries"].([]any); len(ents) != 0 {
		t.Fatalf("nil diag entries: %v", empty)
	}
}

func TestParseIntDefault(t *testing.T) {
	cases := []struct {
		in   string
		def  int
		want int
	}{
		{"", 7, 7},
		{"0", 7, 7},
		{"-1", 7, 7},
		{"x", 7, 7},
		{"12", 7, 12},
	}
	for _, tc := range cases {
		if got := parseIntDefault(tc.in, tc.def); got != tc.want {
			t.Fatalf("%q → %d want %d", tc.in, got, tc.want)
		}
	}
}

func ptrFloat(v float64) *float64 { return &v }

type errBrowser struct {
	browseErr error
	expandErr error
}

func (e errBrowser) BrowseChildren(context.Context, string) ([]core.BrowseNode, error) {
	return nil, e.browseErr
}
func (e errBrowser) ExpandStructure(context.Context, string, string, int) ([]core.ExpandedTag, error) {
	return nil, e.expandErr
}

func TestBrowseAndExpand_Errors(t *testing.T) {
	s := &Server{
		Browser: errBrowser{browseErr: errors.New("browse down"), expandErr: errors.New("expand down")},
		Hub:     NewHub(),
		Live:    store.NewLive(),
		Devices: func() []core.Device { return nil },
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/browse", nil))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("browse err: %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/expand", strings.NewReader(`{"node_id":"ns=1;i=1"}`)))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expand err: %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/expand", strings.NewReader(`{"node_id":"ns=1;i=1","stream":true}`))
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("stream status %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"type":"error"`) {
		t.Fatalf("want error event: %s", rr.Body.String())
	}
}

func TestResolveBrowser_DevHub(t *testing.T) {
	hub := devruntime.NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	br := errBrowser{}
	hub.InjectDriver(core.Device{ID: "plc", Endpoint: "opc.tcp://x"}, onlineNonOPC{}, br)
	s := &Server{DevHub: hub, Hub: NewHub(), Live: store.NewLive()}
	got, err := s.resolveBrowser("plc")
	if err != nil || got == nil {
		t.Fatalf("devhub browser: %v %#v", err, got)
	}
	_, err = s.resolveBrowser("missing")
	if err == nil {
		t.Fatal("expected missing device error")
	}
}

