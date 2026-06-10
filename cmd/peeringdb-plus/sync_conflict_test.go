package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
)

// TestSyncHandler_ConflictWhenRunning locks the 2026-06-10 audit fix:
// POST /sync used to answer 202 Accepted even when the worker's CAS
// guard would silently drop the trigger because a cycle was already in
// flight — the operator's ?mode=full escape hatch could no-op
// invisibly. The handler now probes the worker and answers 409.
func TestSyncHandler_ConflictWhenRunning(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		running    bool
		wantStatus int
		wantCalled bool
	}{
		{"idle accepts", false, http.StatusAccepted, true},
		{"busy conflicts", true, http.StatusConflict, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			called := make(chan struct{}, 1)
			handler := newSyncHandler(t.Context(), SyncHandlerInput{
				IsPrimaryFn: func() bool { return true },
				SyncToken:   "tok",
				DefaultMode: config.SyncModeFull,
				SyncFn: func(_ context.Context, _ config.SyncMode) {
					called <- struct{}{}
				},
				SyncRunning: func() bool { return tt.running },
			})

			req := httptest.NewRequest(http.MethodPost, "/sync", nil)
			req.Header.Set("X-Sync-Token", "tok")
			rec := httptest.NewRecorder()
			handler(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCalled {
				<-called
				return
			}
			select {
			case <-called:
				t.Fatal("SyncFn fired despite 409 conflict")
			default:
			}
		})
	}
}
