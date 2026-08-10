package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIAuth_RoleMatrix401403(t *testing.T) {
	const (
		writeTok = "model-write-token"
		adminTok = "ops-admin-token"
	)
	s := &Server{
		APITokenWrite: func() string { return writeTok },
		APITokenAdmin: func() string { return adminTok },
	}
	handler := s.APIAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	type tc struct {
		name   string
		method string
		path   string
		token  string
		want   int
	}
	cases := []tc{
		// Reads stay open
		{"GET tags open", http.MethodGet, "/api/v1/tags", "", http.StatusNoContent},
		{"GET tags ignore write tok", http.MethodGet, "/api/v1/tags", writeTok, http.StatusNoContent},

		// Write routes
		{"PUT value missing → 401", http.MethodPut, "/api/v1/tags/t1/value", "", http.StatusUnauthorized},
		{"PUT value wrong → 401", http.MethodPut, "/api/v1/tags/t1/value", "nope", http.StatusUnauthorized},
		{"PUT value write ok", http.MethodPut, "/api/v1/tags/t1/value", writeTok, http.StatusNoContent},
		{"PUT value admin ok", http.MethodPut, "/api/v1/tags/t1/value", adminTok, http.StatusNoContent},
		{"POST values write ok", http.MethodPost, "/api/v1/tags/values", writeTok, http.StatusNoContent},
		{"POST values missing → 401", http.MethodPost, "/api/v1/tags/values", "", http.StatusUnauthorized},
		{"WS write ok", http.MethodGet, "/api/v1/ws/stream", writeTok, http.StatusNoContent},
		{"WS missing → 401", http.MethodGet, "/api/v1/ws/stream", "", http.StatusUnauthorized},

		// Admin routes — write token must NOT pass
		{"wipe write → 403", http.MethodPost, "/api/v1/database/wipe-samples", writeTok, http.StatusForbidden},
		{"wipe admin ok", http.MethodPost, "/api/v1/database/wipe-samples", adminTok, http.StatusNoContent},
		{"wipe missing → 401", http.MethodPost, "/api/v1/database/wipe-samples", "", http.StatusUnauthorized},
		{"wipe wrong → 401", http.MethodPost, "/api/v1/database/wipe-samples", "nope", http.StatusUnauthorized},
		{"capacity write → 403", http.MethodPut, "/api/v1/database/capacity-policy", writeTok, http.StatusForbidden},
		{"capacity admin ok", http.MethodPut, "/api/v1/database/capacity-policy", adminTok, http.StatusNoContent},
		{"devices POST write → 403", http.MethodPost, "/api/v1/devices", writeTok, http.StatusForbidden},
		{"devices POST admin ok", http.MethodPost, "/api/v1/devices", adminTok, http.StatusNoContent},
		{"project import write → 403", http.MethodPost, "/api/v1/project/import", writeTok, http.StatusForbidden},
		{"project import admin ok", http.MethodPost, "/api/v1/project/import", adminTok, http.StatusNoContent},
		{"tags upsert write → 403", http.MethodPost, "/api/v1/devices/plc/tags", writeTok, http.StatusForbidden},
		{"diag reset write → 403", http.MethodPost, "/api/v1/diagnostics/reset", writeTok, http.StatusForbidden},
		{"expand write → 403", http.MethodPost, "/api/v1/expand", writeTok, http.StatusForbidden},
		{"expand admin ok", http.MethodPost, "/api/v1/expand", adminTok, http.StatusNoContent},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			handler.ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("got %d want %d body %q", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

func TestAPIAuth_LegacySharedStillBothRoles(t *testing.T) {
	s := &Server{APIToken: func() string { return "shared" }}
	handler := s.APIAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, path := range []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/api/v1/tags/t1/value"},
		{http.MethodPost, "/api/v1/database/wipe-samples"},
		{http.MethodPost, "/api/v1/devices"},
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(path.method, path.path, nil)
		req.Header.Set("X-API-Token", "shared")
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("%s %s: want 204 got %d", path.method, path.path, rr.Code)
		}
	}
}

func TestAPIAuth_WriteOnlyCannotAdmin(t *testing.T) {
	s := &Server{APITokenWrite: func() string { return "only-write" }}
	handler := s.APIAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tags/t1/value", nil)
	req.Header.Set("Authorization", "Bearer only-write")
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("write route: want 204 got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/database/wipe-samples", nil)
	req.Header.Set("Authorization", "Bearer only-write")
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("wipe with write-only: want 403 got %d", rr.Code)
	}
}

func TestRequestRole_Classification(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   apiRole
	}{
		{http.MethodGet, "/api/v1/tags", roleNone},
		{http.MethodGet, "/healthz", roleNone},
		{http.MethodPut, "/api/v1/tags/Motor1.SpeedSP/value", roleWrite},
		{http.MethodPost, "/api/v1/tags/values", roleWrite},
		{http.MethodGet, "/api/v1/ws/stream", roleWrite},
		{http.MethodPost, "/api/v1/database/wipe-samples", roleAdmin},
		{http.MethodPut, "/api/v1/database/capacity-policy", roleAdmin},
		{http.MethodPost, "/api/v1/project/import", roleAdmin},
		{http.MethodDelete, "/api/v1/devices/x", roleAdmin},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if got := requestRole(req); got != tc.want {
			t.Fatalf("%s %s: got %v want %v", tc.method, tc.path, got, tc.want)
		}
	}
}
