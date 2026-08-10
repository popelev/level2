package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// API role for mutating /api/v1 routes (and WS).
type apiRole int

const (
	roleNone apiRole = iota
	roleWrite
	roleAdmin
)

func (s *Server) legacyToken() string {
	if s.APIToken != nil {
		return strings.TrimSpace(s.APIToken())
	}
	if s.Cfg != nil {
		return strings.TrimSpace(s.Cfg.APIToken())
	}
	return ""
}

func (s *Server) writeTokenConfigured() string {
	if s.APITokenWrite != nil {
		return strings.TrimSpace(s.APITokenWrite())
	}
	if s.Cfg != nil {
		return strings.TrimSpace(s.Cfg.APITokenWrite())
	}
	return ""
}

func (s *Server) adminTokenConfigured() string {
	if s.APITokenAdmin != nil {
		return strings.TrimSpace(s.APITokenAdmin())
	}
	if s.Cfg != nil {
		return strings.TrimSpace(s.Cfg.APITokenAdmin())
	}
	return ""
}

// apiToken is the legacy shared getter (tests / backward compat).
func (s *Server) apiToken() string {
	return s.legacyToken()
}

func (s *Server) authEnabled() bool {
	return s.writeTokenConfigured() != "" || s.adminTokenConfigured() != "" || s.legacyToken() != ""
}

// tokensForRole returns secrets that authorize the given role.
// Legacy LEVEL2_API_TOKEN alone still acts as shared write+admin.
// When role tokens are set: write accepts write|admin|legacy; admin accepts admin|legacy
// (never the write-only secret).
func (s *Server) tokensForRole(role apiRole) []string {
	write := s.writeTokenConfigured()
	admin := s.adminTokenConfigured()
	legacy := s.legacyToken()
	roleSplit := write != "" || admin != ""

	switch role {
	case roleWrite:
		if !roleSplit {
			if legacy != "" {
				return []string{legacy}
			}
			return nil
		}
		return uniqueNonEmpty([]string{write, admin, legacy})
	case roleAdmin:
		if !roleSplit {
			if legacy != "" {
				return []string{legacy}
			}
			return nil
		}
		return uniqueNonEmpty([]string{admin, legacy})
	default:
		return nil
	}
}

func uniqueNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func tokenMatchesAny(got string, wants []string) bool {
	if got == "" {
		return false
	}
	gb := []byte(got)
	for _, w := range wants {
		if w == "" {
			continue
		}
		wb := []byte(w)
		if len(gb) != len(wb) {
			continue
		}
		if subtle.ConstantTimeCompare(gb, wb) == 1 {
			return true
		}
	}
	return false
}

// requestRole classifies the request for auth. Reads stay roleNone (open).
// Write: PUT …/tags/{id}/value, POST …/tags/values, WS stream.
// Admin: all other mutating /api/v1/* (wipe, capacity, config, import, …).
func requestRole(r *http.Request) apiRole {
	path := r.URL.Path
	if !strings.HasPrefix(path, "/api/v1/") {
		return roleNone
	}
	if path == "/api/v1/ws/stream" {
		return roleWrite
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return roleNone
	}
	if r.Method == http.MethodPut && strings.HasPrefix(path, "/api/v1/tags/") && strings.HasSuffix(path, "/value") {
		return roleWrite
	}
	if r.Method == http.MethodPost && path == "/api/v1/tags/values" {
		return roleWrite
	}
	return roleAdmin
}

// APIAuth wraps a handler so mutating /api/v1 requests (and WS stream) require
// Authorization: Bearer <token> or X-API-Token / X-API-Key when a token is configured.
// Empty tokens disable the gate (lab backward compatible).
// Write token cannot perform admin actions → 403; missing/wrong → 401.
func (s *Server) APIAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := requestRole(r)
		if role == roleNone || !s.authEnabled() {
			next.ServeHTTP(w, r)
			return
		}
		got := extractAPIToken(r)
		if got == "" {
			http.Error(w, "unauthorized: missing or invalid API token", http.StatusUnauthorized)
			return
		}
		allowed := s.tokensForRole(role)
		if tokenMatchesAny(got, allowed) {
			next.ServeHTTP(w, r)
			return
		}
		// Valid write token on an admin route → 403 (not 401).
		if role == roleAdmin && tokenMatchesAny(got, s.tokensForRole(roleWrite)) {
			http.Error(w, "forbidden: write token cannot perform admin actions", http.StatusForbidden)
			return
		}
		http.Error(w, "unauthorized: missing or invalid API token", http.StatusUnauthorized)
	})
}

func (s *Server) authRequired(r *http.Request) bool {
	return s.authEnabled() && requestRole(r) != roleNone
}

func (s *Server) authorized(r *http.Request) bool {
	role := requestRole(r)
	if role == roleNone || !s.authEnabled() {
		return true
	}
	got := extractAPIToken(r)
	if got == "" {
		return false
	}
	return tokenMatchesAny(got, s.tokensForRole(role))
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
