# Phase 61: Operator-facing observability - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Make the sync mode and effective privacy tier visible to operators at three levels: at startup (slog), on the `/about` surface (web + terminal), and on read-path OTel spans. No silent escalation — if `PDBPLUS_PUBLIC_TIER=users` is in effect, that fact is loud everywhere an operator might look.

Can run in parallel with phase 60 after phase 59 lands. Touches `cmd/peeringdb-plus/main.go` startup, `/about` renderers, and the OTel attribute wiring on the privacy middleware.

</domain>

<decisions>
## Implementation Decisions

### Startup classification log
- **D-01:** Single `slog.Info` with structured attributes — one line, machine-parseable, plays nicely with the OTel slog bridge:
  - `slog.Info("sync mode", slog.String("auth", "authenticated"|"anonymous"), slog.String("public_tier", "public"|"users"))`
  - `auth` is determined by `PDBPLUS_PEERINGDB_API_KEY` presence
  - `public_tier` is the resolved `PDBPLUS_PUBLIC_TIER` value
- **D-02:** Separate `slog.Warn` line whenever `public_tier=users` is in effect, naming the override explicitly so it can't be missed in a log tail or a Grafana log panel:
  - `slog.Warn("public tier override active", slog.String("public_tier", "users"), slog.String("env", "PDBPLUS_PUBLIC_TIER"))`
- **D-03:** Both lines emit during startup in `cmd/peeringdb-plus/main.go`, after config parse, before HTTP listener starts.

### `/about` rendering
- **D-04:** New "Privacy & Sync" section on `/about`, rendered on both `/about` (HTML via templ) and `/ui/about` (terminal via lipgloss). Positioned **after** the existing Sync Status section so freshness comes first, then the privacy/auth model.
- **D-05:** Section content (both renderings):
  - **Sync mode:** Authenticated with PeeringDB API key / Anonymous (no key)
  - **Public tier:** `public` (anonymous callers see Public-only data) / `users` (anonymous callers see Users-tier data — internal/private deployment)
  - One-line plain-language explanation of what the current settings mean for an anonymous caller's data view
- **D-06:** When `public_tier=users`, the section visually flags the override (e.g. an amber/warning badge in HTML, a `!` indicator in terminal). The flag is informational, not alarmist — matches the WARN log's tone.

### OTel attribute
- **D-07:** Single attribute `pdbplus.privacy.tier` with values `public` or `users`, set on the inbound HTTP server span by the privacy-tier middleware (the same middleware from phase 59). Downstream ent-query spans inherit context via OTel propagation; no need to redundantly stamp them.
- **D-08:** Naming uses the project's `pdbplus.*` namespace (matches existing `pdbplus.sync.*`, `pdbplus.data.*`).
- **D-09:** Cardinality: 2 values only (`public`, `users`). Future tier additions inherit the same key. Safe for Grafana dashboards as a low-cardinality filter.

### Tests
- **D-10:** Startup log test: capture slog output during config parse, assert the `auth` and `public_tier` attributes match the expected values for each combination of (`PDBPLUS_PEERINGDB_API_KEY`, `PDBPLUS_PUBLIC_TIER`).
- **D-11:** Startup WARN test: assert the WARN line is emitted iff `public_tier=users`.
- **D-12:** `/about` test: render with each (mode, tier) combo, assert the section text + override-flag rendering.
- **D-13:** OTel attribute test: spin up the privacy middleware on a test span, set the tier, assert the attribute is on the span. Use the existing OTel test pattern in `internal/otel/`.

### Claude's Discretion
- Exact wording of the one-line plain-language explanation in D-05
- Visual treatment of the override flag in HTML (badge style, colour from existing palette)
- Terminal-mode override indicator character (lipgloss styling — pick something readable in 256 colours)

### Folded Todos
None.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plan-of-record
- `/home/dotwaffle/.claude/plans/ancient-tumbling-comet.md`
- `.planning/PROJECT.md` §"Current Milestone: v1.14"
- `.planning/REQUIREMENTS.md` — SYNC-04, OBS-01, OBS-02, OBS-03
- `.planning/ROADMAP.md` §"Phase 61: Operator-facing observability"

### Predecessor outputs
- `.planning/phases/59-ent-privacy-policy-sync-bypass/59-CONTEXT.md` — privacy middleware that sets the OTel attribute lives in code added by phase 59

### Existing code this phase modifies
- `cmd/peeringdb-plus/main.go` — startup logging block; D-01/D-02 emit here
- `internal/web/handler.go` — `/about` HTML handler; D-04/D-05 add the section
- `internal/web/termrender/about.go` (or equivalent) — `/ui/about` terminal renderer; D-04/D-05 add the section
- `internal/middleware/` (new privacy middleware from phase 59) — D-07 stamps the OTel attribute here
- `internal/otel/` — existing OTel setup; reuse patterns for the attribute test

### Project conventions
- `CLAUDE.md` §"Key Decisions" → "Single pdbplus.data.type.count gauge with type attribute" — establishes the `pdbplus.*` attribute namespace convention this phase follows
- Project quick task 260414-2rc reduced OTel cardinality ~30-55% — keep that spirit; 2 values per attribute is the right ceiling

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `slog.Info` with attribute setters (`slog.String`, `slog.Int`) is the established logging style — CLAUDE.md GO-OBS-5 explicitly endorses it
- `/about` page already exists in both HTML and terminal forms; pattern for adding sections is well-established
- OTel span attribute setting via `trace.SpanFromContext(ctx).SetAttributes(attribute.String(...))` already used elsewhere in the project

### Established Patterns
- Structured logging (slog) with consistent fields (CLAUDE.md GO-OBS-1)
- OTel attribute namespace `pdbplus.*` (verified across sync, data, and other custom attributes)
- Test pattern for slog capture: existing tests in `internal/otel/` use a custom handler to assert attributes

### Integration Points
- OTel attribute is consumed by Grafana dashboards (existing `pdbplus-overview.json`) — phase 61 doesn't update the dashboard but the attribute being there means a future dashboard PR can filter by it
- The WARN line at startup will be visible in the existing OTel logs pipeline (autoexport → Grafana Loki)

</code_context>

<specifics>
## Specific Ideas

- **No silent escalation.** The whole point of the WARN line and the `/about` flag is that a misconfigured deployment (e.g. operator typo'd `PDBPLUS_PUBLIC_TIER=users` thinking they were setting something else) is loud enough to catch in normal operations. Don't tone this down.
- **Two values, low cardinality.** Don't introduce intermediate tiers in this phase. `public` / `users` covers the milestone scope; v1.15 OAuth might add per-user identity but the *tier* axis stays binary for now.

</specifics>

<deferred>
## Deferred Ideas

- Per-request log line listing the resolved tier — would explode log volume; the OTel span attribute already covers per-request observability via Grafana.
- Audit log of when the override was last changed — that's an operations workflow concern, not an app concern.
- `/about` exposing per-attribute visibility statistics (e.g. "37 POCs hidden from anonymous callers") — interesting, but adds a query to a page that should stay cheap. Defer to a possible v1.15+ admin surface.

</deferred>

---

*Phase: 61-operator-facing-observability*
*Context gathered: 2026-04-16*
