package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/popelev/level2/internal/diag"
)

func TestHandleDiagReset(t *testing.T) {
	buf := diag.NewBuffer(50)
	buf.Add(diag.Entry{Level: diag.LevelError, Category: diag.CategoryOPCRead, Message: "poll failed"})
	buf.Add(diag.Entry{Level: diag.LevelWarn, Category: diag.CategoryDBWrite, Message: "write lag"})

	inc := diag.NewIncidentTracker(100, time.Hour)
	inc.Record(diag.IncidentOPCDisconnect, "dev1")
	inc.Record(diag.IncidentCollectorDown, "")
	inc.Record(diag.IncidentDBWriteError, "")

	s := &Server{Diag: buf, Incidents: inc}
	mux := http.NewServeMux()
	s.Mount(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/diagnostics/reset", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "reset" {
		t.Fatalf("body %#v", body)
	}

	if got := len(buf.Query("all", false, 100)); got != 0 {
		t.Fatalf("diag ring still has %d entries", got)
	}
	if inc.Count(diag.IncidentOPCDisconnect, 0) != 0 ||
		inc.Count(diag.IncidentCollectorDown, 0) != 0 ||
		inc.Count(diag.IncidentDBWriteError, 0) != 0 {
		t.Fatal("incident counters not cleared")
	}

	// Status summary should reflect empty alarms.
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/api/v1/status/summary", nil))
	var sum statusSummary
	if err := json.Unmarshal(rr2.Body.Bytes(), &sum); err != nil {
		t.Fatal(err)
	}
	if len(sum.RecentErrors) != 0 {
		t.Fatalf("recent_errors %#v", sum.RecentErrors)
	}
	if sum.OPCDisconnectsLastHour != 0 || sum.CollectorDownLastHour != 0 || sum.DBWriteErrorsLastHour != 0 {
		t.Fatalf("summary drops %#v", sum)
	}
}
