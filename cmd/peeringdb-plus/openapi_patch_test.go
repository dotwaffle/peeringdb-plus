package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/ent/rest"
)

// TestPatchOpenAPIErrorResponses verifies the served-spec patch: every
// components.responses.Error* entry must describe the RFC 9457
// application/problem+json body middleware.RESTError actually emits, the
// ProblemDetail schema must exist, and entrest's never-emitted Error*
// schemas must be gone.
func TestPatchOpenAPIErrorResponses(t *testing.T) {
	t.Parallel()

	patched, err := patchOpenAPIErrorResponses(rest.OpenAPI)
	if err != nil {
		t.Fatalf("patchOpenAPIErrorResponses: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(patched, &doc); err != nil {
		t.Fatalf("patched spec is not valid JSON: %v", err)
	}
	components := doc["components"].(map[string]any)
	responses := components["responses"].(map[string]any)
	schemas := components["schemas"].(map[string]any)

	errorResponses := 0
	for name, v := range responses {
		if !strings.HasPrefix(name, "Error") {
			continue
		}
		errorResponses++
		resp := v.(map[string]any)
		content, ok := resp["content"].(map[string]any)
		if !ok {
			t.Fatalf("response %s: missing content object", name)
		}
		if _, ok := content["application/json"]; ok {
			t.Errorf("response %s still declares application/json", name)
		}
		pj, ok := content["application/problem+json"].(map[string]any)
		if !ok {
			t.Fatalf("response %s: missing application/problem+json content", name)
		}
		schema := pj["schema"].(map[string]any)
		if ref := schema["$ref"]; ref != "#/components/schemas/ProblemDetail" {
			t.Errorf("response %s: schema $ref = %v, want ProblemDetail", name, ref)
		}
	}
	if errorResponses == 0 {
		t.Fatal("no Error* responses found in spec — patch target shape changed")
	}

	pd, ok := schemas["ProblemDetail"].(map[string]any)
	if !ok {
		t.Fatal("schemas missing ProblemDetail")
	}
	props := pd["properties"].(map[string]any)
	for _, field := range []string{"type", "title", "status", "detail", "instance"} {
		if _, ok := props[field]; !ok {
			t.Errorf("ProblemDetail missing property %q", field)
		}
	}

	for name := range schemas {
		if strings.HasPrefix(name, "Error") {
			t.Errorf("entrest schema %s should have been removed (never emitted on the wire)", name)
		}
	}

	// No dangling refs: every $ref in the document must resolve to a
	// surviving component.
	var walk func(v any)
	walk = func(v any) {
		switch tv := v.(type) {
		case map[string]any:
			if ref, ok := tv["$ref"].(string); ok {
				parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
				cur := any(doc)
				for _, p := range parts {
					m, ok := cur.(map[string]any)
					if !ok {
						t.Errorf("dangling $ref %q", ref)
						return
					}
					cur, ok = m[p]
					if !ok {
						t.Errorf("dangling $ref %q", ref)
						return
					}
				}
			}
			for _, child := range tv {
				walk(child)
			}
		case []any:
			for _, child := range tv {
				walk(child)
			}
		}
	}
	walk(doc)
}
