package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestTimeFilterSemantics locks upstream's date filter handling
// (rest.py:619-658): time filters accept ISO 8601 alongside epoch
// seconds; a bare 10-char date in gt/lte gets its time forced to
// end-of-day (rest.py:621-623); and bare date equality matches the
// whole day (the __startswith rewrite at rest.py:657-658).
func TestTimeFilterSemantics(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()

	day1 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

	for i, ts := range []time.Time{day1, day2} {
		id := i + 1
		if _, err := client.Network.Create().
			SetID(id).SetName(fmt.Sprintf("TimeNet%d", id)).
			SetNameFold(unifold.Fold("TimeNet")).
			SetAsn(65000 + id).
			SetCreated(ts).SetUpdated(ts).SetStatus("ok").
			Save(ctx); err != nil {
			t.Fatalf("seed net %d: %v", id, err)
		}
	}

	h := NewHandler(client, 0)
	mux := http.NewServeMux()
	h.Register(mux)

	fetchIDs := func(t *testing.T, path string) ([]int, int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			return nil, rec.Code
		}
		var env struct {
			Data []struct {
				ID int `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		ids := make([]int, len(env.Data))
		for i, d := range env.Data {
			ids[i] = d.ID
		}
		return ids, rec.Code
	}

	cases := []struct {
		name, path string
		want       []int
	}{
		// rest.py:657-658: bare date equality = whole-day window.
		{"bare date equality is day window", "/api/net?updated=2026-04-01", []int{1}},
		// rest.py:621-623: gt on a date means "after that whole day".
		{"gt date is end of day", "/api/net?updated__gt=2026-04-01", []int{2}},
		// rest.py:621-623: lte on a date includes the whole day.
		{"lte date includes whole day", "/api/net?updated__lte=2026-04-01", []int{1}},
		// gte keeps start-of-day (only gt/lte adjust upstream).
		{"gte date is start of day", "/api/net?updated__gte=2026-04-01", []int{1, 2}},
		// RFC 3339 instants accepted.
		{"rfc3339 gt", "/api/net?updated__gt=2026-04-01T10%3A00%3A00Z", []int{2}},
		// Epoch seconds still accepted.
		{"epoch equality", fmt.Sprintf("/api/net?updated=%d", day1.Unix()), []int{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ids, code := fetchIDs(t, tc.path)
			if code != http.StatusOK {
				t.Fatalf("%s: status %d", tc.path, code)
			}
			got := map[int]bool{}
			for _, id := range ids {
				got[id] = true
			}
			if len(ids) != len(tc.want) {
				t.Fatalf("%s: got %v, want %v", tc.path, ids, tc.want)
			}
			for _, id := range tc.want {
				if got[id] {
					continue
				}
				t.Errorf("%s: got %v, want %v", tc.path, ids, tc.want)
			}
		})
	}

	t.Run("garbage time value still 400s", func(t *testing.T) {
		t.Parallel()
		_, code := fetchIDs(t, "/api/net?updated__gt=not-a-time")
		if code != http.StatusBadRequest {
			t.Errorf("garbage time value: status %d, want 400", code)
		}
	})

	t.Run("since stays epoch-only", func(t *testing.T) {
		t.Parallel()
		// upstream coerces since with int() — ISO values 400 there, so
		// they must 400 here too rather than silently working.
		_, code := fetchIDs(t, "/api/net?since=2026-04-01")
		if code != http.StatusBadRequest {
			t.Errorf("ISO since value: status %d, want 400", code)
		}
	})
}
