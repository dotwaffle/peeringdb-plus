package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// seedAllTestData creates a full set of related entities for detail page tests.
// Uses well-known IDs so assertions are deterministic.
func seedAllTestData(t *testing.T, client *ent.Client) {
	t.Helper()
	ctx := context.Background()

	org, err := client.Organization.Create().
		SetID(1).SetName("TestOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	net, err := client.Network.Create().
		SetID(10).SetName("Cloudflare").SetAsn(13335).
		SetOrgID(1).SetOrganization(org).
		SetIxCount(1).SetFacCount(1).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	ix, err := client.InternetExchange.Create().
		SetID(20).SetName("DE-CIX Frankfurt").
		SetOrgID(1).SetOrganization(org).
		SetCity("Frankfurt").SetCountry("DE").
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetNetCount(1).SetFacCount(1).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ix: %v", err)
	}

	fac, err := client.Facility.Create().
		SetID(30).SetName("Equinix FR5").
		SetOrgID(1).SetOrganization(org).
		SetCity("Frankfurt").SetCountry("DE").
		SetNetCount(1).SetIxCount(1).SetCarrierCount(1).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating facility: %v", err)
	}

	campusEntity, err := client.Campus.Create().
		SetID(40).SetName("Test Campus").
		SetOrgID(1).SetOrganization(org).
		SetCity("Berlin").SetCountry("DE").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating campus: %v", err)
	}

	carrierEntity, err := client.Carrier.Create().
		SetID(50).SetName("Test Carrier").
		SetOrgID(1).SetOrganization(org).
		SetFacCount(1).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating carrier: %v", err)
	}

	ixlanEntity, err := client.IxLan.Create().
		SetID(100).SetIxID(20).
		SetInternetExchange(ix).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ixlan: %v", err)
	}

	_, err = client.NetworkIxLan.Create().
		SetID(200).
		SetNetID(net.ID).SetNetwork(net).
		SetIxlanID(100).SetIxLan(ixlanEntity).
		SetAsn(13335).SetSpeed(10000).
		SetName("DE-CIX Frankfurt").SetIxID(20).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkixlan: %v", err)
	}

	_, err = client.NetworkFacility.Create().
		SetID(300).
		SetNetID(net.ID).SetNetwork(net).
		SetFacID(fac.ID).SetFacility(fac).
		SetLocalAsn(13335).
		SetName("Equinix FR5").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkfacility: %v", err)
	}

	_, err = client.IxFacility.Create().
		SetID(400).
		SetFacID(fac.ID).SetFacility(fac).
		SetIxID(ix.ID).SetInternetExchange(ix).
		SetName("DE-CIX Frankfurt").
		SetCity("Frankfurt").SetCountry("DE").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ixfacility: %v", err)
	}

	_, err = client.Poc.Create().
		SetID(500).
		SetNetID(net.ID).SetNetwork(net).
		SetName("NOC Contact").SetRole("NOC").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating poc: %v", err)
	}

	_, err = client.CarrierFacility.Create().
		SetID(600).
		SetCarrierID(carrierEntity.ID).SetCarrier(carrierEntity).
		SetFacID(fac.ID).SetFacility(fac).
		SetName("Equinix FR5").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating carrierfacility: %v", err)
	}

	// Create a facility assigned to the campus for campus fragment tests.
	_, err = client.Facility.Create().
		SetID(31).SetName("Campus Facility").
		SetOrgID(1).SetOrganization(org).
		SetCampusID(campusEntity.ID).SetCampus(campusEntity).
		SetCity("Berlin").SetCountry("DE").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating campus facility: %v", err)
	}

	// Create a prefix on the IX for prefix fragment tests.
	_, err = client.IxPrefix.Create().
		SetID(700).
		SetIxlanID(100).SetIxLan(ixlanEntity).
		SetPrefix("80.81.192.0/22").SetProtocol("IPv4").SetInDfz(true).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ixprefix: %v", err)
	}
}

// setupAllTestMux seeds all test data and returns a mux ready for testing.
func setupAllTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	client := testutil.SetupClient(t)
	seedAllTestData(t, client)
	h := NewHandler(client, nil)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func TestDetailPages_AllTypes(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	tests := []struct {
		name     string
		url      string
		wantCode int
		wantBody []string
	}{
		{"network by ASN", "/ui/asn/13335", http.StatusOK, []string{"Cloudflare", "AS13335"}},
		{"IXP by ID", "/ui/ix/20", http.StatusOK, []string{"DE-CIX Frankfurt", "Frankfurt"}},
		{"facility by ID", "/ui/fac/30", http.StatusOK, []string{"Equinix FR5"}},
		{"org by ID", "/ui/org/1", http.StatusOK, []string{"TestOrg"}},
		{"campus by ID", "/ui/campus/40", http.StatusOK, []string{"Test Campus"}},
		{"carrier by ID", "/ui/carrier/50", http.StatusOK, []string{"Test Carrier"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d", tt.wantCode, rec.Code)
			}

			body := rec.Body.String()
			for _, want := range tt.wantBody {
				if !strings.Contains(body, want) {
					t.Errorf("response missing %q", want)
				}
			}
		})
	}
}

func TestDetailPages_NotFound(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	tests := []struct {
		name string
		url  string
	}{
		{"network not found", "/ui/asn/99999"},
		{"ix not found", "/ui/ix/99999"},
		{"fac not found", "/ui/fac/99999"},
		{"org not found", "/ui/org/99999"},
		{"campus not found", "/ui/campus/99999"},
		{"carrier not found", "/ui/carrier/99999"},
		{"invalid asn", "/ui/asn/abc"},
		{"invalid ix id", "/ui/ix/abc"},
		{"invalid fac id", "/ui/fac/abc"},
		{"invalid org id", "/ui/org/abc"},
		{"invalid campus id", "/ui/campus/abc"},
		{"invalid carrier id", "/ui/carrier/abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status 404, got %d", rec.Code)
			}
		})
	}
}

func TestDetailPages_HtmxFragment(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/asn/13335", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "<!doctype html>") || strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("htmx fragment should not contain DOCTYPE")
	}
	if !strings.Contains(body, "Cloudflare") {
		t.Error("htmx fragment should contain network name")
	}
}

func TestDetailPages_Stats(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	tests := []struct {
		name string
		url  string
		want []string
	}{
		{"network stats", "/ui/asn/13335", []string{"(1)", "(1)"}},
		{"ix stats", "/ui/ix/20", []string{"(1)", "(1)"}},
		{"facility stats", "/ui/fac/30", []string{"(1)", "(1)", "(1)"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			body := rec.Body.String()
			for _, want := range tt.want {
				if !strings.Contains(body, want) {
					t.Errorf("response missing stat %q", want)
				}
			}
		})
	}
}

func TestDetailPages_OrgLink(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	tests := []struct {
		name string
		url  string
	}{
		{"network org link", "/ui/asn/13335"},
		{"ix org link", "/ui/ix/20"},
		{"facility org link", "/ui/fac/30"},
		{"campus org link", "/ui/campus/40"},
		{"carrier org link", "/ui/carrier/50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			body := rec.Body.String()
			if !strings.Contains(body, "/ui/org/1") {
				t.Error("detail page should contain org link /ui/org/1")
			}
			if !strings.Contains(body, "TestOrg") {
				t.Error("detail page should contain org name")
			}
		})
	}
}

func TestDetailPages_CollapsibleSections(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	tests := []struct {
		name         string
		url          string
		wantFragment string
	}{
		{"network collapsible", "/ui/asn/13335", "fragment/net"},
		{"ix collapsible", "/ui/ix/20", "fragment/ix"},
		{"fac collapsible", "/ui/fac/30", "fragment/fac"},
		{"org collapsible", "/ui/org/1", "fragment/org"},
		{"campus collapsible", "/ui/campus/40", "fragment/campus"},
		{"carrier collapsible", "/ui/carrier/50", "fragment/carrier"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			body := rec.Body.String()
			if !strings.Contains(body, "<details") {
				t.Error("response should contain collapsible details element")
			}
			if !strings.Contains(body, tt.wantFragment) {
				t.Errorf("response should contain fragment URL pattern %q", tt.wantFragment)
			}
		})
	}
}

func TestFragments_AllTypes(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	tests := []struct {
		name     string
		url      string
		wantCode int
		wantBody []string
		noBody   []string
	}{
		{"net ixlans", "/ui/fragment/net/10/ixlans", http.StatusOK, []string{"DE-CIX"}, []string{"<!doctype"}},
		{"net facilities", "/ui/fragment/net/10/facilities", http.StatusOK, []string{"Equinix"}, []string{"<!doctype"}},
		{"net contacts", "/ui/fragment/net/10/contacts", http.StatusOK, []string{"NOC"}, []string{"<!doctype"}},
		{"ix participants", "/ui/fragment/ix/20/participants", http.StatusOK, []string{"13335"}, []string{"<!doctype"}},
		{"ix facilities", "/ui/fragment/ix/20/facilities", http.StatusOK, []string{"DE-CIX Frankfurt"}, []string{"<!doctype"}},
		{"ix prefixes", "/ui/fragment/ix/20/prefixes", http.StatusOK, []string{"80.81.192.0/22"}, []string{"<!doctype"}},
		{"fac networks", "/ui/fragment/fac/30/networks", http.StatusOK, []string{"Equinix"}, []string{"<!doctype"}},
		{"fac ixps", "/ui/fragment/fac/30/ixps", http.StatusOK, []string{"DE-CIX"}, []string{"<!doctype"}},
		{"fac carriers", "/ui/fragment/fac/30/carriers", http.StatusOK, []string{"Equinix FR5"}, []string{"<!doctype"}},
		{"org networks", "/ui/fragment/org/1/networks", http.StatusOK, []string{"Cloudflare", "<table", "data-sortable", "data-sort-value"}, []string{"<!doctype", "px-4 py-3 hover:bg-neutral-800/50"}},
		{"org ixps", "/ui/fragment/org/1/ixps", http.StatusOK, []string{"DE-CIX", "<table"}, []string{"<!doctype", "data-sortable", "px-4 py-3 hover:bg-neutral-800/50"}},
		{"org facilities", "/ui/fragment/org/1/facilities", http.StatusOK, []string{"Equinix", "<table", "data-sortable", "fi fi-", "data-sort-value"}, []string{"<!doctype", "px-4 py-3 hover:bg-neutral-800/50"}},
		{"campus facilities", "/ui/fragment/campus/40/facilities", http.StatusOK, []string{"Campus Facility", "<table", "data-sortable", "fi fi-", "data-sort-value"}, []string{"<!doctype", "px-4 py-3 hover:bg-neutral-800/50"}},
		{"carrier facilities", "/ui/fragment/carrier/50/facilities", http.StatusOK, []string{"Equinix", "<table"}, []string{"<!doctype", "data-sortable", "px-4 py-3 hover:bg-neutral-800/50"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d", tt.wantCode, rec.Code)
			}

			body := rec.Body.String()
			for _, want := range tt.wantBody {
				if !strings.Contains(body, want) {
					t.Errorf("response missing %q", want)
				}
			}
			for _, notWant := range tt.noBody {
				if strings.Contains(body, notWant) {
					t.Errorf("response should not contain %q", notWant)
				}
			}
		})
	}
}

func TestFragments_CrossLinks(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	tests := []struct {
		name      string
		url       string
		wantLinks []string
	}{
		{"net ixlans -> ix", "/ui/fragment/net/10/ixlans", []string{"/ui/ix/"}},
		{"net facilities -> fac", "/ui/fragment/net/10/facilities", []string{"/ui/fac/"}},
		{"fac networks -> asn", "/ui/fragment/fac/30/networks", []string{"/ui/asn/"}},
		{"fac ixps -> ix", "/ui/fragment/fac/30/ixps", []string{"/ui/ix/"}},
		{"fac carriers -> carrier", "/ui/fragment/fac/30/carriers", []string{"/ui/carrier/"}},
		{"ix participants -> asn", "/ui/fragment/ix/20/participants", []string{"/ui/asn/"}},
		{"ix facilities -> fac", "/ui/fragment/ix/20/facilities", []string{"/ui/fac/"}},
		{"org networks -> asn", "/ui/fragment/org/1/networks", []string{"/ui/asn/"}},
		{"org ixps -> ix", "/ui/fragment/org/1/ixps", []string{"/ui/ix/"}},
		{"org facilities -> fac", "/ui/fragment/org/1/facilities", []string{"/ui/fac/"}},
		{"campus facilities -> fac", "/ui/fragment/campus/40/facilities", []string{"/ui/fac/"}},
		{"carrier facilities -> fac", "/ui/fragment/carrier/50/facilities", []string{"/ui/fac/"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			body := rec.Body.String()
			for _, link := range tt.wantLinks {
				if !strings.Contains(body, link) {
					t.Errorf("response missing cross-link %q", link)
				}
			}
		})
	}
}

func TestFragment_InvalidPath(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	tests := []struct {
		name string
		url  string
	}{
		{"invalid type", "/ui/fragment/invalid/1/test"},
		{"invalid net id", "/ui/fragment/net/abc/ixlans"},
		{"invalid net relation", "/ui/fragment/net/10/invalid"},
		{"too few parts", "/ui/fragment/net/10"},
		{"invalid ix relation", "/ui/fragment/ix/20/invalid"},
		{"invalid fac relation", "/ui/fragment/fac/30/invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status 404, got %d", rec.Code)
			}
		})
	}
}

func TestFragments_OrgCampusesAndCarriers(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	tests := []struct {
		name     string
		url      string
		wantCode int
		wantBody []string
		noBody   []string
	}{
		{"org campuses", "/ui/fragment/org/1/campuses", http.StatusOK, []string{"Test Campus", "<table"}, []string{"data-sortable", "px-4 py-3 hover:bg-neutral-800/50"}},
		{"org carriers", "/ui/fragment/org/1/carriers", http.StatusOK, []string{"Test Carrier", "<table"}, []string{"data-sortable", "px-4 py-3 hover:bg-neutral-800/50"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d", tt.wantCode, rec.Code)
			}

			body := rec.Body.String()
			for _, want := range tt.wantBody {
				if !strings.Contains(body, want) {
					t.Errorf("response missing %q", want)
				}
			}
			for _, notWant := range tt.noBody {
				if strings.Contains(body, notWant) {
					t.Errorf("response should not contain %q", notWant)
				}
			}
		})
	}
}

func TestGetFreshness_WithSyncRecord(t *testing.T) {
	t.Parallel()
	client, db := testutil.SetupClientWithDB(t)
	ctx := context.Background()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	startedAt := time.Now().Add(-time.Hour)
	id, err := sync.RecordSyncStart(ctx, db, startedAt)
	if err != nil {
		t.Fatalf("record sync start: %v", err)
	}

	completedAt := time.Now().Add(-30 * time.Minute)
	err = sync.RecordSyncComplete(ctx, db, id, sync.Status{
		LastSyncAt: completedAt,
		Duration:   5 * time.Second,
		Status:     "success",
	})
	if err != nil {
		t.Fatalf("record sync complete: %v", err)
	}

	h := NewHandler(client, db)
	freshness := h.getFreshness(ctx)

	if freshness.IsZero() {
		t.Error("getFreshness should return non-zero time after sync record insertion")
	}
}

func TestGetFreshness_EmptyTable(t *testing.T) {
	t.Parallel()
	client, db := testutil.SetupClientWithDB(t)
	ctx := context.Background()

	if err := sync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	h := NewHandler(client, db)
	freshness := h.getFreshness(ctx)

	if !freshness.IsZero() {
		t.Errorf("getFreshness should return zero time with empty table, got %v", freshness)
	}
}
