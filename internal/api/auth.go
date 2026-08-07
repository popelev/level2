package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (s *Server) apiToken() string {
	if s.APIToken != nil {
		return strings.TrimSpace(s.APIToken())
	}
	if s.Cfg != nil {
		return strings.TrimSpace(s.Cfg.APIToken())
	}
	return ""
}

// APIAuth wraps a handler so mutating /api/v1 requests (and WS stream) require
// Authorization: Bearer <token> or X-API-Token / X-API-Key when a token is configured.
// Empty token disables the gate (lab backward compatible).
func (s *Server) APIAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authRequired(r) && !s.authorized(r) {
			http.Error(w, "unauthorized: missing or invalid API token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authRequired(r *http.Request) bool {
	if s.apiToken() == "" {
		return false
	}
	path := r.URL.Path
	if !strings.HasPrefix(path, "/api/v1/") {
		return false
	}
	// WebSocket stream: protect when token configured (token via header or ?token=).
	if path == "/api/v1/ws/stream" {
		return true
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func (s *Server) authorized(r *http.Request) bool {
	want := s.apiToken()
	if want == "" {
		return true
	}
	got := extractAPIToken(r)
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func extractAPIToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		const prefix = "Bearer "
		if strings.HasPrefix(h, prefix) {
			return strings.TrimSpace(h[len(prefix):])
		}
		if strings.HasPrefix(strings.ToLower(h), "bearer ") {
			return strings.TrimSpace(h[7:])
		}
	}
	if v := strings.TrimSpace(r.Header.Get("X-API-Token")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("X-API-Key")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("token")); v != "" {
		return v
	}
	return ""
}
