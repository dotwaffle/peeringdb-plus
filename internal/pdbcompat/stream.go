package pdbcompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// RowsIter is a pull-style row iterator driving StreamListResponse.
//
// Semantics:
//   - (row, true, nil)  — row is the next payload element; keep iterating.
//   - (nil, false, nil) — clean EOF; end of stream.
//   - (nil, false, err) — abort; err is returned to the caller wrapped with
//     the failing row index.
//
// A "zombie" iterator that returns ok=true forever is caller-unsafe; the
// upstream memory budget (Plan 71-03) is responsible for ensuring bounded
// iteration length by capping the filtered row count pre-flight.
type RowsIter = func() (row any, ok bool, err error)

// FlushEvery is the row count between periodic http.Flusher.Flush() calls.
// Provides chunked backpressure for large payloads without the per-row
// syscall overhead of flushing after every Write.
const FlushEvery = 100

// StreamListResponse writes a PeeringDB envelope {"meta":...,"data":[...]}
// token-by-token without materialising the full result slice, per Phase 71
// CONTEXT.md D-01.
//
// Write sequence:
//  1. Set Content-Type: application/json and X-Powered-By headers.
//  2. Write `{"meta":` + json.Marshal(meta) + `,"data":[`.
//  3. For each row yielded by rowsIter: emit a leading `,` (when not first),
//     then json.Marshal(row) written directly to w.
//  4. Every FlushEvery rows, flush via http.Flusher (if w implements it).
//  5. Write `]}` and do a final flush.
//
// Error handling:
//   - json.Marshal(meta) failure returns the wrapped error BEFORE any bytes
//     hit the wire, so callers can still emit a 500/problem-detail.
//   - Any failure after the prelude has been written returns a wrapped error;
//     previously-written bytes stay on the wire (the response is already
//     committed). The caller is expected to log and drop the connection.
//   - Absence of http.Flusher on w is tolerated: flushes are silently skipped.
//
// ctx is reserved for future per-row cancellation (honor ctx.Done() inside
// the loop for backpressure when a client disconnects); today the iterator is
// driven by ent.All() or a keyset-chunked query that already respects ctx at
// its own boundary, so threading ctx.Err() per row is redundant.
func StreamListResponse(ctx context.Context, w http.ResponseWriter, meta any, rowsIter RowsIter) error {
	_ = ctx // TODO(71-future): honor ctx.Done() inside the loop for cancellation backpressure.

	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Powered-By", poweredByHeader)

	if _, err := w.Write([]byte(`{"meta":`)); err != nil {
		return fmt.Errorf("write prelude: %w", err)
	}
	if _, err := w.Write(metaBytes); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}
	if _, err := w.Write([]byte(`,"data":[`)); err != nil {
		return fmt.Errorf("write data-open: %w", err)
	}

	flusher, _ := w.(http.Flusher)
	idx := 0
	for {
		row, ok, iterErr := rowsIter()
		if iterErr != nil {
			return fmt.Errorf("stream row %d: %w", idx+1, iterErr)
		}
		if !ok {
			break
		}
		idx++
		if idx > 1 {
			if _, err := w.Write([]byte{','}); err != nil {
				return fmt.Errorf("write row-sep %d: %w", idx, err)
			}
		}
		rowBytes, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("marshal row %d: %w", idx, err)
		}
		if _, err := w.Write(rowBytes); err != nil {
			return fmt.Errorf("write row %d: %w", idx, err)
		}
		if flusher != nil && idx%FlushEvery == 0 {
			flusher.Flush()
		}
	}

	if _, err := w.Write([]byte("]}")); err != nil {
		return fmt.Errorf("write data-close: %w", err)
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

// iterFromSlice returns a RowsIter that yields the slice rows in order
// and signals clean EOF after the last one. Useful when a caller has
// already materialised rows (e.g. tc.List) but still wants the
// token-writer's bounded-allocation emission path.
//
// This is a deliberate half-step toward full cursor-based streaming:
// Plan 71-04 adopts it so the handler contract lands today; a future
// plan can flip tc.List to a true pull-iterator without touching
// serveList.
func iterFromSlice(rows []any) RowsIter {
	i := 0
	return func() (any, bool, error) {
		if i >= len(rows) {
			return nil, false, nil
		}
		row := rows[i]
		i++
		return row, true, nil
	}
}
