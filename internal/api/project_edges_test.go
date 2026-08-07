package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/driver/simbrowser"
	"github.com/popelev/level2/internal/projectxlsx"
	"github.com/popelev/level2/internal/store"
)

func TestProjectExport_NoConfig(t *testing.T) {
	s := &Server{Hub: NewHub(), Live: store.NewLive(), Devices: func() []core.Device { return nil }}
	mux := http.NewServeMux()
	s.Mount(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/project.xlsx", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/project/import", strings.NewReader("x")))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("import no cfg: %d", rr.Code)
	}
}

func TestProjectValidate_WithSimBrowser(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{
			{ID: "ok", NodeID: "ns=4;i=4208", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			{ID: "folder", NodeID: "ns=4;i=4207", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			{ID: "missing", NodeID: "ns=4;i=999999", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			{ID: "bad", NodeID: "not-a-node", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000},
			{ID: "off", NodeID: "ns=4;i=4208", DataType: core.ValueFloat64, Enabled: false, IntervalMs: 1000},
		},
	})
	live := store.NewLive()
	n := 1.0
	live.Update(core.Sample{TagID: "ok", ValueNum: &n, Quality: core.QualityGood, Time: time.Now().UTC()})
	s := &Server{
		Cfg:     cfg,
		Devices: cfg.Devices,
		Browser: simbrowser.NewDemo(),
		Hub:     NewHub(),
		Live:    live,
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/project/validate", strings.NewReader(`{"device_id":"plc"}`)))
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var out struct {
		Rows []struct {
			TagID  string `json:"id"`
			Status string `json:"status"`
		} `json:"rows"`
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	by := map[string]string{}
	for _, r := range out.Rows {
		by[r.TagID] = r.Status
	}
	if by["ok"] != "ok" || by["folder"] != "not_variable" || by["missing"] != "missing" {
		t.Fatalf("%v counts=%v", by, out.Counts)
	}
	if by["bad"] != "bad_node_id" || by["off"] != "disabled_skip" {
		t.Fatalf("%v", by)
	}
}

func TestProjectCompareXLSX(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{
		ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true, IntervalMs: 1000}},
	})
	xlsx, err := projectxlsx.Write([]core.Device{{
		ID: "plc", Endpoint: "opc.tcp://y:4840", Security: "None",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=4;i=2", DataType: core.ValueBool, Enabled: true, IntervalMs: 1000}},
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/project/compare.xlsx?a=live&b=file", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "spreadsheetml") {
		t.Fatalf("ct=%q", ct)
	}
	if rr.Body.Len() < 100 {
		t.Fatalf("tiny body %d", rr.Body.Len())
	}
}

func TestErrUnsupportedDataType_Error(t *testing.T) {
	err := errUnsupportedDataType("struct")
	if err.Error() != "unsupported datatype struct" {
		t.Fatalf("%q", err.Error())
	}
}
