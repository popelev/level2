package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/popelev/level2/internal/core"
)

func TestHandleIntegrationTagCatalog_Happy(t *testing.T) {
	s := &Server{
		Devices: func() []core.Device {
			return []core.Device{{
				ID: "s7_1500",
				Tags: []core.Tag{{
					ID: "cell_12_current", NodeID: "ns=4;i=4208", Path: "Cell/12",
					DataType: core.ValueFloat64, Enabled: true, Writable: false, IntervalMs: 1000,
				}},
			}}
		},
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/integration/tag-catalog", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var cat tagCatalogDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &cat); err != nil {
		t.Fatal(err)
	}
	if cat.Level2APIVersion != OpenAPIVersion {
		t.Fatalf("version %q want %q", cat.Level2APIVersion, OpenAPIVersion)
	}
	if cat.ExportedAt.IsZero() {
		t.Fatal("exported_at empty")
	}
	if len(cat.Devices) != 1 || cat.Devices[0].ID != "s7_1500" {
		t.Fatalf("devices %#v", cat.Devices)
	}
	if len(cat.Tags) != 1 {
		t.Fatalf("tags %#v", cat.Tags)
	}
	tag := cat.Tags[0]
	if tag.TagID != "cell_12_current" || tag.DeviceID != "s7_1500" || tag.NodeID != "ns=4;i=4208" {
		t.Fatalf("tag %#v", tag)
	}
	if tag.Datatype != "float64" || !tag.Enabled || tag.Writable || tag.IntervalMs != 1000 || tag.Path != "Cell/12" {
		t.Fatalf("tag fields %#v", tag)
	}
	raw := rr.Body.String()
	if strings.Contains(raw, `"sample"`) || strings.Contains(raw, `"tag":`) {
		t.Fatalf("catalog must be flat, got %s", raw)
	}
}

func TestHandleIntegrationTagCatalog_Empty(t *testing.T) {
	s := &Server{
		Devices: func() []core.Device { return nil },
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/integration/tag-catalog", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var cat tagCatalogDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &cat); err != nil {
		t.Fatal(err)
	}
	if cat.Devices == nil || len(cat.Devices) != 0 {
		t.Fatalf("devices want [] got %#v", cat.Devices)
	}
	if cat.Tags == nil || len(cat.Tags) != 0 {
		t.Fatalf("tags want [] got %#v", cat.Tags)
	}
	if cat.Level2APIVersion != OpenAPIVersion {
		t.Fatalf("version %q", cat.Level2APIVersion)
	}
}
