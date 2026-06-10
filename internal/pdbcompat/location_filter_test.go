package pdbcompat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestLocationFilterCoercion locks the upstream rest.py:562-574 location
// special cases: bare ?city= / ?address1= / ?state= are substring
// (icontains) matches, and bare ?country= is iexact for 2-char values /
// icontains for longer. Explicit operators are untouched.
func TestLocationFilterCoercion(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	org := client.Organization.Create().
		SetName("LocOrg").SetNameFold(unifold.Fold("LocOrg")).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetName("FRA1").SetNameFold(unifold.Fold("FRA1")).
		SetOrganization(org).
		SetCity("Frankfurt am Main").SetCityFold(unifold.Fold("Frankfurt am Main")).
		SetState("Hessen").SetAddress1("Kleyerstrasse 90").SetCountry("DE").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetName("LON1").SetNameFold(unifold.Fold("LON1")).
		SetOrganization(org).
		SetCity("London").SetCityFold(unifold.Fold("London")).
		SetState("Greater London").SetAddress1("Braham Street 6").SetCountry("GB").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	h := NewHandler(client, 0)
	mux := http.NewServeMux()
	h.Register(mux)

	fetchNames := func(t *testing.T, path string) []string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d: %s", path, rec.Code, rec.Body.String())
		}
		var env struct {
			Data []struct {
				Name string `json:"name"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		names := make([]string, len(env.Data))
		for i, d := range env.Data {
			names[i] = d.Name
		}
		return names
	}

	cases := []struct {
		name, path string
		want       []string
	}{
		// upstream rest.py:562-563: bare city is icontains.
		{"city substring", "/api/fac?city=Frankfurt", []string{"FRA1"}},
		{"city substring case-insensitive", "/api/fac?city=frankfurt", []string{"FRA1"}},
		// upstream rest.py:562-563: bare state is icontains.
		{"state substring", "/api/fac?state=London", []string{"LON1"}},
		// upstream rest.py:562-563: bare address1 is icontains.
		{"address1 substring", "/api/fac?address1=Kleyerstrasse", []string{"FRA1"}},
		// upstream rest.py:565-574: 2-char country is iexact.
		{"country 2char iexact", "/api/fac?country=de", []string{"FRA1"}},
		// upstream rest.py:565-574: longer country is icontains... of the
		// stored 2-char code, so a full name matches nothing — but a
		// 1-char fragment matches both rows containing it.
		{"country fragment icontains", "/api/fac?country=B", []string{"LON1"}},
		// Explicit operators keep generic semantics (exact stays exact).
		{"explicit contains untouched", "/api/fac?city__contains=furt", []string{"FRA1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := fetchNames(t, tc.path)
			if len(got) != len(tc.want) {
				t.Fatalf("%s: got %v, want %v", tc.path, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("%s: got %v, want %v", tc.path, got, tc.want)
				}
			}
		})
	}
}
