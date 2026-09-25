package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
)

// scrubDeletedPocContacts sets name, phone, email and url to "" on every
// stored poc with status "deleted" that still holds any of them. It
// returns the number of rows it changed.
//
// Why: the inference-by-absence soft delete (v1.16.0 to v1.18.1) set
// status "deleted" and kept the contact data. Sync never rewrites these
// rows: their updated value is behind the cursor, full syncs fetch only
// live rows, and upstream hard-deletes its own tombstones after 30 days.
// upsertPocs stores every new tombstone blank
// (peeringdb.Poc.BlankDeletedContact), so this repair changes rows only
// on its first run after an upgrade.
//
// Callers: scrubPocContactsAtStartup when the scheduler starts on the
// primary, and Worker.Sync in each sync transaction after the upsert
// pass. Both run on the primary, and LiteFS replicates the change to the
// replicas, which are read-only. The caller logs the count after its
// commit (see logScrubbedPocContacts). A rolled-back transaction changed
// no row, so its count is not logged.
//
// Cost: the status index limits the read to the deleted pocs. When no
// row matches, the UPDATE writes no page, so LiteFS has nothing to ship.
//
// The function does not change updated. The next cursor reads it (see
// watermark.go), and the upstream content of the row has not changed.
//
// Observability: the pdbplus.sync.poc_contacts_scrubbed attribute on the
// sync-scrub-poc-contacts span counts the rows that the UPDATE changed in
// tx. The span ends before the commit.
func scrubDeletedPocContacts(ctx context.Context, tx *ent.Tx) (int, error) {
	ctx, span := otel.Tracer("sync").Start(ctx, "sync-scrub-poc-contacts")
	defer span.End()

	n, err := tx.Poc.Update().
		Where(
			poc.StatusEQ("deleted"),
			poc.Or(poc.NameNEQ(""), poc.PhoneNEQ(""), poc.EmailNEQ(""), poc.URLNEQ("")),
		).
		SetName("").
		SetPhone("").
		SetEmail("").
		SetURL("").
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("scrub deleted poc contacts: %w", err)
	}

	span.SetAttributes(attribute.Int("pdbplus.sync.poc_contacts_scrubbed", n))
	return n, nil
}

// logScrubbedPocContacts logs the number of pocs that a scrub changed: at
// WARN when rows changed, at DEBUG when none did. Call it only after the
// transaction that ran the scrub commits. A scrub whose transaction
// rolled back changed no row, and its caller logs the failure.
func logScrubbedPocContacts(ctx context.Context, logger *slog.Logger, n int) {
	level := slog.LevelDebug
	if n > 0 {
		level = slog.LevelWarn
	}
	logger.LogAttrs(ctx, level, "scrubbed contact fields of deleted pocs", slog.Int("count", n))
}

// scrubPocContactsAtStartup runs scrubDeletedPocContacts once, in its own
// short transaction. StartScheduler calls it on the primary before it
// waits for the first sync cycle.
//
// Why: Worker.Sync runs the repair only after a successful fetch pass,
// and the first cycle after a restart can start up to one interval
// later, or fail when upstream fails. Until a cycle commits the repair,
// GraphQL, REST and ConnectRPC serve the stored contact data of legacy
// tombstones, and /api/ filters such as ?email__startswith= match it,
// although pdbcompat blanks it in the output.
//
// The run holds the running latch, so it does not overlap a sync cycle.
// When a cycle holds the latch, the run is skipped, because that cycle
// runs the repair itself. A transient SQLite lock error runs the
// transaction again under w.lockRetry (see retryOnLock). The UPDATE
// selects only the rows that still hold contact data, so a second
// attempt is safe. Another error, or the last failed attempt, is logged
// at WARN and not returned, because the call in each sync cycle retries
// the repair.
//
// The UPDATE needs no privacy bypass: the poc policy has only a query
// rule, and an update does not evaluate it.
func (w *Worker) scrubPocContactsAtStartup(ctx context.Context) {
	if !w.running.CompareAndSwap(false, true) {
		w.logger.LogAttrs(ctx, slog.LevelDebug, "sync cycle running, skipping startup poc contact scrub")
		return
	}
	defer w.running.Store(false)

	n, err := retryOnLock(ctx, w.logger, w.lockRetry, opStartupPocScrub,
		func(ctx context.Context) (int, error) {
			return scrubPocContactsInTx(ctx, w.entClient)
		})
	if err != nil {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "startup poc contact scrub failed, the next sync cycle retries it",
			slog.Any("error", err))
		return
	}
	logScrubbedPocContacts(ctx, w.logger, n)
}

// scrubPocContactsInTx runs scrubDeletedPocContacts in a transaction of
// its own and commits it. It returns the number of changed rows only when
// the commit succeeds.
func scrubPocContactsInTx(ctx context.Context, client *ent.Client) (int, error) {
	tx, err := client.Tx(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin poc scrub transaction: %w", err)
	}
	n, err := scrubDeletedPocContacts(ctx, tx)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			err = errors.Join(err, fmt.Errorf("rollback poc scrub transaction: %w", rbErr))
		}
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit poc scrub transaction: %w", err)
	}
	return n, nil
}
