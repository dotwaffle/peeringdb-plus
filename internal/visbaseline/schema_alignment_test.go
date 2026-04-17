package visbaseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestSchemaAlignmentWithPhase57Diff locks in the Phase 58 empirical finding:
// the committed Phase 57 diff.json contains only two auth-gated surfaces, both
// already covered by existing ent fields —
//
//  1. poc row-level visibility — ent/schema/poc.go `visible` field
//     (Public vs Users).
//  2. ixlan.ixf_ixp_member_list_url — ent/schema/ixlan.go
//     `ixf_ixp_member_list_url_visible` field.
//
// Any future Phase 57 re-capture that surfaces a new auth-gated field will
// fail this test and force Phase 58 to be re-opened with planning before
// Phase 59 (privacy policy) ships against a stale assumption.
//
// This is a regression guard, not a feature test — if it fails, do not "fix"
// it by editing the allowlist. Instead, re-run /gsd-plan-phase 58 and add the
// appropriate <field>_visible ent field per the v1.14 Key Decision.
func TestSchemaAlignmentWithPhase57Diff(t *testing.T) {
	diffPath := filepath.Join("..", "..", "testdata", "visibility-baseline", "diff.json")

	raw, err := os.ReadFile(diffPath)
	if err != nil {
		t.Fatalf("read %s: %v", diffPath, err)
	}

	var report Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("unmarshal %s: %v", diffPath, err)
	}

	// Test 1 (happy path): schema_version matches the frozen constant.
	t.Run("schema_version_matches", func(t *testing.T) {
		if report.SchemaVersion != ReportSchemaVersion {
			t.Errorf("schema_version: got %d, want %d (diff.json is stale — regenerate with pdbcompat-check)",
				report.SchemaVersion, ReportSchemaVersion)
		}
	})

	// Test 2 (coverage): all 13 PeeringDB types are present under the
	// "beta/" prefix.
	expectedTypes := []string{
		"beta/campus", "beta/carrier", "beta/carrierfac", "beta/fac",
		"beta/ix", "beta/ixfac", "beta/ixlan", "beta/ixpfx",
		"beta/net", "beta/netfac", "beta/netixlan", "beta/org", "beta/poc",
	}
	t.Run("all_13_types_present", func(t *testing.T) {
		for _, tname := range expectedTypes {
			if _, ok := report.Types[tname]; !ok {
				t.Errorf("missing type %q in diff.json — Phase 57 capture is incomplete; re-run capture for all 13 types",
					tname)
			}
		}
	})

	// Test 3 (core assertion): every AuthOnly field across every type must
	// appear in the allowlist. The allowlist intentionally covers only the
	// two pre-known auth-gated surfaces.
	//
	// poc entries are row-level leakage — 460 previously-hidden Users-tier
	// rows entering the result set, each carrying all 10 of their fields.
	// These are NOT new schema fields; they are signal that poc.visible
	// (already in ent/schema/poc.go) is the correct row-level gate.
	//
	// ixlan.ixf_ixp_member_list_url is already gated by the pre-existing
	// ixlan.ixf_ixp_member_list_url_visible field.
	allowedAuthGated := map[string]map[string]struct{}{
		"beta/poc": {
			"created": {}, "email": {}, "id": {}, "name": {}, "net_id": {},
			"phone": {}, "role": {}, "status": {}, "updated": {}, "url": {},
		},
		"beta/ixlan": {
			"ixf_ixp_member_list_url": {},
		},
	}
	t.Run("no_unexpected_auth_gated_fields", func(t *testing.T) {
		for tname, tr := range report.Types {
			allowed := allowedAuthGated[tname]
			for _, fd := range tr.Fields {
				if !fd.AuthOnly {
					continue
				}
				if _, ok := allowed[fd.Name]; ok {
					continue
				}
				t.Errorf(
					"unexpected auth-gated field %q.%q in testdata/visibility-baseline/diff.json — "+
						"Phase 58 planned no schema work for this. Re-run /gsd-plan-phase 58 after "+
						"updating 58-CONTEXT.md, or add an ent <field>_visible field per the "+
						"<field>_visible convention (see ent/schema/ixlan.go "+
						"ixf_ixp_member_list_url_visible for the template).",
					tname, fd.Name,
				)
			}
		}
	})

	// Test 4 (poc row-level sanity): confirm the visible enum drifts the way
	// we expect — anon sees only Public, auth sees Public + Users. This is
	// the signal that poc.visible is the correct row-level discriminator.
	t.Run("poc_visible_drifts_public_to_users", func(t *testing.T) {
		pocRep, ok := report.Types["beta/poc"]
		if !ok {
			t.Fatal("beta/poc missing from report — covered separately by all_13_types_present")
		}
		wantAnon := []string{"Public"}
		if !slices.Equal(pocRep.VisibleValuesAnon, wantAnon) {
			t.Errorf("poc visible_values_anon: got %v, want %v — anonymous responses should only carry Public rows",
				pocRep.VisibleValuesAnon, wantAnon)
		}
		// Auth set must contain both Public AND Users. It may be ordered
		// differently across regenerations, so check membership rather than
		// equality.
		if !slices.Contains(pocRep.VisibleValuesAuth, "Public") {
			t.Errorf("poc visible_values_auth missing \"Public\": got %v", pocRep.VisibleValuesAuth)
		}
		if !slices.Contains(pocRep.VisibleValuesAuth, "Users") {
			t.Errorf("poc visible_values_auth missing \"Users\": got %v — this is the key Phase 57 signal that "+
				"Users-tier rows are hidden from anon responses; if absent, the capture did not include "+
				"any Users-tier POCs and the sample is unrepresentative", pocRep.VisibleValuesAuth)
		}
	})

	// Test 5 (non-AuthOnly visible field on poc): the `visible` field itself
	// is a controlled enum, not an auth-gated field — so it must appear in
	// poc.Fields with AuthOnly==false. ValueSetDrift is an optional flag
	// (may be true or absent), so we do not assert on it here; the drift is
	// already validated in Test 4 via VisibleValuesAnon/Auth.
	t.Run("poc_visible_field_not_authonly", func(t *testing.T) {
		pocRep := report.Types["beta/poc"]
		for _, fd := range pocRep.Fields {
			if fd.Name != "visible" {
				continue
			}
			if fd.AuthOnly {
				t.Errorf("poc.visible is flagged AuthOnly=true, but it must be a controlled enum "+
					"surfaced in both anon and auth responses. Got %+v", fd)
			}
			return
		}
		// Field missing from Fields is acceptable — diff.go only emits the
		// "visible" entry when ValueSetDrift fires. Note it for maintainers
		// but do not fail; Test 4 covers the drift signal directly.
		t.Log("poc.visible not present in Fields — value-set drift synthesis did not fire; Test 4 covers the anon/auth drift directly")
	})
}
