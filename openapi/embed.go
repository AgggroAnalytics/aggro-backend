package openapi

import _ "embed"

//go:embed openapi.yaml
// SpecYAML is served at GET /openapi.yaml for frontend codegen (openapi-generator, orval, etc.).
var SpecYAML []byte
