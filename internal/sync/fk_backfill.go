package sync

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	otelattr "go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/dotwaffle/peeringdb-plus/ent"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
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
	fkBackfillHit         fkBackfillResult = "hit"
	fkBackfillMiss        fkBackfillResult = "miss"
	fkBackfillRateLimited fkBackfillResult = "ratelimited"
	fkBackfillError       fkBackfillResult = "error"
)

// fkBackfillParent attempts to fetch a missing parent from upstream and
// upsert it (preserving upstream status — could be ok, deleted, or
// pending). Returns true iff the row is now present in the local DB
// and the child can be linked.
//
// Caller MUST have already established that the parent is absent
// locally (fkHasParent returned false). This function:
//
//  1. Checks per-cycle dedup cache; on hit, re-checks DB rather than re-fetching.
//  2. Checks per-cycle cap; cap-hit → result=ratelimited, return false.
//  3. Fetches /api/<type>?since=1&id__in=<id>. since=1 ensures
//     status='deleted' rows surface (rest.py:694-727 status × since
//     matrix).
//  4. On 200 + non-empty data: upserts via upsertSingleRaw; result=hit.
//  5. On 200 + empty data (truly absent upstream): result=miss.
//  6. On HTTP error: result=error.
//
// childType is included in the metric attributes for grep symmetry with
// pdbplus.sync.type.orphans{type,parent_type,...} so dashboards can
// pivot either axis (which child type is causing backfill pressure vs
// which parent type is most often missing).
func (w *Worker) fkBackfillParent(ctx context.Context, tx *ent.Tx, childType, parentType string, parentID int) bool {
	key := fkBackfillKey{Type: parentType, ID: parentID}

	// Dedup: same (type,id) within this cycle → re-check DB rather than
	// re-fetch. The first attempt either landed the row (DB now has it)
	// or recorded the miss; subsequent same-cycle fkHasParent() lookups
	// against a re-checked DB give the right answer cheaply.
	if _, tried := w.fkBackfillTried[key]; tried {
		return w.dbHasRecord(ctx, tx, parentType, parentID)
	}
	w.fkBackfillTried[key] = struct{}{}

	if w.fkBackfillCount >= w.fkBackfillCap {
		w.recordBackfill(ctx, childType, parentType, fkBackfillRateLimited)
		w.logger.LogAttrs(ctx, slog.LevelWarn, "fk backfill cap reached",
			slog.String("child_type", childType),
			slog.String("parent_type", parentType),
			slog.Int("parent_id", parentID),
			slog.Int("cap", w.fkBackfillCap))
		return false
	}
	w.fkBackfillCount++

	raw, fetchErr := w.fetchSingleByID(ctx, parentType, parentID)
	if fetchErr != nil {
		w.recordBackfill(ctx, childType, parentType, fkBackfillError)
		w.logger.LogAttrs(ctx, slog.LevelWarn, "fk backfill fetch failed",
			slog.String("child_type", childType),
			slog.String("parent_type", parentType),
			slog.Int("parent_id", parentID),
			slog.Any("error", fetchErr))
		return false
	}
	if len(raw) == 0 {
		w.recordBackfill(ctx, childType, parentType, fkBackfillMiss)
		w.logger.LogAttrs(ctx, slog.LevelDebug, "fk backfill: parent absent upstream",
			slog.String("parent_type", parentType),
			slog.Int("parent_id", parentID))
		return false
	}

	if _, upsertErr := upsertSingleRaw(ctx, tx, parentType, raw); upsertErr != nil {
		w.recordBackfill(ctx, childType, parentType, fkBackfillError)
		w.logger.LogAttrs(ctx, slog.LevelWarn, "fk backfill upsert failed",
			slog.String("child_type", childType),
			slog.String("parent_type", parentType),
			slog.Int("parent_id", parentID),
			slog.Any("error", upsertErr))
		return false
	}

	// Mirror the parent into the in-memory FK registry so subsequent
	// same-cycle children find the parent without another DB round-trip.
	w.fkRegisterIDs(parentType, []int{parentID})

	w.recordBackfill(ctx, childType, parentType, fkBackfillHit)
	w.logger.LogAttrs(ctx, slog.LevelInfo, "fk backfill: parent inserted",
		slog.String("child_type", childType),
		slog.String("parent_type", parentType),
		slog.Int("parent_id", parentID))
	return true
}

// fetchSingleByID issues a list query restricted to one ID with since=1
// to catch tombstones. Returns the single raw JSON object or nil if
// upstream returned an empty array.
//
// Uses since=1 (not the detail endpoint /api/<type>/<id>) because per
// rest.py:694-727 the detail endpoint filters to status IN ('ok',
// 'pending') only — deleted rows return 404. The list endpoint with
// since=1 returns ['ok', 'deleted'] (+ 'pending' for campus), which
// lets us recover the actual upstream state including tombstones.
func (w *Worker) fetchSingleByID(ctx context.Context, typeName string, id int) ([]byte, error) {
	q := url.Values{}
	q.Set("since", "1")
	q.Set("id__in", fmt.Sprintf("%d", id))
	raws, err := w.pdbClient.FetchRaw(ctx, typeName, q)
	if err != nil {
		return nil, err
	}
	if len(raws) == 0 {
		return nil, nil
	}
	return raws[0], nil
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
