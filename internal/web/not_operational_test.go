package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestFragments_ListNotOperationalConnections locks that the network IX
// list and the IX participant list keep a netixlan whose status is
// not-operational. PeeringDB 2.83.0 treats that status as live and lists
// the connection in both views (docs/api/obj_netixlan.md:50-54).
func TestFragments_ListNotOperationalConnections(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()

	org := client.Organization.Create().SetID(1).SetName("Status Org").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).SaveX(ctx)
	net := client.Network.Create().SetID(10).SetName("Dark Net").SetAsn(65002).
		SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).SaveX(ctx)
	ix := client.InternetExchange.Create().SetID(20).SetName("Dark IX").
		SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).SaveX(ctx)
	lan := client.IxLan.Create().SetID(100).SetInternetExchange(ix).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).SaveX(ctx)
	client.NetworkIxLan.Create().SetID(200).
		SetNetwork(net).SetIxLan(lan).SetIxID(20).SetName("Dark IX").
		SetAsn(65002).SetSpeed(10_000).
		SetStatus("not-operational").SetOperational(false).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).SaveX(ctx)

	h := NewHandler(NewHandlerInput{Client: client})
	mux := http.NewServeMux()
	h.Register(mux)

	for _, tt := range []struct {
		name, url, want string
	}{
		{"network IX list", "/ui/fragment/net/10/ixlans", "Dark IX"},
		{"IX participants", "/ui/fragment/ix/20/participants", "AS65002"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.url, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "<table") || !strings.Contains(body, tt.want) {
				t.Errorf("fragment does not list the not-operational row %q:\n%s", tt.want, body)
			}
		})
	}
}
