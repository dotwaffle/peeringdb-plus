package pdbcompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// iterFromSlice returns a RowsIter that drains the given slice one row per call.
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

// iterWithError returns a RowsIter that yields rows 1..failAt-1 cleanly, then
// returns (nil, false, err) on the failAt-th call (1-indexed).
func iterWithError(rows []any, failAt int, err error) RowsIter {
	i := 0
	return func() (any, bool, error) {
		i++
		if i == failAt {
			return nil, false, err
		}
		if i-1 >= len(rows) {
			return nil, false, nil
		}
		return rows[i-1], true, nil
	}
}

// flushCountingRecorder wraps httptest.ResponseRecorder and counts Flush calls.
type flushCountingRecorder struct {
	*httptest.ResponseRecorder
	flushCount int
}

func (f *flushCountingRecorder) Flush() {
	f.flushCount++
	f.ResponseRecorder.Flush()
}

// nonFlusherWriter implements http.ResponseWriter but NOT http.Flusher.
type nonFlusherWriter struct {
	h      http.Header
	buf    bytes.Buffer
	status int
}

func (w *nonFlusherWriter) Header() http.Header {
	if w.h == nil {
		w.h = http.Header{}
	}
	return w.h
}
func (w *nonFlusherWriter) Write(b []byte) (int, error) { return w.buf.Write(b) }
func (w *nonFlusherWriter) WriteHeader(s int)           { w.status = s }

func TestStreamListResponse_Envelope(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	rows := []any{
		map[string]any{"id": 1},
		map[string]any{"id": 2},
		map[string]any{"id": 3},
	}
	if err := StreamListResponse(context.Background(), rec, struct{}{}, iterFromSlice(rows)); err != nil {
		t.Fatalf("StreamListResponse: %v", err)
	}
	want := `{"meta":{},"data":[{"id":1},{"id":2},{"id":3}]}`
	if got := rec.Body.String(); got != want {
		t.Fatalf("body mismatch\n got: %s\nwant: %s", got, want)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if pb := rec.Header().Get("X-Powered-By"); pb != poweredByHeader {
		t.Errorf("X-Powered-By = %q, want %q", pb, poweredByHeader)
	}
}

func TestStreamListResponse_CommaPlacement(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		rows    []any
		want    string
		commas  int
	}{
		{
			name: "zero-rows",
			rows: nil,
			want: `{"meta":{},"data":[]}`,
		},
		{
			name: "one-row",
			rows: []any{map[string]any{"id": 1}},
			want: `{"meta":{},"data":[{"id":1}]}`,
		},
		{
			name: "five-rows",
			rows: []any{
				map[string]any{"id": 1},
				map[string]any{"id": 2},
				map[string]any{"id": 3},
				map[string]any{"id": 4},
				map[string]any{"id": 5},
			},
			commas: 4,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			if err := StreamListResponse(context.Background(), rec, struct{}{}, iterFromSlice(tc.rows)); err != nil {
				t.Fatalf("StreamListResponse: %v", err)
			}
			got := rec.Body.String()
			if tc.want != "" && got != tc.want {
				t.Fatalf("body mismatch\n got: %s\nwant: %s", got, tc.want)
			}
			if tc.commas > 0 {
				// Count commas INSIDE the data array only.
				openIdx := strings.Index(got, "[")
				closeIdx := strings.LastIndex(got, "]")
				if openIdx < 0 || closeIdx < 0 || closeIdx <= openIdx {
					t.Fatalf("malformed envelope: %s", got)
				}
				inner := got[openIdx+1 : closeIdx]
				if c := strings.Count(inner, ","); c != tc.commas {
					t.Fatalf("commas inside data = %d, want %d; inner=%q", c, tc.commas, inner)
				}
			}
		})
	}
}

func TestStreamListResponse_FlushCadence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		rowCount      int
		wantFlushes   int
	}{
		// 250 rows with FlushEvery=100: periodic at row 100, 200; final flush at end = 3.
		{name: "250-rows-3-flushes", rowCount: 250, wantFlushes: 3},
		// 50 rows: no periodic flush; final flush only = 1.
		{name: "50-rows-1-flush", rowCount: 50, wantFlushes: 1},
		// Exactly 100 rows: periodic at row 100, then final flush — both fire.
		{name: "100-rows-2-flushes", rowCount: 100, wantFlushes: 2},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows := make([]any, tc.rowCount)
			for i := range rows {
				rows[i] = map[string]any{"id": i + 1}
			}
			rec := &flushCountingRecorder{ResponseRecorder: httptest.NewRecorder()}
			if err := StreamListResponse(context.Background(), rec, struct{}{}, iterFromSlice(rows)); err != nil {
				t.Fatalf("StreamListResponse: %v", err)
			}
			if rec.flushCount != tc.wantFlushes {
				t.Fatalf("flushCount = %d, want %d", rec.flushCount, tc.wantFlushes)
			}
		})
	}
}

func TestStreamListResponse_IteratorError(t *testing.T) {
	t.Parallel()
	rows := []any{
		map[string]any{"id": 1},
		map[string]any{"id": 2},
	}
	sentinel := errors.New("db exploded")
	rec := httptest.NewRecorder()
	err := StreamListResponse(context.Background(), rec, struct{}{}, iterWithError(rows, 3, sentinel))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is(err, sentinel) = false; err=%v", err)
	}
	// We do NOT assert body is valid JSON — partial write is by design per D-01.
	// But we do assert the prelude and first two rows made it out.
	got := rec.Body.String()
	if !strings.HasPrefix(got, `{"meta":{},"data":[`) {
		t.Fatalf("body missing prelude: %s", got)
	}
	if !strings.Contains(got, `{"id":1}`) || !strings.Contains(got, `{"id":2}`) {
		t.Fatalf("body missing pre-error rows: %s", got)
	}
}

func TestStreamListResponse_ZeroRows(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	if err := StreamListResponse(context.Background(), rec, struct{}{}, iterFromSlice(nil)); err != nil {
		t.Fatalf("StreamListResponse: %v", err)
	}
	if got, want := rec.Body.String(), `{"meta":{},"data":[]}`; got != want {
		t.Fatalf("body mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestStreamListResponse_NonFlusherWriter(t *testing.T) {
	t.Parallel()
	w := &nonFlusherWriter{}
	rows := []any{
		map[string]any{"id": 1},
		map[string]any{"id": 2},
	}
	// Must not panic even though w is not an http.Flusher.
	if err := StreamListResponse(context.Background(), w, struct{}{}, iterFromSlice(rows)); err != nil {
		t.Fatalf("StreamListResponse: %v", err)
	}
	want := `{"meta":{},"data":[{"id":1},{"id":2}]}`
	if got := w.buf.String(); got != want {
		t.Fatalf("body mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestStreamListResponse_CustomMeta(t *testing.T) {
	t.Parallel()
	meta := map[string]any{
		"total":     42,
		"generated": "2026-04-19T00:00:00Z",
	}
	rows := []any{
		map[string]any{"id": 7},
	}
	rec := httptest.NewRecorder()
	if err := StreamListResponse(context.Background(), rec, meta, iterFromSlice(rows)); err != nil {
		t.Fatalf("StreamListResponse: %v", err)
	}

	// Unmarshal for key-order-insensitive comparison of meta.
	var parsed struct {
		Meta map[string]any `json:"meta"`
		Data []any          `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, rec.Body.String())
	}
	// json.Unmarshal normalizes numbers to float64; fix up for DeepEqual.
	wantMeta := map[string]any{
		"total":     float64(42),
		"generated": "2026-04-19T00:00:00Z",
	}
	if !reflect.DeepEqual(parsed.Meta, wantMeta) {
		t.Fatalf("meta mismatch\n got: %#v\nwant: %#v", parsed.Meta, wantMeta)
	}
	if len(parsed.Data) != 1 {
		t.Fatalf("data len = %d, want 1", len(parsed.Data))
	}
}
