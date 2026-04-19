package parity

import (
	"net/http"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	parityfix "github.com/dotwaffle/peeringdb-plus/internal/testutil/parity"
)

// TestHarness_SeedFixtures_NoFKPath asserts the Pass-1 (FK-free) path:
// a single org row passed in as a Fixture results in exactly one
// Organization row in the ent client with the matching name.
//
// synthesised: phase72-04-harness — keeps the harness independent of
// the ported fixture slices so a future fixture regeneration can't
// break harness-internal correctness assertions.
func TestHarness_SeedFixtures_NoFKPath(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)

	fx := []parityfix.Fixture{
		{
			Entity:   "org",
			ID:       42,
			Fields:   map[string]string{"name": `"HarnessProbe-NoFK"`, "status": `"ok"`},
			Upstream: "synthesised: phase72-04-harness",
		},
	}
	seedFixtures(t, c, fx)

	got, err := c.Organization.Query().Where(organization.IDEQ(42)).Only(t.Context())
	if err != nil {
		t.Fatalf("query org id=42: %v", err)
	}
	if got.Name != "HarnessProbe-NoFK" {
		t.Errorf("name = %q, want %q", got.Name, "HarnessProbe-NoFK")
	}
}

// TestHarness_SeedFixtures_FKPath asserts the Pass-2 path: a child
// fixture carrying __fk="org:7" is linked to the persisted parent
// org with id=7.
//
// synthesised: phase72-04-harness
func TestHarness_SeedFixtures_FKPath(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)

	fx := []parityfix.Fixture{
		{
			Entity:   "org",
			ID:       7,
			Fields:   map[string]string{"name": `"HarnessProbe-Parent"`, "status": `"ok"`},
			Upstream: "synthesised: phase72-04-harness",
		},
		{
			Entity: "net",
			ID:     8,
			Fields: map[string]string{
				"name":   `"HarnessProbe-Child"`,
				"asn":    `"65500"`,
				"status": `"ok"`,
				"__fk":   `"org:7"`,
			},
			Upstream: "synthesised: phase72-04-harness",
		},
	}
	seedFixtures(t, c, fx)

	got, err := c.Network.Query().Where(network.IDEQ(8)).WithOrganization().Only(t.Context())
	if err != nil {
		t.Fatalf("query net id=8: %v", err)
	}
	if got.OrgID == nil || *got.OrgID != 7 {
		t.Errorf("net.OrgID = %v, want *7 (FK resolution failed)", got.OrgID)
	}
	if got.Edges.Organization == nil || got.Edges.Organization.ID != 7 {
		t.Errorf("net.Organization edge not loaded or wrong: %+v", got.Edges.Organization)
	}
}

// TestHarness_NewTestServer_RoundTrip asserts the test server wires
// the canonical pdbcompat handler correctly: GET /api/ returns the
// index document with HTTP 200 + a non-empty JSON body.
//
// synthesised: phase72-04-harness
func TestHarness_NewTestServer_RoundTrip(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	srv := newTestServer(t, c)

	status, body := httpGet(t, srv, "/api/")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, string(body))
	}
	if len(body) == 0 {
		t.Errorf("body empty; want non-empty index JSON")
	}
}

// TestHarness_DecodeDataArray asserts the envelope decoder handles a
// well-formed list response. Wraps the index endpoint isn't a list
// shape, so seeds one network and lists /api/net.
//
// synthesised: phase72-04-harness
func TestHarness_DecodeDataArray(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	fx := []parityfix.Fixture{
		{
			Entity: "net",
			ID:     1000,
			Fields: map[string]string{
				"name":   `"DecoderProbe"`,
				"asn":    `"65000"`,
				"status": `"ok"`,
			},
			Upstream: "synthesised: phase72-04-harness",
		},
	}
	seedFixtures(t, c, fx)

	srv := newTestServer(t, c)
	status, body := httpGet(t, srv, "/api/net")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, string(body))
	}
	rows := decodeDataArray(t, body)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if name, _ := rows[0]["name"].(string); name != "DecoderProbe" {
		t.Errorf("name = %q, want DecoderProbe", name)
	}
}
