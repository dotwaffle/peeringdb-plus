package visbaseline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// AllTypes is the canonical list of PeeringDB object types captured in the
// visibility baseline walk. Mirrored from cmd/pdbcompat-check/main.go
// (lines 22-26) because Go import-direction hygiene forbids `internal`
// packages from importing `cmd` packages.
var AllTypes = []string{
	"campus", "carrier", "carrierfac", "fac", "ix", "ixfac",
	"ixlan", "ixpfx", "net", "netfac", "netixlan", "org", "poc",
}

// ProdTypes is the reduced list used for the prod confirmation pass per
// phase 57 D-04. Only the high-signal privacy types are fetched against
// production to stay well inside the rate-limit quota.
var ProdTypes = []string{"net", "org", "poc"}

// defaultRateLimitJitter is the extra sleep added on top of
// RateLimitError.RetryAfter before a tuple is retried. Five seconds matches
// research Pattern 3 and keeps enough margin for clock skew.
const defaultRateLimitJitter = 5 * time.Second

// Capture coordinates the per-tuple walk of PeeringDB pages for the phase
// 57 visibility baseline. Construct via New; run with Run.
type Capture struct {
	cfg        Config
	rawAuthDir string
	anonClient *peeringdb.Client
	authClient *peeringdb.Client
	jitter     time.Duration
}

// New validates cfg and constructs a Capture. Fails fast when required
// fields are missing or when auth mode is requested without an API key —
// no HTTP calls happen in New.
func New(cfg Config) (*Capture, error) {
	if cfg.Target == "" {
		return nil, errors.New("visbaseline.New: Target required")
	}
	if cfg.BaseURL == "" {
		return nil, errors.New("visbaseline.New: BaseURL required")
	}
	if cfg.OutDir == "" {
		return nil, errors.New("visbaseline.New: OutDir required")
	}
	if cfg.Logger == nil {
		return nil, errors.New("visbaseline.New: Logger required")
	}
	if len(cfg.Modes) == 0 {
		return nil, errors.New("visbaseline.New: Modes must contain at least one of anon/auth")
	}
	for _, m := range cfg.Modes {
		if m != "anon" && m != "auth" {
			return nil, fmt.Errorf("visbaseline.New: invalid mode %q (want anon | auth)", m)
		}
	}
	if len(cfg.Types) == 0 {
		return nil, errors.New("visbaseline.New: Types required")
	}
	hasAuth := slices.Contains(cfg.Modes, "auth")
	if hasAuth && cfg.APIKey == "" {
		return nil, errors.New("visbaseline.New: APIKey required when mode=auth")
	}
	if cfg.Pages < 1 {
		cfg.Pages = 2
	}
	if cfg.StatePath == "" {
		cfg.StatePath = DefaultStatePath
	}

	c := &Capture{
		cfg:    cfg,
		jitter: cfg.RateLimitJitter,
	}
	if c.jitter <= 0 {
		c.jitter = defaultRateLimitJitter
	}
	if cfg.ClientOverride != nil {
		c.anonClient = cfg.ClientOverride
		c.authClient = cfg.ClientOverride
	} else {
		c.anonClient = peeringdb.NewClient(cfg.BaseURL, cfg.Logger)
		if hasAuth {
			c.authClient = peeringdb.NewClient(cfg.BaseURL, cfg.Logger, peeringdb.WithAPIKey(cfg.APIKey))
		}
	}
	return c, nil
}

// Run executes the capture walk. Returns the path to the private /tmp
// directory holding raw auth bytes (for the redactor to consume in plan
// 03), or an error. Context cancellation is honoured between tuples and
// during rate-limit sleeps — returned error wraps ctx.Err().
//
// On successful completion the checkpoint file is removed. On early exit
// (error, cancel) the checkpoint persists so a later invocation can resume.
func (c *Capture) Run(ctx context.Context) (string, error) {
	// 1. Private dir for raw auth bytes (mode 0700 via POSIX default on
	//    os.MkdirTemp). Guaranteed outside the repo tree.
	tmpDir, err := os.MkdirTemp("", "pdb-vis-capture-*")
	if err != nil {
		return "", fmt.Errorf("mkdir tmp: %w", err)
	}
	c.rawAuthDir = tmpDir

	// 2. Resolve state: load or enumerate fresh. On existing state, prompt
	//    the operator Resume/Restart. Restart wipes the slate.
	state, err := c.resolveState(ctx)
	if err != nil {
		return c.rawAuthDir, err
	}

	// 3. Walk pending tuples. On successful advance, persist checkpoint.
	pending := state.PendingTuples()
	for _, t := range pending {
		select {
		case <-ctx.Done():
			return c.rawAuthDir, fmt.Errorf("capture cancelled: %w", ctx.Err())
		default:
		}
		if err := c.runTuple(ctx, t); err != nil {
			return c.rawAuthDir, err
		}
		if err := state.Advance(t, c.cfg.StatePath); err != nil {
			return c.rawAuthDir, fmt.Errorf("advance checkpoint: %w", err)
		}
		if c.cfg.InterTupleDelay > 0 {
			select {
			case <-time.After(c.cfg.InterTupleDelay):
			case <-ctx.Done():
				return c.rawAuthDir, fmt.Errorf("capture cancelled: %w", ctx.Err())
			}
		}
	}

	// 4. Clean the checkpoint on clean completion. Log but continue on
	//    cleanup failure — the run itself succeeded.
	if err := CleanupStatePath(c.cfg.StatePath); err != nil {
		c.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "checkpoint cleanup failed",
			slog.String("path", c.cfg.StatePath),
			slog.String("error", err.Error()),
		)
	}
	return c.rawAuthDir, nil
}

// resolveState either loads an existing checkpoint (with operator prompt) or
// enumerates a fresh one and persists it.
func (c *Capture) resolveState(ctx context.Context) (*State, error) {
	existing, err := LoadState(c.cfg.StatePath)
	switch {
	case err == nil:
		promptR := c.cfg.PromptReader
		if promptR == nil {
			promptR = os.Stdin
		}
		promptW := c.cfg.PromptWriter
		if promptW == nil {
			promptW = os.Stderr
		}
		answer := PromptResumeOrRestart(promptR, promptW)
		if answer == Resume {
			c.cfg.Logger.LogAttrs(ctx, slog.LevelInfo, "resuming capture from checkpoint",
				slog.String("state_path", c.cfg.StatePath),
				slog.Int("tuples", len(existing.Tuples)),
			)
			return existing, nil
		}
		c.cfg.Logger.LogAttrs(ctx, slog.LevelInfo, "restarting capture (discarding checkpoint)",
			slog.String("state_path", c.cfg.StatePath),
		)
		// WR-05: Restart discards the checkpoint but the State does not know
		// where the previous run's raw-auth /tmp dir lived. Warn the operator
		// so they can clean it up manually. State intentionally carries no
		// PII, so recording the prior rawAuthDir in State would widen T-57-04
		// exposure; a log line is the right trade-off.
		c.cfg.Logger.LogAttrs(ctx, slog.LevelWarn,
			"restart discards checkpoint; prior /tmp/pdb-vis-capture-* dir (if any) must be cleaned manually",
			slog.String("state_path", c.cfg.StatePath),
		)
		// Fall through to fresh enumeration.
	case errors.Is(err, os.ErrNotExist):
		// No checkpoint — fresh run.
	default:
		return nil, fmt.Errorf("load state: %w", err)
	}

	fresh := &State{
		Tuples: EnumerateTuples(c.cfg.Target, c.cfg.Modes, c.cfg.Types, c.cfg.Pages),
	}
	if err := fresh.Save(c.cfg.StatePath); err != nil {
		return nil, fmt.Errorf("save fresh state: %w", err)
	}
	return fresh, nil
}

// runTuple fetches one tuple's page and writes it to the correct destination.
// On RateLimitError, sleeps the indicated interval (plus jitter) and retries
// the SAME tuple. Other errors are returned to the caller.
//
// IMPORTANT: the API key is NEVER included in any slog attribute here.
// Only tuple metadata is logged (T-57-05 mitigation).
func (c *Capture) runTuple(ctx context.Context, t Tuple) error {
	client := c.clientFor(t.Mode)
	if client == nil {
		return fmt.Errorf("runTuple: no client for mode %q", t.Mode)
	}
	c.cfg.Logger.LogAttrs(ctx, slog.LevelInfo, "capture tuple",
		slog.String("target", t.Target),
		slog.String("mode", t.Mode),
		slog.String("type", t.Type),
		slog.Int("page", t.Page),
	)

	for {
		raw, err := client.FetchRawPage(ctx, t.Type, t.Page)
		if err == nil {
			return c.writeBytes(t, raw)
		}
		if rlErr, ok := errors.AsType[*peeringdb.RateLimitError](err); ok {
			wait := rlErr.RetryAfter + c.jitter
			c.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "rate-limited, sleeping",
				slog.String("tuple", t.String()),
				slog.Duration("retry_after", rlErr.RetryAfter),
				slog.Duration("sleep", wait),
			)
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return fmt.Errorf("rate-limit sleep cancelled: %w", ctx.Err())
			}
		}
		return fmt.Errorf("fetch %s: %w", t, err)
	}
}

// clientFor returns the client for the given capture mode.
func (c *Capture) clientFor(mode string) *peeringdb.Client {
	switch mode {
	case "anon":
		return c.anonClient
	case "auth":
		return c.authClient
	default:
		return nil
	}
}

// writeBytes lays down one tuple's raw bytes to the correct destination.
// anon → <OutDir>/anon/api/{type}/page-{N}.json (repo-side, committable).
// auth → <rawAuthDir>/auth/api/{type}/page-{N}.json (private /tmp).
//
// Directory mode 0700, file mode 0600. Auth path is NEVER under OutDir.
func (c *Capture) writeBytes(t Tuple, raw []byte) error {
	var base string
	switch t.Mode {
	case "anon":
		base = c.cfg.OutDir
	case "auth":
		base = c.rawAuthDir
	default:
		return fmt.Errorf("writeBytes: unknown mode %q", t.Mode)
	}
	dir := filepath.Join(base, t.Mode, "api", t.Type)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	name := filepath.Join(dir, fmt.Sprintf("page-%d.json", t.Page))
	if err := os.WriteFile(name, raw, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
