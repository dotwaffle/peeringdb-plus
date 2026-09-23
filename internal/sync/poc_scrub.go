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
// replicas, which are read-only.
//
// Cost: the status index limits the read to the deleted pocs. When no
// row matches, the UPDATE writes no page, so LiteFS has nothing to ship.
//
// The function does not change updated. The incremental cursor is
// MAX(updated), and the upstream content of the row has not changed.
//
// Observability: a WARN log line with the row count when rows change
// (DEBUG when none do), and the pdbplus.sync.poc_contacts_scrubbed
// attribute on the sync-scrub-poc-contacts span.
func scrubDeletedPocContacts(ctx context.Context, tx *ent.Tx, logger *slog.Logger) (int, error) {
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
	level := slog.LevelDebug
	if n > 0 {
		level = slog.LevelWarn
	}
	logger.LogAttrs(ctx, level, "scrubbed contact fields of deleted pocs", slog.Int("count", n))
	return n, nil
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
// runs the repair itself. An error is logged at WARN and not returned,
// because the call in each sync cycle retries the repair.
//
// The UPDATE needs no privacy bypass: the poc policy has only a query
// rule, and an update does not evaluate it.
func (w *Worker) scrubPocContactsAtStartup(ctx context.Context) {
	if !w.running.CompareAndSwap(false, true) {
		w.logger.LogAttrs(ctx, slog.LevelDebug, "sync cycle running, skipping startup poc contact scrub")
		return
	}
	defer w.running.Store(false)

	if err := scrubPocContactsInTx(ctx, w.entClient, w.logger); err != nil {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "startup poc contact scrub failed, the next sync cycle retries it",
			slog.Any("error", err))
	}
}

// scrubPocContactsInTx runs scrubDeletedPocContacts in a transaction of
// its own and commits it.
func scrubPocContactsInTx(ctx context.Context, client *ent.Client, logger *slog.Logger) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin poc scrub transaction: %w", err)
	}
	if _, err := scrubDeletedPocContacts(ctx, tx, logger); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			err = errors.Join(err, fmt.Errorf("rollback poc scrub transaction: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit poc scrub transaction: %w", err)
	}
	return nil
}
