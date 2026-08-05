package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/store"
)

func TestHandleTagsAndValue(t *testing.T) {
	live := store.NewLive()
	n := 91.5
	live.Update(core.Sample{TagID: "opc_measure_rvalue", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	s := &Server{
		Live: live,
		Tags: func() []core.Tag {
			return []core.Tag{{ID: "opc_measure_rvalue", NodeID: "ns=4;i=4208", DataType: core.ValueFloat64, Enabled: true}}
		},
		Devices: func() []core.Device {
			return []core.Device{{
				ID: "s7_1500",
				Tags: []core.Tag{{ID: "opc_measure_rvalue", NodeID: "ns=4;i=4208", DataType: core.ValueFloat64, Enabled: true}},
			}}
		},
		Hub: NewHub(),
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags?device_id=s7_1500", nil))
	if rr.Code != 200 {
		t.Fatalf("tags status %d", rr.Code)
	}
	var list []store.TagValue
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].DeviceID != "s7_1500" {
		t.Fatalf("unexpected %#v", list)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/opc_measure_rvalue/value", nil))
	if rr.Code != 200 {
		t.Fatalf("value status %d body %s", rr.Code, rr.Body.String())
	}
	var dto sampleDTOType
	if err := json.Unmarshal(rr.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.ValueNum == nil || *dto.ValueNum != 91.5 {
		t.Fatalf("unexpected %#v", dto)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/opc_measure_rvalue/value", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("write want 501 got %d", rr.Code)
	}
}
