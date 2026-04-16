package visbaseline_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/visbaseline"
)

// TestCheckpointRoundTrip saves a State with some Done tuples, loads it back,
// and asserts deep equality of the tuple slice.
func TestCheckpointRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	orig := &visbaseline.State{
		Tuples: []visbaseline.Tuple{
			{Target: "beta", Mode: "anon", Type: "poc", Page: 1, Done: true},
			{Target: "beta", Mode: "anon", Type: "poc", Page: 2, Done: false},
			{Target: "beta", Mode: "auth", Type: "org", Page: 1, Done: true},
		},
	}
	if err := orig.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := visbaseline.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !reflect.DeepEqual(orig.Tuples, loaded.Tuples) {
		t.Errorf("round-trip mismatch:\n  orig:   %+v\n  loaded: %+v", orig.Tuples, loaded.Tuples)
	}
	if loaded.Version != 1 {
		t.Errorf("Version = %d, want 1 (auto-set on Save)", loaded.Version)
	}
}

// TestCheckpointResumeSkipsDoneTuples asserts PendingTuples() returns only
// Done=false tuples in enumeration order.
func TestCheckpointResumeSkipsDoneTuples(t *testing.T) {
	t.Parallel()

	s := &visbaseline.State{
		Tuples: []visbaseline.Tuple{
			{Target: "beta", Mode: "anon", Type: "poc", Page: 1, Done: true},
			{Target: "beta", Mode: "anon", Type: "poc", Page: 2, Done: false},
			{Target: "beta", Mode: "anon", Type: "org", Page: 1, Done: false},
			{Target: "beta", Mode: "anon", Type: "org", Page: 2, Done: true},
		},
	}
	pending := s.PendingTuples()
	if len(pending) != 2 {
		t.Fatalf("PendingTuples len = %d, want 2", len(pending))
	}
	// Order preserved from the state slice.
	if pending[0].Type != "poc" || pending[0].Page != 2 {
		t.Errorf("pending[0] = %+v, want poc page 2", pending[0])
	}
	if pending[1].Type != "org" || pending[1].Page != 1 {
		t.Errorf("pending[1] = %+v, want org page 1", pending[1])
	}
}

// TestCheckpointAtomicWrite asserts that after Save the .tmp sibling does not
// exist — os.Rename moved it into place atomically.
func TestCheckpointAtomicWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s := &visbaseline.State{
		Tuples: []visbaseline.Tuple{{Target: "beta", Mode: "anon", Type: "net", Page: 1}},
	}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf(".tmp file still present after Save (expected rename to remove it): err=%v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("final state file missing: %v", err)
	}
}

// TestCheckpointPromptSafeDefaults asserts EOF input returns Restart
// (the safe default — on a corrupted state or empty pipe, start over).
func TestCheckpointPromptSafeDefaults(t *testing.T) {
	t.Parallel()

	var w bytes.Buffer
	got := visbaseline.PromptResumeOrRestart(strings.NewReader(""), &w)
	if got != visbaseline.Restart {
		t.Errorf("EOF input returned %v, want Restart", got)
	}
}

// TestCheckpointPromptAcceptsResume asserts the "resume" keyword returns Resume.
func TestCheckpointPromptAcceptsResume(t *testing.T) {
	t.Parallel()

	cases := []string{"resume\n", "Resume\n", " resume \n", "r\n", "R\n", "continue\n", "c\n"}
	for _, in := range cases {
		var w bytes.Buffer
		got := visbaseline.PromptResumeOrRestart(strings.NewReader(in), &w)
		if got != visbaseline.Resume {
			t.Errorf("input %q returned %v, want Resume", in, got)
		}
	}
}

// TestCheckpointPromptAcceptsRestart asserts explicit restart + any other
// input returns Restart.
func TestCheckpointPromptAcceptsRestart(t *testing.T) {
	t.Parallel()

	cases := []string{"restart\n", "RESTART\n", "no\n", "garbage\n", "\n"}
	for _, in := range cases {
		var w bytes.Buffer
		got := visbaseline.PromptResumeOrRestart(strings.NewReader(in), &w)
		if got != visbaseline.Restart {
			t.Errorf("input %q returned %v, want Restart", in, got)
		}
	}
}

// TestCheckpointContainsNoPayload asserts the serialised state has only the
// whitelisted top-level and tuple keys — T-57-04 mitigation.
func TestCheckpointContainsNoPayload(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	s := &visbaseline.State{
		Tuples: []visbaseline.Tuple{
			{Target: "beta", Mode: "anon", Type: "poc", Page: 1, Done: true},
			{Target: "beta", Mode: "auth", Type: "org", Page: 2, Done: false},
		},
	}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal top: %v", err)
	}
	wantTop := map[string]bool{"version": true, "tuples": true}
	for k := range raw {
		if !wantTop[k] {
			t.Errorf("unexpected top-level key %q in state file", k)
		}
	}
	var tuples []map[string]json.RawMessage
	if err := json.Unmarshal(raw["tuples"], &tuples); err != nil {
		t.Fatalf("Unmarshal tuples: %v", err)
	}
	wantTupleKeys := map[string]bool{"target": true, "mode": true, "type": true, "page": true, "done": true}
	for i, tup := range tuples {
		for k := range tup {
			if !wantTupleKeys[k] {
				t.Errorf("tuple %d: unexpected key %q", i, k)
			}
		}
	}
}

// TestCheckpointCorruptedFileReturnsError asserts garbage bytes produce a
// wrapped error, not a panic.
func TestCheckpointCorruptedFileReturnsError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("this is not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := visbaseline.LoadState(path)
	if err == nil {
		t.Fatal("LoadState on garbage returned nil, want error")
	}
}

// TestCheckpointLoadStateMissingReturnsNotExist asserts a missing file
// returns os.ErrNotExist wrapped so callers can errors.Is-check it.
func TestCheckpointLoadStateMissingReturnsNotExist(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	_, err := visbaseline.LoadState(path)
	if err == nil {
		t.Fatal("LoadState on missing file returned nil, want error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want errors.Is(err, os.ErrNotExist)", err)
	}
}

// TestEnumerateTuplesBetaBoth asserts the tuple space for 13 types × 2 modes
// × 2 pages is exactly 52 tuples in a deterministic order.
func TestEnumerateTuplesBetaBoth(t *testing.T) {
	t.Parallel()

	types := []string{
		"campus", "carrier", "carrierfac", "fac", "ix", "ixfac",
		"ixlan", "ixpfx", "net", "netfac", "netixlan", "org", "poc",
	}
	got := visbaseline.EnumerateTuples("beta", []string{"anon", "auth"}, types, 2)
	if len(got) != 52 {
		t.Fatalf("len = %d, want 52 (13 types × 2 modes × 2 pages)", len(got))
	}
	// Order: mode outer, type middle, page inner.
	if got[0].Mode != "anon" || got[0].Type != "campus" || got[0].Page != 1 {
		t.Errorf("got[0] = %+v, want anon/campus/page-1", got[0])
	}
	if got[1].Mode != "anon" || got[1].Type != "campus" || got[1].Page != 2 {
		t.Errorf("got[1] = %+v, want anon/campus/page-2", got[1])
	}
	if got[2].Mode != "anon" || got[2].Type != "carrier" || got[2].Page != 1 {
		t.Errorf("got[2] = %+v, want anon/carrier/page-1", got[2])
	}
	// First auth tuple at index 26 (13*2).
	if got[26].Mode != "auth" || got[26].Type != "campus" || got[26].Page != 1 {
		t.Errorf("got[26] = %+v, want auth/campus/page-1", got[26])
	}
	if got[51].Mode != "auth" || got[51].Type != "poc" || got[51].Page != 2 {
		t.Errorf("got[51] = %+v, want auth/poc/page-2", got[51])
	}
	// All tuples start with Done=false.
	for i, tup := range got {
		if tup.Done {
			t.Errorf("got[%d].Done = true, want false for fresh enumeration", i)
		}
		if tup.Target != "beta" {
			t.Errorf("got[%d].Target = %q, want beta", i, tup.Target)
		}
	}
}

// TestCheckpointAdvanceMarksTupleDone asserts State.Advance flips Done=true
// for the matching tuple and persists via Save.
func TestCheckpointAdvanceMarksTupleDone(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	s := &visbaseline.State{
		Tuples: []visbaseline.Tuple{
			{Target: "beta", Mode: "anon", Type: "poc", Page: 1},
			{Target: "beta", Mode: "anon", Type: "poc", Page: 2},
		},
	}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	target := visbaseline.Tuple{Target: "beta", Mode: "anon", Type: "poc", Page: 1}
	if err := s.Advance(target, path); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if !s.Tuples[0].Done {
		t.Error("s.Tuples[0].Done = false, want true after Advance")
	}
	if s.Tuples[1].Done {
		t.Error("s.Tuples[1].Done = true, want false (Advance must only flip the match)")
	}

	loaded, err := visbaseline.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !loaded.Tuples[0].Done {
		t.Error("persisted state lost Done=true flag")
	}
}

// TestCheckpointCleanupStatePath asserts CleanupStatePath is idempotent —
// removing a missing file is not an error.
func TestCheckpointCleanupStatePath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.json")
	// Cleanup on a missing file is a no-op.
	if err := visbaseline.CleanupStatePath(path); err != nil {
		t.Errorf("CleanupStatePath on missing file: %v", err)
	}
	// Create the file then cleanup.
	if err := os.WriteFile(path, []byte(`{"version":1,"tuples":[]}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := visbaseline.CleanupStatePath(path); err != nil {
		t.Errorf("CleanupStatePath on present file: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file not removed: err=%v", err)
	}
}

// TestTupleString asserts the (target,mode,type,page) stringification for
// log attributes and error wrapping.
func TestTupleString(t *testing.T) {
	t.Parallel()

	tup := visbaseline.Tuple{Target: "beta", Mode: "anon", Type: "poc", Page: 1}
	got := tup.String()
	want := "beta/anon/poc/page-1"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// discardWriter is used to keep prompt output from littering test output.
var _ io.Writer = (*bytes.Buffer)(nil)
