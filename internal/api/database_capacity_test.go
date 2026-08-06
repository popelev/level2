package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestHandleCapacity_NoHistorian(t *testing.T) {
	s := &Server{
		Tags: func() []core.Tag {
			return []core.Tag{{ID: "a"}, {ID: "b"}}
		},
	}
	mux := http.NewServeMux()
	s.mountDiagnostics(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/capacity", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["error"] != "historian not configured" {
		t.Fatalf("%#v", out)
	}
	if out["tag_count"].(float64) != 2 {
		t.Fatalf("tag_count %#v", out["tag_count"])
	}
}

func TestHandleGetCapacityPolicy_Defaults(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.mountDatabase(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/database/capacity-policy", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["capacity_percent"].(float64) != 90 || out["full_policy"] != "stop" {
		t.Fatalf("%#v", out)
	}
}

func TestHandlePutCapacityPolicy_NoConfig(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.mountDatabase(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/database/capacity-policy",
		strings.NewReader(`{"capacity_percent":80,"full_policy":"stop"}`))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleDatabaseStatus_NoDB(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.mountDatabase(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/database/status", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["connected"] != false || out["ping_error"] != "historian not configured" {
		t.Fatalf("%#v", out)
	}
}
