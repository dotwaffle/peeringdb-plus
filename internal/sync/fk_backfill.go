package sync

import (
	"context"
	"encoding/json"
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

// Quick task 260428-2zl Task 3: when fkCheckParent finds a missing
// parent, attempt one live HTTP fetch from upstream to recover the row
// before declaring the child an orphan. This closes the structural-drop
// gap caused by upstream's soft-delete model never being represented in
// our DB pre-2zl (carrier 277→278 worth, 575+/day across all child
// types per production observation 2026-04-26).
//
// Per-cycle dedup cache prevents repeat fetches for the same (type, id)
// pair within one sync — if 200 child rows reference the same missing
// parent, we issue exactly ONE backfill fetch and let the in-cache
// short-circuit handle the next 199.
//
// Per-cycle cap (PDBPLUS_FK_BACKFILL_MAX_PER_CYCLE, default 200)
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
// dispatchScratchChunk:fkCheckParent and the NetworkIxLan side-FK
// null-on-miss path at nullSideFK). Quick task 260428-5xt refactored
// the body to a thin wrapper around fkBackfillBatch so single-row and
// batched paths share one HTTP / dedup / cap / deadline / recursion
// implementation.
//
// Returns true iff the row is now present in the local DB and the
// child can be linked.
func (w *Worker) fkBackfillParent(ctx context.Context, tx *ent.Tx, childType, parentType string, parentID int) bool {
	w.fkBackfillBatch(ctx, tx, parentType, []int{parentID}, childType)
	return w.dbHasRecord(ctx, tx, parentType, parentID)
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
// Quick task 260428-5xt — replaces the per-row HTTP fan-out from quick
// task 260428-2zl. Catch-up / recovery cycles with hundreds-to-thousands
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
//   - Cap is per-row (not per-HTTP-request): fkBackfillCount is bumped
//     by len(idsToFetch). SEMANTIC SHIFT from 260428-2zl, where the cap
//     was effectively 1-per-call. The dashboard interpretation of
//     fk_backfill{result=hit} does not change — both old and new code
//     emit one hit per inserted row — but the cap now meaningfully
//     limits total parent rows fetched per cycle, regardless of how
//     they're batched. Documented at the metric call site below.
//   - Deadline check fires WITHOUT issuing any HTTP request once
//     fkBackfillDeadline has passed; all remaining IDs are recorded as
//     fkBackfillDeadlineExceeded.
//   - Cap overflow records fkBackfillRateLimited for each dropped ID.
//
// childType is the metric "type" attribute. Single-row callers pass
// the originating child type ("net", "carrierfac", …); the chunk
// pre-pass (Task 3) and recursive grandparent path pass "" because no
// single child triggered the lookup.
//
// Single-writer: fkBackfillTried, fkBackfillCount, fkBackfillDeadline,
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

	// 3. Cap budget: take a prefix that fits, mark the rest as
	//    ratelimited. SEMANTIC SHIFT (260428-5xt): cap is now per-row,
	//    so a single batch with N IDs consumes N units of cap budget.
	//    See godoc above.
	available := w.fkBackfillCap - w.fkBackfillCount
	if available < 0 {
		available = 0
	}
	idsToFetch := remaining
	if len(idsToFetch) > available {
		dropped := idsToFetch[available:]
		idsToFetch = idsToFetch[:available]
		for _, id := range dropped {
			w.fkBackfillTried[fkBackfillKey{Type: parentType, ID: id}] = struct{}{}
			w.recordBackfill(ctx, childType, parentType, fkBackfillRateLimited)
		}
		w.logger.LogAttrs(ctx, slog.LevelWarn, "fk backfill cap reached",
			slog.String("child_type", childType),
			slog.String("parent_type", parentType),
			slog.Int("ids_dropped", len(dropped)),
			slog.Int("cap", w.fkBackfillCap))
	}
	if len(idsToFetch) == 0 {
		return nil
	}

	// 4. Mark all to-fetch IDs in tried BEFORE the HTTP — preserves the
	//    dedup invariant even if the HTTP fails partway through.
	for _, id := range idsToFetch {
		w.fkBackfillTried[fkBackfillKey{Type: parentType, ID: id}] = struct{}{}
	}
	w.fkBackfillCount += len(idsToFetch)

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
			// will be re-tried on the next cycle if upstream returns it.
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
	// childType="" — recursion is parent-driven, not child-driven.
	for _, gpType := range slices.Sorted(maps.Keys(gpMissing)) {
		gpIDs := slices.Sorted(maps.Keys(gpMissing[gpType]))
		w.fkBackfillBatch(ctx, tx, gpType, gpIDs, "")
	}

	// 7. Upsert each parent row; record per-row hit/error. Per-row
	//    upsert failures do NOT abort the batch — one bad row should
	//    not cascade-drop the rest of the chunk.
	returnedIDs := make(map[int]struct{}, len(rows))
	inserted := make([]int, 0, len(rows))
	for _, r := range rows {
		returnedIDs[r.id] = struct{}{}
		if _, upsertErr := upsertSingleRaw(ctx, tx, parentType, r.raw); upsertErr != nil {
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
// NetworkIXLan.net_side_id / ix_side_id) are handled by the existing
// fkFilter null-on-miss path in worker.go; they're omitted here so the
// recursive backfill doesn't gratuitously chase optional references.
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

// prefetchMissingParentsForChunk is the chunk-level pre-pass that
// runs ONCE per chunk in dispatchScratchChunk before the per-type
// fkFilter closures fire. For each FK field declared in
// parentFKSpec[chunkType], it walks every row in the chunk, groups
// missing required-parent IDs per parent type, and issues ONE batched
// fkBackfillBatch call per parent type. So a chunk of 50 carrierfacs
// missing 30 distinct carriers + 25 distinct facs collapses to
// exactly 2 batched HTTP calls (carriers, then facs) instead of 55
// sequential per-row HTTP calls through the legacy fkBackfillParent
// path.
//
// Quick task 260428-5xt: the per-cycle dedup cache (fkBackfillTried)
// makes the per-row fkCheckParent → fkBackfillParent path a no-op for
// any parent already loaded by this pre-pass — the dispatch order is
// pre-pass first, dispatch switch after.
//
// No-ops for entity types with no required parent FKs (org). Errors
// from individual fkBackfillBatch calls are logged inside the batch
// path and do NOT abort the chunk — the legacy per-row path remains
// as a fallback for any IDs the pre-pass couldn't recover (the
// dedup cache will short-circuit them after they've been attempted).
//
// Single-writer: fkRegistry / fkBackfillTried writes are serialised by
// Worker.Sync (atomic Worker.running guard). Same assumption as
// fkBackfillBatch.
//
// Nullable FKs (Facility.campus_id, NetworkIxLan.{net_side_id,
// ix_side_id}) are intentionally NOT batched here — they're handled
// by the existing fkFilter null-on-miss path. The goal is to batch the
// REQUIRED FKs that drive drops, not the optional FKs that drive
// null-overrides. A future quick task could add a nullableFKSpec for
// the optional path if the dashboard ever shows them as a hot source
// of per-row HTTP fan-out.
func (w *Worker) prefetchMissingParentsForChunk(ctx context.Context, tx *ent.Tx, chunkType string, rows []scratchRow) {
	spec, ok := parentFKSpec[chunkType]
	if !ok || len(spec) == 0 {
		// Org has no required parents; nothing to prefetch.
		return
	}
	missing := make(map[string]map[int]struct{})
	for i := range rows {
		for _, fk := range parentFKsOf(chunkType, rows[i].raw) {
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
	}
	// Sequential per parent type — concurrent fetches would fight the
	// rate limiter and add zero throughput. Sorted parent-type
	// iteration for deterministic call ordering (test assertions on
	// recorded URL sequences depend on it).
	for _, parentType := range slices.Sorted(maps.Keys(missing)) {
		ids := slices.Sorted(maps.Keys(missing[parentType]))
		// childType="" — chunk-driven, no single child triggered the
		// lookup. The metric still records parent_type correctly.
		w.fkBackfillBatch(ctx, tx, parentType, ids, "")
	}
}

// recordBackfill emits the per-attempt fk_backfill counter.
// Nil-guarded because tests run without InitMetrics().
func (w *Worker) recordBackfill(ctx context.Context, childType, parentType string, result fkBackfillResult) {
	if pdbotel.SyncFKBackfill == nil {
		return
	}
	pdbotel.SyncFKBackfill.Add(ctx, 1, metric.WithAttributes(
		otelattr.String("type", childType),
		otelattr.String("parent_type", parentType),
		otelattr.String("result", string(result)),
	))
}
