package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	otelattr "go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/dotwaffle/peeringdb-plus/ent"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// When fkCheckParent finds a missing
// parent, attempt one live HTTP fetch from upstream to recover the row
// before declaring the child an orphan. This closes the structural-drop
// gap caused by upstream's soft-delete model never being represented in
// our DB before FK backfill existed (carrier 277→278 worth, 575+/day
// across all child types per production observation 2026-04-26).
//
// Per-cycle dedup cache prevents repeat fetches for the same (type, id)
// pair within one sync — if 200 child rows reference the same missing
// parent, we issue exactly ONE backfill fetch and let the in-cache
// short-circuit handle the next 199.
//
// Per-cycle cap (PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE, default 20)
// prevents runaway upstream traffic when many distinct parents are
// missing — cap-hit logs WARN with result=ratelimited and the remaining
// child rows fall through to the legacy drop-and-record-orphan path.
// Operators set the cap to 0 to disable backfill entirely (legacy
// behavior); a non-zero cap is the steady-state default.

// fkBackfillKey is the dedup-cache key — typeName + id pair, per-cycle scope.
type fkBackfillKey struct {
	Type string
	ID   int
}

// fkBackfillResult mirrors the otel attribute "result" so the same
// constant drives both the metric label and the function's internal
// flow.  Values: "hit", "miss", "ratelimited", "error".
type fkBackfillResult string

const (
	fkBackfillHit              fkBackfillResult = "hit"
	fkBackfillMiss             fkBackfillResult = "miss"
	fkBackfillRateLimited      fkBackfillResult = "ratelimited"
	fkBackfillError            fkBackfillResult = "error"
	fkBackfillDeadlineExceeded fkBackfillResult = "deadline_exceeded"
)

// fkBackfillParent is the single-row entry point preserved for the
// existing per-row callers in worker.go (carrier→org check at
// dispatchScratchChunk:fkCheckParent and the null-on-miss path of the
// fac campus_id and NetworkIxLan side FKs at nullOptionalFK). The body
// was refactored to a thin wrapper around fkBackfillBatch so single-row
// and batched paths share one HTTP / dedup / cap / deadline / recursion
// implementation.
//
// Returns true iff the row is now present in the local DB and the
// child can be linked.
//
// The fkMissing negative cache short-circuits repeat callers: once a
// parent's backfill has run this cycle and the row is confirmed absent,
// sibling child rows referencing the same parent skip both the (deduped,
// no-op) batch call and the repeat dbHasRecord Exist() query.
func (w *Worker) fkBackfillParent(ctx context.Context, tx *ent.Tx, childType, parentType string, parentID int) bool {
	key := fkBackfillKey{Type: parentType, ID: parentID}
	if _, missing := w.fkMissing[key]; missing {
		return false
	}
	w.fkBackfillBatch(ctx, tx, parentType, []int{parentID}, childType)
	if w.dbHasRecord(ctx, tx, parentType, parentID) {
		return true
	}
	w.fkMissing[key] = struct{}{}
	return false
}

// fkBackfillBatch is the dataloader-style entry point: given a set of
// missing parent IDs of a single parent type, it issues ONE batched
// HTTP request per ⌈len(ids)/100⌉ chunk via peeringdb.Client.FetchByIDs
// and upserts the returned rows. Recursive grandparent backfill walks
// each fetched row's own FKs, groups missing IDs per parent type, and
// recursively calls fkBackfillBatch — so a chunk of 50 carrierfacs
// missing 50 carriers each missing 50 distinct orgs collapses to
// exactly 2 batched HTTP requests (carriers, then orgs), bounded by
// the per-cycle dedup cache.
//
// Replaces the per-row HTTP fan-out from the original single-row
// backfill. Catch-up / recovery cycles with hundreds-to-thousands
// of distinct missing parents previously bricked v1.18.2 by hitting
// upstream's API_THROTTLE_REPEATED_REQUEST cap; batching collapses the
// exposure to a small constant per parent type per chunk.
//
// Semantics carried over from fkBackfillParent (preserved by all 7
// existing TestFKCheckParent_Backfill* tests via the thin wrapper):
//
//   - Dedup-first: ids already in fkBackfillTried are filtered out
//     BEFORE the cap check (so previously-tried IDs do not re-consume
//     cap budget).
//   - Cap is per-HTTP-request (v1.18.5): fkBackfillRequestCount is
//     bumped by ⌈len(idsToFetch)/peeringdb.FetchByIDsBatchSize⌉ — the
//     actual number of underlying HTTP requests this batch will issue
//     through the rate-limited transport. SEMANTIC SHIFT from the
//     earlier per-row cap, which was a weak circuit breaker once
//     batching collapsed N rows into 1 request. The cap now directly
//     bounds upstream HTTP traffic, which is the actual surface
//     protected by upstream's API_THROTTLE_REPEATED_REQUEST and our
//     local rate limiter. Default 20 requests/cycle (≈40s of upstream
//     pressure at 30 req/min auth) — generous but firm.
//     The dashboard interpretation of fk_backfill{result=hit} does not
//     change: still one hit per inserted row.
//   - Deadline check fires WITHOUT issuing any HTTP request once
//     fkBackfillDeadline has passed; all remaining IDs are recorded as
//     fkBackfillDeadlineExceeded.
//   - Cap overflow records fkBackfillRateLimited for each dropped ID.
//
// childType is the metric "type" attribute. Single-row callers and the
// chunk pre-pass pass the originating child type ("net", "carrierfac",
// …) — in the pre-pass every row in the chunk shares it. Only the
// recursive grandparent path passes the "recursive" sentinel, because
// no child row triggered that lookup.
//
// Single-writer: fkBackfillTried, fkBackfillRequestCount, fkBackfillDeadline,
// and fkRegistry are all touched here without locks because
// Worker.Sync is single-goroutine (Worker.running atomic guard
// serialises concurrent Sync calls). If sync ever fans out across
// goroutines, this map and counter need a sync.Mutex.
//
// Returns the IDs successfully inserted by this call (NOT including
// recursive grandparents). Callers who need a per-ID success answer
// should re-check via fkHasParent / dbHasRecord — the wrapper above
// does exactly that.
func (w *Worker) fkBackfillBatch(ctx context.Context, tx *ent.Tx, parentType string, ids []int, childType string) []int {
	if len(ids) == 0 {
		return nil
	}

	// 1. Dedup against per-cycle tried cache (ordering preserved).
	remaining := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		key := fkBackfillKey{Type: parentType, ID: id}
		if _, tried := w.fkBackfillTried[key]; tried {
			continue
		}
		remaining = append(remaining, id)
	}
	if len(remaining) == 0 {
		return nil
	}

	// 2. Deadline check fires BEFORE any HTTP. Mark tried so subsequent
	//    same-cycle attempts dedup-short-circuit instead of cascading
	//    more deadline_exceeded events through the metric.
	if !w.fkBackfillDeadline.IsZero() && time.Now().After(w.fkBackfillDeadline) {
		for _, id := range remaining {
			w.fkBackfillTried[fkBackfillKey{Type: parentType, ID: id}] = struct{}{}
			w.recordBackfill(ctx, childType, parentType, fkBackfillDeadlineExceeded)
		}
		w.logger.LogAttrs(ctx, slog.LevelWarn, "fk backfill deadline exceeded",
			slog.String("child_type", childType),
			slog.String("parent_type", parentType),
			slog.Int("ids_dropped", len(remaining)),
			slog.Time("deadline", w.fkBackfillDeadline))
		return nil
	}

	// 3. Cap budget: cap is on HTTP requests, not rows (v1.18.5). One
	//    FetchByIDs(N) call issues ⌈N/FetchByIDsBatchSize⌉ HTTP requests.
	//    Take a prefix that fits in the remaining request budget; mark
	//    the rest as ratelimited.
	availableRequests := max(w.fkBackfillRequestCap-w.fkBackfillRequestCount, 0)
	maxIDs := availableRequests * peeringdb.FetchByIDsBatchSize
	idsToFetch := remaining
	if len(idsToFetch) > maxIDs {
		dropped := idsToFetch[maxIDs:]
		idsToFetch = idsToFetch[:maxIDs]
		for _, id := range dropped {
			w.fkBackfillTried[fkBackfillKey{Type: parentType, ID: id}] = struct{}{}
			w.recordBackfill(ctx, childType, parentType, fkBackfillRateLimited)
		}
		w.logger.LogAttrs(ctx, slog.LevelWarn, "fk backfill cap reached",
			slog.String("child_type", childType),
			slog.String("parent_type", parentType),
			slog.Int("ids_dropped", len(dropped)),
			slog.Int("requests_remaining", availableRequests),
			slog.Int("request_cap", w.fkBackfillRequestCap))
	}
	if len(idsToFetch) == 0 {
		return nil
	}

	// 4. Mark all to-fetch IDs in tried BEFORE the HTTP — preserves the
	//    dedup invariant even if the HTTP fails partway through.
	for _, id := range idsToFetch {
		w.fkBackfillTried[fkBackfillKey{Type: parentType, ID: id}] = struct{}{}
	}
	// Consume one request unit per underlying HTTP chunk (ceil division).
	w.fkBackfillRequestCount += (len(idsToFetch) + peeringdb.FetchByIDsBatchSize - 1) / peeringdb.FetchByIDsBatchSize

	// 5. ONE batched fetch per ⌈N/100⌉ chunk via the rate-limited
	//    transport. Single-ID callers (the fkBackfillParent wrapper)
	//    still issue exactly ONE HTTP request — no behavioural change
	//    for the legacy hot path.
	raws, fetchErr := w.pdbClient.FetchByIDs(ctx, parentType, idsToFetch)
	if fetchErr != nil {
		for _, id := range idsToFetch {
			w.recordBackfill(ctx, childType, parentType, fkBackfillError)
			w.logger.LogAttrs(ctx, slog.LevelWarn, "fk backfill fetch failed",
				slog.String("child_type", childType),
				slog.String("parent_type", parentType),
				slog.Int("parent_id", id),
				slog.Any("error", fetchErr))
		}
		return nil
	}

	// 6. Decode each row's id, group missing grandparent FKs by parent
	//    type, then recursively batch-backfill before upserting parents.
	//    Recursion is bounded by the per-cycle dedup cache (each
	//    (type,id) pair fires exactly once across the whole cycle).
	type rawWithID struct {
		id  int
		raw []byte
	}
	rows := make([]rawWithID, 0, len(raws))
	gpMissing := make(map[string]map[int]struct{})
	for _, raw := range raws {
		var idHolder struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(raw, &idHolder); err != nil || idHolder.ID <= 0 {
			// Best-effort: skip rows we can't identify. The original
			// id__in still consumed its cap slot; the unrecoverable row
			// is re-tried by the next full-mode cycle's bare list
			// re-fetch (an incremental's MAX(updated) cursor has
			// typically advanced past it by then).
			continue
		}
		rows = append(rows, rawWithID{id: idHolder.ID, raw: raw})
		for _, gp := range parentFKsOf(parentType, raw) {
			if gp.ID == 0 {
				continue
			}
			if w.fkHasParent(ctx, tx, gp.ParentType, gp.ID) {
				continue
			}
			set, exists := gpMissing[gp.ParentType]
			if !exists {
				set = make(map[int]struct{})
				gpMissing[gp.ParentType] = set
			}
			set[gp.ID] = struct{}{}
		}
	}

	// Recurse one parent type at a time, sorted IDs for deterministic
	// URL shape (test assertions on id__in= rely on stable ordering).
	// childType="recursive" — the recursion is parent-driven, no child
	// row triggered it; the sentinel keeps the metric's type label
	// non-empty so dashboards don't render a blank series.
	for _, gpType := range slices.Sorted(maps.Keys(gpMissing)) {
		gpIDs := slices.Sorted(maps.Keys(gpMissing[gpType]))
		w.fkBackfillBatch(ctx, tx, gpType, gpIDs, "recursive")
	}

	// 7. Upsert each parent row; record per-row hit/error. Per-row
	//    upsert failures do NOT abort the batch — one bad row should
	//    not cascade-drop the rest of the chunk.
	//
	//    Before upserting, re-validate that every REQUIRED (non-null)
	//    grandparent FK of this parent is now present in the DB. The
	//    step-6 recursion can silently land NOTHING for a grandparent
	//    when it trips the per-cycle request cap, the deadline, an
	//    upstream miss, or a fetch error. Upserting the parent anyway
	//    would write a row whose non-NULL FK points at a missing
	//    grandparent. The per-statement FK check then fails that upsert
	//    (logged below as "fk backfill upsert failed"). Before
	//    2026-09-24 the sync tx deferred FK checks, so the dangling row
	//    failed the whole cycle at tx.Commit() with
	//    SQLITE_CONSTRAINT_FOREIGNKEY (787), and again on every cycle.
	//    Withholding the dangling parent here mirrors the fkFilter
	//    drop-on-miss contract: the orphan is recorded and the commit
	//    succeeds. Recovery comes from the next FULL-mode cycle,
	//    which re-fetches every row,
	//    stages the tombstone window, and relaxes the upsert skip gate
	//    (reconcile-all) — an incremental cycle does NOT retry the
	//    withheld row, because its MAX(updated) cursor has typically
	//    advanced past the row's updated by the time the cycle commits.
	//    Only REQUIRED FKs gate the upsert. Nullable FKs are not in
	//    parentFKSpec and never block here: nullMissingOptionalFKs sets
	//    a missing fac campus_id to NULL before the upsert, and backfill
	//    never lands a netixlan (the side FKs).
	returnedIDs := make(map[int]struct{}, len(rows))
	inserted := make([]int, 0, len(rows))
	for _, r := range rows {
		returnedIDs[r.id] = struct{}{}
		if gp, missing := w.firstMissingRequiredFK(ctx, tx, parentType, r.raw); missing {
			w.recordBackfill(ctx, childType, parentType, fkBackfillMiss)
			w.logger.LogAttrs(ctx, slog.LevelWarn, "fk backfill: parent withheld, required grandparent still missing",
				slog.String("child_type", childType),
				slog.String("parent_type", parentType),
				slog.Int("parent_id", r.id),
				slog.String("grandparent_type", gp.ParentType),
				slog.String("grandparent_field", gp.FieldName),
				slog.Int("grandparent_id", gp.ID))
			continue
		}
		raw := w.nullMissingOptionalFKs(ctx, tx, parentType, r.id, r.raw)
		if _, upsertErr := upsertSingleRaw(ctx, tx, parentType, raw); upsertErr != nil {
			w.recordBackfill(ctx, childType, parentType, fkBackfillError)
			w.logger.LogAttrs(ctx, slog.LevelWarn, "fk backfill upsert failed",
				slog.String("child_type", childType),
				slog.String("parent_type", parentType),
				slog.Int("parent_id", r.id),
				slog.Any("error", upsertErr))
			continue
		}
		inserted = append(inserted, r.id)
		w.recordBackfill(ctx, childType, parentType, fkBackfillHit)
		w.logger.LogAttrs(ctx, slog.LevelInfo, "fk backfill: parent inserted",
			slog.String("child_type", childType),
			slog.String("parent_type", parentType),
			slog.Int("parent_id", r.id))
	}
	if len(inserted) > 0 {
		// Mirror inserted parents into the in-memory FK registry so
		// subsequent same-cycle children find them without a DB round-
		// trip. Bulk-register in one call to avoid map churn.
		w.fkRegisterIDs(parentType, inserted)
	}

	// 8. IDs requested but not returned by upstream are truly absent
	//    (deleted both server-side and from any since=1 tombstone window
	//    older than the upstream retention). Record one miss per ID.
	for _, id := range idsToFetch {
		if _, ok := returnedIDs[id]; ok {
			continue
		}
		w.recordBackfill(ctx, childType, parentType, fkBackfillMiss)
		w.logger.LogAttrs(ctx, slog.LevelDebug, "fk backfill: parent absent upstream",
			slog.String("parent_type", parentType),
			slog.Int("parent_id", id))
	}

	return inserted
}

// parentFKRef names a single FK on a child type's row.
type parentFKRef struct {
	FieldName  string // JSON key on the upstream record
	ParentType string // peeringdb.Type* constant
	ID         int    // populated by parentFKsOf decode
}

// parentFKSpec maps each entity type to its required-non-null parent
// FK fields, mirroring the upstream Django on_delete=CASCADE FKs in
// peeringdb_server/models.py. Nullable FKs (Facility.campus_id,
// NetworkIXLan.net_side_id / ix_side_id) are omitted so the recursive
// backfill does not chase optional references. For chunk rows, the
// fkFilter closures in registry.go try a backfill and null them on a
// miss (nullOptionalFK). For backfilled rows, nullMissingOptionalFKs
// (optionalFKSpec) nulls them without a backfill.
//
// Mirrors the upstream FK audit table in CLAUDE.md § Soft-delete
// tombstones — keep these two in sync when a new FK is added.
var parentFKSpec = map[string][]parentFKRef{
	peeringdb.TypeOrg:        {},
	peeringdb.TypeCampus:     {{FieldName: "org_id", ParentType: peeringdb.TypeOrg}},
	peeringdb.TypeFac:        {{FieldName: "org_id", ParentType: peeringdb.TypeOrg}},
	peeringdb.TypeIX:         {{FieldName: "org_id", ParentType: peeringdb.TypeOrg}},
	peeringdb.TypeIXLan:      {{FieldName: "ix_id", ParentType: peeringdb.TypeIX}},
	peeringdb.TypeIXPfx:      {{FieldName: "ixlan_id", ParentType: peeringdb.TypeIXLan}},
	peeringdb.TypeIXFac:      {{FieldName: "ix_id", ParentType: peeringdb.TypeIX}, {FieldName: "fac_id", ParentType: peeringdb.TypeFac}},
	peeringdb.TypeCarrier:    {{FieldName: "org_id", ParentType: peeringdb.TypeOrg}},
	peeringdb.TypeCarrierFac: {{FieldName: "carrier_id", ParentType: peeringdb.TypeCarrier}, {FieldName: "fac_id", ParentType: peeringdb.TypeFac}},
	peeringdb.TypeNet:        {{FieldName: "org_id", ParentType: peeringdb.TypeOrg}},
	peeringdb.TypePoc:        {{FieldName: "net_id", ParentType: peeringdb.TypeNet}},
	peeringdb.TypeNetFac:     {{FieldName: "net_id", ParentType: peeringdb.TypeNet}, {FieldName: "fac_id", ParentType: peeringdb.TypeFac}},
	peeringdb.TypeNetIXLan:   {{FieldName: "net_id", ParentType: peeringdb.TypeNet}, {FieldName: "ixlan_id", ParentType: peeringdb.TypeIXLan}},
}

// optionalFKSpec lists the nullable FKs of the types that FK backfill
// can land. On the chunk path, the fkFilter closures in registry.go set
// a nullable FK to NULL when backfill cannot recover its parent. A
// backfilled row skips fkFilter, so nullMissingOptionalFKs sets the FK
// to NULL when its parent is missing.
// Without this, a backfilled facility stored a campus_id for a campus
// that is not in the database. NetworkIxLan side FKs are not listed:
// no type references netixlan, so backfill never lands one.
var optionalFKSpec = map[string][]parentFKRef{
	peeringdb.TypeFac: {{FieldName: "campus_id", ParentType: peeringdb.TypeCampus}},
}

// nullMissingOptionalFKs returns raw with each optionalFKSpec field set
// to null when its parent is not in the database. It records each one
// as an orphan with action "null". Unlike the fac fkFilter, it does not
// backfill the optional parent. A backfilled fac is not in the fac rows
// of the cycle, so in practice it is a deleted facility that only a
// child row references. A campus request for such a row would use the
// request budget that the required FKs of the cycle need. It returns raw
// unchanged when no parent is missing or when raw does not decode (the
// upsert then reports the decode error).
func (w *Worker) nullMissingOptionalFKs(ctx context.Context, tx *ent.Tx, typeName string, id int, raw []byte) []byte {
	spec := optionalFKSpec[typeName]
	if len(spec) == 0 {
		return raw
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return raw
	}
	changed := false
	for _, fk := range spec {
		rawVal, present := fields[fk.FieldName]
		if !present || string(rawVal) == "null" {
			continue
		}
		var parentID int
		if err := json.Unmarshal(rawVal, &parentID); err == nil && w.fkHasParent(ctx, tx, fk.ParentType, parentID) {
			continue
		}
		w.recordOrphan(ctx, fkOrphanKey{
			ChildType:  typeName,
			ParentType: fk.ParentType,
			Field:      fk.FieldName,
			Action:     "null",
		}, id, parentID)
		fields[fk.FieldName] = json.RawMessage("null")
		changed = true
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return out
}

// parentFKsOf decodes the upstream JSON for one row and returns the
// list of (FK field, parent type, parent id) tuples for required FKs.
// Returns nil for entity types with no parent FKs (e.g. org), or when
// JSON decoding fails (caller proceeds without recursive backfill —
// the parent upsert still happens via the existing path).
func parentFKsOf(parentType string, raw []byte) []parentFKRef {
	spec, ok := parentFKSpec[parentType]
	if !ok || len(spec) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	out := make([]parentFKRef, 0, len(spec))
	for _, fk := range spec {
		rawVal, present := fields[fk.FieldName]
		if !present || string(rawVal) == "null" {
			continue
		}
		var id int
		if err := json.Unmarshal(rawVal, &id); err != nil {
			continue
		}
		if id <= 0 {
			continue
		}
		out = append(out, parentFKRef{FieldName: fk.FieldName, ParentType: fk.ParentType, ID: id})
	}
	return out
}

// firstMissingRequiredFK reports the first REQUIRED parent FK of a
// freshly-fetched backfill row that is still absent from the local DB,
// after the step-6 grandparent recursion has run. It mirrors the
// fkFilter drop-on-miss decision: it checks each FK in parentFKSpec
// with fkHasParent (registry, then DB). An absent, null or nonpositive
// FK is missing, because the upsert would store 0 and reference a
// parent that cannot exist. Returns (ref, true) for the first missing
// required FK so the caller can withhold the dangling parent and record
// the orphan; (parentFKRef{}, false) when every required grandparent is
// present, or when raw does not decode (the upsert then reports the
// decode error).
//
// Nullable FKs (campus, net_side_id, ix_side_id) are not in
// parentFKSpec. nullMissingOptionalFKs handles campus.
func (w *Worker) firstMissingRequiredFK(ctx context.Context, tx *ent.Tx, parentType string, raw []byte) (parentFKRef, bool) {
	spec := parentFKSpec[parentType]
	if len(spec) == 0 {
		return parentFKRef{}, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return parentFKRef{}, false
	}
	for _, fk := range spec {
		ref := parentFKRef{FieldName: fk.FieldName, ParentType: fk.ParentType}
		// An absent, null or non-integer value leaves ref.ID at 0.
		_ = json.Unmarshal(fields[fk.FieldName], &ref.ID)
		if !w.fkHasParent(ctx, tx, ref.ParentType, ref.ID) {
			return ref, true
		}
	}
	return parentFKRef{}, false
}

// prefetchMissingParents is the chunk-level pre-pass, called ONCE per
// chunk from syncIncremental (via the prefetch hook) before the
// per-type fkFilter closures fire. Given the chunk's required parent
// FK references — produced by the per-type fkRefs accessors on the
// typed decode, so no second JSON pass over the raw rows — it groups
// missing-parent IDs per parent type and issues ONE batched
// fkBackfillBatch call per parent type. So a chunk of 50 carrierfacs
// missing 30 distinct carriers + 25 distinct facs collapses to
// exactly 2 batched HTTP calls (carriers, then facs) instead of 55
// sequential per-row HTTP calls through the legacy fkBackfillParent
// path.
//
// The per-cycle dedup cache (fkBackfillTried)
// makes the per-row fkCheckParent → fkBackfillParent path a no-op for
// any parent already loaded by this pre-pass.
//
// Errors from individual fkBackfillBatch calls are logged inside the
// batch path and do NOT abort the chunk — the legacy per-row path
// remains as a fallback for any IDs the pre-pass couldn't recover
// (the dedup cache will short-circuit them after they've been
// attempted).
//
// Single-writer: fkRegistry / fkBackfillTried writes are serialised by
// Worker.Sync (atomic Worker.running guard). Same assumption as
// fkBackfillBatch.
//
// Nullable FKs are not in the fkRefs accessors (mirroring
// parentFKSpec). The pre-pass never drops a row, it only fetches, so
// prefetchStagedFacCampuses also calls it once per type with the
// nullable fac campus_id refs. The NetworkIxLan side FKs are left to
// the per-row nullSideFK path.
func (w *Worker) prefetchMissingParents(ctx context.Context, tx *ent.Tx, chunkType string, refs []parentFKRef) {
	// Backfill disabled (cap=0 escape-hatch): mirror the fkCheckParent /
	// nullOptionalFK gates and skip the pre-pass entirely. Without this,
	// every chunk with missing parents would flow into fkBackfillBatch,
	// where a zero budget records every id as result="ratelimited" and
	// emits a per-chunk "fk backfill cap reached" WARN — falsely
	// suggesting cap pressure when the operator deliberately turned
	// backfill off.
	if w.fkBackfillRequestCap <= 0 {
		return
	}
	missing := make(map[string]map[int]struct{})
	for _, fk := range refs {
		if fk.ID <= 0 {
			continue
		}
		if w.fkHasParent(ctx, tx, fk.ParentType, fk.ID) {
			continue
		}
		set, exists := missing[fk.ParentType]
		if !exists {
			set = make(map[int]struct{})
			missing[fk.ParentType] = set
		}
		set[fk.ID] = struct{}{}
	}
	// Sequential per parent type — concurrent fetches would fight the
	// rate limiter and add zero throughput. Sorted parent-type
	// iteration for deterministic call ordering (test assertions on
	// recorded URL sequences depend on it).
	for _, parentType := range slices.Sorted(maps.Keys(missing)) {
		ids := slices.Sorted(maps.Keys(missing[parentType]))
		// Every row in the chunk is of chunkType, so the batched path
		// carries the same metric "type" attribute as the per-row path.
		// (This pre-pass handles most backfills since v1.18.5 — an empty
		// label here made dashboards grouping by type show one unlabeled
		// series absorbing the majority of activity.)
		w.fkBackfillBatch(ctx, tx, parentType, ids, chunkType)
	}
}

// scratchFacCampusIDsSQL selects the distinct campus ids that the
// staged facilities reference. A null or absent campus_id does not
// match. data is a BLOB; the CAST reads it as text JSON.
const scratchFacCampusIDsSQL = `SELECT DISTINCT json_extract(CAST(data AS TEXT), '$.campus_id') AS campus_id FROM "fac"
WHERE json_type(CAST(data AS TEXT), '$.campus_id') = 'integer'
ORDER BY campus_id`

// prefetchStagedFacCampuses backfills the missing campuses of all
// staged facilities before the first fac chunk replays (the fac
// descriptor's prefetchStaged pass). It sends one request for each
// peeringdb.FetchByIDsBatchSize missing campuses. prefetchMissingParents
// applies the per-cycle dedup, request cap and deadline.
//
// Why one pass for the type and not the chunk pre-pass: upstream keeps
// a campus pending while it has fewer than two facilities, and a bare
// /api/campus list holds only ok campuses. So each missing campus has
// at most one facility, and on a first sync the missing campuses are
// spread across the fac chunks. The chunk pre-pass sends one request
// for each chunk that holds one. That uses the request cap that the
// required FKs of the later types share, and can use all of it before
// they run.
//
// campus_id is nullable: a campus that stays missing does not drop the
// facility. The fac fkFilter sets campus_id to NULL (nullOptionalFK),
// and its backfill attempt sends no second request for an id that
// this pass tried. A scratch read error fails the type, as it does in
// drainChunk.
func (w *Worker) prefetchStagedFacCampuses(ctx context.Context, tx *ent.Tx, scratch *scratchDB) error {
	if w.fkBackfillRequestCap <= 0 {
		return nil
	}
	ids, err := queryIDs(ctx, scratch.db, scratchFacCampusIDsSQL)
	if err != nil {
		return fmt.Errorf("read staged fac campus ids: %w", err)
	}
	refs := make([]parentFKRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, parentFKRef{FieldName: "campus_id", ParentType: peeringdb.TypeCampus, ID: id})
	}
	w.prefetchMissingParents(ctx, tx, peeringdb.TypeFac, refs)
	return nil
}

// recordBackfill emits the per-attempt fk_backfill counter.
func (w *Worker) recordBackfill(ctx context.Context, childType, parentType string, result fkBackfillResult) {
	pdbotel.SyncFKBackfill.Add(ctx, 1, metric.WithAttributes(
		otelattr.String("type", childType),
		otelattr.String("parent_type", parentType),
		otelattr.String("result", string(result)),
	))
}
