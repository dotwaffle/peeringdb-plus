package sync

import (
	"context"
	"database/sql"
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

// cascadeRIRNetsSQL marks deleted the live netixlans of the networks in
// ?1 (a JSON array of the ids that rirTransitionNets returned). The guard
// and the network's status are checked again against the state after the
// upsert pass, as in cascadeVerifiedSQL. updated does not change.
const cascadeRIRNetsSQL = `UPDATE network_ix_lans SET status = 'deleted', operational = false
WHERE net_id IN (SELECT value FROM json_each(?1))
  AND likely(status IN ('ok', 'not-operational'))
  AND updated <= (SELECT n.updated FROM networks AS n
                  WHERE n.id = network_ix_lans.net_id AND n.status = 'deleted')
RETURNING net_id`

// scratchRIRTombstonesSQL selects the network tombstones in scratch that
// carry the signature of the pdb_rir_status reclaim in their upstream
// JSON: rir_status present and null, rir_status_updated a string. It
// reads the JSON and not the stored columns, because the batch upsert can
// keep a stale stored rir_status. A missing rir_status key does not
// match. data is a BLOB; the CAST reads it as text JSON.
const scratchRIRTombstonesSQL = `SELECT id FROM "net"
WHERE json_extract(CAST(data AS TEXT), '$.status') = 'deleted'
  AND json_type(CAST(data AS TEXT), '$.rir_status') = 'null'
  AND json_type(CAST(data AS TEXT), '$.rir_status_updated') = 'text'
ORDER BY id`

// liveAtStartNetsSQL keeps the network ids in ?1 (a JSON array) that are
// not deleted in the committed DB. An id that is not in the DB is kept.
const liveAtStartNetsSQL = `SELECT j.value FROM json_each(?1) AS j
WHERE NOT EXISTS (SELECT 1 FROM networks AS n WHERE n.id = j.value AND n.status = 'deleted')`

// cascadePlan is the work of one cascadeDeletedNetIxLans call. Gone holds
// the ids that verifyNetIxLanCandidates found gone upstream. RIRNets
// holds the networks that rirTransitionNets found deleted by the RIR
// reclaim in this cycle.
type cascadePlan struct {
	Mode    string
	Gone    []int
	RIRNets []int
}

// cascadeResult counts the rows that one cascadeDeletedNetIxLans call
// marked deleted, and their distinct networks. Backlog counts the rows
// marked through Gone: rows of networks that were deleted before this
// cycle. Rows marked through RIRNets are not backlog.
type cascadeResult struct {
	Rows    int
	Nets    int
	Backlog int
}

// cascadeDeletedNetIxLans sets status 'deleted' and operational false on
// the live netixlans of deleted networks that p selects: the rows in
// p.Gone, which upstream no longer returns live, and the rows of the
// networks in p.RIRNets, which the RIR reclaim deleted in this cycle.
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
	if len(p.RIRNets) > 0 {
		n, err := runNetIxLanCascade(ctx, tx, cascadeRIRNetsSQL, p.RIRNets, nets)
		if err != nil {
			return cascadeResult{}, err
		}
		res.Rows += n
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
// cycle: it reads the netixlan cursor, finds the networks that the RIR
// reclaim deleted in this cycle, and verifies the candidates, which can
// stage upstream tombstones into scratch. It returns the plan for
// cascadeDeletedNetIxLans. It never fails the cycle: an unknown cursor
// only defers upstream tombstones to the next cycle, and a failed RIR
// read leaves those networks to the verified path.
func (w *Worker) prepareNetIxLanCascade(ctx context.Context, scratch *scratchDB, mode config.SyncMode, now time.Time) cascadePlan {
	cursor, err := GetMaxUpdated(ctx, w.db, entityTables[peeringdb.TypeNetIXLan])
	if err != nil {
		w.logger.LogAttrs(ctx, slog.LevelDebug, "netixlan cursor read failed, deferring upstream netixlan tombstones",
			slog.Any("error", err))
	}
	rirNets, rirErr := rirTransitionNets(ctx, scratch, w.db)
	if rirErr != nil {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "rir reclaim read failed, netixlans wait for verification",
			slog.String("mode", string(mode)),
			slog.Any("error", rirErr))
	}
	v := w.verifyNetIxLanCandidates(ctx, netIxLanVerifyInput{
		Scratch:     scratch,
		NixCursor:   cursor,
		CursorKnown: err == nil,
		Now:         now,
		Mode:        string(mode),
	})
	return cascadePlan{Mode: string(mode), Gone: v.Gone, RIRNets: rirNets}
}

// rirTransitionNets returns the ids of the networks that the pdb_rir_status
// reclaim deleted in this cycle: the tombstones in scratch with the reclaim
// signature (scratchRIRTombstonesSQL) that are not deleted in the committed
// DB. It does not query db when scratch holds no such tombstone.
//
// Why no verification: the reclaim deletes the live netixlans of the
// network and soft-deletes the network in one transaction (2.83.0
// pdb_rir_status.py:440-443), a manual delete cascades tombstones to the
// netixlans, and upstream refuses a new live netixlan under a deleted
// network. So a network that turns deleted with the signature has no live
// netixlan left upstream. A tombstone that upstream re-saves later (org
// merge, handle_version) is already deleted in the committed DB and does
// not qualify; verifyNetIxLanCandidates covers its rows.
func rirTransitionNets(ctx context.Context, scratch *scratchDB, db *sql.DB) ([]int, error) {
	ids, err := queryIDs(ctx, scratch.db, scratchRIRTombstonesSQL)
	if err != nil {
		return nil, fmt.Errorf("read scratch rir network tombstones: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	nets, err := queryIDs(ctx, db, liveAtStartNetsSQL, jsonIDs(ids))
	if err != nil {
		return nil, fmt.Errorf("read network status at cycle start: %w", err)
	}
	return nets, nil
}

// queryIDs runs query on db and returns the int of each row's single
// column.
func queryIDs(ctx context.Context, db *sql.DB, query string, args ...any) ([]int, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
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
// the row keeps upstream's own updated. The run has no scratch DB, so its
// plan holds no RIRNets.
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
