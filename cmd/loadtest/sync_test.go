package main

import (
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

// TestSync_BuildSyncEndpointsFull asserts full-mode produces 13
// pdbcompat GETs against /api/<short>?limit=250&skip=0&depth=0,
// mirroring internal/peeringdb/client.go FetchRawPage exactly.
func TestSync_BuildSyncEndpointsFull(t *testing.T) {
	t.Parallel()

	eps := buildSyncEndpoints("full", time.Time{})
	if len(eps) != 13 {
		t.Fatalf("got %d endpoints, want 13", len(eps))
	}
	for i, ep := range eps {
		if ep.Surface != SurfacePdbCompat {
			t.Errorf("ep[%d].Surface = %q, want pdbcompat", i, ep.Surface)
		}
		if ep.Method != "GET" {
			t.Errorf("ep[%d].Method = %q, want GET", i, ep.Method)
		}
		// Path: /api/<type>?limit=250&skip=0&depth=0
		want := "/api/" + syncOrder[i] + "?limit=250&skip=0&depth=0"
		if ep.Path != want {
			t.Errorf("ep[%d].Path = %q, want %q", i, ep.Path, want)
		}
		if strings.Contains(ep.Path, "since=") {
			t.Errorf("ep[%d].Path = %q: full mode should not have since=", i, ep.Path)
		}
	}
}

// TestSync_BuildSyncEndpointsIncremental asserts incremental mode
// appends &since=<unix> to every URL, using the provided cursor.
func TestSync_BuildSyncEndpointsIncremental(t *testing.T) {
	t.Parallel()

	cursor := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	eps := buildSyncEndpoints("incremental", cursor)
	if len(eps) != 13 {
		t.Fatalf("got %d endpoints, want 13", len(eps))
	}
	wantSuffix := "&since=" + strconv.FormatInt(cursor.Unix(), 10)
	for i, ep := range eps {
		if !strings.HasSuffix(ep.Path, wantSuffix) {
			t.Errorf("ep[%d].Path = %q, want suffix %q", i, ep.Path, wantSuffix)
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
