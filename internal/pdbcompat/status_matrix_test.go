package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// fetchDataLength GETs the given URL, decodes the PeeringDB envelope, and
// returns the length of the data array plus the HTTP status code.
func fetchDataLength(t *testing.T, url string) (int, int) {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx // test code, local httptest server
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, resp.StatusCode
	}

	var env struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope from %s: %v", url, err)
	}
	return len(env.Data), resp.StatusCode
}

// fetchStatusCode GETs the given URL and returns only the HTTP status code.
func fetchStatusCode(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx // test code
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// TestStatusMatrix covers the Phase 68 Plan 68-03 STATUS-01, STATUS-02,
// STATUS-04 + LIMIT-01 + LIMIT-02 requirements via end-to-end HTTP
// exercise of the pdbcompat handler against mixed-status fixtures.
// Each subtest uses its own isolated client per testutil.SetupClient,
// so the subtests are safe to t.Parallel() independently.
func TestStatusMatrix(t *testing.T) {
	t.Parallel()

	// Base timestamp for fixture rows. Spread updated by 1h so any
	// default-ordering tiebreaks are deterministic.
	t0 := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	// Helper: seed a Network with a given id/asn/status/updated.
	seedNet := func(t *testing.T, client *ent.Client, id, asn int, status string, updated time.Time) {
		t.Helper()
		_, err := client.Network.Create().
			SetID(id).SetName("Net").SetAsn(asn).SetStatus(status).
			SetCreated(t0).SetUpdated(updated).
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed network id=%d: %v", id, err)
		}
	}
	// Helper: seed a Campus with a given id/status/updated under a single Org.
	seedCampus := func(t *testing.T, client *ent.Client, id int, status string, updated time.Time) {
		t.Helper()
		ctx := t.Context()
		// Ensure parent org exists (id=1 reused across campus rows).
		if _, err := client.Organization.Query().Where().Count(ctx); err == nil {
			if n, _ := client.Organization.Query().Count(ctx); n == 0 {
				if _, err := client.Organization.Create().
					SetID(1).SetName("Campus Parent").
					SetCreated(t0).SetUpdated(t0).
					Save(ctx); err != nil {
					t.Fatalf("seed parent org: %v", err)
				}
			}
		}
		_, err := client.Campus.Create().
			SetID(id).SetName("Camp").
			SetOrgID(1).SetCity("Berlin").SetCountry("DE").
			SetStatus(status).
			SetCreated(t0).SetUpdated(updated).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed campus id=%d: %v", id, err)
		}
	}

	t.Run("list_no_since_returns_only_ok", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		// 1 ok, 1 pending, 1 deleted.
		seedNet(t, client, 1, 64501, "ok", t0)
		seedNet(t, client, 2, 64502, "pending", t0.Add(1*time.Hour))
		seedNet(t, client, 3, 64503, "deleted", t0.Add(2*time.Hour))

		srv := httptest.NewServer(newMuxForOrdering(client))
		t.Cleanup(srv.Close)

		n, code := fetchDataLength(t, srv.URL+"/api/net")
		if code != http.StatusOK {
			t.Fatalf("GET /api/net: status %d", code)
		}
		if n != 1 {
			t.Errorf("list without since: got %d items, want 1 (only status=ok) per STATUS-01", n)
		}
	})

	t.Run("list_with_since_non_campus_returns_ok_and_deleted", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		// 1 ok, 1 pending, 1 deleted — all with updated in the future so
		// they all land inside the since window.
		seedNet(t, client, 1, 64501, "ok", t0)
		seedNet(t, client, 2, 64502, "pending", t0.Add(1*time.Hour))
		seedNet(t, client, 3, 64503, "deleted", t0.Add(2*time.Hour))

		srv := httptest.NewServer(newMuxForOrdering(client))
		t.Cleanup(srv.Close)

		// since=0 admits all historical rows.
		n, code := fetchDataLength(t, srv.URL+"/api/net?since=0")
		if code != http.StatusOK {
			t.Fatalf("GET /api/net?since=0: status %d", code)
		}
		if n != 2 {
			t.Errorf("list with since=0 (non-campus): got %d items, want 2 (ok + deleted; pending excluded) per STATUS-03", n)
		}
	})

	t.Run("list_with_since_campus_includes_pending", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		seedCampus(t, client, 1, "ok", t0)
		seedCampus(t, client, 2, "pending", t0.Add(1*time.Hour))
		seedCampus(t, client, 3, "deleted", t0.Add(2*time.Hour))

		srv := httptest.NewServer(newMuxForOrdering(client))
		t.Cleanup(srv.Close)

		n, code := fetchDataLength(t, srv.URL+"/api/campus?since=0")
		if code != http.StatusOK {
			t.Fatalf("GET /api/campus?since=0: status %d", code)
		}
		if n != 3 {
			t.Errorf("list campus with since=0: got %d items, want 3 (ok + pending + deleted) per D-05", n)
		}
	})

	t.Run("pk_ok_returns_200", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		seedNet(t, client, 10, 64510, "ok", t0)

		srv := httptest.NewServer(newMuxForOrdering(client))
		t.Cleanup(srv.Close)

		if code := fetchStatusCode(t, srv.URL+"/api/net/10"); code != http.StatusOK {
			t.Errorf("pk lookup on ok row: got %d, want 200", code)
		}
	})

	t.Run("pk_pending_returns_200", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		seedNet(t, client, 20, 64520, "pending", t0)

		srv := httptest.NewServer(newMuxForOrdering(client))
		t.Cleanup(srv.Close)

		if code := fetchStatusCode(t, srv.URL+"/api/net/20"); code != http.StatusOK {
			t.Errorf("pk lookup on pending row: got %d, want 200 per STATUS-02/D-06", code)
		}
	})

	t.Run("pk_deleted_returns_404", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		seedNet(t, client, 30, 64530, "deleted", t0)

		srv := httptest.NewServer(newMuxForOrdering(client))
		t.Cleanup(srv.Close)

		if code := fetchStatusCode(t, srv.URL+"/api/net/30"); code != http.StatusNotFound {
			t.Errorf("pk lookup on deleted row: got %d, want 404 per STATUS-02/D-06", code)
		}
	})

	t.Run("status_deleted_no_since_is_empty", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		// Seed only a deleted row. With no ?since, the list filters to
		// status=ok regardless of any ?status= override (D-07).
		seedNet(t, client, 1, 64501, "deleted", t0)

		srv := httptest.NewServer(newMuxForOrdering(client))
		t.Cleanup(srv.Close)

		// Plain ?status=deleted without since.
		n, code := fetchDataLength(t, srv.URL+"/api/net?status=deleted")
		if code != http.StatusOK {
			t.Fatalf("GET /api/net?status=deleted: status %d", code)
		}
		if n != 0 {
			t.Errorf("?status=deleted without since: got %d items, want 0 per STATUS-04/D-07", n)
		}
	})

	t.Run("limit_zero_returns_all_rows", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		ctx := t.Context()
		// Seed 300 status=ok networks (above the historical 250 cap,
		// below MaxLimit=1000). Both bare URL and ?limit=0 return all
		// rows per LIMIT-01 (matches upstream rest.py:495 + :737).
		const seedN = 300
		for i := 1; i <= seedN; i++ {
			if _, err := client.Network.Create().
				SetID(i).SetName("N").SetAsn(60000 + i).SetStatus("ok").
				SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed network %d: %v", i, err)
			}
		}

		srv := httptest.NewServer(newMuxForOrdering(client))
		t.Cleanup(srv.Close)

		// Bare URL: upstream parity — returns all 300 rows.
		n, _ := fetchDataLength(t, srv.URL+"/api/net")
		if n != seedN {
			t.Errorf("bare /api/net: got %d, want %d (LIMIT-01: bare URL returns all rows, matching upstream)", n, seedN)
		}

		// Explicit ?limit=0: also returns all 300 rows.
		n, _ = fetchDataLength(t, srv.URL+"/api/net?limit=0")
		if n != seedN {
			t.Errorf("?limit=0: got %d, want %d (all rows) per LIMIT-01", n, seedN)
		}
	})

	t.Run("depth_on_list_is_silently_ignored", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		seedNet(t, client, 1, 64501, "ok", t0)
		seedNet(t, client, 2, 64502, "ok", t0.Add(1*time.Hour))

		srv := httptest.NewServer(newMuxForOrdering(client))
		t.Cleanup(srv.Close)

		nPlain, codePlain := fetchDataLength(t, srv.URL+"/api/net")
		nDepth, codeDepth := fetchDataLength(t, srv.URL+"/api/net?depth=2")

		if codePlain != http.StatusOK || codeDepth != http.StatusOK {
			t.Fatalf("status: plain=%d depth=%d (both must be 200)", codePlain, codeDepth)
		}
		if nPlain != nDepth {
			t.Errorf("LIMIT-02 guardrail failed: /api/net returned %d, /api/net?depth=2 returned %d (expected equal)", nPlain, nDepth)
		}
	})
}

// TestStatusMatrix_AllEntities asserts the Phase 68 status×since matrix is
// wired into the list closure of EVERY one of the 13 entity types, with the
// correct isCampus flag. The focused TestStatusMatrix above proves the
// applyStatusMatrix function body against net + campus; this table-driven
// test guards against a per-entity wiring omission — dropping the matrix
// predicate, or flipping isCampus, in exactly one of the 13
// registry_funcs.go closures — which would silently leak status=deleted
// tombstones (or pending rows) onto the anonymous /api list surface for
// that one type while leaving net + campus intact.
//
// Matrix (per applyStatusMatrix in filter.go):
//   - no ?since        → status == "ok"            (deleted + pending hidden)
//   - ?since=N, generic → status IN (ok, deleted)  (pending still hidden)
//   - ?since=N, campus  → status IN (ok, deleted, pending)
//
// Each subtest seeds one ok + one deleted + one pending row of its type
// (under freshly-seeded FK parents of OTHER types, which never appear on
// the type's own list endpoint) in an isolated client.
func TestStatusMatrix_AllEntities(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	// seedParentsFor creates the (status=ok) FK ancestors needed by tag,
	// using fixed IDs distinct from the child rows' 9xx band. The entity
	// under test is never its own parent, so parents (a different type)
	// never pollute the type's list endpoint.
	seedParentsFor := func(t *testing.T, c *ent.Client, tag string) {
		t.Helper()
		ctx := t.Context()
		org := func() {
			c.Organization.Create().SetID(1).SetName("ParentOrg").SetStatus("ok").
				SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		net := func() {
			c.Network.Create().SetID(10).SetName("ParentNet").SetAsn(13335).SetOrgID(1).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		ix := func() {
			c.InternetExchange.Create().SetID(20).SetName("ParentIX").SetOrgID(1).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		fac := func() {
			c.Facility.Create().SetID(30).SetName("ParentFac").SetOrgID(1).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		carrier := func() {
			c.Carrier.Create().SetID(50).SetName("ParentCarrier").SetOrgID(1).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		ixlan := func() {
			c.IxLan.Create().SetID(70).SetIxID(20).SetStatus("ok").
				SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		switch tag {
		case peeringdb.TypeOrg:
			// root entity, no FK parents
		case peeringdb.TypeNet, peeringdb.TypeFac, peeringdb.TypeIX,
			peeringdb.TypeCarrier, peeringdb.TypeCampus:
			org()
		case peeringdb.TypePoc:
			org()
			net()
		case peeringdb.TypeIXLan:
			org()
			ix()
		case peeringdb.TypeIXPfx:
			org()
			ix()
			ixlan()
		case peeringdb.TypeNetIXLan:
			org()
			net()
			ix()
			ixlan()
		case peeringdb.TypeNetFac:
			org()
			net()
			fac()
		case peeringdb.TypeIXFac:
			org()
			ix()
			fac()
		case peeringdb.TypeCarrierFac:
			org()
			carrier()
			fac()
		default:
			t.Fatalf("seedParentsFor: unknown tag %q", tag)
		}
	}

	// seedStatusRow creates one row of tag with the given id and status,
	// mirroring the known-good required-field setters from
	// internal/testutil/seed. net asn and ixpfx prefix are derived from id
	// to stay unique across the three rows.
	seedStatusRow := func(t *testing.T, c *ent.Client, tag string, id int, status string) {
		t.Helper()
		ctx := t.Context()
		var err error
		switch tag {
		case peeringdb.TypeOrg:
			_, err = c.Organization.Create().SetID(id).SetName("Org").SetStatus(status).
				SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypeNet:
			_, err = c.Network.Create().SetID(id).SetName("Net").SetAsn(900000 + id).
				SetOrgID(1).SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypeFac:
			_, err = c.Facility.Create().SetID(id).SetName("Fac").SetOrgID(1).
				SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypeIX:
			_, err = c.InternetExchange.Create().SetID(id).SetName("IX").SetOrgID(1).
				SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypeCarrier:
			_, err = c.Carrier.Create().SetID(id).SetName("Carrier").SetOrgID(1).
				SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypeCampus:
			_, err = c.Campus.Create().SetID(id).SetName("Campus").SetOrgID(1).
				SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypePoc:
			// visible defaults to "Public" → anon-visible, so only status gates it.
			_, err = c.Poc.Create().SetID(id).SetNetID(10).SetName("Poc").SetRole("NOC").
				SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypeIXLan:
			_, err = c.IxLan.Create().SetID(id).SetIxID(20).SetStatus(status).
				SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypeIXPfx:
			_, err = c.IxPrefix.Create().SetID(id).SetIxlanID(70).
				SetPrefix(fmt.Sprintf("10.0.%d.0/24", id-900)).SetProtocol("IPv4").
				SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypeNetIXLan:
			_, err = c.NetworkIxLan.Create().SetID(id).SetNetID(10).SetIxlanID(70).
				SetIxID(20).SetAsn(13335).SetSpeed(10000).SetName("NetIXLan").
				SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypeNetFac:
			_, err = c.NetworkFacility.Create().SetID(id).SetNetID(10).SetFacID(30).
				SetLocalAsn(13335).SetName("NetFac").SetStatus(status).
				SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypeIXFac:
			_, err = c.IxFacility.Create().SetID(id).SetIxID(20).SetFacID(30).
				SetName("IXFac").SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
		case peeringdb.TypeCarrierFac:
			_, err = c.CarrierFacility.Create().SetID(id).SetCarrierID(50).SetFacID(30).
				SetName("CarrierFac").SetStatus(status).SetCreated(t0).SetUpdated(t0).Save(ctx)
		default:
			t.Fatalf("seedStatusRow: unknown tag %q", tag)
		}
		if err != nil {
			t.Fatalf("seed %s id=%d status=%s: %v", tag, id, status, err)
		}
	}

	entities := []struct {
		tag      string
		isCampus bool
	}{
		{peeringdb.TypeOrg, false},
		{peeringdb.TypeNet, false},
		{peeringdb.TypeFac, false},
		{peeringdb.TypeIX, false},
		{peeringdb.TypePoc, false},
		{peeringdb.TypeIXLan, false},
		{peeringdb.TypeIXPfx, false},
		{peeringdb.TypeNetIXLan, false},
		{peeringdb.TypeNetFac, false},
		{peeringdb.TypeIXFac, false},
		{peeringdb.TypeCarrier, false},
		{peeringdb.TypeCarrierFac, false},
		{peeringdb.TypeCampus, true},
	}

	for _, e := range entities {
		t.Run(e.tag, func(t *testing.T) {
			t.Parallel()
			c := testutil.SetupClient(t)
			seedParentsFor(t, c, e.tag)
			seedStatusRow(t, c, e.tag, 901, "ok")
			seedStatusRow(t, c, e.tag, 902, "deleted")
			seedStatusRow(t, c, e.tag, 903, "pending")

			srv := httptest.NewServer(newMuxForOrdering(c))
			t.Cleanup(srv.Close)

			// No ?since: only the status=ok row. A missing matrix
			// predicate would also return deleted (+pending) here.
			if n, code := fetchDataLength(t, srv.URL+"/api/"+e.tag); code != http.StatusOK || n != 1 {
				t.Errorf("GET /api/%s (no since): got n=%d code=%d, want n=1 code=200 (only status=ok; deleted+pending must be hidden)",
					e.tag, n, code)
			}

			// ?since=0: ok+deleted for generic types; +pending for campus.
			// A flipped isCampus flag would change this count.
			wantSince := 2
			if e.isCampus {
				wantSince = 3
			}
			if n, code := fetchDataLength(t, srv.URL+"/api/"+e.tag+"?since=0"); code != http.StatusOK || n != wantSince {
				t.Errorf("GET /api/%s?since=0: got n=%d code=%d, want n=%d code=200 (isCampus=%v: pending %s)",
					e.tag, n, code, wantSince, e.isCampus,
					map[bool]string{true: "admitted", false: "hidden"}[e.isCampus])
			}
		})
	}
}
