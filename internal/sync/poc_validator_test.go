package sync

import (
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestPoc_RoleEmptyAcceptedByValidator is the unit-level proof for
// Phase 73 BUG-02 per CONTEXT.md D-03 (the fast-feedback affordance
// alongside the httptest fake-upstream conformance test
// TestSync_IncrementalRoleTombstone in worker_test.go).
//
// Asserts: an upsert with role="" must succeed against an existing poc
// row. If a future regression re-introduces NotEmpty() on poc.role
// (e.g., the cmd/pdb-schema-generate isTombstoneVulnerableField gate
// is accidentally reverted), this test fails immediately on the
// validator chain — without needing to spin up the httptest fake-upstream
// sync harness. Runtime: <100ms vs. seconds for the httptest version.
//
// upstream: SEED-001 spike 2026-04-26 (260426-pms confirmed name=""
// tombstones for the 6 folded entities; role="" on poc is the
// symmetric case for v1.18.0 Phase 73).
func TestPoc_RoleEmptyAcceptedByValidator(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	ctx := t.Context()
	t0 := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	// Seed parent network. Network has many required scalar fields
	// (allow_ixp_update, asn, info_ipv6, info_multicast,
	// info_never_via_route_servers, info_unicast, name, policy_ratio,
	// created, updated). Set only the required ones; leave Optional
	// fields as their schema defaults.
	const netID = 100
	if _, err := c.Network.Create().
		SetID(netID).
		SetName("ParentNet").
		SetNameFold("parentnet").
		SetAsn(64500).
		SetAllowIxpUpdate(false).
		SetInfoIpv6(false).
		SetInfoMulticast(false).
		SetInfoNeverViaRouteServers(false).
		SetInfoUnicast(false).
		SetPolicyRatio(false).
		SetStatus("ok").
		SetCreated(t0).
		SetUpdated(t0).
		Save(ctx); err != nil {
		t.Fatalf("seed network: %v", err)
	}

	// Seed a poc with non-empty role.
	if _, err := c.Poc.Create().
		SetID(1).
		SetNetID(netID).
		SetRole("Technical").
		SetVisible("Public").
		SetStatus("ok").
		SetCreated(t0).
		SetUpdated(t0).
		Save(ctx); err != nil {
		t.Fatalf("seed poc: %v", err)
	}

	// The actual proof: empty-role tombstone update must succeed.
	// If NotEmpty() on poc.role ever returns, this Save() fails with
	// "validator failed for field \"Poc.role\"".
	if _, err := c.Poc.UpdateOneID(1).
		SetRole("").
		SetStatus("deleted").
		Save(ctx); err != nil {
		t.Fatalf("update poc with role=\"\": %v (validator regression?)", err)
	}

	got, err := c.Poc.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get poc: %v", err)
	}
	if got.Role != "" {
		t.Errorf("poc.Role = %q, want %q (PII-scrubbed tombstone)", got.Role, "")
	}
	if got.Status != "deleted" {
		t.Errorf("poc.Status = %q, want %q", got.Status, "deleted")
	}
}
