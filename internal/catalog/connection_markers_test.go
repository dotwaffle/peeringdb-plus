package catalog

import (
	"testing"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

func TestConnectionMarkersFor(t *testing.T) {
	t.Parallel()

	plan := func(status, date any) map[string]any {
		return map[string]any{"planned_status_change": map[string]any{"status": status, "date": date}}
	}
	tests := []struct {
		name string
		nix  ent.NetworkIxLan
		want ConnectionMarkers
	}{
		{
			name: "ok and operational",
			nix:  ent.NetworkIxLan{Status: "ok", Operational: true, Meta: map[string]any{}},
		},
		{
			name: "not-operational status",
			nix:  ent.NetworkIxLan{Status: "not-operational"},
			want: ConnectionMarkers{NotOperational: true},
		},
		{
			// A row that upstream has not migrated to not-operational yet.
			name: "ok with operational false",
			nix:  ent.NetworkIxLan{Status: "ok", Operational: false},
			want: ConnectionMarkers{NotOperational: true},
		},
		{
			// Upstream 2.83.0 saves pending rows with operational=false.
			name: "pending",
			nix:  ent.NetworkIxLan{Status: "pending", Operational: false},
		},
		{
			name: "planned removal and rfc8950",
			nix: ent.NetworkIxLan{Status: "ok", Operational: true, Meta: map[string]any{
				"planned_status_change": map[string]any{"status": "deleted", "date": "2026-12-31"},
				"rfc8950":               true,
			}},
			want: ConnectionMarkers{PlannedStatus: "deleted", PlannedDate: "2026-12-31", RFC8950: true},
		},
		{
			name: "planned activation",
			nix:  ent.NetworkIxLan{Status: "ok", Operational: true, Meta: plan("ok", "2027-01-15")},
			want: ConnectionMarkers{PlannedStatus: "ok", PlannedDate: "2027-01-15"},
		},
		{
			// rfc8950=false is a declaration upstream, but it has no marker.
			name: "rfc8950 false",
			nix:  ent.NetworkIxLan{Status: "ok", Operational: true, Meta: map[string]any{"rfc8950": false}},
		},
		{
			name: "values of unexpected types",
			nix: ent.NetworkIxLan{Status: "ok", Operational: true, Meta: map[string]any{
				"planned_status_change": "deleted",
				"rfc8950":               "true",
			}},
		},
		{
			name: "plan parts of unexpected types",
			nix:  ent.NetworkIxLan{Status: "ok", Operational: true, Meta: plan(1, false)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ConnectionMarkersFor(&tt.nix); got != tt.want {
				t.Errorf("ConnectionMarkersFor() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestPlannedChangeLabel locks the upstream marker text
// (2.83.0 templates/site/view_exchange_side.html): "planned removal" for
// status deleted, "planned activation" for any other status, then the date.
func TestPlannedChangeLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		m    ConnectionMarkers
		want string
	}{
		{ConnectionMarkers{}, ""},
		{ConnectionMarkers{NotOperational: true, RFC8950: true}, ""},
		{ConnectionMarkers{PlannedStatus: "deleted", PlannedDate: "2026-12-31"}, "planned removal 2026-12-31"},
		{ConnectionMarkers{PlannedStatus: "ok", PlannedDate: "2027-01-15"}, "planned activation 2027-01-15"},
		{ConnectionMarkers{PlannedDate: "2027-01-15"}, "planned activation 2027-01-15"},
		{ConnectionMarkers{PlannedStatus: "deleted"}, "planned removal"},
	}
	for _, tt := range tests {
		if got := tt.m.PlannedChangeLabel(); got != tt.want {
			t.Errorf("%+v.PlannedChangeLabel() = %q, want %q", tt.m, got, tt.want)
		}
	}
}
