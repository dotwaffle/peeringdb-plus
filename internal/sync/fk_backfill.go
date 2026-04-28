package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
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

	// v1.18.3: per-cycle wall-clock deadline. Backfill HTTP calls happen
	// inside the sync tx; without a deadline a cascade of slow / rate-
	// limited backfills could hold the tx open for tens of minutes,
	// stalling LiteFS replication. After the deadline fire, drop the
	// orphan and let the next cycle pick it up.
	if !w.fkBackfillDeadline.IsZero() && time.Now().After(w.fkBackfillDeadline) {
		w.recordBackfill(ctx, childType, parentType, fkBackfillDeadlineExceeded)
		w.logger.LogAttrs(ctx, slog.LevelWarn, "fk backfill deadline exceeded",
			slog.String("child_type", childType),
			slog.String("parent_type", parentType),
			slog.Int("parent_id", parentID),
			slog.Time("deadline", w.fkBackfillDeadline))
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

	// v1.18.3 recursive backfill: before upserting the parent, walk its
	// own FK fields and recursively backfill any missing grandparents.
	// Bounded by the per-cycle dedup cache (each (type,id) pair fires
	// exactly once) and the per-cycle cap. Max effective depth is the
	// schema's longest FK chain (currently 3-hop: netixlan → ixlan → ix
	// → org). Grandparent failures don't block parent insert — the
	// schema's Optional().Nillable() FKs accept dangling references, so
	// "parent with maybe-dangling FK" is strictly better than "missing
	// parent + dropped child".
	for _, gp := range parentFKsOf(parentType, raw) {
		if gp.ID == 0 {
			continue
		}
		if w.fkHasParent(ctx, tx, gp.ParentType, gp.ID) {
			continue
		}
		// Best-effort: ignore return value (see comment above).
		w.fkBackfillParent(ctx, tx, parentType, gp.ParentType, gp.ID)
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
