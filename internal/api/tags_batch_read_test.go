package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/store"
)

func TestHandleBatchTagValues(t *testing.T) {
	live := store.NewLive()
	n1, n2 := 10.0, 20.0
	live.Update(core.Sample{TagID: "a", Time: time.Now().UTC(), ValueNum: &n1, Quality: core.QualityGood})
	live.Update(core.Sample{TagID: "b", Time: time.Now().UTC(), ValueNum: &n2, Quality: core.QualityGood})
	s := &Server{
		Live: live,
		Devices: func() []core.Device {
			return []core.Device{{
				ID: "plc",
				Tags: []core.Tag{
					{ID: "a", NodeID: "ns=4;i=1", DataType: core.ValueFloat64, Enabled: true},
					{ID: "b", NodeID: "ns=4;i=2", DataType: core.ValueFloat64, Enabled: true},
					{ID: "c", NodeID: "ns=4;i=3", DataType: core.ValueFloat64, Enabled: true},
				},
			}}
		},
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/values?id=a&id=b&id=missing", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var out []sampleDTOType
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 samples (missing omitted), got %#v", out)
	}
	if out[0].TagID != "a" || out[0].ValueNum == nil || *out[0].ValueNum != 10 {
		t.Fatalf("first %#v", out[0])
	}
	if out[1].TagID != "b" || out[1].ValueNum == nil || *out[1].ValueNum != 20 {
		t.Fatalf("second %#v", out[1])
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/values?ids=b,a,a", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("ids status %d", rr.Code)
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].TagID != "b" || out[1].TagID != "a" {
		t.Fatalf("dedupe/order %#v", out)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/values", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty want 400 got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	ids := make([]string, maxBatchLiveReads+1)
	for i := range ids {
		ids[i] = "id" + strconv.Itoa(i)
	}
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/values?ids="+strings.Join(ids, ","), nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("oversize want 400 got %d", rr.Code)
	}
}

func TestHandleBatchTagValues_NoConflictWithWrite(t *testing.T) {
	s := &Server{
		Live:            store.NewLive(),
		Devices:         func() []core.Device { return nil },
		OPCWriteEnabled: func() bool { return false },
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/tags/values?id=x", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET batch %d", rr.Code)
	}
	var out []sampleDTOType
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("want empty got %#v", out)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tags/values", strings.NewReader(`{"writes":[{"tag_id":"x","value":1}]}`))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("POST write still gated want 403 got %d body %s", rr.Code, rr.Body.String())
	}
}
