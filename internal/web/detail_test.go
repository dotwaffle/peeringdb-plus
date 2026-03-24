package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// seedNetworkTestData creates a minimal org + network for detail page tests.
// Returns the network's internal ID for use in fragment endpoint tests.
func seedNetworkTestData(t *testing.T, mux *http.ServeMux) (*http.ServeMux, int) {
	t.Helper()
	client := testutil.SetupClient(t)
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
		SetIxCount(2).SetFacCount(3).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	h := NewHandler(client)
	m := http.NewServeMux()
	h.Register(m)
	return m, net.ID
}

func TestNetworkDetail_FullPage(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := context.Background()

	org, err := client.Organization.Create().
		SetID(1).SetName("TestOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	_, err = client.Network.Create().
		SetID(10).SetName("Cloudflare").SetAsn(13335).
		SetOrgID(1).SetOrganization(org).
		SetIxCount(2).SetFacCount(3).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/ui/asn/13335", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	checks := []string{
		"Cloudflare",
		"AS13335",
		"(2)",
		"(3)",
		"<!doctype html>",
		"hx-get",
		"fragment/net/",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("full page response missing %q", want)
		}
	}
}

func TestNetworkDetail_HtmxFragment(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := context.Background()

	org, err := client.Organization.Create().
		SetID(1).SetName("TestOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	_, err = client.Network.Create().
		SetID(10).SetName("Cloudflare").SetAsn(13335).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)

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

func TestNetworkDetail_NotFound(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/asn/99999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestNetworkDetail_InvalidASN(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/asn/abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestNetworkDetail_OrgLink(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := context.Background()

	org, err := client.Organization.Create().
		SetID(1).SetName("TestOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	_, err = client.Network.Create().
		SetID(10).SetName("Cloudflare").SetAsn(13335).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/ui/asn/13335", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "/ui/org/") {
		t.Error("network detail page should contain org link")
	}
	if !strings.Contains(body, "TestOrg") {
		t.Error("network detail page should contain org name")
	}
}

func TestNetworkFragment_IXLans(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
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
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	_, err = client.InternetExchange.Create().
		SetID(20).SetName("DE-CIX Frankfurt").
		SetOrgID(1).SetOrganization(org).
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ix: %v", err)
	}

	_, err = client.IxLan.Create().
		SetID(100).SetIxID(20).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ixlan: %v", err)
	}

	_, err = client.NetworkIxLan.Create().
		SetID(200).
		SetNetID(net.ID).SetNetwork(net).
		SetIxlanID(100).
		SetAsn(13335).SetSpeed(10000).
		SetName("DE-CIX Frankfurt").SetIxID(20).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkixlan: %v", err)
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/ui/fragment/net/10/ixlans", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "<!doctype html>") || strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("fragment should not contain DOCTYPE (no layout wrapper)")
	}
	if !strings.Contains(body, "DE-CIX Frankfurt") {
		t.Error("fragment should contain IX name 'DE-CIX Frankfurt'")
	}
	if !strings.Contains(body, "/ui/ix/") {
		t.Error("fragment should contain cross-link to IX detail page")
	}
	if !strings.Contains(body, "10G") {
		t.Error("fragment should contain formatted speed '10G'")
	}
}

func TestNetworkFragment_Facilities(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
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
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	_, err = client.Facility.Create().
		SetID(30).SetName("Equinix FR5").
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating facility: %v", err)
	}

	_, err = client.NetworkFacility.Create().
		SetID(300).
		SetNetID(net.ID).SetNetwork(net).
		SetFacID(30).
		SetLocalAsn(13335).
		SetName("Equinix FR5").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating networkfacility: %v", err)
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/ui/fragment/net/10/facilities", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Equinix FR5") {
		t.Error("fragment should contain facility name 'Equinix FR5'")
	}
	if !strings.Contains(body, "/ui/fac/") {
		t.Error("fragment should contain cross-link to facility detail page")
	}
}

func TestNetworkFragment_Contacts(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
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
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	_, err = client.Poc.Create().
		SetID(400).
		SetNetID(net.ID).SetNetwork(net).
		SetName("NOC Contact").SetRole("NOC").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating poc: %v", err)
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/ui/fragment/net/10/contacts", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "NOC Contact") {
		t.Error("fragment should contain contact name 'NOC Contact'")
	}
	if !strings.Contains(body, "NOC") {
		t.Error("fragment should contain contact role 'NOC'")
	}
}
