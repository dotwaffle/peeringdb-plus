package web

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// testTimestamp provides a consistent timestamp for all search tests.
var testSearchTimestamp = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// seedSearchData creates test records for all 6 searchable entity types.
// Returns the ent client with seeded data ready for search queries.
func seedSearchData(t *testing.T) *SearchService {
	t.Helper()
	client := testutil.SetupClient(t)
	ctx := t.Context()

	// Organization (parent for Network, IXP, Facility, Campus, Carrier)
	org, err := client.Organization.Create().
		SetID(1).
		SetName("Cloudflare Inc").
		SetAka("CF").
		SetCountry("US").
		SetCity("San Francisco").
		SetCreated(testSearchTimestamp).
		SetUpdated(testSearchTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	// Network
	_, err = client.Network.Create().
		SetID(10).
		SetName("Cloudflare").
		SetAsn(13335).
		SetIrrAsSet("AS-CLOUDFLARE").
		SetOrgID(1).
		SetOrganization(org).
		SetCreated(testSearchTimestamp).
		SetUpdated(testSearchTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	// Internet Exchange
	_, err = client.InternetExchange.Create().
		SetID(20).
		SetName("DE-CIX Frankfurt").
		SetCity("Frankfurt").
		SetCountry("DE").
		SetOrgID(1).
		SetOrganization(org).
		SetRegionContinent("Europe").
		SetMedia("Ethernet").
		SetCreated(testSearchTimestamp).
		SetUpdated(testSearchTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating internet exchange: %v", err)
	}

	// Facility
	_, err = client.Facility.Create().
		SetID(30).
		SetName("Equinix DC5").
		SetCity("Ashburn").
		SetCountry("US").
		SetOrgID(1).
		SetOrganization(org).
		SetClli("ASHBVA01").
		SetCreated(testSearchTimestamp).
		SetUpdated(testSearchTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating facility: %v", err)
	}

	// Campus
	_, err = client.Campus.Create().
		SetID(40).
		SetName("Equinix Campus Ashburn").
		SetCity("Ashburn").
		SetCountry("US").
		SetOrgID(1).
		SetOrganization(org).
		SetCreated(testSearchTimestamp).
		SetUpdated(testSearchTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating campus: %v", err)
	}

	// Carrier
	_, err = client.Carrier.Create().
		SetID(50).
		SetName("Zayo Group").
		SetOrgID(1).
		SetOrganization(org).
		SetCreated(testSearchTimestamp).
		SetUpdated(testSearchTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating carrier: %v", err)
	}

	return NewSearchService(client)
}

func TestSearchServiceNew(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	svc := NewSearchService(client)
	if svc == nil {
		t.Fatal("NewSearchService returned nil")
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	results, err := svc.Search(ctx, "")
	if err != nil {
		t.Fatalf("Search with empty query: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestSearchSingleCharQuery(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	results, err := svc.Search(ctx, "a")
	if err != nil {
		t.Fatalf("Search with single char: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for single char query, got %d", len(results))
	}
}

func TestSearchWhitespaceQuery(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	results, err := svc.Search(ctx, "  ")
	if err != nil {
		t.Fatalf("Search with whitespace: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for whitespace query, got %d", len(results))
	}
}

func TestSearchMatchesMultipleTypes(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	// "Cloud" should match Network ("Cloudflare") and Organization ("Cloudflare Inc")
	results, err := svc.Search(ctx, "Cloud")
	if err != nil {
		t.Fatalf("Search for Cloud: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 type groups for 'Cloud', got %d", len(results))
	}

	// Check that we got Networks and Organizations (in that order per type order)
	typeNames := make(map[string]bool)
	for _, r := range results {
		typeNames[r.TypeName] = true
	}
	if !typeNames["Networks"] {
		t.Error("expected Networks in results for 'Cloud'")
	}
	if !typeNames["Organizations"] {
		t.Error("expected Organizations in results for 'Cloud'")
	}
}

func TestSearchMatchesSingleType(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	// "DE-CIX" should match only the IXP
	results, err := svc.Search(ctx, "DE-CIX")
	if err != nil {
		t.Fatalf("Search for DE-CIX: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 type group for 'DE-CIX', got %d", len(results))
	}
	if results[0].TypeName != "IXPs" {
		t.Errorf("expected TypeName 'IXPs', got %q", results[0].TypeName)
	}
	if results[0].TypeSlug != "ix" {
		t.Errorf("expected TypeSlug 'ix', got %q", results[0].TypeSlug)
	}
}

func TestSearchMatchesFacilityAndCampus(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	// "Equinix" matches Facility and Campus
	results, err := svc.Search(ctx, "Equinix")
	if err != nil {
		t.Fatalf("Search for Equinix: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 type groups for 'Equinix', got %d", len(results))
	}
	typeNames := make(map[string]bool)
	for _, r := range results {
		typeNames[r.TypeName] = true
	}
	if !typeNames["Facilities"] {
		t.Error("expected Facilities in results for 'Equinix'")
	}
	if !typeNames["Campuses"] {
		t.Error("expected Campuses in results for 'Equinix'")
	}
}

func TestSearchNoMatches(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	results, err := svc.Search(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Search for nonexistent: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'nonexistent', got %d", len(results))
	}
}

func TestSearchTypeResultFields(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	results, err := svc.Search(ctx, "DE-CIX")
	if err != nil {
		t.Fatalf("Search for DE-CIX: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 type group, got %d", len(results))
	}

	r := results[0]
	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"TypeName", r.TypeName, "IXPs"},
		{"TypeSlug", r.TypeSlug, "ix"},
		{"AccentColor", r.AccentColor, "sky"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.field, tt.got, tt.want)
		}
	}
	if r.HasMore {
		t.Errorf("HasMore = true, want false (only 1 result)")
	}
	if len(r.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(r.Results))
	}
}

func TestSearchNetworkDetailURL(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	results, err := svc.Search(ctx, "Cloudflare")
	if err != nil {
		t.Fatalf("Search for Cloudflare: %v", err)
	}

	// Find the Networks result
	for _, r := range results {
		if r.TypeName != "Networks" {
			continue
		}
		if len(r.Results) == 0 {
			t.Fatal("Networks group has no results")
		}
		hit := r.Results[0]
		if hit.Name != "Cloudflare" {
			t.Errorf("hit.Name = %q, want %q", hit.Name, "Cloudflare")
		}
		// Networks use ASN in URL, not ID
		if hit.DetailURL != "/ui/asn/13335" {
			t.Errorf("hit.DetailURL = %q, want %q", hit.DetailURL, "/ui/asn/13335")
		}
		// Networks populate ASN field (no Country/City without org join per D-07)
		if hit.ASN != 13335 {
			t.Errorf("hit.ASN = %d, want %d", hit.ASN, 13335)
		}
		return
	}
	t.Error("Networks group not found in results")
}

func TestSearchIXPDetailURL(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	results, err := svc.Search(ctx, "DE-CIX")
	if err != nil {
		t.Fatalf("Search for DE-CIX: %v", err)
	}
	if len(results) != 1 || len(results[0].Results) != 1 {
		t.Fatal("expected exactly 1 IXP result")
	}

	hit := results[0].Results[0]
	if hit.DetailURL != "/ui/ix/20" {
		t.Errorf("hit.DetailURL = %q, want %q", hit.DetailURL, "/ui/ix/20")
	}
	if hit.Country != "DE" {
		t.Errorf("hit.Country = %q, want %q", hit.Country, "DE")
	}
	if hit.City != "Frankfurt" {
		t.Errorf("hit.City = %q, want %q", hit.City, "Frankfurt")
	}
}

func TestSearchFacilityMetadata(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	results, err := svc.Search(ctx, "Equinix DC5")
	if err != nil {
		t.Fatalf("Search for Equinix DC5: %v", err)
	}

	for _, r := range results {
		if r.TypeName != "Facilities" {
			continue
		}
		if len(r.Results) == 0 {
			t.Fatal("Facilities group has no results")
		}
		hit := r.Results[0]
		if hit.DetailURL != "/ui/fac/30" {
			t.Errorf("hit.DetailURL = %q, want %q", hit.DetailURL, "/ui/fac/30")
		}
		if hit.Country != "US" {
			t.Errorf("hit.Country = %q, want %q", hit.Country, "US")
		}
		if hit.City != "Ashburn" {
			t.Errorf("hit.City = %q, want %q", hit.City, "Ashburn")
		}
		return
	}
	t.Error("Facilities group not found in results")
}

func TestSearchCampusDetailURL(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	results, err := svc.Search(ctx, "Campus Ashburn")
	if err != nil {
		t.Fatalf("Search for Campus Ashburn: %v", err)
	}

	for _, r := range results {
		if r.TypeName != "Campuses" {
			continue
		}
		if len(r.Results) == 0 {
			t.Fatal("Campuses group has no results")
		}
		hit := r.Results[0]
		if hit.DetailURL != "/ui/campus/40" {
			t.Errorf("hit.DetailURL = %q, want %q", hit.DetailURL, "/ui/campus/40")
		}
		if hit.Country != "US" {
			t.Errorf("hit.Country = %q, want %q", hit.Country, "US")
		}
		if hit.City != "Ashburn" {
			t.Errorf("hit.City = %q, want %q", hit.City, "Ashburn")
		}
		return
	}
	t.Error("Campuses group not found in results")
}

func TestSearchCarrierDetailURL(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	results, err := svc.Search(ctx, "Zayo")
	if err != nil {
		t.Fatalf("Search for Zayo: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 type group for 'Zayo', got %d", len(results))
	}
	if results[0].TypeName != "Carriers" {
		t.Errorf("expected TypeName 'Carriers', got %q", results[0].TypeName)
	}
	if len(results[0].Results) == 0 {
		t.Fatal("Carriers group has no results")
	}
	hit := results[0].Results[0]
	if hit.DetailURL != "/ui/carrier/50" {
		t.Errorf("hit.DetailURL = %q, want %q", hit.DetailURL, "/ui/carrier/50")
	}
	if hit.Country != "" {
		t.Errorf("hit.Country = %q, want empty string", hit.Country)
	}
	if hit.City != "" {
		t.Errorf("hit.City = %q, want empty string", hit.City)
	}
	if hit.ASN != 0 {
		t.Errorf("hit.ASN = %d, want 0", hit.ASN)
	}
}

func TestSearchOrganizationDetailURL(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	// "Cloudflare Inc" matches the Organization name exactly
	results, err := svc.Search(ctx, "Cloudflare Inc")
	if err != nil {
		t.Fatalf("Search for Cloudflare Inc: %v", err)
	}

	for _, r := range results {
		if r.TypeName != "Organizations" {
			continue
		}
		if len(r.Results) == 0 {
			t.Fatal("Organizations group has no results")
		}
		hit := r.Results[0]
		if hit.DetailURL != "/ui/org/1" {
			t.Errorf("hit.DetailURL = %q, want %q", hit.DetailURL, "/ui/org/1")
		}
		if hit.Country != "US" {
			t.Errorf("hit.Country = %q, want %q", hit.Country, "US")
		}
		if hit.City != "San Francisco" {
			t.Errorf("hit.City = %q, want %q", hit.City, "San Francisco")
		}
		return
	}
	t.Error("Organizations group not found in results")
}

func TestSearchNetworkASNField(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	results, err := svc.Search(ctx, "Cloudflare")
	if err != nil {
		t.Fatalf("Search for Cloudflare: %v", err)
	}

	for _, r := range results {
		if r.TypeName != "Networks" {
			continue
		}
		if len(r.Results) == 0 {
			t.Fatal("Networks group has no results")
		}
		hit := r.Results[0]
		if hit.ASN != 13335 {
			t.Errorf("hit.ASN = %d, want %d", hit.ASN, 13335)
		}
		// Networks have no direct Country/City (no org join per D-07)
		if hit.Country != "" {
			t.Errorf("hit.Country = %q, want empty string (no org join)", hit.Country)
		}
		if hit.City != "" {
			t.Errorf("hit.City = %q, want empty string (no org join)", hit.City)
		}
		return
	}
	t.Error("Networks group not found in results")
}

func TestSearchHasMore(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()

	// Create organization
	org, err := client.Organization.Create().
		SetID(1).
		SetName("Parent Org").
		SetCreated(testSearchTimestamp).
		SetUpdated(testSearchTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	// Create 15 networks (more than the 10-result limit)
	for i := range 15 {
		_, err := client.Network.Create().
			SetID(100 + i).
			SetName("TestNet" + string(rune('A'+i))).
			SetAsn(64500 + i).
			SetOrgID(1).
			SetOrganization(org).
			SetCreated(testSearchTimestamp).
			SetUpdated(testSearchTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating network %d: %v", i, err)
		}
	}

	svc := NewSearchService(client)
	results, err := svc.Search(ctx, "TestNet")
	if err != nil {
		t.Fatalf("Search for TestNet: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 type group, got %d", len(results))
	}

	r := results[0]
	if r.TypeName != "Networks" {
		t.Errorf("TypeName = %q, want 'Networks'", r.TypeName)
	}
	if len(r.Results) != 10 {
		t.Errorf("len(Results) = %d, want 10 (capped)", len(r.Results))
	}
	if !r.HasMore {
		t.Error("HasMore = false, want true (15 matches exceeds display limit)")
	}
}

func TestSearchTypeOrder(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()

	// Create org
	org, err := client.Organization.Create().
		SetID(1).
		SetName("XSearch Org").
		SetCreated(testSearchTimestamp).
		SetUpdated(testSearchTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	// Create one of each type with "XSearch" in the name
	_, err = client.Network.Create().
		SetID(10).SetName("XSearch Net").SetAsn(65000).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testSearchTimestamp).SetUpdated(testSearchTimestamp).Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}
	_, err = client.InternetExchange.Create().
		SetID(20).SetName("XSearch IX").
		SetOrgID(1).SetOrganization(org).
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetCreated(testSearchTimestamp).SetUpdated(testSearchTimestamp).Save(ctx)
	if err != nil {
		t.Fatalf("creating ix: %v", err)
	}
	_, err = client.Facility.Create().
		SetID(30).SetName("XSearch Fac").
		SetOrgID(1).SetOrganization(org).SetClli("XSRCH01").
		SetCreated(testSearchTimestamp).SetUpdated(testSearchTimestamp).Save(ctx)
	if err != nil {
		t.Fatalf("creating facility: %v", err)
	}
	_, err = client.Campus.Create().
		SetID(40).SetName("XSearch Campus").
		SetOrgID(1).SetOrganization(org).
		SetCreated(testSearchTimestamp).SetUpdated(testSearchTimestamp).Save(ctx)
	if err != nil {
		t.Fatalf("creating campus: %v", err)
	}
	_, err = client.Carrier.Create().
		SetID(50).SetName("XSearch Carrier").
		SetOrgID(1).SetOrganization(org).
		SetCreated(testSearchTimestamp).SetUpdated(testSearchTimestamp).Save(ctx)
	if err != nil {
		t.Fatalf("creating carrier: %v", err)
	}

	svc := NewSearchService(client)
	results, err := svc.Search(ctx, "XSearch")
	if err != nil {
		t.Fatalf("Search for XSearch: %v", err)
	}

	// Expected order: Networks, IXPs, Facilities, Organizations, Campuses, Carriers
	wantOrder := []string{"Networks", "IXPs", "Facilities", "Organizations", "Campuses", "Carriers"}
	if len(results) != len(wantOrder) {
		t.Fatalf("expected %d type groups, got %d", len(wantOrder), len(results))
	}
	for i, want := range wantOrder {
		if results[i].TypeName != want {
			t.Errorf("results[%d].TypeName = %q, want %q", i, results[i].TypeName, want)
		}
	}
}

func TestSearchService_DBError(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	svc := NewSearchService(client)

	// Close the client to trigger DB error on the next query.
	client.Close()

	_, err := svc.Search(t.Context(), "test")
	if err == nil {
		t.Fatal("expected error for closed DB, got nil")
	}
	if !strings.Contains(err.Error(), "search") {
		t.Errorf("error = %q, want substring %q", err.Error(), "search")
	}
}

func TestSearchNetworkByASN(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	// The seeded network has Asn=13335 and name="Cloudflare" with no digits
	// in its text fields, so these queries exercise the asn-equality branch.
	cases := []struct {
		name     string
		query    string
		wantHit  bool
		wantASN  int
		wantName string
	}{
		{"digits only", "13335", true, 13335, "Cloudflare"},
		{"uppercase AS prefix", "AS13335", true, 13335, "Cloudflare"},
		{"lowercase as prefix", "as13335", true, 13335, "Cloudflare"},
		{"mixed case aS prefix", "aS13335", true, 13335, "Cloudflare"},
		{"prefix-of-asn must not match", "1333", false, 0, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results, err := svc.Search(ctx, tc.query)
			if err != nil {
				t.Fatalf("Search(%q): %v", tc.query, err)
			}
			var nets []SearchHit
			for _, r := range results {
				if r.TypeSlug == "net" {
					nets = r.Results
					break
				}
			}
			if !tc.wantHit {
				if len(nets) != 0 {
					t.Fatalf("Search(%q) expected no network hits, got %d", tc.query, len(nets))
				}
				return
			}
			if len(nets) != 1 {
				t.Fatalf("Search(%q) expected 1 network hit, got %d", tc.query, len(nets))
			}
			hit := nets[0]
			if hit.ASN != tc.wantASN {
				t.Errorf("Search(%q) ASN = %d, want %d", tc.query, hit.ASN, tc.wantASN)
			}
			if hit.Name != tc.wantName {
				t.Errorf("Search(%q) Name = %q, want %q", tc.query, hit.Name, tc.wantName)
			}
			if hit.DetailURL != fmt.Sprintf("/ui/asn/%d", tc.wantASN) {
				t.Errorf("Search(%q) DetailURL = %q, want /ui/asn/%d", tc.query, hit.DetailURL, tc.wantASN)
			}
		})
	}
}

func TestSearchTextStillWorks(t *testing.T) {
	t.Parallel()
	svc := seedSearchData(t)
	ctx := t.Context()

	// Regression: non-numeric queries must still hit the text-field predicate.
	results, err := svc.Search(ctx, "cloudflare")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var found bool
	for _, r := range results {
		if r.TypeSlug == "net" && len(r.Results) == 1 && r.Results[0].ASN == 13335 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("regression: text search for 'cloudflare' no longer finds AS13335")
	}
}

func TestParseASNQuery(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		wantN   int64
		wantOK  bool
	}{
		{"8075", 8075, true},
		{"AS8075", 8075, true},
		{"as8075", 8075, true},
		{"aS8075", 8075, true},
		{"  8075  ", 8075, true},
		{"1", 1, true},
		{"4294967295", 4294967295, true},
		{"0", 0, false},
		{"-5", 0, false},
		{"AS0", 0, false},
		{"4294967296", 0, false},
		{"", 0, false},
		{"   ", 0, false},
		{"AS", 0, false},
		{"as", 0, false},
		{"abc", 0, false},
		{"8075abc", 0, false},
		{"AS8075x", 0, false},
		{"cloudflare", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			n, ok := parseASNQuery(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("parseASNQuery(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
			}
			if n != tc.wantN {
				t.Errorf("parseASNQuery(%q) n = %d, want %d", tc.in, n, tc.wantN)
			}
		})
	}
}
