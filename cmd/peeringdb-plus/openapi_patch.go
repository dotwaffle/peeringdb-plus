package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// patchOpenAPIErrorResponses rewrites the entrest-generated OpenAPI spec so
// every components.responses.Error* entry declares the RFC 9457
// application/problem+json body that middleware.RESTError actually emits,
// referencing a single ProblemDetail schema (mirroring
// internal/httperr.ProblemDetail). The now-unreferenced entrest Error*
// schemas are removed so the spec carries no dead shapes. The input bytes
// are not modified; a freshly marshalled spec is returned.
func patchOpenAPIErrorResponses(spec []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(spec, &doc); err != nil {
		return nil, fmt.Errorf("parse openapi spec: %w", err)
	}
	components, ok := doc["components"].(map[string]any)
	if !ok {
		return nil, errors.New("openapi spec: missing components object")
	}
	responses, ok := components["responses"].(map[string]any)
	if !ok {
		return nil, errors.New("openapi spec: missing components.responses object")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return nil, errors.New("openapi spec: missing components.schemas object")
	}

	for name, v := range responses {
		if !strings.HasPrefix(name, "Error") {
			continue
		}
		resp, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("openapi spec: response %s is not an object", name)
		}
		resp["content"] = map[string]any{
			"application/problem+json": map[string]any{
				"schema": map[string]any{"$ref": "#/components/schemas/ProblemDetail"},
			},
		}
		// The same-named entrest ErrorResponse schema is only referenced
		// from this response entry; drop it now that the ref is gone.
		delete(schemas, name)
	}

	schemas["ProblemDetail"] = map[string]any{
		"type":        "object",
		"description": "RFC 9457 Problem Details error response.",
		"properties": map[string]any{
			"type": map[string]any{
				"type":        "string",
				"description": "A URI reference identifying the problem type.",
				"example":     "about:blank",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Short, human-readable summary of the problem type.",
				"example":     "Bad Request",
			},
			"status": map[string]any{
				"type":        "integer",
				"description": "The HTTP status code.",
				"example":     400,
			},
			"detail": map[string]any{
				"type":        "string",
				"description": "Human-readable explanation specific to this occurrence.",
			},
			"instance": map[string]any{
				"type":        "string",
				"description": "URI reference identifying this occurrence.",
				"example":     "/rest/v1/networks",
			},
		},
		"required": []any{"type", "title", "status"},
	}

	return json.Marshal(doc)
}
