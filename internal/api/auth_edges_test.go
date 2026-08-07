package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/popelev/level2/internal/config"
)

func TestAPIAuth_CfgTokenAndExtractors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	f := &config.File{
		Listen:   ":0",
		SpoolDir: dir,
		UIDir:    dir,
		Database: config.Database{URL: "postgres://u:p@localhost/db", CapacityPercent: 90, FullPolicy: config.FullPolicyStop},
		APIToken: "cfg-secret",
	}
	cfg := config.NewStore(path, f)
	s := &Server{Cfg: cfg}
	if got := s.apiToken(); got != "cfg-secret" {
		t.Fatalf("apiToken via Cfg: %q", got)
	}

	empty := &Server{}
	if empty.apiToken() != "" {
		t.Fatal("empty server token")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil)
	if s.authRequired(req) {
		t.Fatal("GET should not require auth")
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/devices", nil)
	if !s.authRequired(req) {
		t.Fatal("POST should require auth")
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ws/stream", nil)
	if !s.authRequired(req) {
		t.Fatal("WS should require auth when token set")
	}
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	if s.authRequired(req) {
		t.Fatal("non-api path")
	}
	if empty.authRequired(httptest.NewRequest(http.MethodDelete, "/api/v1/devices/x", nil)) {
		t.Fatal("no token → no auth")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
	if s.authorized(req) {
		t.Fatal("missing token")
	}
	req.Header.Set("Authorization", "Bearer cfg-secret")
	if !s.authorized(req) {
		t.Fatal("Bearer")
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
	req.Header.Set("Authorization", "bearer cfg-secret")
	if extractAPIToken(req) != "cfg-secret" {
		t.Fatalf("lowercase bearer: %q", extractAPIToken(req))
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
	req.Header.Set("X-API-Key", "cfg-secret")
	if extractAPIToken(req) != "cfg-secret" {
		t.Fatalf("X-API-Key: %q", extractAPIToken(req))
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ws/stream?token=cfg-secret", nil)
	if extractAPIToken(req) != "cfg-secret" {
		t.Fatalf("query token: %q", extractAPIToken(req))
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
	req.Header.Set("Authorization", "Basic nope")
	if extractAPIToken(req) != "" {
		t.Fatalf("non-bearer auth: %q", extractAPIToken(req))
	}
	if empty.authorized(httptest.NewRequest(http.MethodGet, "/", nil)) {
		// empty token → authorized always
	} else {
		t.Fatal("empty token should authorize")
	}

	handler := s.APIAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/v1/devices/x", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rr.Code)
	}
}
