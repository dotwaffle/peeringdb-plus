package visbaseline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DefaultStatePath is the canonical location of the capture checkpoint file.
// /tmp is used deliberately so the checkpoint survives process exit but not
// reboots, matching operator expectations for a one-shot CLI.
const DefaultStatePath = "/tmp/pdb-vis-capture-state.json"

// stateVersion is the current State schema version. Incremented only on
// breaking changes to the persisted JSON shape. Unknown versions on load
// are rejected rather than silently migrated.
const stateVersion = 1

// Tuple identifies one unit of work in the capture walk: one (target, mode,
// type, page) combination. Tuples are created up-front by EnumerateTuples
// and flipped to Done=true as each successful fetch+write completes.
//
// Field names map to lowercase JSON keys so the persisted file uses the
// PeeringDB-style lowercase convention and so the T-57-04 whitelist test
// can assert exact-key equality.
type Tuple struct {
	Target string `json:"target"` // "beta" | "prod"
	Mode   string `json:"mode"`   // "anon" | "auth"
	Type   string `json:"type"`   // PeeringDB object type (poc, net, …)
	Page   int    `json:"page"`   // 1-based page number
	Done   bool   `json:"done"`   // true once bytes are on disk
}

// String returns a deterministic stringification suitable for log attributes
// and error wrapping: "{target}/{mode}/{type}/page-{N}".
func (t Tuple) String() string {
	return fmt.Sprintf("%s/%s/%s/page-%d", t.Target, t.Mode, t.Type, t.Page)
}

// State is the capture checkpoint.
//
// State carries ONLY tuple metadata. No response bytes, no API keys, no
// payload. See threat T-57-04 in the phase 57-02 threat register: the
// checkpoint file is written to /tmp and could be read by other users on
// multi-tenant hosts, so it must never contain sensitive data. The
// TestCheckpointContainsNoPayload unit test enforces this invariant by
// asserting the serialised top-level and tuple key sets are exactly the
// declared JSON tags.
type State struct {
	Version int     `json:"version"`
	Tuples  []Tuple `json:"tuples"`
}

// Save serialises s to path atomically. The write goes to path+".tmp" first
// with mode 0600, then os.Rename moves it into place. POSIX rename on the
// same filesystem is atomic — a concurrent reader sees either the old state
// or the new state, never a partial write.
//
// Version is auto-stamped to the current schema version on Save if unset.
func (s *State) Save(path string) error {
	if s.Version == 0 {
		s.Version = stateVersion
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := path + ".tmp"
	// 0600 — readable only by the invoking user. Checkpoint may live on
	// shared hosts in /tmp; deny others by default.
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write state tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		// Attempt cleanup of the stale tmp file; ignore secondary errors.
		_ = os.Remove(tmp)
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

// Advance flips the matching tuple's Done flag to true and persists via Save.
// Matching is by (Target, Mode, Type, Page). Returns error if no tuple matches.
func (s *State) Advance(t Tuple, path string) error {
	for i := range s.Tuples {
		cur := &s.Tuples[i]
		if cur.Target == t.Target && cur.Mode == t.Mode && cur.Type == t.Type && cur.Page == t.Page {
			cur.Done = true
			return s.Save(path)
		}
	}
	return fmt.Errorf("Advance: no matching tuple for %s", t)
}

// PendingTuples returns the Done=false tuples in their existing slice order.
// The returned slice is a copy — callers may mutate without affecting State.
func (s *State) PendingTuples() []Tuple {
	out := make([]Tuple, 0, len(s.Tuples))
	for _, t := range s.Tuples {
		if !t.Done {
			out = append(out, t)
		}
	}
	return out
}

// LoadState reads and deserialises a State from path. Returns a wrapped
// os.ErrNotExist when the file is missing (callers may errors.Is-check),
// a wrapped json error on parse failure, and an error for unsupported
// versions.
func LoadState(path string) (*State, error) {
	// path is an operator-supplied checkpoint location (-state flag or
	// DefaultStatePath). Permitted by design — this is a CLI tool.
	path = filepath.Clean(path)
	data, err := os.ReadFile(path) //nolint:gosec // visbaseline is a CLI tool — paths are operator-supplied by contract
	if err != nil {
		// os.ReadFile already wraps errors appropriately for errors.Is(err, os.ErrNotExist).
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state file %s: %w", path, err)
	}
	if s.Version != stateVersion {
		return nil, fmt.Errorf("unsupported state version %d (want %d)", s.Version, stateVersion)
	}
	return &s, nil
}

// PromptAnswer enumerates operator responses to the resume/restart prompt.
type PromptAnswer int

const (
	// Restart is the safe default — discard old state and enumerate fresh
	// tuples. Returned on EOF, empty input, unrecognised keywords.
	Restart PromptAnswer = iota
	// Resume continues with the saved state, skipping Done=true tuples.
	Resume
)

// PromptResumeOrRestart asks the operator via r (typically os.Stdin) whether
// to resume from a saved checkpoint or restart. Writes the prompt text to w
// (typically os.Stderr so it does not contaminate stdout pipelines). Reads
// one line via bufio.NewScanner.
//
// Safe defaults: any input that is not recognised as a resume keyword returns
// Restart. This includes EOF, empty lines, garbage, and "no"/"restart". The
// bias is "when in doubt, start fresh" — a replayed fetch is cheap; a
// skipped tuple is a silent correctness failure.
//
// Resume keywords (case-insensitive, TrimSpace): "resume", "r", "continue", "c".
func PromptResumeOrRestart(r io.Reader, w io.Writer) PromptAnswer {
	_, _ = fmt.Fprint(w, "Existing capture checkpoint found. Resume or restart? [resume/Restart]: ")
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		// EOF or IO error — safe default.
		return Restart
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	switch answer {
	case "resume", "r", "continue", "c":
		return Resume
	default:
		return Restart
	}
}

// EnumerateTuples produces the ordered (mode outer, type middle, page inner)
// tuple space for a capture run. Order matters: anon fetches come before
// auth fetches so an interrupted partial run keeps the anon-only signal
// intact for downstream inspection. Within a mode, iterate types
// alphabetically, and within a type, pages ascending.
//
// Returns tuples with Done=false; Target is stamped on every tuple.
func EnumerateTuples(target string, modes []string, types []string, pages int) []Tuple {
	out := make([]Tuple, 0, len(modes)*len(types)*pages)
	for _, mode := range modes {
		for _, ty := range types {
			for page := 1; page <= pages; page++ {
				out = append(out, Tuple{
					Target: target,
					Mode:   mode,
					Type:   ty,
					Page:   page,
				})
			}
		}
	}
	return out
}

// CleanupStatePath removes the checkpoint file. A missing file is not an
// error — removal is idempotent so callers (New / Run completion) do not
// have to pre-check existence.
func CleanupStatePath(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove state: %w", err)
	}
	return nil
}
