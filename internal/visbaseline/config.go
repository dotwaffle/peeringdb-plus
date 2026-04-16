package visbaseline

import (
	"io"
	"log/slog"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// Config parameterises a capture run. Grouped per GO-CS-5 (input structs
// for >2-arg callers).
//
// Required fields: Target, BaseURL, Modes, Types, OutDir, Logger. Optional
// fields fall back to documented defaults. When Modes contains "auth",
// APIKey MUST be set or New returns an error.
type Config struct {
	// Target is the capture label ("beta" | "prod"). Used only as a tag
	// inside State tuples; the actual base URL is BaseURL.
	Target string

	// BaseURL is the PeeringDB root (e.g. https://beta.peeringdb.com). No
	// trailing slash. Must be non-empty.
	BaseURL string

	// Modes is the subset of {"anon", "auth"} to capture.
	Modes []string

	// Types is the subset of PeeringDB object types to walk. Use AllTypes
	// for beta; ProdTypes for prod.
	Types []string

	// Pages is the number of pages per (mode, type) to fetch. Defaults to
	// 2 when <1. The phase 57 baseline uses 2.
	Pages int

	// OutDir is the REPO-SIDE output root for anon fixtures. Auth fixtures
	// are written EXCLUSIVELY under a private /tmp directory created per
	// run — NEVER under OutDir. This invariant is enforced by the capture
	// loop and asserted by TestCaptureWritesAuthBytesToTmpOnly.
	OutDir string

	// APIKey is the PeeringDB API key. Required when Modes contains "auth".
	// Must never be logged (T-57-05); the Capture implementation uses the
	// peeringdb.Client's built-in Authorization header path, which does
	// not log the value.
	APIKey string

	// StatePath is the checkpoint file path. Defaults to DefaultStatePath
	// when empty.
	StatePath string

	// Logger receives structured capture events. Required.
	Logger *slog.Logger

	// InterTupleDelay is an optional pause between tuples. Tests set this
	// small enough to keep wall-clock low; prod runs leave it zero (the
	// peeringdb.Client rate limiter already paces requests).
	InterTupleDelay time.Duration

	// RateLimitJitter is the extra pause added on top of RateLimitError
	// RetryAfter before re-fetching the same tuple. Defaults to 5 seconds
	// in prod; tests override to 1ms to keep runtime low.
	RateLimitJitter time.Duration

	// ClientOverride, when non-nil, is used by New instead of constructing
	// a *peeringdb.Client internally. TESTS ONLY. Prod callers must leave
	// nil.
	//
	// Rationale: tests need to call client.SetRateLimit and
	// SetRetryBaseDelay before Run to keep wall-clock under 1 second. A
	// package-global mutable hook is race-prone under t.Parallel(); a
	// per-Config field has no shared mutable state.
	//
	// When both Modes contains "auth" and ClientOverride is set, the
	// override is used for BOTH anon and auth clients — tests that need
	// to assert auth-header presence should set the override WithAPIKey
	// and inspect the request on the httptest server side.
	ClientOverride *peeringdb.Client

	// PromptReader is the source for the Resume/Restart prompt. Defaults
	// to os.Stdin when nil. Tests inject strings.NewReader.
	PromptReader io.Reader

	// PromptWriter is the sink for the Resume/Restart prompt text.
	// Defaults to os.Stderr when nil. Tests inject io.Discard.
	PromptWriter io.Writer
}
