package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestFragment_Pagination locks the fragment page bound: a relation
// larger than fragmentPageSize returns exactly one page plus a "Load
// more" row, the offset request returns the remainder without a
// button, and an out-of-range offset renders nothing (no crash).
func TestFragment_Pagination(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()

	org, err := client.Organization.Create().
		SetID(1).SetName("PageOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}
	total := fragmentPageSize + 25
	for i := range total {
		_, err := client.Network.Create().
			SetID(1000 + i).SetName(fmt.Sprintf("Net %04d", i)).SetAsn(64512 + i).
			SetOrgID(1).SetOrganization(org).
			SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
			Save(ctx)
		if err != nil {
			t.Fatalf("creating network %d: %v", i, err)
		}
	}

	h := NewHandler(NewHandlerInput{Client: client})
	mux := http.NewServeMux()
	h.Register(mux)

	get := func(url string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d", url, rec.Code)
		}
		return rec.Body.String()
	}

	// Page 1: fragmentPageSize rows and a load-more row pointing at the offset.
	body := get("/ui/fragment/org/1/networks")
	// thead row + fragmentPageSize data rows + the load-more row.
	if got := strings.Count(body, "<tr"); got != fragmentPageSize+2 {
		t.Errorf("first page rows = %d, want %d", got, fragmentPageSize+2)
	}
	if !strings.Contains(body, fmt.Sprintf("offset=%d", fragmentPageSize)) {
		t.Error("first page missing load-more URL with next offset")
	}
	if !strings.Contains(body, "Load more") {
		t.Error("first page missing Load more button")
	}

	// Page 2: the remaining rows, no further button.
	body2 := get(fmt.Sprintf("/ui/fragment/org/1/networks?offset=%d", fragmentPageSize))
	if strings.Contains(body2, "Load more") {
		t.Error("final page still renders Load more")
	}
	if got := strings.Count(body2, "<tr"); got != total-fragmentPageSize {
		t.Errorf("final page rows = %d, want %d", got, total-fragmentPageSize)
	}
	if strings.Contains(body2, "<table") {
		t.Error("offset page must render bare rows, not a fresh table")
	}

	// Out-of-range offset: empty response, not an error.
	body3 := get("/ui/fragment/org/1/networks?offset=100000")
	if strings.Contains(body3, "<tr") {
		t.Errorf("out-of-range offset returned rows: %q", body3)
	}
}
