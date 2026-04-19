package parity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
	parityfix "github.com/dotwaffle/peeringdb-plus/internal/testutil/parity"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// newTestServer wraps the canonical pdbcompat handler in an
// httptest.Server. It mirrors cmd/peeringdb-plus/main.go's wiring
// (NewHandler + Register) without the production middleware chain so
// parity failures are localised to the pdbcompat layer rather than to
// CSP/CORS/gzip/recovery surfaces. The middleware chain is exercised
// elsewhere (cmd/peeringdb-plus/*_test.go); duplicating it here would
// only obscure regression signals.
//
// budget=0 disables the Phase 71 pre-flight CheckBudget gate; tests
// that exercise the 413 path pass a non-zero budget explicitly via
// newTestServerWithBudget.
//
// Accepts testing.TB so bench_test.go can reuse the same server setup;
// *testing.T and *testing.B both satisfy the interface.
func newTestServer(t testing.TB, c *ent.Client) *httptest.Server {
	t.Helper()
	return newTestServerWithBudget(t, c, 0)
}

// newTestServerWithBudget mirrors newTestServer but exposes the
// per-response memory budget knob. Used by limit_test.go to drive the
// CheckBudget 413 path with a deliberately tiny budget.
func newTestServerWithBudget(t testing.TB, c *ent.Client, budget int64) *httptest.Server {
	t.Helper()
	h := pdbcompat.NewHandler(c, budget)
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// httpGet does a GET against srv and returns (status, body). Transport
// errors fail the test via t.Fatal — they indicate harness/server
// breakage, not behavioural regression.
func httpGet(t testing.TB, srv *httptest.Server, path string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("httpGet: build request for %s: %v", path, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("httpGet: GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("httpGet: read body from %s: %v", path, err)
	}
	return resp.StatusCode, body
}

// envelope is the on-the-wire {"meta": ..., "data": [...]} shape.
type envelope struct {
	Meta json.RawMessage   `json:"meta"`
	Data []json.RawMessage `json:"data"`
}

// decodeDataArray decodes the {"data":[...]} envelope into a slice of
// objects. Each element is a generic map so individual subtests can
// pluck out the field they care about (id, name, asn, ...) without
// per-test struct definitions.
func decodeDataArray(t testing.TB, body []byte) []map[string]any {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decodeDataArray: unmarshal envelope: %v\nbody=%s", err, string(body))
	}
	out := make([]map[string]any, 0, len(env.Data))
	for i, raw := range env.Data {
		var row map[string]any
		if err := json.Unmarshal(raw, &row); err != nil {
			t.Fatalf("decodeDataArray: row[%d]: %v\nraw=%s", i, err, string(raw))
		}
		out = append(out, row)
	}
	return out
}

// extractIDs decodes data[].id as ints. Missing or non-numeric ids
// fail the test — they indicate a broken serializer, which is itself
// a regression worth catching.
func extractIDs(t testing.TB, body []byte) []int {
	t.Helper()
	rows := decodeDataArray(t, body)
	ids := make([]int, 0, len(rows))
	for i, row := range rows {
		raw, ok := row["id"]
		if !ok {
			t.Fatalf("extractIDs: row[%d] missing id: %+v", i, row)
		}
		f, ok := raw.(float64)
		if !ok {
			t.Fatalf("extractIDs: row[%d].id = %T(%v), want number", i, raw, raw)
		}
		ids = append(ids, int(f))
	}
	return ids
}

// problem is the budget-exceeded RFC 9457 problem-detail body shape
// (mirrors pdbcompat.budgetProblemBody / WriteBudgetProblem). Only the
// fields parity tests assert on are surfaced here.
type problem struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Status      int    `json:"status"`
	Detail      string `json:"detail"`
	Instance    string `json:"instance,omitempty"`
	MaxRows     int    `json:"max_rows"`
	BudgetBytes int64  `json:"budget_bytes"`
}

// mustDecodeProblem decodes an application/problem+json body. Used by
// limit_test's 413 case. Failure to decode is a hard error — it
// signals a serializer regression, not a behavioural one.
func mustDecodeProblem(t testing.TB, body []byte) problem {
	t.Helper()
	var p problem
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("mustDecodeProblem: %v\nbody=%s", err, string(body))
	}
	return p
}

// unquote strips a single pair of outer double quotes from s. Mirrors
// the helper in internal/testutil/parity/fixtures_test.go — Fixture
// values are stored in their Python-source-form quoted shape, so
// consumers strip the quotes before assigning typed values.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// fkRefs parses a __fk marker like "\"org:200001,ix:200007\"" into a
// map of entity → upstream-fixture-ID. Used during the FK-resolution
// pass of seedFixtures to look up persisted parent IDs.
func fkRefs(raw string) map[string]int {
	out := make(map[string]int)
	for _, part := range strings.Split(unquote(raw), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			continue
		}
		out[strings.TrimSpace(k)] = id
	}
	return out
}

// seedFixtures realises a category fixture slice into the ent client.
// Two passes:
//
//  1. Seed fixtures with no __fk dependency (organisations and other
//     leaf entities). Records persisted IDs in idMap.
//  2. Seed fixtures whose Fields contains __fk; resolves each FK
//     reference against idMap and links the parent edges.
//
// Unknown Entity strings are skipped with t.Logf — the harness aims to
// be permissive against the noisy fixture set so individual subtests
// can rely on the rows they care about being present without the whole
// seed failing on an unrecognised entity. Subtests that need an exact
// row count seed inline.
//
// The seeded `created` and `updated` timestamps default to a fixed
// epoch unless the fixture supplies its own; the (-updated, -created)
// default ordering assertions in ordering_test.go use an inline seed
// path that controls timestamps explicitly, so this helper's defaults
// are only material to status_test.go's STATUS-01..05 subtests where
// the matrix outcome is independent of relative timestamps.
func seedFixtures(t testing.TB, c *ent.Client, fixtures []parityfix.Fixture) {
	t.Helper()
	ctx := t.Context()
	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	// idMap[entity][fixtureID] = persistedID. With deterministic
	// fixture IDs (Phase 72 D-04) the persisted ID equals fixtureID,
	// so the map is mostly tautological — keeping it keyed for clarity
	// and to anchor any future fixture-ID rewrites.
	idMap := make(map[string]map[int]int)
	record := func(entity string, id int) {
		if idMap[entity] == nil {
			idMap[entity] = make(map[int]int)
		}
		idMap[entity][id] = id
	}

	// Pass 1: FK-free fixtures.
	for _, fx := range fixtures {
		if _, hasFK := fx.Fields["__fk"]; hasFK {
			continue
		}
		if persistFixture(ctx, t, c, fx, idMap, t0) {
			record(fx.Entity, fx.ID)
		}
	}
	// Pass 2: FK-dependent fixtures.
	for _, fx := range fixtures {
		if _, hasFK := fx.Fields["__fk"]; !hasFK {
			continue
		}
		if persistFixture(ctx, t, c, fx, idMap, t0) {
			record(fx.Entity, fx.ID)
		}
	}
}

// persistFixture creates a single fixture row in the ent client.
// Returns true on successful insert; false (with t.Logf, never
// t.Fatal) when the entity is unrecognised or the row's noisy field
// values reject schema validation. The harness deliberately tolerates
// partial seeds so individual category tests can still rely on the
// clean rows present in their fixture slice.
func persistFixture(
	ctx context.Context,
	t testing.TB,
	c *ent.Client,
	fx parityfix.Fixture,
	idMap map[string]map[int]int,
	t0 time.Time,
) bool {
	t.Helper()

	name := unquote(fx.Fields["name"])
	status := unquote(fx.Fields["status"])
	if status == "" {
		status = "ok"
	}

	// Reject obviously-noisy values (Python source artefacts) — the
	// fixtures.go ports preserve byte-identical Python source which
	// occasionally embeds commas / **kwargs. Fixtures with such values
	// are not seedable; subtests that need them seed inline.
	if strings.ContainsAny(name, ",*") {
		return false
	}
	if strings.ContainsAny(status, ",*") {
		return false
	}

	fks := fkRefs(fx.Fields["__fk"])

	switch fx.Entity {
	case "org":
		if name == "" {
			name = fmt.Sprintf("FixtureOrg-%d", fx.ID)
		}
		if _, err := c.Organization.Create().
			SetID(fx.ID).
			SetName(name).
			SetNameFold(unifold.Fold(name)).
			SetStatus(status).
			SetCreated(t0).
			SetUpdated(t0).
			Save(ctx); err != nil {
			t.Logf("persistFixture org id=%d: %v (skipped)", fx.ID, err)
			return false
		}
		return true

	case "net":
		asn := atoiOrZero(fx.Fields["asn"])
		if asn <= 0 {
			asn = 4_200_000_000 + fx.ID
		}
		if name == "" {
			name = fmt.Sprintf("FixtureNet-%d", fx.ID)
		}
		b := c.Network.Create().
			SetID(fx.ID).
			SetName(name).
			SetNameFold(unifold.Fold(name)).
			SetAsn(asn).
			SetStatus(status).
			SetCreated(t0).
			SetUpdated(t0)
		if orgID, ok := fks["org"]; ok {
			if _, persisted := idMap["org"][orgID]; persisted {
				b = b.SetOrgID(orgID)
			}
		}
		if _, err := b.Save(ctx); err != nil {
			t.Logf("persistFixture net id=%d: %v (skipped)", fx.ID, err)
			return false
		}
		return true

	case "fac":
		if name == "" {
			name = fmt.Sprintf("FixtureFac-%d", fx.ID)
		}
		b := c.Facility.Create().
			SetID(fx.ID).
			SetName(name).
			SetNameFold(unifold.Fold(name)).
			SetStatus(status).
			SetCity("TestCity").
			SetCountry("DE").
			SetCreated(t0).
			SetUpdated(t0)
		if orgID, ok := fks["org"]; ok {
			if _, persisted := idMap["org"][orgID]; persisted {
				b = b.SetOrgID(orgID)
			}
		}
		if _, err := b.Save(ctx); err != nil {
			t.Logf("persistFixture fac id=%d: %v (skipped)", fx.ID, err)
			return false
		}
		return true

	case "ix":
		if name == "" {
			name = fmt.Sprintf("FixtureIX-%d", fx.ID)
		}
		b := c.InternetExchange.Create().
			SetID(fx.ID).
			SetName(name).
			SetNameFold(unifold.Fold(name)).
			SetStatus(status).
			SetCity("TestCity").
			SetCountry("DE").
			SetRegionContinent("Europe").
			SetMedia("Ethernet").
			SetCreated(t0).
			SetUpdated(t0)
		if orgID, ok := fks["org"]; ok {
			if _, persisted := idMap["org"][orgID]; persisted {
				b = b.SetOrgID(orgID)
			}
		}
		if _, err := b.Save(ctx); err != nil {
			t.Logf("persistFixture ix id=%d: %v (skipped)", fx.ID, err)
			return false
		}
		return true

	case "ixlan":
		b := c.IxLan.Create().
			SetID(fx.ID).
			SetName(name).
			SetCreated(t0).
			SetUpdated(t0)
		if ixID, ok := fks["ix"]; ok {
			if _, persisted := idMap["ix"][ixID]; persisted {
				b = b.SetIxID(ixID)
			}
		}
		if _, err := b.Save(ctx); err != nil {
			t.Logf("persistFixture ixlan id=%d: %v (skipped)", fx.ID, err)
			return false
		}
		return true

	case "ixfac":
		ixID, ok1 := fks["ix"]
		facID, ok2 := fks["fac"]
		if !ok1 || !ok2 {
			return false
		}
		if _, p1 := idMap["ix"][ixID]; !p1 {
			return false
		}
		if _, p2 := idMap["fac"][facID]; !p2 {
			return false
		}
		if _, err := c.IxFacility.Create().
			SetID(fx.ID).
			SetIxID(ixID).
			SetFacID(facID).
			SetStatus(status).
			SetCreated(t0).
			SetUpdated(t0).
			Save(ctx); err != nil {
			t.Logf("persistFixture ixfac id=%d: %v (skipped)", fx.ID, err)
			return false
		}
		return true

	case "campus":
		if name == "" {
			name = fmt.Sprintf("FixtureCampus-%d", fx.ID)
		}
		// Campus requires an org parent. Reuse fks if present; else
		// fall back to a synthesised campus parent created on first
		// use. Skipping the row would lose UNICODE-01 coverage on
		// campus, so prefer best-effort linking.
		var orgID int
		if id, ok := fks["org"]; ok {
			if _, p := idMap["org"][id]; p {
				orgID = id
			}
		}
		if orgID == 0 {
			orgID = ensureFixtureOrgParent(ctx, t, c, idMap, t0)
			if orgID == 0 {
				return false
			}
		}
		if _, err := c.Campus.Create().
			SetID(fx.ID).
			SetName(name).
			SetNameFold(unifold.Fold(name)).
			SetOrgID(orgID).
			SetStatus(status).
			SetCity("TestCity").
			SetCountry("DE").
			SetCreated(t0).
			SetUpdated(t0).
			Save(ctx); err != nil {
			t.Logf("persistFixture campus id=%d: %v (skipped)", fx.ID, err)
			return false
		}
		return true

	case "carrier":
		if name == "" {
			name = fmt.Sprintf("FixtureCarrier-%d", fx.ID)
		}
		var orgID int
		if id, ok := fks["org"]; ok {
			if _, p := idMap["org"][id]; p {
				orgID = id
			}
		}
		if orgID == 0 {
			orgID = ensureFixtureOrgParent(ctx, t, c, idMap, t0)
			if orgID == 0 {
				return false
			}
		}
		if _, err := c.Carrier.Create().
			SetID(fx.ID).
			SetName(name).
			SetNameFold(unifold.Fold(name)).
			SetOrgID(orgID).
			SetStatus(status).
			SetCreated(t0).
			SetUpdated(t0).
			Save(ctx); err != nil {
			t.Logf("persistFixture carrier id=%d: %v (skipped)", fx.ID, err)
			return false
		}
		return true

	case "carrierfac":
		// CarrierFacility is a junction; without explicit __fk to a
		// carrier and fac the row is meaningless. Skip silently.
		return false

	default:
		// Unrecognised entity — skip with a debug-style log so a
		// future entity addition is surfaced without breaking the
		// suite.
		return false
	}
}

// fixtureOrgParentID is the synthesised parent-org ID used by orphan
// campus/carrier fixtures. Far above any real fixture range to avoid
// collision.
const fixtureOrgParentID = 909001

// ensureFixtureOrgParent creates (or returns the cached) synthesised
// parent organisation used to link orphan campus / carrier fixtures
// whose __fk marker is absent. Returns 0 on failure (which propagates
// as "skip this fixture" up the persist chain).
func ensureFixtureOrgParent(
	ctx context.Context,
	t testing.TB,
	c *ent.Client,
	idMap map[string]map[int]int,
	t0 time.Time,
) int {
	t.Helper()
	if idMap["org"] != nil {
		if _, ok := idMap["org"][fixtureOrgParentID]; ok {
			return fixtureOrgParentID
		}
	}
	if _, err := c.Organization.Create().
		SetID(fixtureOrgParentID).
		SetName("ParityFixtureParent").
		SetNameFold(unifold.Fold("ParityFixtureParent")).
		SetStatus("ok").
		SetCreated(t0).
		SetUpdated(t0).
		Save(ctx); err != nil {
		t.Logf("ensureFixtureOrgParent: %v", err)
		return 0
	}
	if idMap["org"] == nil {
		idMap["org"] = make(map[int]int)
	}
	idMap["org"][fixtureOrgParentID] = fixtureOrgParentID
	return fixtureOrgParentID
}

// atoiOrZero parses a possibly-quoted integer string; returns 0 on
// any failure. Used to coerce Fields["asn"] (stored as Python source
// like "4300000000") into a typed int.
func atoiOrZero(raw string) int {
	v := strings.Trim(unquote(raw), `" `)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

