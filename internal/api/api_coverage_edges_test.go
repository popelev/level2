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
	"github.com/popelev/level2/internal/historian/timescale"
	"github.com/popelev/level2/internal/importexcel"
	"github.com/popelev/level2/internal/metrics"
	"github.com/popelev/level2/internal/store"
)

func TestCapacityPolicy_WithConfig(t *testing.T) {
	cfg := testAPIConfig(t)
	if err := cfg.SetCapacityPolicy(77, "drop_oldest"); err != nil {
		t.Fatal(err)
	}
	s := &Server{Cfg: cfg}
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
	if got["capacity_percent"].(float64) != 77 || got["full_policy"] != "drop_oldest" {
		t.Fatalf("%v", got)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/database/capacity-policy",
		strings.NewReader(`{"capacity_percent":55,"full_policy":"rotate"}`))
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("put %d %s", rr.Code, rr.Body.String())
	}
	snap := cfg.Snapshot()
	if snap.Database.CapacityPercent != 55 || snap.Database.FullPolicy != "rotate" {
		t.Fatalf("%+v", snap.Database)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/v1/database/capacity-policy",
		strings.NewReader(`{`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/api/v1/database/capacity-policy",
		strings.NewReader(`{"capacity_percent":0,"full_policy":"stop"}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid %d", rr.Code)
	}
}

func TestDatabaseStatus_WithCfgURL(t *testing.T) {
	cfg := testAPIConfig(t)
	s := &Server{Cfg: cfg}
	mux := http.NewServeMux()
	s.mountDatabase(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/database/status", nil))
	if rr.Code != 200 {
		t.Fatalf("%d", rr.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["connected"] != false {
		t.Fatalf("%v", out)
	}
	if out["url_masked"] == nil || out["url_masked"] == "" {
		t.Fatalf("expected masked url: %v", out)
	}
	if !strings.Contains(out["url_masked"].(string), "postgres") {
		t.Fatalf("url_masked=%v", out["url_masked"])
	}
}

func TestWipeSamples_EmptyHistorianAndBadJSON(t *testing.T) {
	s := &Server{DB: &timescale.Historian{}, Live: store.NewLive()}
	mux := http.NewServeMux()
	s.mountDatabase(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/database/wipe-samples?confirm=wipe",
		strings.NewReader(`not-json`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json want 400 got %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid JSON") {
		t.Fatalf("body=%s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/database/wipe-samples?confirm=wipe", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("empty historian want 500 got %d %s", rr.Code, rr.Body.String())
	}
}

func TestSamplesWrittenRate(t *testing.T) {
	rateMu.Lock()
	ratePrev = 0
	ratePrevTime = time.Time{}
	rateMu.Unlock()

	if got := samplesWrittenRate(); got != 0 {
		t.Fatalf("first call want 0 got %v", got)
	}
	// second call within 1s still 0
	if got := samplesWrittenRate(); got != 0 {
		t.Fatalf("within 1s want 0 got %v", got)
	}
	rateMu.Lock()
	ratePrevTime = time.Now().Add(-2 * time.Second)
	ratePrev = counterValue(metrics.SamplesWritten) - 10
	rateMu.Unlock()
	if got := samplesWrittenRate(); got <= 0 {
		t.Fatalf("expected positive rate, got %v", got)
	}
}

func TestImportExcel_OK(t *testing.T) {
	cfg := testAPIConfig(t, core.Device{ID: "plc", Endpoint: "opc.tcp://x:4840", Security: "None"})
	s := &Server{Cfg: cfg, Devices: cfg.Devices, Hub: NewHub(), Live: store.NewLive()}
	mux := http.NewServeMux()
	s.Mount(mux)

	raw, err := importexcel.Write([]core.Tag{
		{ID: "tank_level", NodeID: "ns=4;i=10", DataType: core.ValueFloat64},
		{ID: "pump_run", NodeID: "ns=4;i=11", DataType: core.ValueBool},
	})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "tags.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/import?replace=1", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if int(resp["added"].(float64)) != 2 {
		t.Fatalf("%v", resp)
	}

	// missing file field
	var body2 bytes.Buffer
	w2 := multipart.NewWriter(&body2)
	_ = w2.WriteField("replace", "1")
	_ = w2.Close()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/import", &body2)
	req2.Header.Set("Content-Type", w2.FormDataContentType())
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("missing file %d", rr2.Code)
	}

	s2 := &Server{}
	mux2 := http.NewServeMux()
	s2.Mount(mux2)
	rr3 := httptest.NewRecorder()
	mux2.ServeHTTP(rr3, httptest.NewRequest(http.MethodPost, "/api/v1/devices/plc/tags/import", nil))
	if rr3.Code != http.StatusServiceUnavailable {
		t.Fatalf("no cfg %d", rr3.Code)
	}
}

func TestParseBoolQueryAndResolveWriteOptions(t *testing.T) {
	for _, v := range []string{"1", "true", "YES", "on"} {
		if !parseBoolQuery(v) {
			t.Fatalf("%q", v)
		}
	}
	if parseBoolQuery("no") || parseBoolQuery("") {
		t.Fatal("false cases")
	}
	req := httptest.NewRequest(http.MethodPut, "/x?verify=1&optimistic=0&verify_timeout_ms=2500", nil)
	opts := resolveWriteOptions(nil, nil, nil, req)
	if !opts.verify || opts.optimistic || opts.verifyTimeoutMs != 2500 {
		t.Fatalf("%+v", opts)
	}
	v := true
	o := false
	ms := 100
	req = httptest.NewRequest(http.MethodPut, "/x", nil)
	opts = resolveWriteOptions(&v, &ms, &o, req)
	if !opts.verify || opts.optimistic || opts.verifyTimeoutMs != 100 {
		t.Fatalf("body flags %+v", opts)
	}
}

func TestDeleteAllTags_NoConfig(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.mountTagBulk(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/v1/devices/plc/tags", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("%d", rr.Code)
	}
}
