package main

import (
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// TestRegistry_AllReturnsAtLeast100 asserts the registry covers the
// 13 entity × 5 surface inventory at >= 100 endpoints (loose pin so
// future shape additions don't break the assertion).
func TestRegistry_AllReturnsAtLeast100(t *testing.T) {
	t.Parallel()

	eps := registryAll()
	if got := len(eps); got < 100 {
		t.Fatalf("registry.All() returned %d endpoints, want >= 100", got)
	}

	for i, ep := range eps {
		if ep.Surface == "" {
			t.Errorf("endpoint[%d] %+v has empty Surface", i, ep)
		}
		if ep.Method == "" {
			t.Errorf("endpoint[%d] %+v has empty Method", i, ep)
		}
		if ep.Path == "" {
			t.Errorf("endpoint[%d] %+v has empty Path", i, ep)
		}
	}
}

// TestRegistry_PerEntitySurfaceCoverage asserts every entity-bearing
// type has at least one endpoint per non-UI surface.
func TestRegistry_PerEntitySurfaceCoverage(t *testing.T) {
	t.Parallel()

	types := []string{
		peeringdb.TypeOrg, peeringdb.TypeNet, peeringdb.TypeFac, peeringdb.TypeIX,
		peeringdb.TypePoc, peeringdb.TypeIXLan, peeringdb.TypeIXPfx,
		peeringdb.TypeNetIXLan, peeringdb.TypeNetFac, peeringdb.TypeIXFac,
		peeringdb.TypeCarrier, peeringdb.TypeCarrierFac, peeringdb.TypeCampus,
	}
	surfaces := []Surface{
		SurfacePdbCompat, SurfaceEntRest, SurfaceGraphQL, SurfaceConnectRPC,
	}
	eps := registryAll()
	for _, typ := range types {
		for _, surf := range surfaces {
			found := false
			for _, ep := range eps {
				if ep.EntityType == typ && ep.Surface == surf {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("missing %s endpoint for entity %q", surf, typ)
			}
		}
	}
}

// TestRegistry_PdbCompatNetURLs asserts the pdbcompat shapes for the
// net entity look like the upstream PeeringDB URL convention.
func TestRegistry_PdbCompatNetURLs(t *testing.T) {
	t.Parallel()

	eps := registryAll()
	var sawListDefault, sawAsnFilter bool
	for _, ep := range eps {
		if ep.Surface != SurfacePdbCompat || ep.EntityType != peeringdb.TypeNet {
			continue
		}
		switch {
		case ep.Path == "/api/net?limit=10":
			sawListDefault = true
		case strings.Contains(ep.Path, "/api/net?asn=15169"):
			sawAsnFilter = true
		}
	}
	if !sawListDefault {
		t.Errorf("missing pdbcompat list-default for net (/api/net?limit=10)")
	}
	if !sawAsnFilter {
		t.Errorf("missing pdbcompat list-filtered for net (/api/net?asn=15169)")
	}

	// Folded contains filter for org.
	sawFolded := false
	for _, ep := range eps {
		if ep.Surface == SurfacePdbCompat && ep.EntityType == peeringdb.TypeOrg &&
			strings.Contains(ep.Path, "name__contains=") {
			sawFolded = true
			break
		}
	}
	if !sawFolded {
		t.Errorf("missing pdbcompat folded contains filter for org (name__contains)")
	}
}

// TestRegistry_ConnectRPCNetworkGet asserts the ConnectRPC endpoint
// for net Get matches the proto service URL convention.
func TestRegistry_ConnectRPCNetworkGet(t *testing.T) {
	t.Parallel()

	eps := registryAll()
	for _, ep := range eps {
		if ep.Surface != SurfaceConnectRPC || ep.EntityType != peeringdb.TypeNet {
			continue
		}
		if ep.Path != "/peeringdb.v1.NetworkService/GetNetwork" {
			continue
		}
		if ep.Method != "POST" {
			t.Errorf("expected POST, got %q", ep.Method)
		}
		if string(ep.Body) != `{"id":1}` {
			t.Errorf("expected body {\"id\":1}, got %q", string(ep.Body))
		}
		return
	}
	t.Errorf("no connectrpc rpc-get endpoint for net (/peeringdb.v1.NetworkService/GetNetwork)")
}

// TestRegistry_RESTPluralCoverage asserts the REST plural map matches
// the openapi.json values for all 13 entity constants and that no
// entity is missing.
func TestRegistry_RESTPluralCoverage(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		peeringdb.TypeOrg:        "organizations",
		peeringdb.TypeNet:        "networks",
		peeringdb.TypeFac:        "facilities",
		peeringdb.TypeIX:         "internet-exchanges",
		peeringdb.TypePoc:        "pocs",
		peeringdb.TypeIXLan:      "ix-lans",
		peeringdb.TypeIXPfx:      "ix-prefixes",
		peeringdb.TypeNetIXLan:   "network-ix-lans",
		peeringdb.TypeNetFac:     "network-facilities",
		peeringdb.TypeIXFac:      "ix-facilities",
		peeringdb.TypeCarrier:    "carriers",
		peeringdb.TypeCarrierFac: "carrier-facilities",
		peeringdb.TypeCampus:     "campuses",
	}
	if len(restPlurals) != len(want) {
		t.Fatalf("restPlurals has %d entries, want %d", len(restPlurals), len(want))
	}
	for k, v := range want {
		got, ok := restPlurals[k]
		if !ok {
			t.Errorf("restPlurals missing key %q", k)
			continue
		}
		if got != v {
			t.Errorf("restPlurals[%q] = %q, want %q", k, got, v)
		}
	}
}

// TestRegistry_WebUIThreeRoutes asserts the surface-wide UI routes
// are in the registry.
func TestRegistry_WebUIThreeRoutes(t *testing.T) {
	t.Parallel()

	want := map[string]bool{
		"/ui/":           false,
		"/ui/about":      false,
		"/ui/asn/15169":  false,
	}
	for _, ep := range registryAll() {
		if ep.Surface != SurfaceWebUI {
			continue
		}
		if _, ok := want[ep.Path]; ok {
			want[ep.Path] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("missing webui endpoint %q", path)
		}
	}
}
