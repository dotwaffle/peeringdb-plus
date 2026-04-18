// Package privfield provides serializer-layer field-level privacy redaction.
// It composes with internal/privctx (row-level tier stamping) but operates
// one level lower: on a single field within an already-admitted row.
//
// Use Redact at each API surface's response-assembly site, passing the
// pre-existing <field>_visible companion string stored on the ent row.
// Every surface MUST call Redact for every field guarded by a _visible
// companion; there is no centralised enforcement — it's a per-serializer
// discipline locked by the 5-surface E2E test in Plan 64-03.
//
// Design decisions locked in Phase 64 CONTEXT.md:
//   - D-01 serializer-layer redaction (not ent Policy)
//   - D-02 reusable package (this one)
//   - D-03 fail-closed on unstamped ctx
//   - D-04 omit key entirely when redacted (caller uses json omitempty)
package privfield

import (
	"context"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// Redact returns (value, false) if the caller's tier on ctx admits the
// field, or ("", true) if the serializer should omit the field entirely.
//
// Admission rules:
//   - visible == "Public"                → always admit (any tier)
//   - visible == "Users" && tier Users+  → admit
//   - visible == "Users" && tier Public  → redact (the gated case)
//   - visible == "Private"               → redact in all tiers (upstream parity)
//   - any unrecognised visible value     → redact (fail-closed)
//
// Fail-closed semantics:
// privctx.TierFrom(ctx) already returns TierPublic for un-stamped
// contexts, so an un-plumbed ctx naturally lands in the most
// restrictive branch — no extra check needed here.
func Redact(ctx context.Context, visible, value string) (out string, omit bool) {
	tier := privctx.TierFrom(ctx)

	switch visible {
	case "Public":
		return value, false
	case "Users":
		if tier >= privctx.TierUsers {
			return value, false
		}
		return "", true
	default:
		// "Private" or unknown → always redact.
		return "", true
	}
}
