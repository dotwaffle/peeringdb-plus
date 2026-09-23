package catalog

import (
	"strings"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

// ConnectionMarkers holds the public markers that PeeringDB 2.83.0 shows
// beside a network's connection to an exchange in its network and exchange
// views (templates/site/view_network_side.html, view_exchange_side.html).
type ConnectionMarkers struct {
	// NotOperational marks a live connection that is not operational.
	NotOperational bool
	// PlannedStatus is meta.planned_status_change.status: "deleted" for a
	// planned removal, "ok" for a planned activation. Empty when no plan
	// is set.
	PlannedStatus string
	// PlannedDate is meta.planned_status_change.date (YYYY-MM-DD).
	PlannedDate string
	// RFC8950 reports meta.rfc8950 = true: the network supports RFC 8950
	// extended next hop on this connection.
	RFC8950 bool
}

// ConnectionMarkersFor derives the markers of one netixlan row from its
// status, operational flag and meta document.
func ConnectionMarkersFor(nix *ent.NetworkIxLan) ConnectionMarkers {
	m := ConnectionMarkers{
		// Upstream 2.83.0 moved this state into status "not-operational".
		// A row that upstream has not migrated yet is still ok with
		// operational=false, and upstream marks it the same way
		// (docs/api/obj_netixlan.md:50-54).
		NotOperational: nix.Status == "not-operational" || (nix.Status == "ok" && !nix.Operational),
	}
	// The meta document is open and user-supplied, so read each key
	// defensively: a value of an unexpected type sets no marker.
	if plan, ok := nix.Meta["planned_status_change"].(map[string]any); ok {
		m.PlannedStatus, _ = plan["status"].(string)
		m.PlannedDate, _ = plan["date"].(string)
	}
	m.RFC8950, _ = nix.Meta["rfc8950"].(bool)
	return m
}

// PlannedChangeLabel returns the upstream text for a planned status change,
// "planned removal <date>" or "planned activation <date>", or "" when no
// plan is set. Upstream shows removal for status "deleted" and activation
// for any other status.
func (m ConnectionMarkers) PlannedChangeLabel() string {
	if m.PlannedStatus == "" && m.PlannedDate == "" {
		return ""
	}
	label := "planned activation"
	if m.PlannedStatus == "deleted" {
		label = "planned removal"
	}
	return strings.TrimSpace(label + " " + m.PlannedDate)
}
