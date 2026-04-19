package main

import (
	"fmt"
)

// traversalRingOrgID anchors the synthesised TRAVERSAL ring topology.
// Test consumers query `org=200001` (or `org__name="TraversalOrg-Root"`)
// expecting all related child rows to be reachable via the 1-hop
// allowlist (Phase 70 Path A) and 2-hop introspection (Path B).
//
// 200000+ chosen above all other category ranges (IN bulk peaks at
// 105000, sentinel at 999999, status/limit synth peaks ~30k) so
// cross-category row IDs never collide when fixtures are loaded
// together.
const traversalRingOrgID = 200001

// traversalUpstream2HopLine cites pdb_api_test.py:2340 — upstream's
// `ixlan__ix__fac_count__gt=0` 2-hop assertion. The synthesised
// ring fixtures inherit this citation so auditors land on the
// upstream behaviour they verify.
const traversalUpstream2HopLine = 2340

// traversalUpstream1HopLine cites pdb_api_test.py:5081 — upstream's
// `ixlan__ix__id=N` 1-hop assertion.
const traversalUpstream1HopLine = 5081

// traversalUpstreamSilentIgnoreLine cites the upstream behaviour
// silent-ignore is supposed to mirror — Phase 70 D-04 enshrined the
// hard-2-hop cap as upstream-equivalent silent-ignore for 3+ hops.
// No literal upstream line exercises this; cite line 1 sentinel.
const traversalUpstreamSilentIgnoreLine = 1

// parseTraversal emits the TRAVERSAL-category fixture set: a ring
// topology rooted at one Organization (id=200001) wired to networks,
// facilities, an InternetExchange, ixlan, and ixfac children.
// Test consumers seed the topology and assert:
//   - TRAVERSAL-01: 1-hop `ix__name=...` resolves
//   - TRAVERSAL-02: 1-hop `ixlan__ix__id=N` resolves (2-segment via Path A)
//   - TRAVERSAL-03: 2-hop `ixlan__ix__fac_count__gt=0` resolves
//   - TRAVERSAL-04: 3+-hop chain silent-ignored (HTTP 200 + unfiltered)
//
// Plan 72-03 must_have: ≥1 fixture tagged 2-hop (Fields["__hop"]="2")
// AND ≥1 silent-ignore fixture (Fields["__expected_outcome"]="silent-ignore").
//
// The Fields type is map[string]string (preserved byte-identical
// from plans 72-01/02), so __hop is encoded as the literal string
// "2"; the test consumer parses it via strconv.Atoi.
//
// Note on FK encoding: Plan 72-03 mentions Fields["__fk"]=map[string]int
// but Fixture.Fields is map[string]string. We encode FKs as
// "<entity>:<id>" string pairs (e.g. "org:200001"). Plan 72-04's
// seeding helper splits and resolves at row creation time.
func parseTraversal(srcBytes []byte) []Fixture {
	_ = srcBytes // tool API symmetry; traversal fixtures are pure synthesis.

	out := []Fixture{
		// Root organisation — anchor of the ring.
		{
			Entity: "org",
			ID:     traversalRingOrgID,
			Fields: map[string]string{
				"status": `"ok"`,
				"name":   `"TraversalOrg-Root"`,
				"__hop":  `"0"`,
			},
			Upstream: lineCitation(traversalUpstream1HopLine),
		},

		// 3 child networks (1-hop via net.org → org.id).
		{
			Entity: "net",
			ID:     200002,
			Fields: map[string]string{
				"status": `"ok"`,
				"name":   `"TraversalNet-A"`,
				"asn":    "65401",
				"__fk":   `"org:200001"`,
				"__hop":  `"1"`,
			},
			Upstream: lineCitation(traversalUpstream1HopLine),
		},
		{
			Entity: "net",
			ID:     200003,
			Fields: map[string]string{
				"status": `"ok"`,
				"name":   `"TraversalNet-B"`,
				"asn":    "65402",
				"__fk":   `"org:200001"`,
				"__hop":  `"1"`,
			},
			Upstream: lineCitation(traversalUpstream1HopLine),
		},
		{
			Entity: "net",
			ID:     200004,
			Fields: map[string]string{
				"status": `"ok"`,
				"name":   `"TraversalNet-C"`,
				"asn":    "65403",
				"__fk":   `"org:200001"`,
				"__hop":  `"1"`,
			},
			Upstream: lineCitation(traversalUpstream1HopLine),
		},

		// 2 facilities (1-hop via fac.org → org.id).
		{
			Entity: "fac",
			ID:     200005,
			Fields: map[string]string{
				"status": `"ok"`,
				"name":   `"TraversalFac-A"`,
				"__fk":   `"org:200001"`,
				"__hop":  `"1"`,
			},
			Upstream: lineCitation(traversalUpstream1HopLine),
		},
		{
			Entity: "fac",
			ID:     200006,
			Fields: map[string]string{
				"status": `"ok"`,
				"name":   `"TraversalFac-B"`,
				"__fk":   `"org:200001"`,
				"__hop":  `"1"`,
			},
			Upstream: lineCitation(traversalUpstream1HopLine),
		},

		// 1 InternetExchange (1-hop via ix.org → org.id).
		// fac_count varies for the upstream-cited fac_count__gt=0
		// 2-hop assertion (line 2340/2348).
		{
			Entity: "ix",
			ID:     200007,
			Fields: map[string]string{
				"status":    `"ok"`,
				"name":      `"TraversalIX-Root"`,
				"fac_count": "5",
				"__fk":      `"org:200001"`,
				"__hop":     `"1"`,
			},
			Upstream: lineCitation(traversalUpstream1HopLine),
		},

		// 1 ixlan (2-hop via ixlan.ix.org). The __hop=2 marker is
		// load-bearing for TestFixtures_TraversalSanity.
		{
			Entity: "ixlan",
			ID:     200008,
			Fields: map[string]string{
				"status": `"ok"`,
				"name":   `"TraversalIxlan-Root"`,
				"__fk":   `"ix:200007"`,
				"__hop":  `"2"`,
			},
			Upstream: lineCitation(traversalUpstream2HopLine),
		},

		// 2 ixfacs (2-hop via ixfac.ix.org → org.id, OR via
		// ixfac.fac.org → org.id; either Path A or Path B resolves).
		{
			Entity: "ixfac",
			ID:     200009,
			Fields: map[string]string{
				"status": `"ok"`,
				"__fk":   `"ix:200007,fac:200005"`,
				"__hop":  `"2"`,
			},
			Upstream: lineCitation(traversalUpstream2HopLine),
		},
		{
			Entity: "ixfac",
			ID:     200010,
			Fields: map[string]string{
				"status": `"ok"`,
				"__fk":   `"ix:200007,fac:200006"`,
				"__hop":  `"2"`,
			},
			Upstream: lineCitation(traversalUpstream2HopLine),
		},

		// fac_count variation rows: same IX ring shape with
		// fac_count ∈ {0, 1, 10} so the upstream-cited
		// fac_count__gt=0 / __gt=1 / __gt=5 boundary assertions
		// (pdb_api_test.py:2340/2348) all have hits.
		{
			Entity: "ix",
			ID:     200011,
			Fields: map[string]string{
				"status":    `"ok"`,
				"name":      `"TraversalIX-FacCount0"`,
				"fac_count": "0",
				"__fk":      `"org:200001"`,
				"__hop":     `"1"`,
			},
			Upstream: lineCitation(traversalUpstream2HopLine),
		},
		{
			Entity: "ix",
			ID:     200012,
			Fields: map[string]string{
				"status":    `"ok"`,
				"name":      `"TraversalIX-FacCount1"`,
				"fac_count": "1",
				"__fk":      `"org:200001"`,
				"__hop":     `"1"`,
			},
			Upstream: lineCitation(traversalUpstream2HopLine),
		},
		{
			Entity: "ix",
			ID:     200013,
			Fields: map[string]string{
				"status":    `"ok"`,
				"name":      `"TraversalIX-FacCount10"`,
				"fac_count": "10",
				"__fk":      `"org:200001"`,
				"__hop":     `"1"`,
			},
			Upstream: lineCitation(traversalUpstream2HopLine),
		},

		// Silent-ignore probe (TRAVERSAL-04). The filter key
		// `ixlan__ix__org__name=FOO` is a 3-hop chain — Phase 70
		// D-04 caps at 2 hops and silent-ignores the third segment.
		// Test consumer asserts HTTP 200 + unfiltered result on this
		// query, NOT a 400 error.
		{
			Entity: "ixlan",
			ID:     200014,
			Fields: map[string]string{
				"status":              `"ok"`,
				"name":                `"TraversalSilentIgnoreProbe"`,
				"__fk":                `"ix:200007"`,
				"__hop":               `"3"`,
				"__expected_filter":   `"ixlan__ix__org__name=FOO"`,
				"__expected_outcome":  `"silent-ignore"`,
			},
			Upstream: lineCitation(traversalUpstreamSilentIgnoreLine),
		},
	}

	// Touch fmt to keep the import even if all literal IDs above
	// are inlined later — defensive against dead-code analysers.
	_ = fmt.Sprintf

	return out
}
