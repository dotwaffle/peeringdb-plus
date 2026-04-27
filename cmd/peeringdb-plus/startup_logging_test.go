package main

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// captureHandler records every slog.Record the logger receives. It exists as
// a per-package shim because the logger type crosses package boundaries, so
// each test package maintains its own handler (mirrors the pattern in
// internal/middleware/logging_test.go). Safe for parallel use — Handle takes
// the mutex.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler         { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler              { return h }

// snapshot returns a copy of the recorded slog records so assertions can
// iterate without holding the handler's lock.
func (h *captureHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(h.records))
	copy(out, h.records)
	return out
}

// collectAttrs flattens a slog.Record's attributes into a map[string]string.
// All values emitted by logStartupClassification are slog.String, so the
// string-only value space is safe; a non-string attr would fail the
// exact-count assertion above.
func collectAttrs(r slog.Record) map[string]string {
	out := make(map[string]string, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		out[a.Key] = a.Value.String()
		return true
	})
	return out
}

// runStartupClassification is a test helper that calls logStartupClassification
// against a freshly-minted captureHandler and returns the captured records.
// It exists to keep the individual Test* functions focused on assertions.
type classificationInput struct {
	apiKey string
	tier   privctx.Tier
}

func runStartupClassification(t *testing.T, in classificationInput) []slog.Record {
	t.Helper()
	ch := &captureHandler{}
	logger := slog.New(ch)
	cfg := &config.Config{
		PeeringDBAPIKey: in.apiKey,
		PublicTier:      in.tier,
	}
	logStartupClassification(logger, cfg)
	return ch.snapshot()
}

// assertSyncModeInfo asserts the first record is an INFO "sync mode" line
// carrying exactly {auth, public_tier} with the expected values.
func assertSyncModeInfo(t *testing.T, r slog.Record, wantAuth, wantTier string) {
	t.Helper()
	if r.Level != slog.LevelInfo {
		t.Errorf("sync mode record level = %v, want INFO", r.Level)
	}
	if r.Message != "sync mode" {
		t.Errorf("sync mode record message = %q, want %q", r.Message, "sync mode")
	}
	attrs := collectAttrs(r)
	if attrs["auth"] != wantAuth {
		t.Errorf("sync mode attr auth = %q, want %q", attrs["auth"], wantAuth)
	}
	if attrs["public_tier"] != wantTier {
		t.Errorf("sync mode attr public_tier = %q, want %q", attrs["public_tier"], wantTier)
	}
	if len(attrs) != 2 {
		t.Errorf("sync mode record has %d attrs, want exactly 2; got %+v", len(attrs), attrs)
	}
}

// assertUsersTierOverrideWarn asserts a record is a WARN "public tier override
// active" line carrying exactly {public_tier=users, env=PDBPLUS_PUBLIC_TIER}.
func assertUsersTierOverrideWarn(t *testing.T, r slog.Record) {
	t.Helper()
	if r.Level != slog.LevelWarn {
		t.Errorf("override record level = %v, want WARN", r.Level)
	}
	if r.Message != "public tier override active" {
		t.Errorf("override record message = %q, want %q", r.Message, "public tier override active")
	}
	attrs := collectAttrs(r)
	if attrs["public_tier"] != "users" {
		t.Errorf("override attr public_tier = %q, want %q", attrs["public_tier"], "users")
	}
	if attrs["env"] != "PDBPLUS_PUBLIC_TIER" {
		t.Errorf("override attr env = %q, want %q", attrs["env"], "PDBPLUS_PUBLIC_TIER")
	}
	if len(attrs) != 2 {
		t.Errorf("override record has %d attrs, want exactly 2; got %+v", len(attrs), attrs)
	}
}

// TestStartup_LogsSyncMode_Anonymous — OBS-01: when no PeeringDB API key is
// configured, the startup classification Info line must report auth=anonymous.
// The tier defaults to public so no override warning is emitted.
func TestStartup_LogsSyncMode_Anonymous(t *testing.T) {
	t.Parallel()
	recs := runStartupClassification(t, classificationInput{
		apiKey: "",
		tier:   privctx.TierPublic,
	})
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1 (Info only, tier=public); records=%+v", len(recs), recs)
	}
	assertSyncModeInfo(t, recs[0], "anonymous", "public")
}

// TestStartup_LogsSyncMode_Authenticated — OBS-01: when PeeringDB API key is
// configured, the startup classification Info line must report
// auth=authenticated. The tier defaults to public so no override warning is
// emitted.
func TestStartup_LogsSyncMode_Authenticated(t *testing.T) {
	t.Parallel()
	recs := runStartupClassification(t, classificationInput{
		apiKey: "secret-token-value",
		tier:   privctx.TierPublic,
	})
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1 (Info only, tier=public); records=%+v", len(recs), recs)
	}
	assertSyncModeInfo(t, recs[0], "authenticated", "public")
}

// TestStartup_WarnsOnUsersTier — SYNC-04: when PDBPLUS_PUBLIC_TIER=users is in
// effect, the startup classification must emit BOTH the Info line (tier=users)
// AND a Warn line naming the override env var. Verified for both auth states
// so the override warning is not swallowed by the auth classification.
func TestStartup_WarnsOnUsersTier(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		apiKey   string
		wantAuth string
	}{
		{"anonymous", "", "anonymous"},
		{"authenticated", "secret-token-value", "authenticated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			recs := runStartupClassification(t, classificationInput{
				apiKey: tc.apiKey,
				tier:   privctx.TierUsers,
			})
			if len(recs) != 2 {
				t.Fatalf("got %d records, want 2 (Info + Warn, tier=users); records=%+v", len(recs), recs)
			}
			assertSyncModeInfo(t, recs[0], tc.wantAuth, "users")
			assertUsersTierOverrideWarn(t, recs[1])
		})
	}
}

// TestStartup_NoWarnOnPublicTier — SYNC-04 guarantees the override warning
// fires only when tier=users. This test asserts the WARN record is absent
// from the capture under both auth states when tier=public.
func TestStartup_NoWarnOnPublicTier(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		apiKey string
	}{
		{"anonymous", ""},
		{"authenticated", "secret-token-value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			recs := runStartupClassification(t, classificationInput{
				apiKey: tc.apiKey,
				tier:   privctx.TierPublic,
			})
			if len(recs) != 1 {
				t.Fatalf("got %d records, want exactly 1 (Info only, no override WARN); records=%+v", len(recs), recs)
			}
			// Defensive: scan every record for the override message.
			for i, r := range recs {
				if r.Level == slog.LevelWarn && r.Message == "public tier override active" {
					t.Fatalf("unexpected override WARN at record[%d]; tier=public must not emit it", i)
				}
			}
		})
	}
}

// TestStartupLogging is the plan-mandated table-driven regression that walks
// all four (auth × tier) combinations in one place. It exists alongside the
// single-purpose Test* functions above so changes to the attribute-key wire
// contract fail both the targeted tests (clear error locality) and this
// matrix test (broad coverage).
func TestStartupLogging(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		apiKey   string
		tier     privctx.Tier
		wantAuth string
		wantTier string
		wantRecs int // 1 when tier=public, 2 when tier=users
	}{
		{"anon_public", "", privctx.TierPublic, "anonymous", "public", 1},
		{"auth_public", "secret", privctx.TierPublic, "authenticated", "public", 1},
		{"anon_users", "", privctx.TierUsers, "anonymous", "users", 2},
		{"auth_users", "secret", privctx.TierUsers, "authenticated", "users", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			recs := runStartupClassification(t, classificationInput{
				apiKey: tc.apiKey,
				tier:   tc.tier,
			})
			if got := len(recs); got != tc.wantRecs {
				t.Fatalf("got %d records, want %d; records=%+v", got, tc.wantRecs, recs)
			}
			assertSyncModeInfo(t, recs[0], tc.wantAuth, tc.wantTier)
			if tc.tier == privctx.TierUsers {
				assertUsersTierOverrideWarn(t, recs[1])
			}
		})
	}
}
