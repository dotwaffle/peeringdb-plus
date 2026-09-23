// Package privfield provides serializer-layer field-level privacy redaction.
// It composes with internal/privctx (row-level tier stamping) but operates
// one level lower: on a single field within an already-admitted row.
//
// Use Redact at each API surface's response-assembly site, passing the
// pre-existing <field>_visible companion string stored on the ent row.
// Every surface MUST call Redact for every field guarded by a _visible
// companion. There is no centralized enforcement. Each serializer calls
// Redact, and the 5-surface end-to-end test locks this.
//
// Design rationale:
//   - redaction happens at the serializer layer, not via an ent Policy,
//     because ent's Privacy operates at the query/row level only.
//   - it lives in this reusable package so every surface shares one
//     implementation.
//   - it fails closed on an unstamped ctx (the most restrictive tier).
//   - it signals omission so the caller can drop the field entirely
//     (callers rely on json omitempty for absence on the wire).
package privfield

import (
	"context"
	"slices"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// Redact returns (value, false) if the caller's tier on ctx admits the
// field, or ("", true) if the serializer should omit the field entirely.
//
// Admission rule: admit when visible is in the caller tier's
// privctx.Tier.AdmittedVisibilities, the same set that the row gates use.
// That gives:
//   - visible == "Public"                → always admit (any tier)
//   - visible == "Users" && tier Users+  → admit
//   - visible == "Users" && tier Public  → redact (the gated case)
//   - visible == "Private"               → redact in all tiers (upstream parity)
//   - any unrecognized visible value     → redact (fail-closed)
//
// Fail-closed semantics:
// privctx.TierFrom(ctx) already returns TierPublic for un-stamped
// contexts, so an un-plumbed ctx naturally lands in the most
// restrictive branch — no extra check needed here.
func Redact(ctx context.Context, visible, value string) (out string, omit bool) {
	if slices.Contains(privctx.TierFrom(ctx).AdmittedVisibilities(), visible) {
		return value, false
	}
	return "", true
}
