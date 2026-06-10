package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// seedTombstoneData seeds ok/deleted sibling rows so tests can assert that
// soft-deleted tombstones (status='deleted', written by the sync worker for
// upstream deletions) never render as live data in the web UI.
func seedTombstoneData(t *testing.T, client *ent.Client) {
	t.Helper()
	ctx := t.Context()

	org, err := client.Organization.Create().
		SetID(1).SetName("TombOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	// (a)+(b): an ok network and a deleted sibling with a distinct ASN/name.
	liveNet, err := client.Network.Create().
		SetID(10).SetName("TombNet Live").SetAsn(65001).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating live network: %v", err)
	}

	_, err = client.Network.Create().
		SetID(11).SetName("TombNet Gone").SetAsn(65002).
		SetOrgID(1).SetOrganization(org).
		SetStatus("deleted").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating deleted network: %v", err)
	}

	// (c): an IX with one ok participant and one deleted participant.
	ix, err := client.InternetExchange.Create().
		SetID(20).SetName("Tomb IX").
		SetOrgID(1).SetOrganization(org).
		SetCity("Frankfurt").SetCountry("DE").
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ix: %v", err)
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
		SetNetID(liveNet.ID).SetNetwork(liveNet).
		SetIxlanID(100).SetIxLan(ixlanEntity).
		SetAsn(65001).SetSpeed(10000).
		SetName("Tomb IX").SetIxID(20).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ok networkixlan: %v", err)
	}

	_, err = client.NetworkIxLan.Create().
		SetID(201).
		SetNetID(liveNet.ID).SetNetwork(liveNet).
		SetIxlanID(100).SetIxLan(ixlanEntity).
		SetAsn(64512).SetSpeed(10000).
		SetName("Tomb IX").SetIxID(20).
		SetStatus("deleted").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating deleted networkixlan: %v", err)
	}
}

// setupTombstoneMux seeds tombstone test data and returns a mux ready for testing.
func setupTombstoneMux(t *testing.T) *http.ServeMux {
	t.Helper()
	client := testutil.SetupClient(t)
	seedTombstoneData(t, client)
	h := NewHandler(NewHandlerInput{Client: client})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

// TestTombstones_ExcludedFromWebUI verifies that status='deleted' rows do not
// render as live data, while ok rows stay visible.
func TestTombstones_ExcludedFromWebUI(t *testing.T) {
	t.Parallel()
	mux := setupTombstoneMux(t)

	tests := []struct {
		name     string
		url      string
		wantCode int
		wantBody []string
		noBody   []string
	}{
		{
			name:     "search excludes deleted network",
			url:      "/ui/search?q=TombNet",
			wantCode: http.StatusOK,
			wantBody: []string{"TombNet Live"},
			noBody:   []string{"TombNet Gone"},
		},
		{
			name:     "live network detail by ASN renders",
			url:      "/ui/asn/65001",
			wantCode: http.StatusOK,
			wantBody: []string{"TombNet Live"},
		},
		{
			name:     "deleted network detail by ASN is not found",
			url:      "/ui/asn/65002",
			wantCode: http.StatusNotFound,
			noBody:   []string{"TombNet Gone"},
		},
		{
			name:     "ix participants fragment excludes deleted netixlan",
			url:      "/ui/fragment/ix/20/participants",
			wantCode: http.StatusOK,
			wantBody: []string{"65001"},
			noBody:   []string{"64512"},
		},
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
