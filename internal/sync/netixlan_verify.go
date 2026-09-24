package sync

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/url"
	"slices"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// Limits of one verification pass (see verifyNetIxLanCandidates).
const (
	// netIxLanVerifyMaxRequests caps the id__in requests of one pass.
	// PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE lowers the cap, and 0
	// turns verification off.
	netIxLanVerifyMaxRequests = 10
	// netIxLanVerifyTimeout bounds the HTTP of one pass. It counts
	// against PDBPLUS_SYNC_TIMEOUT.
	netIxLanVerifyTimeout = 2 * time.Minute
	// netIxLanVerifyLiveTTL keeps a netixlan that upstream still serves
	// live out of the requests.
	netIxLanVerifyLiveTTL = 24 * time.Hour
	// netIxLanVerifyGoneTTL lets a gone verdict be used again without a
	// request, for example by the retries of a cycle that failed in
	// Phase B.
	netIxLanVerifyGoneTTL = 24 * time.Hour
	// netIxLanVerifyFailureBackoff keeps the ids of a failed chunk out of
	// the requests.
	netIxLanVerifyFailureBackoff = 6 * time.Hour
	// netIxLanVerifyLogIDs caps the id lists in the summary log.
	netIxLanVerifyLogIDs = 10
)

// cascadeCandidatesSQL reads the netixlans that the cascade can mark
// deleted: live rows of a deleted network whose updated is not later than
// the network's. A later row was saved after the network's delete (for
// example after an undelete and a second delete), so the cascade leaves
// it alone. likely() keeps the plan on the networkixlan_net_id index;
// without it SQLite reads networkixlan_status.
const cascadeCandidatesSQL = `SELECT nix.id, nix.updated, n.updated
FROM networks AS n
JOIN network_ix_lans AS nix ON nix.net_id = n.id
WHERE n.status = 'deleted'
  AND likely(nix.status IN ('ok', 'not-operational'))
  AND nix.updated <= n.updated
ORDER BY nix.id`

// stageNetIxLanSQL stages an upstream netixlan row in the scratch DB, so
// that Phase B upserts it like a fetched row.
const stageNetIxLanSQL = `INSERT OR REPLACE INTO "netixlan" (id, data) VALUES (?, ?)`

// netIxLanCandidate is a cascade candidate with the stored updated values
// of the row and of its network.
type netIxLanCandidate struct {
	ID         int
	Updated    time.Time
	NetUpdated time.Time
}

// verifyOutcome is the upstream verdict that a memo entry holds.
type verifyOutcome uint8

const (
	// verifyLive: upstream serves the row live.
	verifyLive verifyOutcome = iota + 1
	// verifyGone: upstream no longer serves the row live.
	verifyGone
	// verifyFailed: the request for the row failed.
	verifyFailed
)

// netIxLanVerifyEntry is an entry of Worker.netIxLanVerifyMemo. Updated
// and NetUpdated are the stored values that the verdict applies to; an
// entry whose values no longer match is dropped. Until ends the entry's
// effect. Failures counts the failed requests in a row, which selects the
// chunk size of the next request.
type netIxLanVerifyEntry struct {
	Outcome    verifyOutcome
	Updated    time.Time
	NetUpdated time.Time
	Until      time.Time
	Failures   int
}

// netIxLanVerifyInput is the input of one verification pass.
type netIxLanVerifyInput struct {
	// Scratch is the cycle's scratch DB, where upstream tombstones are
	// staged. Nil at startup.
	Scratch *scratchDB
	// NixCursor is the netixlan MAX(updated) before the cycle. Only an
	// upstream tombstone at or before it is staged, so staging never
	// moves the cursor.
	NixCursor time.Time
	// CursorKnown is false when the cursor read failed.
	CursorKnown bool
	// Now is the memo clock.
	Now time.Time
	// Mode is the log attribute: incremental, full or startup.
	Mode string
}

// netIxLanVerification is the result of one verification pass. Gone
// holds the ids that the cascade may mark deleted. Waiting counts the
// upstream deleted verdicts that the pass leaves for the first cycle (at
// startup, or with an unknown cursor); unlike Deferred, they do not
// raise the summary level. LiveIDs and FailedIDs hold the first
// netIxLanVerifyLogIDs ids, for the log. Err is the first error of the
// pass.
type netIxLanVerification struct {
	Gone       []int
	Candidates int
	MemoHits   int
	Backoff    int
	Requests   int
	Absent     int
	Live       int
	Deleted    int
	Staged     int
	Failed     int
	Deferred   int
	Waiting    int
	Disabled   bool
	LiveIDs    []int
	FailedIDs  []int
	Err        error
}

// netIxLanVerdict is what an id__in response says about one requested id.
// Raw is kept only for a deleted row, which may be staged.
type netIxLanVerdict struct {
	Status  string
	Updated time.Time
	Raw     json.RawMessage
}

// chunkResult is the outcome of one id__in request. Budget reports an
// error that says nothing about the requested ids (see
// verifyBudgetError).
type chunkResult struct {
	Verdicts map[int]netIxLanVerdict
	Err      error
	Budget   bool
}

// verifyNetIxLanCandidates asks upstream about the cascade candidates and
// returns the ids that the cascade may mark deleted (Gone).
//
// Why: upstream pdb_rir_status deletes the live netixlans of a reclaimed
// network with SQL, so no tombstone ever reaches ?since, and the mirror
// keeps those rows live under a deleted network. The network's delete
// alone does not prove that a row is gone: upstream still serves some
// netixlans of deleted networks live, and those must stay live here.
//
// A pass reads the candidates from the committed DB and sends one
// GET /api/netixlan?hide_ix_no_fac=0&id__in=<csv>&since=1 per chunk.
// since>0 skips upstream's API cache and returns every live or deleted
// row that still exists. It leaves out pending rows (2.83.0 rest.py:723),
// but a pending netixlan cannot exist under a deleted network: upstream
// runs validate_parent_status on every save (models.py:6513), and the
// network's delete soft-deletes its pending children. hide_ix_no_fac=0
// turns off the IX filter that a user key applies when its owner hides
// IXs without facilities. A requested id that is absent is gone for good,
// because the only netixlan hard delete is pdb_rir_status and ids are
// never reused.
// Verdicts:
//   - absent: Gone.
//   - deleted, with an updated after in.NixCursor: Gone, and the next
//     ?since fetch lands upstream's tombstone.
//   - deleted, with an updated at or before in.NixCursor: no ?since
//     fetch returns the row again, so the pass stages it into the scratch
//     DB and puts it in Gone, and Phase B stores upstream's own
//     tombstone. The memo keeps no entry for the id: a retry of the cycle
//     has a new scratch DB, so it must ask again and stage the row again.
//     A derived tombstone would hide upstream's updated.
//   - deleted, at startup or with an unknown cursor: left for the first
//     cycle, which stages the row (Waiting).
//   - live (pdbtypes.LiveStatuses): left alone.
//   - any other status, or a staging error: left for the next pass.
//
// The memo (Worker.netIxLanVerifyMemo) keeps live verdicts for
// netIxLanVerifyLiveTTL and gone verdicts for netIxLanVerifyGoneTTL, so a
// steady state sends no request. A failed chunk (5xx after the retry
// ladder, 4xx, a row that does not decode, a body without a data array)
// backs off its ids for netIxLanVerifyFailureBackoff, and later requests
// send them in chunks of 10 and then 1, so one bad id cannot block the
// others. The pass goes on after a failed chunk only when the chunk
// before it succeeded, so a failure of the whole endpoint costs one
// request per pass. A budget error (see verifyBudgetError) stops the
// pass and leaves the rest for the next pass without a memo entry.
//
// A pass sends at most min(netIxLanVerifyMaxRequests,
// PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE) requests within
// netIxLanVerifyTimeout; 0 turns verification off. It runs outside any
// transaction, never returns an error and never deletes from scratch.
// The caller must hold the running latch.
func (w *Worker) verifyNetIxLanCandidates(ctx context.Context, in netIxLanVerifyInput) netIxLanVerification {
	ctx, span := otel.Tracer("sync").Start(ctx, "sync-verify-netixlan-cascade")
	defer span.End()

	v := w.runNetIxLanVerification(ctx, in)
	slices.Sort(v.Gone)
	w.reportNetIxLanVerification(ctx, span, in.Mode, v)
	return v
}

// runNetIxLanVerification does the work of verifyNetIxLanCandidates.
func (w *Worker) runNetIxLanVerification(ctx context.Context, in netIxLanVerifyInput) netIxLanVerification {
	var v netIxLanVerification
	maxRequests := min(netIxLanVerifyMaxRequests, w.fkBackfillRequestCap)
	if maxRequests <= 0 {
		v.Disabled = true
		return v
	}
	cands, err := loadCascadeCandidates(ctx, w.db)
	if err != nil {
		v.Err = err
		return v
	}
	v.Candidates = len(cands)
	byID := make(map[int]netIxLanCandidate, len(cands))
	for _, c := range cands {
		byID[c.ID] = c
	}
	maps.DeleteFunc(w.netIxLanVerifyMemo, func(id int, e netIxLanVerifyEntry) bool {
		c, ok := byID[id]
		return !ok || !c.Updated.Equal(e.Updated) || !c.NetUpdated.Equal(e.NetUpdated)
	})

	var pending []int
	failures := make(map[int]int)
	for _, c := range cands {
		e, ok := w.netIxLanVerifyMemo[c.ID]
		if !ok || !in.Now.Before(e.Until) {
			if ok && e.Outcome == verifyFailed {
				failures[c.ID] = e.Failures
			}
			pending = append(pending, c.ID)
			continue
		}
		switch e.Outcome {
		case verifyGone:
			v.MemoHits++
			v.Gone = append(v.Gone, c.ID)
		case verifyLive:
			v.MemoHits++
		case verifyFailed:
			v.Backoff++
		}
	}
	chunks, deferred := netIxLanVerifyChunks(pending, failures, maxRequests)
	v.Deferred = deferred
	if len(chunks) == 0 {
		return v
	}

	fetchCtx, cancel := context.WithTimeout(ctx, netIxLanVerifyTimeout)
	results := fetchNetIxLanVerdicts(fetchCtx, w.pdbClient, chunks)
	cancel()

	for i, chunk := range chunks {
		if i >= len(results) {
			v.Deferred += len(chunk)
			continue
		}
		r := results[i]
		v.Requests++
		switch {
		case r.Budget:
			v.Deferred += len(chunk)
			v.Err = cmp.Or(v.Err, r.Err)
		case r.Err != nil:
			v.Failed += len(chunk)
			v.Err = cmp.Or(v.Err, r.Err)
			for _, id := range chunk {
				v.FailedIDs = appendLogID(v.FailedIDs, id)
				w.memoNetIxLan(byID[id], verifyFailed, in.Now.Add(netIxLanVerifyFailureBackoff), failures[id]+1)
			}
		default:
			w.applyNetIxLanVerdicts(ctx, &v, in, chunk, r.Verdicts, byID)
		}
	}
	return v
}

// applyNetIxLanVerdicts classifies the ids of a chunk whose request
// succeeded and writes their memo entries.
func (w *Worker) applyNetIxLanVerdicts(ctx context.Context, v *netIxLanVerification, in netIxLanVerifyInput, chunk []int, verdicts map[int]netIxLanVerdict, byID map[int]netIxLanCandidate) {
	live := pdbtypes.LiveStatuses(peeringdb.TypeNetIXLan)
	for _, id := range chunk {
		verdict, found := verdicts[id]
		switch {
		case !found:
			v.Absent++
			v.Gone = append(v.Gone, id)
			w.memoNetIxLan(byID[id], verifyGone, in.Now.Add(netIxLanVerifyGoneTTL), 0)
		case slices.Contains(live, verdict.Status):
			v.Live++
			v.LiveIDs = appendLogID(v.LiveIDs, id)
			w.memoNetIxLan(byID[id], verifyLive, in.Now.Add(netIxLanVerifyLiveTTL), 0)
		case verdict.Status == "deleted":
			v.Deleted++
			w.takeNetIxLanTombstone(ctx, v, in, byID[id], verdict)
		default:
			v.Deferred++
		}
	}
}

// takeNetIxLanTombstone applies an upstream deleted verdict for
// candidate c (see verifyNetIxLanCandidates): a gone memo entry only for
// an updated after the cursor, staging and no memo entry for an updated
// at or before it. At startup, with an unknown cursor, or after a
// staging error, the id stays out of Gone.
func (w *Worker) takeNetIxLanTombstone(ctx context.Context, v *netIxLanVerification, in netIxLanVerifyInput, c netIxLanCandidate, verdict netIxLanVerdict) {
	delete(w.netIxLanVerifyMemo, c.ID)
	switch {
	case in.Scratch == nil || !in.CursorKnown:
		v.Waiting++
	case verdict.Updated.After(in.NixCursor):
		v.Gone = append(v.Gone, c.ID)
		w.memoNetIxLan(c, verifyGone, in.Now.Add(netIxLanVerifyGoneTTL), 0)
	default:
		if _, err := in.Scratch.db.ExecContext(ctx, stageNetIxLanSQL, c.ID, []byte(verdict.Raw)); err != nil {
			v.Deferred++
			v.Err = cmp.Or(v.Err, fmt.Errorf("stage netixlan %d tombstone: %w", c.ID, err))
			return
		}
		v.Staged++
		v.Gone = append(v.Gone, c.ID)
	}
}

// memoNetIxLan writes the memo entry of candidate c.
func (w *Worker) memoNetIxLan(c netIxLanCandidate, outcome verifyOutcome, until time.Time, failures int) {
	w.netIxLanVerifyMemo[c.ID] = netIxLanVerifyEntry{
		Outcome:    outcome,
		Updated:    c.Updated,
		NetUpdated: c.NetUpdated,
		Until:      until,
		Failures:   failures,
	}
}

// appendLogID appends id to ids while ids holds fewer than
// netIxLanVerifyLogIDs ids.
func appendLogID(ids []int, id int) []int {
	if len(ids) < netIxLanVerifyLogIDs {
		ids = append(ids, id)
	}
	return ids
}

// reportNetIxLanVerification logs the pass and sets its span attributes.
// The summary logs at WARN when ids failed or were deferred, at INFO when
// the pass sent requests, and at DEBUG otherwise. Waiting ids do not
// raise the level.
func (w *Worker) reportNetIxLanVerification(ctx context.Context, span trace.Span, mode string, v netIxLanVerification) {
	span.SetAttributes(
		attribute.Int("pdbplus.sync.netixlan_verify.candidates", v.Candidates),
		attribute.Int("pdbplus.sync.netixlan_verify.memo_hits", v.MemoHits),
		attribute.Int("pdbplus.sync.netixlan_verify.backoff", v.Backoff),
		attribute.Int("pdbplus.sync.netixlan_verify.requests", v.Requests),
		attribute.Int("pdbplus.sync.netixlan_verify.absent", v.Absent),
		attribute.Int("pdbplus.sync.netixlan_verify.live", v.Live),
		attribute.Int("pdbplus.sync.netixlan_verify.deleted", v.Deleted),
		attribute.Int("pdbplus.sync.netixlan_verify.staged", v.Staged),
		attribute.Int("pdbplus.sync.netixlan_verify.failed", v.Failed),
		attribute.Int("pdbplus.sync.netixlan_verify.deferred", v.Deferred),
		attribute.Int("pdbplus.sync.netixlan_verify.waiting", v.Waiting),
	)
	if v.Err != nil {
		span.RecordError(v.Err)
		span.SetStatus(codes.Error, "netixlan cascade verification failed")
		w.logger.LogAttrs(ctx, slog.LevelWarn, "netixlan cascade verification failed, cascade deferred",
			slog.String("mode", mode),
			slog.Any("error", v.Err),
			slog.Int("requests", v.Requests),
			slog.Int("failed", v.Failed),
			slog.Int("deferred", v.Deferred),
		)
	}
	level := slog.LevelDebug
	switch {
	case v.Failed > 0 || v.Deferred > 0:
		level = slog.LevelWarn
	case v.Requests > 0:
		level = slog.LevelInfo
	}
	w.logger.LogAttrs(ctx, level, "verified netixlan cascade candidates",
		slog.String("mode", mode),
		slog.Int("candidates", v.Candidates),
		slog.Int("memo_hits", v.MemoHits),
		slog.Int("backoff", v.Backoff),
		slog.Int("requests", v.Requests),
		slog.Int("absent", v.Absent),
		slog.Int("live", v.Live),
		slog.Int("deleted", v.Deleted),
		slog.Int("staged", v.Staged),
		slog.Int("failed", v.Failed),
		slog.Int("deferred", v.Deferred),
		slog.Int("waiting", v.Waiting),
		slog.Bool("disabled", v.Disabled),
		slog.Any("live_ids", v.LiveIDs),
		slog.Any("failed_ids", v.FailedIDs),
	)
}

// netIxLanVerifyChunks splits ids into the id__in chunks of one pass. It
// orders ids by (failures, id), so ids that never failed go first. The
// chunk size depends on the failures of its ids: 100 at none, 10 after
// one, 1 after two or more; a chunk holds ids of one size only. It keeps
// at most maxRequests chunks and returns the number of ids left over.
func netIxLanVerifyChunks(ids []int, failures map[int]int, maxRequests int) (chunks [][]int, deferred int) {
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, func(a, b int) int {
		return cmp.Or(cmp.Compare(failures[a], failures[b]), cmp.Compare(a, b))
	})
	for i := 0; i < len(sorted); {
		if len(chunks) == maxRequests {
			return chunks, len(sorted) - i
		}
		size := netIxLanVerifyChunkSize(failures[sorted[i]])
		j := i + 1
		for j < len(sorted) && j-i < size && netIxLanVerifyChunkSize(failures[sorted[j]]) == size {
			j++
		}
		chunks = append(chunks, sorted[i:j])
		i = j
	}
	return chunks, 0
}

// netIxLanVerifyChunkSize returns the chunk size for an id with the given
// number of failed requests.
func netIxLanVerifyChunkSize(failures int) int {
	switch failures {
	case 0:
		return peeringdb.FetchByIDsBatchSize
	case 1:
		return 10
	default:
		return 1
	}
}

// fetchNetIxLanVerdicts sends one id__in request per chunk and returns
// one chunkResult per chunk it sent. It keeps {id, status, updated} of
// the requested ids and ignores other rows. A row that does not decode
// fails its chunk. It stops after a chunk with a budget error, and after
// a failed chunk unless the chunk before it succeeded: a single bad
// chunk does not stop the pass, but a failing endpoint gets one request.
func fetchNetIxLanVerdicts(ctx context.Context, client *peeringdb.Client, chunks [][]int) []chunkResult {
	extra := url.Values{"hide_ix_no_fac": {"0"}}
	results := make([]chunkResult, 0, len(chunks))
	prevOK := false
	for _, chunk := range chunks {
		requested := make(map[int]struct{}, len(chunk))
		for _, id := range chunk {
			requested[id] = struct{}{}
		}
		verdicts := make(map[int]netIxLanVerdict, len(chunk))
		err := client.StreamByIDs(ctx, peeringdb.TypeNetIXLan, chunk, extra, func(raw json.RawMessage) error {
			var row struct {
				ID      int       `json:"id"`
				Status  string    `json:"status"`
				Updated time.Time `json:"updated"`
			}
			if err := json.Unmarshal(raw, &row); err != nil {
				return fmt.Errorf("decode netixlan verdict: %w", err)
			}
			if _, ok := requested[row.ID]; !ok {
				return nil
			}
			verdict := netIxLanVerdict{Status: row.Status, Updated: row.Updated}
			if row.Status == "deleted" {
				verdict.Raw = bytes.Clone(raw)
			}
			verdicts[row.ID] = verdict
			return nil
		})
		r := chunkResult{Verdicts: verdicts}
		if err != nil {
			r = chunkResult{
				Err:    fmt.Errorf("verify netixlan cascade candidates: %w", err),
				Budget: verifyBudgetError(ctx, err),
			}
		}
		results = append(results, r)
		if r.Budget || (r.Err != nil && !prevOK) {
			break
		}
		prevOK = r.Err == nil
	}
	return results
}

// verifyBudgetError reports whether a chunk error says nothing about the
// requested ids: a 429, a WAF block, a done context, or a request that got
// no HTTP response (DNS, connection, TLS or response header timeout,
// which the HTTP client returns as a *url.Error). Such an error stops the
// pass with no memo entry, so it neither backs off nor shrinks the chunks
// of ids that upstream never answered for.
func verifyBudgetError(ctx context.Context, err error) bool {
	_, noResponse := errors.AsType[*url.Error](err)
	return noResponse || rateLimited(err) || peeringdb.IsWAFBlocked(err) || ctx.Err() != nil
}

// loadCascadeCandidates runs cascadeCandidatesSQL on db.
func loadCascadeCandidates(ctx context.Context, db *sql.DB) ([]netIxLanCandidate, error) {
	rows, err := db.QueryContext(ctx, cascadeCandidatesSQL)
	if err != nil {
		return nil, fmt.Errorf("load netixlan cascade candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []netIxLanCandidate
	for rows.Next() {
		var c netIxLanCandidate
		if err := rows.Scan(&c.ID, &c.Updated, &c.NetUpdated); err != nil {
			return nil, fmt.Errorf("scan netixlan cascade candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load netixlan cascade candidates: %w", err)
	}
	return out, nil
}
