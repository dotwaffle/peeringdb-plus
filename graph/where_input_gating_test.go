package graph_test

import (
	"strings"
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

// TestGraphQLAPI_PocEdgeNotFilterable verifies that NetworkWhereInput has
// no predicates over the pocs edge. hasPocsWith runs as a plain SQL
// neighbor query without the poc privacy policy, so a caller could match
// networks on the name, email or phone of a Users or Private poc and
// read the value one prefix at a time. hasPocs would show that hidden
// pocs exist. The nested form through OrganizationWhereInput must fail
// too, because it uses the same NetworkWhereInput.
func TestGraphQLAPI_PocEdgeNotFilterable(t *testing.T) {
	t.Parallel()
	srv := seedFullTestServer(t)

	for _, q := range []string{
		`{ networks(where: {hasPocsWith: [{emailHasPrefix: "a"}]}) { edges { node { id } } } }`,
		`{ networks(where: {hasPocs: true}) { edges { node { id } } } }`,
		`{ organizations(where: {hasNetworksWith: [{hasPocsWith: [{nameContains: "a"}]}]}) { edges { node { id } } } }`,
		`{ networksList(where: {hasPocsWith: [{phoneHasPrefix: "+"}]}) { id } }`,
	} {
		res := postGraphQL(t, srv.URL, q)
		if len(res.Errors) == 0 || !strings.Contains(res.Errors[0].Message, "hasPocs") {
			t.Errorf("expected a validation error that names hasPocs, the pocs edge must not be filterable: %s\nerrors: %v", q, res.Errors)
		}
	}

	// The pocs output field stays; the poc privacy policy filters its rows.
	res := postGraphQL(t, srv.URL, `{ networks { edges { node { id pocs { id } } } } }`)
	if len(res.Errors) != 0 {
		t.Fatalf("selecting the pocs output field should still succeed, got: %v", res.Errors)
	}

	// PocWhereInput on the pocs query stays: that query runs the policy.
	res = postGraphQL(t, srv.URL, `{ pocs(where: {emailHasPrefix: "a"}) { edges { node { id } } } }`)
	if len(res.Errors) != 0 {
		t.Fatalf("filtering the pocs query should still succeed, got: %v", res.Errors)
	}
}
