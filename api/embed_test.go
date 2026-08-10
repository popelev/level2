package apispec

import (
	"regexp"
	"strings"
	"testing"
)

// Matches info.version under the top-level info: block (handles multiline description).
var openAPIInfoVersionRE = regexp.MustCompile(`(?m)^info:\r?\n(?:.*\r?\n)*?^[ \t]+version:[ \t]*["']?([0-9]+\.[0-9]+\.[0-9]+)`)

func TestOpenAPIYAMLEmbedded(t *testing.T) {
	if len(OpenAPIYAML) < 16 {
		t.Fatalf("embedded openapi.yaml too small: %d bytes", len(OpenAPIYAML))
	}
	s := string(OpenAPIYAML)
	if !strings.Contains(s, "openapi") && !strings.Contains(s, "OpenAPI") {
		t.Fatalf("embed missing openapi marker, prefix=%q", s[:min(64, len(s))])
	}

	m := openAPIInfoVersionRE.FindStringSubmatch(s)
	if m == nil {
		t.Fatalf("embedded openapi missing info.version (semver major.minor.patch)")
	}
	ver := m[1]
	parts := strings.Split(ver, ".")
	if len(parts) < 2 {
		t.Fatalf("embedded openapi version %q missing major.minor", ver)
	}
	majorMinor := parts[0] + "." + parts[1]
	if majorMinor != "1.4" {
		t.Fatalf("embedded openapi major.minor=%q want 1.4 (got full %q)", majorMinor, ver)
	}
	// Pin current patch so accidental YAML bumps fail until this assert is updated.
	if ver != "1.4.0" {
		t.Fatalf("embedded openapi version=%q want 1.4.0", ver)
	}

	for _, must := range []string{
		"/api/v1/tags/values",
		"/api/v1/integration/tag-catalog",
		"/api/v1/database/wipe-samples",
		"/api/v1/project.xlsx",
		"/api/v1/devices/{id}/tags/sync",
		"/metrics",
		"BearerAuthWrite",
		"BearerAuthAdmin",
		"IdempotencyKey",
		"schemas/Error",
	} {
		if !strings.Contains(s, must) {
			t.Fatalf("openapi missing %q", must)
		}
	}
}
