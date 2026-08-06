package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/popelev/level2/internal/config"
	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/importexcel"
	"github.com/popelev/level2/internal/store"
)

func testAPIConfig(t *testing.T, devices ...core.Device) *config.Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	f := &config.File{
		Listen:   ":0",
		SpoolDir: dir,
		UIDir:    dir,
		Database: config.Database{URL: "postgres://u:p@localhost/db", CapacityPercent: 90, FullPolicy: config.FullPolicyStop},
	}
	s := config.NewStore(path, f)
	if len(devices) == 0 {
		if err := s.SetCapacityPolicy(90, config.FullPolicyStop); err != nil {
			t.Fatal(err)
		}
		return s
	}
	for _, d := range devices {
		if err := s.UpsertDevice(d); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestDeviceCRUD_AndTagLifecycle(t *testing.T) {
	cfg := testAPIConfig(t)
	changed := []string{}
	s := &Server{
		Cfg:     cfg,
		Live:    store.NewLive(),
		Hub:     NewHub(),
		Devices: cfg.Devices,
		OnDeviceChanged: func(id string, removed bool) {
			if removed {
				changed = append(changed, "rm:"+id)
			} else {
				changed = append(changed, "up:"+id)
			}
		},
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"create missing cfg body", http.MethodPost, "/api/v1/devices", `{`, http.StatusBadRequest},
		{"create ok", http.MethodPost, "/api/v1/devices", `{"id":"plc","endpoint":"opc.tcp://x:4840","security":"None","poll_concurrency":2}`, http.StatusCreated},
		{"create conflict", http.MethodPost, "/api/v1/devices", `{"id":"plc","endpoint":"opc.tcp://x:4840"}`, http.StatusConflict},
		{"update ok", http.MethodPut, "/api/v1/devices/plc", `{"endpoint":"opc.tcp://y:4840","security":"None","poll_concurrency":4}`, http.StatusOK},
		{"update bad json", http.MethodPut, "/api/v1/devices/plc", `{`, http.StatusBadRequest},
		{"upsert tag", http.MethodPost, "/api/v1/devices/plc/tags", `{"id":"t1","node_id":"ns=4;i=1","datatype":"float64","enabled":true}`, http.StatusOK},
		{"put tag", http.MethodPut, "/api/v1/devices/plc/tags/t1", `{"node_id":"ns=4;i=2","datatype":"bool","enabled":true,"interval_ms":250}`, http.StatusOK},
		{"upsert bad json", http.MethodPost, "/api/v1/devices/plc/tags", `{`, http.StatusBadRequest},
		{"delete tag", http.MethodDelete, "/api/v1/devices/plc/tags/t1", "", http.StatusNoContent},
		{"delete missing tag", http.MethodDelete, "/api/v1/devices/plc/tags/t1", "", http.StatusNotFound},
		{"delete device", http.MethodDelete, "/api/v1/devices/plc", "", http.StatusNoContent},
		{"delete missing device", http.MethodDelete, "/api/v1/devices/plc", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			} else {
				body = strings.NewReader("")
			}
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, body)
			mux.ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("got %d want %d body=%s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
	if len(changed) == 0 {
		t.Fatal("expected OnDeviceChanged callbacks")
	}
}

func TestDeviceCRUD_NoConfig(t *testing.T) {
	s := &Server{Devices: func() []core.Device { return nil }, Hub: NewHub(), Live: store.NewLive()}
	mux := http.NewServeMux()
	s.Mount(mux)
	paths := []struct {
		method, path string
	}{
		{http.MethodPost, "/api/v1/devices"},
		{http.MethodPut, "/api/v1/devices/x"},
		{http.MethodDelete, "/api/v1/devices/x"},
		{http.MethodPost, "/api/v1/devices/x/tags"},
		{http.MethodDelete, "/api/v1/devices/x/tags/y"},
		{http.MethodGet, "/api/v1/devices/x/tags.xlsx"},
	}
	for _, p := range paths {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(p.method, p.path, strings.NewReader(`{}`)))
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s: got %d", p.method, p.path, rr.Code)
		}
	}
}

func TestBulkUpsertAndDeleteAllTags(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None"})
	s := &Server{Cfg: cfg, Devices: cfg.Devices, Hub: NewHub(), Live: store.NewLive()}
	mux := http.NewServeMux()
	s.Mount(mux)

	body := `{
		"tags": [
			{"id":"a","node_id":"ns=4;i=1","datatype":"float64","enabled":true},
			{"id":"b","node_id":"ns=4;i=2","datatype":"bool","enabled":true},
			{"id":"","node_id":"ns=4;i=3"},
			{"id":"bad","node_id":"not-a-node"},
			{"id":"dup","node_id":"ns=4;i=1"},
			{"id":"a","node_id":"ns=4;i=9"}
		]
	}`
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/bulk", strings.NewReader(body)))
	if rr.Code != 200 {
		t.Fatalf("bulk %d %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if int(resp["added"].(float64)) != 2 {
		t.Fatalf("added=%v resp=%v", resp["added"], resp)
	}
	if int(resp["skipped_duplicates"].(float64)) < 1 {
		t.Fatalf("skipped=%v", resp["skipped_duplicates"])
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/v1/devices/plc/tags", nil))
	if rr.Code != 200 {
		t.Fatalf("delete all %d %s", rr.Code, rr.Body.String())
	}
	tags, err := cfg.DeviceTags("plc")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Fatalf("tags left: %#v", tags)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/bulk", strings.NewReader(`{`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json: %d", rr.Code)
	}
}

func TestExportTagsExcel(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	})
	s := &Server{Cfg: cfg, Devices: cfg.Devices, Hub: NewHub(), Live: store.NewLive()}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/devices/plc/tags.xlsx", nil))
	if rr.Code != 200 {
		t.Fatalf("export %d %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "spreadsheetml") {
		t.Fatalf("ct=%q", ct)
	}
	if _, err := importexcel.Parse(bytes.NewReader(rr.Body.Bytes())); err != nil {
		t.Fatal(err)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/devices/missing/tags.xlsx", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing device: %d", rr.Code)
	}
}

func TestImportExcel_Errors(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None"})
	s := &Server{Cfg: cfg, Devices: cfg.Devices, Hub: NewHub(), Live: store.NewLive()}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/import", strings.NewReader("not-multipart")))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want multipart error, got %d %s", rr.Code, rr.Body.String())
	}
}

func TestSyncTags_EmptySelection(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{{ID: "a", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	})
	s := &Server{Cfg: cfg}
	mux := http.NewServeMux()
	s.mountTagBulk(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/sync", strings.NewReader(`{"tag_ids":["nope"]}`))
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 0 {
		t.Fatalf("%v", resp)
	}
}
