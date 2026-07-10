package parity

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_Unicode locks the unifold + shadow-column pipeline
// against future regression. The matrix covers `_fold` shadow-column
// routing and `__contains` / `__startswith` coerced to
// case-insensitive via the fold path.
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

	t.Run("net_name_contains_diacritic_matches_ascii", func(t *testing.T) {
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
			t.Errorf("diacritic→ASCII fold: got %v, want [1]", ids)
		}
	})

	t.Run("fac_city_cjk_roundtrip", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:576 (CJK passes through unidecode unchanged
		// since it has no Latin transliteration; the fold column
		// stores the lowered form which equals the input).
		// synthesised: no upstream test corpus exercises CJK substring
		// matching against a folded shadow column; this case locks our
		// pipeline's CJK passthrough behaviour.
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
			t.Errorf("CJK roundtrip: got %v, want [100]", ids)
		}
	})

	t.Run("ix_name_startswith_coerced_case_insensitive", func(t *testing.T) {
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
			t.Errorf("case+diacritic startswith: got %v, want [50]", ids)
		}
	})

	t.Run("combining_mark_NFKD_equivalent", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:576 (NFKD normalises combining marks; both
		// composed `Zürich` and decomposed `Zu\u0308rich` fold to
		// `zurich`).
		// synthesised: combining-mark equivalence is not exercised by
		// upstream's test corpus; lock our NFKD pipeline's behaviour
		// against future Unicode-version drift.
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
			t.Errorf("NFKD combining-mark: got %v, want [20]", ids)
		}
	})
}

// TestParity_Unicode_FoldWindow_DIVERGENCE locks the intentional divergence
// registered in docs/API.md § Known Divergences (fold-window row): upstream
// folds the query value via unidecode at request time (rest.py:576), so
// non-ASCII matching works the moment a row exists. This mirror instead
// matches against a sync-populated `_fold` shadow column — a row whose shadow
// column has not yet been (re)populated by a sync cycle is unreachable by a
// folded query until the next sync's OnConflict().UpdateNewValues() rewrites
// it. The two halves below pin both sides of the window: unsynced rows miss,
// and the sync-shaped fold-column write closes the window.
//
// upstream: peeringdb_server/rest.py:576 (query-time unidecode — no window)
func TestParity_Unicode_FoldWindow_DIVERGENCE(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	c := testutil.SetupClient(t)
	ctx := t.Context()

	// A row in the pre-sync window: canonical name present, _fold
	// shadow column still empty (the state a schema migration leaves
	// pre-existing rows in until the first post-deploy sync).
	name := "Köln Exchange"
	if _, err := c.Network.Create().
		SetID(1).SetName(name).SetNameFold("").
		SetAsn(64501).SetStatus("ok").
		SetCreated(t0).SetUpdated(t0).
		Save(ctx); err != nil {
		t.Fatalf("seed net: %v", err)
	}
	srv := newTestServer(t, c)

	// DIVERGENCE: upstream would match immediately; the empty fold
	// column makes the folded query miss during the window.
	status, body := httpGet(t, srv, "/api/net?name__contains=koln")
	if status != http.StatusOK {
		t.Fatalf("window query status = %d; body=%s", status, string(body))
	}
	if ids := extractIDs(t, body); len(ids) != 0 {
		t.Errorf("fold window: unsynced row matched %v; the registered divergence says it must NOT match until sync populates name_fold", ids)
	}

	// Simulate the next sync cycle's fold-column write
	// (upsert chains .SetNameFold(unifold.Fold(name))).
	if err := c.Network.UpdateOneID(1).
		SetNameFold(unifold.Fold(name)).
		Exec(ctx); err != nil {
		t.Fatalf("simulate sync fold write: %v", err)
	}
	status, body = httpGet(t, srv, "/api/net?name__contains=koln")
	if status != http.StatusOK {
		t.Fatalf("post-sync query status = %d; body=%s", status, string(body))
	}
	if ids := extractIDs(t, body); len(ids) != 1 || ids[0] != 1 {
		t.Errorf("post-sync fold query: got %v, want [1] (window must close after sync)", ids)
	}
}

// TestParity_Unicode_QSearchExtension_DIVERGENCE locks the registered
// divergence for the `?q=` list parameter (docs/API.md § Known
// Divergences).
//
// upstream: rest.py:545 — the db-field filter loop explicitly skips
// `k == "q"`, and no other consumer of the parameter exists on the /api
// surface (the separate `name_search` parameter triggers the
// Elasticsearch-backed search_v2 at rest.py:512-532; there is no DRF
// SearchFilter backend). Upstream therefore returns the UNFILTERED list
// for any `?q=` value.
//
// peeringdb-plus treats `?q=` as a convenience extension: substring
// search across the type's SearchFields (ContainsFold — case-insensitive
// but NOT diacritic-folded; the `_fold` shadow routing applies to
// `__contains`-family filters only, so `?q=munchen` does not match
// "München" while `?name__contains=munchen` does).
func TestParity_Unicode_QSearchExtension_DIVERGENCE(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	c := testutil.SetupClient(t)
	ctx := t.Context()

	seed := []struct {
		id   int
		name string
	}{
		{1, "München Exchange Peering"},
		{2, "Unrelated Networks"},
	}
	for _, s := range seed {
		if _, err := c.Network.Create().
			SetID(s.id).SetName(s.name).SetNameFold(unifold.Fold(s.name)).
			SetAsn(64500 + s.id).SetStatus("ok").
			SetCreated(t0).SetUpdated(t0).
			Save(ctx); err != nil {
			t.Fatalf("seed net %d: %v", s.id, err)
		}
	}
	srv := newTestServer(t, c)

	// DIVERGENCE: upstream ignores ?q= and would return BOTH rows here;
	// the extension narrows to the substring match.
	status, body := httpGet(t, srv, "/api/net?q="+url.QueryEscape("München"))
	if status != http.StatusOK {
		t.Fatalf("status = %d; body=%s", status, string(body))
	}
	if ids := extractIDs(t, body); len(ids) != 1 || ids[0] != 1 {
		t.Errorf("?q= extension: got %v, want [1] (substring-filtered, unlike upstream's unfiltered list)", ids)
	}

	// Case-insensitivity comes from ContainsFold.
	status, body = httpGet(t, srv, "/api/net?q="+url.QueryEscape("münchen"))
	if status != http.StatusOK {
		t.Fatalf("status = %d; body=%s", status, string(body))
	}
	if ids := extractIDs(t, body); len(ids) != 1 || ids[0] != 1 {
		t.Errorf("?q= case fold: got %v, want [1]", ids)
	}

	// No diacritic folding: the ASCII form does NOT match, unlike the
	// __contains filter family, which routes to the _fold shadow.
	status, body = httpGet(t, srv, "/api/net?q=munchen")
	if status != http.StatusOK {
		t.Fatalf("status = %d; body=%s", status, string(body))
	}
	if ids := extractIDs(t, body); len(ids) != 0 {
		t.Errorf("?q= diacritic fold: got %v, want [] (q search is not fold-routed; registered divergence)", ids)
	}
	status, body = httpGet(t, srv, "/api/net?name__contains=munchen")
	if status != http.StatusOK {
		t.Fatalf("status = %d; body=%s", status, string(body))
	}
	if ids := extractIDs(t, body); len(ids) != 1 || ids[0] != 1 {
		t.Errorf("__contains contrast: got %v, want [1] (the filter family IS fold-routed)", ids)
	}
}
