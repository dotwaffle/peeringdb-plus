package parity

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_Unicode locks the Phase 69 unifold + shadow-column
// pipeline against future regression. The matrix covers UNICODE-01
// (`_fold` shadow-column routing) and UNICODE-02 (`__contains` /
// `__startswith` coerced to case-insensitive via the fold path).
//
// Cross-entity sweep: 4 of the 6 folded entities (network, facility,
// internet exchange, campus) are exercised across the subtests.
// Organization and Carrier are covered by the folded-fields set in
// Registry but their _fold pipeline is identical to net's; this
// suite asserts the user-visible behaviour, not per-entity wiring.
//
// upstream: peeringdb_server/rest.py:576 (unidecode pipeline)
// upstream: pdb_api_test.py:5133 (`fac unaccented` substring filter)
//
// `internal/unifold.Fold` is the reference folder used to BUILD
// expected matches against fixture-style inputs — never as the
// system-under-test. The SUT is the pdbcompat handler routing into
// the `_fold` shadow column.
func TestParity_Unicode(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	t.Run("UNICODE-01_net_name_contains_diacritic_matches_ascii", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:576 (unidecode folding before substring
		// match)
		// upstream: pdb_api_test.py:5133 (`fac unaccented` matched by
		// ASCII substring against folded shadow column)
		c := testutil.SetupClient(t)
		ctx := t.Context()
		// Seed a network whose canonical name has diacritics; the
		// _fold shadow column stores the unidecoded form. ?name__
		// contains=zurich (ASCII) must match via the fold column.
		name := "Zürich GmbH"
		if _, err := c.Network.Create().
			SetID(1).SetName(name).SetNameFold(unifold.Fold(name)).
			SetAsn(64501).SetStatus("ok").
			SetCreated(t0).SetUpdated(t0).
			Save(ctx); err != nil {
			t.Fatalf("seed net: %v", err)
		}
		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?name__contains=zurich")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		if len(ids) != 1 || ids[0] != 1 {
			t.Errorf("UNICODE-01 diacritic→ASCII fold: got %v, want [1]", ids)
		}
	})

	t.Run("UNICODE-01_fac_city_cjk_roundtrip", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:576 (CJK passes through unidecode unchanged
		// since it has no Latin transliteration; the fold column
		// stores the lowered form which equals the input).
		// synthesised: phase69-plan-04 (no upstream test corpus
		// exercises CJK substring matching against a folded shadow
		// column; the synthesised case locks our pipeline's CJK
		// passthrough behaviour).
		c := testutil.SetupClient(t)
		ctx := t.Context()
		org, err := c.Organization.Create().
			SetID(1).SetName("CJKOrg").SetNameFold(unifold.Fold("CJKOrg")).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed org: %v", err)
		}
		// UnicodeFixtures contains city=東京 entries at id 16029+; we
		// build a clean fac inline because the fixture rows lack the
		// FK link to a parent org.
		city := "東京"
		// fac.city is in FoldedFields per Registry. Set city plus the
		// fold shadow.
		if _, err := c.Facility.Create().
			SetID(100).SetName("Tokyo Edge").SetNameFold(unifold.Fold("Tokyo Edge")).
			SetCity(city).SetCityFold(unifold.Fold(city)).
			SetCountry("JP").SetOrgID(org.ID).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).
			Save(ctx); err != nil {
			t.Fatalf("seed fac: %v", err)
		}
		srv := newTestServer(t, c)
		// CJK substring on the folded city — both sides go through Fold.
		path := "/api/fac?city__contains=" + url.QueryEscape(city)
		status, body := httpGet(t, srv, path)
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		if len(ids) != 1 || ids[0] != 100 {
			t.Errorf("UNICODE-01 CJK roundtrip: got %v, want [100]", ids)
		}
	})

	t.Run("UNICODE-02_ix_name_startswith_coerced_case_insensitive", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:576 (`__startswith` is coerced to
		// `__istartswith` semantics via the fold pipeline — both sides
		// of the comparison are lowered before matching).
		// upstream: pdb_api_test.py:1479 (case-insensitive name prefix
		// search across the IX corpus).
		c := testutil.SetupClient(t)
		ctx := t.Context()
		org, err := c.Organization.Create().
			SetID(1).SetName("PrefixOrg").SetNameFold(unifold.Fold("PrefixOrg")).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed org: %v", err)
		}
		name := "AS-Zürich-IX"
		if _, err := c.InternetExchange.Create().
			SetID(50).SetName(name).SetNameFold(unifold.Fold(name)).
			SetCity("Zurich").SetCountry("CH").
			SetRegionContinent("Europe").SetMedia("Ethernet").
			SetOrgID(org.ID).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).
			Save(ctx); err != nil {
			t.Fatalf("seed ix: %v", err)
		}
		srv := newTestServer(t, c)
		// Lowercase ASCII prefix matches mixed-case diacritic original.
		status, body := httpGet(t, srv, "/api/ix?name__startswith=as-zurich")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		if len(ids) != 1 || ids[0] != 50 {
			t.Errorf("UNICODE-02 case+diacritic startswith: got %v, want [50]", ids)
		}
	})

	t.Run("UNICODE-01_combining_mark_NFKD_equivalent", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:576 (NFKD normalises combining marks; both
		// composed `Zürich` and decomposed `Zu\u0308rich` fold to
		// `zurich`).
		// synthesised: phase69-plan-04 (combining-mark equivalence is
		// not exercised by upstream's test corpus; lock our NFKD
		// pipeline's behaviour against future Unicode-version drift).
		c := testutil.SetupClient(t)
		ctx := t.Context()
		org, err := c.Organization.Create().
			SetID(1).SetName("CMOrg").SetNameFold(unifold.Fold("CMOrg")).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed org: %v", err)
		}
		// Decomposed form: Z u + combining diaeresis (U+0308) r i c h.
		decomposed := "Zu\u0308rich"
		if _, err := c.Campus.Create().
			SetID(20).SetName(decomposed).SetNameFold(unifold.Fold(decomposed)).
			SetOrgID(org.ID).SetCity("Zurich").SetCountry("CH").
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).
			Save(ctx); err != nil {
			t.Fatalf("seed campus: %v", err)
		}
		srv := newTestServer(t, c)
		// Composed query (Zürich, single code point ü = U+00FC) must
		// match the decomposed seed via NFKD equivalence in Fold().
		path := "/api/campus?name__contains=" + url.QueryEscape("Zürich")
		status, body := httpGet(t, srv, path)
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		ids := extractIDs(t, body)
		if len(ids) != 1 || ids[0] != 20 {
			t.Errorf("UNICODE-01 NFKD combining-mark: got %v, want [20]", ids)
		}
	})
}
