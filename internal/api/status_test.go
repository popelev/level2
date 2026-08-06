package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/diag"
	"github.com/popelev/level2/internal/store"
)

func TestHandleStatusSummary(t *testing.T) {
	live := store.NewLive()
	n := 1.0
	live.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	live.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	live.Update(core.Sample{TagID: "t2", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityBad})

	buf := diag.NewBuffer(50)
	buf.Add(diag.Entry{Level: diag.LevelError, Category: diag.CategoryOPCRead, Message: "read fail"})

	s := &Server{
		Live: live,
		Diag: buf,
		ReadyCheck: func() bool { return true },
		Devices: func() []core.Device {
			return []core.Device{{
				ID: "dev1",
				Tags: []core.Tag{
					{ID: "t1", Enabled: true, IntervalMs: 1000},
					{ID: "t2", Enabled: true, IntervalMs: 1000},
					{ID: "t3", Enabled: false, IntervalMs: 1000},
				},
			}}
		},
	}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	if rr.Code != 200 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var out statusSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.APIOK || !out.CollectorReady || out.ReadyDetail != "ready" {
		t.Fatalf("ready fields %#v", out)
	}
	if out.DevicesTotal != 1 || out.DevicesDisconnected != 1 {
		t.Fatalf("devices %#v", out)
	}
	if out.TagsTotal != 3 || out.TagsEnabled != 2 {
		t.Fatalf("tags %#v", out)
	}
	if out.QualityGood != 1 || out.QualityBad != 1 {
		t.Fatalf("quality %#v", out)
	}
	if out.QualityGoodPct == nil || *out.QualityGoodPct != 50 {
		t.Fatalf("pct %#v", out.QualityGoodPct)
	}
	if len(out.RecentErrors) != 1 {
		t.Fatalf("recent_errors %#v", out.RecentErrors)
	}
}
