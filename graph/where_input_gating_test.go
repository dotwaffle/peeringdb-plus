package graph_test

import (
	"testing"
)

// TestGraphQLAPI_GatedURLNotFilterable verifies that the redaction-gated
// ixfIxpMemberListURL field is not exposed as a filter in IxLanWhereInput.
// Output redaction nulls the value for anonymous callers, but a value-filter
// predicate (HasPrefix/Contains/EqualFold) operates on the real column and is
// a boolean oracle that reconstructs the gated URL one probe at a time. The
// entgql.SkipWhereInput annotation removes those predicates from the schema.
func TestGraphQLAPI_GatedURLNotFilterable(t *testing.T) {
	t.Parallel()
	srv := seedFullTestServer(t)

	// A filter on the gated URL value must be a schema validation error.
	res := postGraphQL(t, srv.URL,
		`{ ixLans(where: {ixfIxpMemberListURLHasPrefix: "https://"}) { edges { node { id } } } }`)
	if len(res.Errors) == 0 {
		t.Fatal("expected a validation error: the redaction-gated ixfIxpMemberListURL must not be filterable via IxLanWhereInput")
	}

	// The output field is still selectable — only its value is redacted at the
	// resolver; the field stays in the IxLan type.
	res = postGraphQL(t, srv.URL,
		`{ ixLans { edges { node { id ixfIxpMemberListURL } } } }`)
	if len(res.Errors) != 0 {
		t.Fatalf("selecting the gated output field should still succeed, got: %v", res.Errors)
	}

	// The _visible companion remains filterable (upstream parity — only the
	// URL value is gated).
	res = postGraphQL(t, srv.URL,
		`{ ixLans(where: {ixfIxpMemberListURLVisible: "Public"}) { edges { node { id } } } }`)
	if len(res.Errors) != 0 {
		t.Fatalf("the _visible companion should remain filterable, got: %v", res.Errors)
	}
}
