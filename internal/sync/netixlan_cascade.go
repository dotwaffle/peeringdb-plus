package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// cascadeVerifiedSQL marks deleted the live netixlans in ?1 (a JSON array
// of the ids that verifyNetIxLanCandidates found gone upstream). It
// checks the guard and the network's status again against the state
// after the upsert pass: a network undeleted in this cycle makes the
// subquery NULL, and a staged upstream tombstone that already landed is
// no longer live, so neither row changes. updated does not change.
const cascadeVerifiedSQL = `UPDATE network_ix_lans SET status = 'deleted', operational = false
WHERE id IN (SELECT value FROM json_each(?1))
  AND likely(status IN ('ok', 'not-operational'))
  AND updated <= (SELECT n.updated FROM networks AS n
                  WHERE n.id = network_ix_lans.net_id AND n.status = 'deleted')
RETURNING net_id`

// cascadePlan is the work of one cascadeDeletedNetIxLans call. Gone holds
// the ids that verifyNetIxLanCandidates found gone upstream.
type cascadePlan struct {
	Mode string
	Gone []int
}

// cascadeResult counts the rows that one cascadeDeletedNetIxLans call
// marked deleted, and their distinct networks. Backlog counts the rows
// marked through Gone: rows of networks that were deleted before this
// cycle.
type cascadeResult struct {
	Rows    int
	Nets    int
	Backlog int
}

// cascadeDeletedNetIxLans sets status 'deleted' and operational false on
// the live netixlans of deleted networks that p selects.
//
// Why: upstream pdb_rir_status (2.83.0 pdb_rir_status.py:440-443) deletes
// the live netixlans of a reclaimed network with SQL and then soft-deletes
// the network. No netixlan tombstone ever reaches ?since, so without the
// cascade every surface keeps serving those connections live.
//
// Scope: a row changes only when it is live, its network is deleted, and
// its updated is not later than the network's. A later row was saved
// after the network's delete. The function does not change updated: the
// incremental cursor is MAX(updated), and the upsert gate keeps the
// tombstone against a stale re-list with the same updated (see
// netIxLanUpsertPredicate). When no row matches, the UPDATE writes no
// page.
//
// Callers: Worker.Sync in each sync transaction after the upsert pass and
// the poc scrub, and cascadeNetIxLansAtStartup in a transaction of its
// own. The caller emits pdbplus.sync.type.deleted after its commit (see
// recordCascadeDeleted).
//
// Observability: a WARN log line with the counts when rows change (DEBUG
// when none do), and the pdbplus.sync.netixlans_cascaded and
// pdbplus.sync.netixlans_cascaded_backlog attributes on the
// sync-cascade-netixlan-deletes span.
func cascadeDeletedNetIxLans(ctx context.Context, tx *ent.Tx, logger *slog.Logger, p cascadePlan) (cascadeResult, error) {
	ctx, span := otel.Tracer("sync").Start(ctx, "sync-cascade-netixlan-deletes")
	defer span.End()

	var res cascadeResult
	nets := make(map[int]struct{})
	if len(p.Gone) > 0 {
		n, err := runNetIxLanCascade(ctx, tx, cascadeVerifiedSQL, p.Gone, nets)
		if err != nil {
			return cascadeResult{}, err
		}
		res.Rows += n
		res.Backlog += n
	}
	res.Nets = len(nets)

	span.SetAttributes(
		attribute.Int("pdbplus.sync.netixlans_cascaded", res.Rows),
		attribute.Int("pdbplus.sync.netixlans_cascaded_backlog", res.Backlog),
	)
	logCascadedNetIxLans(ctx, logger, p.Mode, res)
	return res, nil
}

// runNetIxLanCascade runs one cascade UPDATE with ids bound as a JSON
// array, adds the net_id of each changed row to nets, and returns the
// number of changed rows.
func runNetIxLanCascade(ctx context.Context, tx *ent.Tx, query string, ids []int, nets map[int]struct{}) (int, error) {
	rows, err := tx.QueryContext(ctx, query, jsonIDs(ids))
	if err != nil {
		return 0, fmt.Errorf("cascade network deletes to netixlans: %w", err)
	}
	defer func() { _ = rows.Close() }()
	n := 0
	for rows.Next() {
		var netID int
		if err := rows.Scan(&netID); err != nil {
			return 0, fmt.Errorf("scan cascaded netixlan: %w", err)
		}
		nets[netID] = struct{}{}
		n++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("cascade network deletes to netixlans: %w", err)
	}
	return n, nil
}

// logCascadedNetIxLans logs the result of a cascade: WARN when rows
// changed, DEBUG when none did.
func logCascadedNetIxLans(ctx context.Context, logger *slog.Logger, mode string, res cascadeResult) {
	level := slog.LevelDebug
	if res.Rows > 0 {
		level = slog.LevelWarn
	}
	logger.LogAttrs(ctx, level, "cascaded network deletes to netixlans",
		slog.Int("count", res.Rows),
		slog.Int("nets", res.Nets),
		slog.Int("backlog", res.Backlog),
		slog.String("mode", mode),
	)
}

// prepareNetIxLanCascade runs the Phase A part of the cascade for a sync
// cycle: it reads the netixlan cursor and verifies the candidates, which
// can stage upstream tombstones into scratch. It returns the plan for
// cascadeDeletedNetIxLans. It never fails the cycle: an unknown cursor
// only defers upstream tombstones to the next cycle.
func (w *Worker) prepareNetIxLanCascade(ctx context.Context, scratch *scratchDB, mode config.SyncMode, now time.Time) cascadePlan {
	cursor, err := GetMaxUpdated(ctx, w.db, entityTables[peeringdb.TypeNetIXLan])
	if err != nil {
		w.logger.LogAttrs(ctx, slog.LevelDebug, "netixlan cursor read failed, deferring upstream netixlan tombstones",
			slog.Any("error", err))
	}
	v := w.verifyNetIxLanCandidates(ctx, netIxLanVerifyInput{
		Scratch:     scratch,
		NixCursor:   cursor,
		CursorKnown: err == nil,
		Now:         now,
		Mode:        string(mode),
	})
	return cascadePlan{Mode: string(mode), Gone: v.Gone}
}

// cascadeNetIxLansAtStartup verifies the cascade candidates and runs
// cascadeDeletedNetIxLans once, in its own short transaction.
// StartScheduler calls it on the primary before it waits for the first
// sync cycle.
//
// Why: the first cycle after a restart can start up to one interval
// later. After an upgrade, the rows that upstream removed before the
// upgrade would stay live until then.
//
// Verification runs before the transaction opens. An upstream deleted
// verdict is left for the first cycle, which stages the upstream row, so
// the row keeps upstream's own updated.
//
// The run holds the running latch, so it does not overlap a sync cycle;
// when a cycle holds the latch, the run is skipped, because that cycle
// runs the cascade itself. A panic is recovered and logged, so a bad row
// cannot crash the primary on every start. An error is logged at WARN
// and not returned, because each sync cycle retries the cascade.
func (w *Worker) cascadeNetIxLansAtStartup(ctx context.Context) {
	if !w.running.CompareAndSwap(false, true) {
		w.logger.LogAttrs(ctx, slog.LevelDebug, "sync cycle running, skipping startup netixlan cascade")
		return
	}
	defer w.running.Store(false)
	defer recoverStartupPanic(ctx, w.logger, "startup netixlan cascade panic recovered")

	const mode = "startup"
	v := w.verifyNetIxLanCandidates(ctx, netIxLanVerifyInput{Now: time.Now(), Mode: mode})
	plan := cascadePlan{Mode: mode, Gone: v.Gone}
	if len(plan.Gone) == 0 {
		logCascadedNetIxLans(ctx, w.logger, mode, cascadeResult{})
		return
	}
	res, err := cascadeNetIxLansInTx(ctx, w.entClient, w.logger, plan)
	if err != nil {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "startup netixlan cascade failed, the next sync cycle retries it",
			slog.Any("error", err))
		return
	}
	recordCascadeDeleted(ctx, res.Rows)
}

// recoverStartupPanic is the deferred panic firewall of a startup run. It
// logs msg at ERROR with the panic value and the stack. It records no
// sync failure, because no sync cycle ran. Non-panic returns are a no-op.
func recoverStartupPanic(ctx context.Context, logger *slog.Logger, msg string) {
	rec := recover()
	if rec == nil {
		return
	}
	logger.LogAttrs(ctx, slog.LevelError, msg,
		slog.Any("panic", rec),
		slog.String("stack", string(debug.Stack())),
	)
}

// cascadeNetIxLansInTx runs cascadeDeletedNetIxLans in a transaction of
// its own and commits it.
func cascadeNetIxLansInTx(ctx context.Context, client *ent.Client, logger *slog.Logger, p cascadePlan) (cascadeResult, error) {
	tx, err := client.Tx(ctx)
	if err != nil {
		return cascadeResult{}, fmt.Errorf("begin netixlan cascade transaction: %w", err)
	}
	res, err := cascadeDeletedNetIxLans(ctx, tx, logger, p)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			err = errors.Join(err, fmt.Errorf("rollback netixlan cascade transaction: %w", rbErr))
		}
		return cascadeResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return cascadeResult{}, fmt.Errorf("commit netixlan cascade transaction: %w", err)
	}
	return res, nil
}

// recordCascadeDeleted adds the cascaded rows to
// pdbplus.sync.type.deleted{type=netixlan}. Call it only after the
// transaction that ran the cascade commits.
func recordCascadeDeleted(ctx context.Context, rows int) {
	if rows <= 0 {
		return
	}
	pdbotel.SyncTypeDeleted.Add(ctx, int64(rows),
		metric.WithAttributes(attribute.String("type", peeringdb.TypeNetIXLan)))
}

// jsonIDs returns ids as a JSON array for json_each. An empty or nil list
// gives "[]": json.Marshal of a nil slice gives "null", and json_each
// reads 'null' as one NULL row.
func jsonIDs(ids []int) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(id))
	}
	b.WriteByte(']')
	return b.String()
}
