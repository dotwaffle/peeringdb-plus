// Package testdata provides bench-only fixture helpers used by the Phase
// 70 traversal benchmarks (internal/pdbcompat/bench_traversal_test.go).
//
// Go's build system excludes `testdata/` directories from package
// discovery via `./...`, but explicit imports still compile. The helpers
// here are imported from a //go:build bench file only; they do not run
// during the normal `go test ./...` path.
//
// Determinism: seeding is driven by math/rand/v2's PCG source with a
// fixed seed pair so repeated invocations produce byte-identical row
// sets. This is required for benchstat n=6 comparisons to be stable.
package testdata

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// BenchSeedShape captures the row counts for each entity type in the
// traversal bench fixture. Ratios mirror real PeeringDB cardinality
// approximately: ~10k networks per ~20k facilities per ~1k ixes.
type BenchSeedShape struct {
	Orgs       int
	Networks   int
	Facilities int
	Ixes       int
	IxLans     int // 1:1 with Ixes
	NetIxLans  int // ~3 per network
}

// Default10k returns the 10k-row bench fixture shape used by
// BenchmarkTraversal_* entries. Total ~63k rows across six entity
// types. In-memory SQLite footprint stays under ~100 MB and seed time
// is a one-time cost absorbed before b.ResetTimer.
func Default10k() BenchSeedShape {
	return BenchSeedShape{
		Orgs:       1000,
		Networks:   10000,
		Facilities: 20000,
		Ixes:       1000,
		IxLans:     1000,
		NetIxLans:  30000,
	}
}

// Seed populates client with the given shape. Deterministic via a
// fixed RNG seed pair; repeated invocations produce byte-identical row
// sets for benchstat n=6 stability.
//
// Accepts testing.TB so both *testing.B (from BenchmarkTraversal_*) and
// *testing.T (from TestBenchTraversal_D07_Ceiling) can use it.
func Seed(tb testing.TB, client *ent.Client, shape BenchSeedShape) {
	tb.Helper()
	ctx := context.Background()
	// Fixed PCG seed pair — math/rand/v2 guarantees cross-version
	// determinism for PCG (unlike the legacy math/rand source).
	r := rand.New(rand.NewPCG(0xCAFEBABE, 0xDEADBEEF))
	now := time.Now().UTC()

	// Phase 1: Organizations. Plain Create loop is fine at 1k; stays
	// simple so the test is easy to read.
	for i := 1; i <= shape.Orgs; i++ {
		name := fmt.Sprintf("BenchOrg-%06d", i)
		_, err := client.Organization.Create().
			SetID(i).
			SetName(name).
			SetNameFold(unifold.Fold(name)).
			SetStatus("ok").
			SetCreated(now).
			SetUpdated(now).
			Save(ctx)
		if err != nil {
			tb.Fatalf("seed org %d: %v", i, err)
		}
	}

	// Phase 2: Networks in bulk. CreateBulk is much faster than a
	// Create loop at 10k scale and each row has ~30 columns — keep
	// chunks under SQLite's 32,766 variable cap.
	const netChunk = 500
	for start := 0; start < shape.Networks; start += netChunk {
		end := start + netChunk
		if end > shape.Networks {
			end = shape.Networks
		}
		builders := make([]*ent.NetworkCreate, 0, end-start)
		for i := start; i < end; i++ {
			id := i + 1
			orgID := 1 + r.IntN(shape.Orgs)
			name := fmt.Sprintf("BenchNet-%06d", id)
			builders = append(builders, client.Network.Create().
				SetID(id).
				SetName(name).
				SetNameFold(unifold.Fold(name)).
				SetAsn(64000+id).
				SetOrgID(orgID).
				// Required bool/string fields — zero-values are fine
				// for bench purposes; we just need a valid row.
				SetAllowIxpUpdate(false).
				SetInfoIpv6(false).
				SetInfoMulticast(false).
				SetInfoNeverViaRouteServers(false).
				SetInfoUnicast(false).
				SetPolicyRatio(false).
				SetStatus("ok").
				SetCreated(now).
				SetUpdated(now))
		}
		if err := client.Network.CreateBulk(builders...).Exec(ctx); err != nil {
			tb.Fatalf("bulk seed networks [%d,%d): %v", start, end, err)
		}
	}

	// Phase 3: Facilities in bulk.
	const facChunk = 500
	for start := 0; start < shape.Facilities; start += facChunk {
		end := start + facChunk
		if end > shape.Facilities {
			end = shape.Facilities
		}
		builders := make([]*ent.FacilityCreate, 0, end-start)
		for i := start; i < end; i++ {
			id := i + 1
			orgID := 1 + r.IntN(shape.Orgs)
			name := fmt.Sprintf("BenchFac-%06d", id)
			builders = append(builders, client.Facility.Create().
				SetID(id).
				SetName(name).
				SetNameFold(unifold.Fold(name)).
				SetOrgID(orgID).
				SetStatus("ok").
				SetCreated(now).
				SetUpdated(now))
		}
		if err := client.Facility.CreateBulk(builders...).Exec(ctx); err != nil {
			tb.Fatalf("bulk seed facilities [%d,%d): %v", start, end, err)
		}
	}

	// Phase 4: Ixes + 1:1 IxLans. fac_count populated with random 0-19
	// so the __fac_count__gt=0 predicate has a non-trivial match set
	// (~95% of rows).
	for i := 1; i <= shape.Ixes; i++ {
		orgID := 1 + r.IntN(shape.Orgs)
		name := fmt.Sprintf("BenchIX-%06d", i)
		_, err := client.InternetExchange.Create().
			SetID(i).
			SetName(name).
			SetNameFold(unifold.Fold(name)).
			SetOrgID(orgID).
			SetFacCount(r.IntN(20)).
			// Required bools for InternetExchange.
			SetProtoIpv6(false).
			SetProtoMulticast(false).
			SetProtoUnicast(false).
			SetStatus("ok").
			SetCreated(now).
			SetUpdated(now).
			Save(ctx)
		if err != nil {
			tb.Fatalf("seed ix %d: %v", i, err)
		}
		if _, err = client.IxLan.Create().
			SetID(i).
			SetIxID(i).
			SetInternetExchangeID(i).
			// Required bools for IxLan.
			SetDot1qSupport(false).
			SetIxfIxpImportEnabled(false).
			SetStatus("ok").
			SetCreated(now).
			SetUpdated(now).
			Save(ctx); err != nil {
			tb.Fatalf("seed ixlan %d: %v", i, err)
		}
	}

	// Phase 5: NetworkIxLans — 3 per network, random ixlan per row.
	// id starts at 1; net_id field and ixlan_id field populated along
	// with their edge counterparts so ent's FK integrity holds.
	const nixChunk = 500
	netIxLans := shape.NetIxLans
	if want := shape.Networks * 3; netIxLans > want {
		netIxLans = want
	}
	id := 1
	for start := 0; start < netIxLans; start += nixChunk {
		end := start + nixChunk
		if end > netIxLans {
			end = netIxLans
		}
		builders := make([]*ent.NetworkIxLanCreate, 0, end-start)
		for i := start; i < end; i++ {
			netID := (i / 3) + 1
			ixlanID := 1 + r.IntN(shape.IxLans)
			builders = append(builders, client.NetworkIxLan.Create().
				SetID(id).
				SetNetID(netID).
				SetNetworkID(netID).
				SetIxlanID(ixlanID).
				SetIxLanID(ixlanID).
				SetIxID(ixlanID).
				SetAsn(64000+netID).
				// Required bools + speed on NetworkIxLan.
				SetBfdSupport(false).
				SetIsRsPeer(false).
				SetOperational(true).
				SetSpeed(10000).
				SetStatus("ok").
				SetCreated(now).
				SetUpdated(now))
			id++
		}
		if err := client.NetworkIxLan.CreateBulk(builders...).Exec(ctx); err != nil {
			tb.Fatalf("bulk seed netixlans [%d,%d): %v", start, end, err)
		}
	}
}
