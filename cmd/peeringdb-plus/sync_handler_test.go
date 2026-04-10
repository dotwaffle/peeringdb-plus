package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
)

// TestSyncHandler_TokenCompare verifies SEC-04: the /sync token compare is
// constant-time AND preserves the empty-token "disabled" mode early-out.
//
// Rows cover: disabled-mode with empty header, disabled-mode with nonempty
// header (MUST still reject — v1.0 semantic), configured with empty header,
// configured with matching header, configured with prefix-match header,
// configured with wrong-length header, configured with trailing whitespace.
//
// No timing assertions — we trust crypto/subtle.ConstantTimeCompare and test
// correctness only. See .planning/phases/51-quick-security-wins/51-CONTEXT.md.
func TestSyncHandler_TokenCompare(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		header     string
		wantStatus int
		wantCalled bool
	}{
		{
			name:       "disabled mode empty header rejects",
			configured: "",
			header:     "",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "disabled mode nonempty header rejects",
			configured: "",
			header:     "anything",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "configured empty header rejects",
			configured: "s3cret-token",
			header:     "",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "configured matching accepts",
			configured: "s3cret-token",
			header:     "s3cret-token",
			wantStatus: http.StatusAccepted,
			wantCalled: true,
		},
		{
			name:       "configured prefix match rejects",
			configured: "s3cret-token",
			header:     "s3cret",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "configured wrong length rejects",
			configured: "s3cret-token",
			header:     "s3cret-token-extra",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "configured trailing whitespace rejects",
			configured: "s3cret-token",
			header:     "s3cret-token ",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "configured wrong token same length rejects",
			configured: "s3cret-token",
			header:     "w0rng-4-token",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Do not parallelize: t.Setenv prohibits t.Parallel.
			t.Setenv("FLY_REGION", "")

			// SyncFn is invoked via `go in.SyncFn(...)` in newSyncHandler, so
			// a plain bool would race with the test goroutine under -race.
			// Buffered channel (size 1) lets the handler-spawned goroutine
			// signal once without blocking and gives us a sync point.
			calledCh := make(chan struct{}, 1)
			handler := newSyncHandler(t.Context(), SyncHandlerInput{
				IsPrimaryFn: func() bool { return true },
				SyncToken:   tt.configured,
				DefaultMode: config.SyncModeFull,
				SyncFn: func(_ context.Context, _ config.SyncMode) {
					calledCh <- struct{}{}
				},
			})

			req := httptest.NewRequest("POST", "/sync", nil)
			if tt.header != "" {
				req.Header.Set("X-Sync-Token", tt.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status: want %d, got %d (body=%q)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			if tt.wantCalled {
				// Block until the goroutine spawned by the handler fires SyncFn.
				// 2s is generous — this is a sync point, not a timing assertion.
				select {
				case <-calledCh:
				case <-time.After(2 * time.Second):
					t.Errorf("SyncFn was not called within 2s (want called=true)")
				}
			} else {
				// Rejection paths `return` before `go in.SyncFn(...)` is reached,
				// so no goroutine is ever spawned. A short drain window catches any
				// regression that would let the handler invoke SyncFn on rejection.
				select {
				case <-calledCh:
					t.Errorf("SyncFn was called (want called=false)")
				case <-time.After(50 * time.Millisecond):
				}
			}
		})
	}
}

// TestSyncHandler_DisabledModeRejectsAll is an explicit regression test for
// the v1.0 "disabled means locked, not open" semantic. Independent of the
// table above so a future refactor that accidentally drops the empty-token
// early-out trips THIS test first.
func TestSyncHandler_DisabledModeRejectsAll(t *testing.T) {
	t.Setenv("FLY_REGION", "")

	// Buffered so an unexpected SyncFn call never blocks (and is observable).
	calledCh := make(chan struct{}, 8)
	handler := newSyncHandler(t.Context(), SyncHandlerInput{
		IsPrimaryFn: func() bool { return true },
		SyncToken:   "", // disabled mode
		DefaultMode: config.SyncModeFull,
		SyncFn: func(_ context.Context, _ config.SyncMode) {
			calledCh <- struct{}{}
		},
	})

	// Try a variety of header values — all must be rejected.
	for _, header := range []string{"", " ", "anything", "admin", "\x00"} {
		req := httptest.NewRequest("POST", "/sync", nil)
		if header != "" {
			req.Header.Set("X-Sync-Token", header)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header=%q: want 401, got %d", header, rec.Code)
		}
	}

	// Drain window: if any regression wired up a path where disabled mode
	// still reaches `go in.SyncFn(...)`, catch it here.
	select {
	case <-calledCh:
		t.Errorf("SyncFn must NEVER be called in disabled mode")
	case <-time.After(50 * time.Millisecond):
	}
}
