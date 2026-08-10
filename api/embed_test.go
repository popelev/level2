package apispec

import (
	"strings"
	"testing"
)

func TestOpenAPIYAMLEmbedded(t *testing.T) {
	if len(OpenAPIYAML) < 16 {
		t.Fatalf("embedded openapi.yaml too small: %d bytes", len(OpenAPIYAML))
	}
	s := string(OpenAPIYAML)
	if !strings.Contains(s, "openapi") && !strings.Contains(s, "OpenAPI") {
		t.Fatalf("embed missing openapi marker, prefix=%q", s[:min(64, len(s))])
	}
	for _, must := range []string{
		"version: 1.2.1",
		"/api/v1/database/wipe-samples",
		"/api/v1/project.xlsx",
		"/api/v1/devices/{id}/tags/sync",
		"/metrics",
	} {
		if !strings.Contains(s, must) {
			t.Fatalf("openapi missing %q", must)
		}
	}
}
