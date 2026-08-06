package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWipeSamplesRequiresConfirm(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.mountDatabase(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/database/wipe-samples", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "confirm=wipe") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestWipeSamplesHistorianMissing(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.mountDatabase(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/database/wipe-samples?confirm=wipe", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
