package apispec

import _ "embed"

// OpenAPIYAML is the canonical OpenAPI 3 contract for Level2 /api/v1.
//
//go:embed openapi.yaml
var OpenAPIYAML []byte
