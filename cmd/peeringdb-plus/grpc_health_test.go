package main

import (
	"testing"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
)

// fakeReadiness drives syncHealthChecker through the sync-state
// transition without a real worker.
type fakeReadiness struct{ synced bool }

func (f *fakeReadiness) HasCompletedSync() bool { return f.synced }

// TestSyncHealthChecker locks the live-state health contract that
// replaced the one-shot 1s poller: NOT_SERVING before the first sync,
// SERVING after — evaluated per Check with no push-state — plus the
// grpc.health.v1 CodeNotFound contract for unknown services.
func TestSyncHealthChecker(t *testing.T) {
	t.Parallel()

	worker := &fakeReadiness{}
	checker := newSyncHealthChecker(worker, []string{"peeringdb.v1.NetworkService"})
	ctx := t.Context()

	for _, service := range []string{"", "peeringdb.v1.NetworkService"} {
		resp, err := checker.Check(ctx, &grpchealth.CheckRequest{Service: service})
		if err != nil {
			t.Fatalf("Check(%q) pre-sync: %v", service, err)
		}
		if resp.Status != grpchealth.StatusNotServing {
			t.Errorf("Check(%q) pre-sync = %v, want NOT_SERVING", service, resp.Status)
		}
	}

	// The transition needs no callback or poller: the next Check reads
	// the flipped state directly.
	worker.synced = true
	for _, service := range []string{"", "peeringdb.v1.NetworkService"} {
		resp, err := checker.Check(ctx, &grpchealth.CheckRequest{Service: service})
		if err != nil {
			t.Fatalf("Check(%q) post-sync: %v", service, err)
		}
		if resp.Status != grpchealth.StatusServing {
			t.Errorf("Check(%q) post-sync = %v, want SERVING", service, resp.Status)
		}
	}

	// Unknown services return CodeNotFound per the grpc.health.v1
	// contract (mirrors the previous StaticChecker behaviour).
	if _, err := checker.Check(ctx, &grpchealth.CheckRequest{Service: "no.such.Service"}); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("Check(unknown) error = %v, want CodeNotFound", err)
	}
}
