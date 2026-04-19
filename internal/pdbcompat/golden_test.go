package pdbcompat

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"maps"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

var update = flag.Bool("update", false, "update golden files")

// goldenTime is the fixed timestamp used for all golden test entities.
// Using a fixed time ensures deterministic output across runs.
var goldenTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// compareOrUpdate either updates the golden file (when -update is set) or
// compares the got bytes against the existing golden file content.
func compareOrUpdate(t *testing.T, goldenPath string, got []byte) {
	t.Helper()

	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("create golden dir %s: %v", filepath.Dir(goldenPath), err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden file %s: %v", goldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v (run with -update to create)", goldenPath, err)
	}
	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("golden mismatch for %s (-want +got):\n%s", goldenPath, diff)
	}
}

// setupGoldenTestData creates all 13 PeeringDB entity types with fixed IDs
// and timestamps for deterministic golden file output. Returns the mux with
// the handler registered and a map of type name to entity ID for detail/depth
// requests.
func setupGoldenTestData(t *testing.T) (*http.ServeMux, map[string]int) {
	t.Helper()
	client := testutil.SetupClient(t)
	ctx := t.Context()

	// 1. Organization (ID=100)
	org, err := client.Organization.Create().
		SetID(100).
		SetName("Golden Org").
		SetAka("GO").
		SetNameLong("Golden Organization Inc").
		SetWebsite("https://golden-org.example.com").
		SetAddress1("100 Main St").
		SetCity("San Francisco").
		SetState("CA").
		SetCountry("US").
		SetZipcode("94105").
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// 2. Campus (ID=200)
	campus, err := client.Campus.Create().
		SetID(200).
		SetName("Golden Campus").
		SetOrganization(org).
		SetCity("San Francisco").
		SetCountry("US").
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create campus: %v", err)
	}

	// 3. Facility (ID=300)
	fac, err := client.Facility.Create().
		SetID(300).
		SetName("Golden Facility").
		SetOrganization(org).
		SetCampus(campus).
		SetAddress1("300 Data Center Way").
		SetCity("San Jose").
		SetState("CA").
		SetCountry("US").
		SetZipcode("95113").
		SetLatitude(37.5).
		SetLongitude(-122.5).
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create facility: %v", err)
	}

	// 4. Network (ID=400)
	net, err := client.Network.Create().
		SetID(400).
		SetName("Golden Net").
		SetAsn(65000).
		SetOrganization(org).
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	// 5. InternetExchange (ID=500)
	ix, err := client.InternetExchange.Create().
		SetID(500).
		SetName("Golden IX").
		SetOrganization(org).
		SetCity("San Francisco").
		SetCountry("US").
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create ix: %v", err)
	}

	// 6. IxLan (ID=600)
	ixlan, err := client.IxLan.Create().
		SetID(600).
		SetInternetExchange(ix).
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create ixlan: %v", err)
	}

	// 7. Carrier (ID=700)
	carrier, err := client.Carrier.Create().
		SetID(700).
		SetName("Golden Carrier").
		SetOrganization(org).
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create carrier: %v", err)
	}

	// 8. Poc (ID=800)
	_, err = client.Poc.Create().
		SetID(800).
		SetName("Golden Contact").
		SetRole("Abuse").
		SetNetwork(net).
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create poc: %v", err)
	}

	// 9. IxPrefix (ID=900)
	_, err = client.IxPrefix.Create().
		SetID(900).
		SetIxLan(ixlan).
		SetProtocol("IPv4").
		SetPrefix("192.0.2.0/24").
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create ixpfx: %v", err)
	}

	// 10. NetworkFacility (ID=1000)
	_, err = client.NetworkFacility.Create().
		SetID(1000).
		SetNetwork(net).
		SetFacility(fac).
		SetLocalAsn(65000).
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create netfac: %v", err)
	}

	// 11. NetworkIxLan (ID=1100)
	_, err = client.NetworkIxLan.Create().
		SetID(1100).
		SetNetwork(net).
		SetIxLan(ixlan).
		SetIxID(500).
		SetSpeed(10000).
		SetAsn(65000).
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create netixlan: %v", err)
	}

	// 12. IxFacility (ID=1200)
	_, err = client.IxFacility.Create().
		SetID(1200).
		SetInternetExchange(ix).
		SetFacility(fac).
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create ixfac: %v", err)
	}

	// 13. CarrierFacility (ID=1300)
	_, err = client.CarrierFacility.Create().
		SetID(1300).
		SetCarrier(carrier).
		SetFacility(fac).
		SetCreated(goldenTime).
		SetUpdated(goldenTime).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create carrierfac: %v", err)
	}

	h := NewHandler(client, 0)
	mux := http.NewServeMux()
	h.Register(mux)

	ids := map[string]int{
		"org":        100,
		"campus":     200,
		"fac":        300,
		"net":        400,
		"ix":         500,
		"ixlan":      600,
		"carrier":    700,
		"poc":        800,
		"ixpfx":      900,
		"netfac":     1000,
		"netixlan":   1100,
		"ixfac":      1200,
		"carrierfac": 1300,
	}
	return mux, ids
}

// TestGoldenFiles verifies that the PeeringDB compat layer JSON output matches
// committed golden files for all 13 types across list, detail, and
// depth-expanded scenarios. Run with -update to regenerate golden files.
func TestGoldenFiles(t *testing.T) {
	t.Parallel()

	mux, ids := setupGoldenTestData(t)

	// Sorted type names for deterministic test order.
	typeNames := slices.Sorted(maps.Keys(Registry))

	scenarios := []struct {
		name   string
		urlFmt string // %s = type name, %d = entity ID (unused for list)
	}{
		{"list", "/api/%s"},
		{"detail", "/api/%s/%d"},
		{"depth", "/api/%s/%d?depth=2"},
	}

	for _, typeName := range typeNames {
		for _, scenario := range scenarios {
			t.Run(typeName+"/"+scenario.name, func(t *testing.T) {
				t.Parallel()

				var url string
				switch scenario.name {
				case "list":
					url = fmt.Sprintf(scenario.urlFmt, typeName)
				default:
					id, ok := ids[typeName]
					if !ok {
						t.Fatalf("no ID for type %q", typeName)
					}
					url = fmt.Sprintf(scenario.urlFmt, typeName, id)
				}

				req := httptest.NewRequest(http.MethodGet, url, nil)
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
				}

				goldenPath := filepath.Join("testdata", "golden", typeName, scenario.name+".json")
				compareOrUpdate(t, goldenPath, rec.Body.Bytes())
			})
		}
	}
}
