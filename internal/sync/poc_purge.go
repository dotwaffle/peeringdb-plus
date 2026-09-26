package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
)

// pocDeletionPeriod is the age of the updated value at which upstream
// hard-deletes a poc with status "deleted": POC_DELETION_PERIOD, 30 days
// (2.83.0 mainsite/settings/__init__.py:684).
const pocDeletionPeriod = 30 * 24 * time.Hour

// purgeDeletedPocs deletes every stored poc with status "deleted" whose
// updated value is pocDeletionPeriod or more before now. It returns the
// number of deleted rows.
//
// Why: upstream pdb_delete_pocs removes these rows with the same rule
// (2.83.0 management/commands/pdb_delete_pocs.py:34-38,58), so an
// upstream ?since= window no longer returns them. Without this purge the
// mirror kept every poc tombstone, and /api/poc?since=N returned rows
// that upstream no longer has. It is the only hard delete in sync.
//
// A purged row does not come back: a ?since= fetch gets only rows that
// changed after the cursor, a bare list holds only live rows, the
// history sweep skips poc, and FK backfill never fetches a poc. A
// tombstone older than the period that a cycle lands is deleted in the
// same transaction.
//
// Worker.Sync calls it in each sync transaction after the poc contact
// scrub, with the cycle start time. LiteFS replicates the delete to the
// replicas. The caller logs the count after its commit (see
// logPurgedPocs). The status index limits the read to the deleted pocs.
// When no row matches, the DELETE writes no page.
//
// The DELETE needs no privacy bypass: the poc policy has only a query
// rule, and a delete does not evaluate it.
//
// Observability: the pdbplus.sync.pocs_purged attribute on the
// sync-purge-deleted-pocs span counts the rows that the DELETE removed
// in tx. The span ends before the commit.
func purgeDeletedPocs(ctx context.Context, tx *ent.Tx, now time.Time) (int, error) {
	ctx, span := otel.Tracer("sync").Start(ctx, "sync-purge-deleted-pocs")
	defer span.End()

	// Stored updated values have whole seconds. UTC() also removes the
	// monotonic clock reading, which the bound value must not carry.
	cutoff := now.UTC().Truncate(time.Second).Add(-pocDeletionPeriod)
	n, err := tx.Poc.Delete().
		Where(poc.StatusEQ("deleted"), poc.UpdatedLTE(cutoff)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("purge deleted pocs: %w", err)
	}

	span.SetAttributes(attribute.Int("pdbplus.sync.pocs_purged", n))
	return n, nil
}

// logPurgedPocs logs the number of pocs that a purge deleted: at INFO
// when rows were deleted, at DEBUG when none were. Call it only after
// the transaction that ran the purge commits. A purge whose transaction
// rolled back deleted no row, and its caller logs the failure.
func logPurgedPocs(ctx context.Context, logger *slog.Logger, n int) {
	level := slog.LevelDebug
	if n > 0 {
		level = slog.LevelInfo
	}
	logger.LogAttrs(ctx, level, "purged deleted pocs", slog.Int("count", n))
}
