package main

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	syncpkg "github.com/dotwaffle/peeringdb-plus/internal/sync"
)

// TestSync_OrderingMatchesWorker is the parity guard: the loadtest's
// syncOrder MUST match internal/sync.StepOrder() exactly. A future
// reorder of syncSteps() in the live worker fails this test, forcing
// the loadtest to track the change.
func TestSync_OrderingMatchesWorker(t *testing.T) {
	t.Parallel()

	want := syncpkg.StepOrder()
	if !reflect.DeepEqual(syncOrder, want) {
		t.Fatalf("syncOrder mismatch:\n  loadtest=%v\n  worker  =%v", syncOrder, want)
	}
	if len(syncOrder) != 13 {
		t.Fatalf("syncOrder has %d entries, want 13", len(syncOrder))
	}
}

// TestSync_BuildSyncEndpointsFull asserts full-mode produces 39
// pdbcompat GETs (13 types × 3 depths) against
// /api/<short>?depth=N&limit=0. The `limit=0` sentinel is the
// upstream-compatible "unlimited" marker (parity-locked by
// internal/pdbcompat/parity/limit_test.go LIMIT-01); without it the
// mirror's DefaultLimit=250 would cap each response at 250 rows,
// defeating the purpose of a full-sync loadtest. See the
// loadtest-body-size-mismatch debug session (.planning/debug/, 2026-04-28)
// for the root-cause investigation.
func TestSync_BuildSyncEndpointsFull(t *testing.T) {
	t.Parallel()

	eps := buildSyncEndpoints("full", time.Time{})
	wantCount := len(syncOrder) * len(syncDepths)
	if len(eps) != wantCount {
		t.Fatalf("got %d endpoints, want %d (%d types × %d depths)",
			len(eps), wantCount, len(syncOrder), len(syncDepths))
	}
	// Each depth band issues all 13 types in syncOrder. Bands are
	// emitted in the order syncDepths declares.
	for bandIdx, depth := range syncDepths {
		for typeIdx, ty := range syncOrder {
			i := bandIdx*len(syncOrder) + typeIdx
			ep := eps[i]
			if ep.Surface != SurfacePdbCompat {
				t.Errorf("ep[%d].Surface = %q, want pdbcompat", i, ep.Surface)
			}
			if ep.Method != "GET" {
				t.Errorf("ep[%d].Method = %q, want GET", i, ep.Method)
			}
			want := fmt.Sprintf("/api/%s?depth=%d&limit=0", ty, depth)
			if ep.Path != want {
				t.Errorf("ep[%d].Path = %q, want %q", i, ep.Path, want)
			}
			if strings.Contains(ep.Path, "since=") {
				t.Errorf("ep[%d].Path = %q: full mode should not have since=",
					i, ep.Path)
			}
			if strings.Contains(ep.Path, "skip=") {
				t.Errorf("ep[%d].Path = %q: full mode must NOT include "+
					"skip (a single unbounded request, not paginated)",
					i, ep.Path)
			}
			if !strings.Contains(ep.Path, "limit=0") {
				t.Errorf("ep[%d].Path = %q: full mode MUST include "+
					"limit=0 (mirror DefaultLimit=250 would cap "+
					"otherwise — see LIMIT-01 / debug-session "+
					"loadtest-body-size-mismatch)", i, ep.Path)
			}
		}
	}
}

// TestSync_BuildSyncEndpointsIncremental asserts incremental mode
// appends &since=<unix> to every URL across all 39 (type, depth)
// permutations, using the provided cursor.
func TestSync_BuildSyncEndpointsIncremental(t *testing.T) {
	t.Parallel()

	cursor := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	eps := buildSyncEndpoints("incremental", cursor)
	wantCount := len(syncOrder) * len(syncDepths)
	if len(eps) != wantCount {
		t.Fatalf("got %d endpoints, want %d", len(eps), wantCount)
	}
	wantSuffix := "&since=" + strconv.FormatInt(cursor.Unix(), 10)
	for i, ep := range eps {
		if !strings.HasSuffix(ep.Path, wantSuffix) {
			t.Errorf("ep[%d].Path = %q, want suffix %q",
				i, ep.Path, wantSuffix)
		}
		if !strings.Contains(ep.Path, "limit=250&skip=0") {
			t.Errorf("ep[%d].Path = %q: incremental mode must include "+
				"limit=250&skip=0 (StreamAll's first paginated page)",
				i, ep.Path)
		}
	}
}

// TestSync_IncrementalDefaultsSinceToHourAgo asserts parseSinceFlag
// defaults the cursor to now-1h when --mode=incremental and --since
// is empty.
func TestSync_IncrementalDefaultsSinceToHourAgo(t *testing.T) {
	t.Parallel()

	before := time.Now()
	got, err := parseSinceFlag("incremental", "")
	if err != nil {
		t.Fatalf("parseSinceFlag: %v", err)
	}
	after := time.Now()

	wantMin := before.Add(-time.Hour - time.Second)
	wantMax := after.Add(-time.Hour + time.Second)
	if got.Before(wantMin) || got.After(wantMax) {
		t.Errorf("default since = %v, want within [now-1h-1s, now-1h+1s] (=[%v, %v])",
			got, wantMin, wantMax)
	}
}

// TestSync_FullIgnoresSinceFlag asserts full mode returns the zero
// time even when --since is non-empty (the flag is informational
// only in full mode).
func TestSync_FullIgnoresSinceFlag(t *testing.T) {
	t.Parallel()

	got, err := parseSinceFlag("full", "2026-04-27T12:00:00Z")
	if err != nil {
		t.Fatalf("parseSinceFlag: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("full mode parseSinceFlag returned %v, want zero", got)
	}
}

// TestSync_ParseSinceFlagAcceptsRFC3339AndUnix asserts both formats
// round-trip correctly under --mode=incremental.
func TestSync_ParseSinceFlagAcceptsRFC3339AndUnix(t *testing.T) {
	t.Parallel()

	wantUnix := int64(1714219200)
	want := time.Unix(wantUnix, 0)

	got, err := parseSinceFlag("incremental", "2024-04-27T12:00:00Z")
	if err != nil {
		t.Fatalf("RFC3339 parseSinceFlag: %v", err)
	}
	if !got.Equal(time.Date(2024, 4, 27, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("RFC3339: got %v, want 2024-04-27T12:00:00Z", got)
	}

	got, err = parseSinceFlag("incremental", "1714219200")
	if err != nil {
		t.Fatalf("unix parseSinceFlag: %v", err)
	}
	if got.Unix() != want.Unix() {
		t.Errorf("unix: got %d, want %d", got.Unix(), want.Unix())
	}
}
