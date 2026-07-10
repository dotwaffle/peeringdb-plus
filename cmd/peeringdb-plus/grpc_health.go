package main

import (
	"context"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
)

// syncHealthChecker reports gRPC health from the sync worker's live
// state: NOT_SERVING until the first successful sync (primary) or until
// replicated sync history is observed (replica), SERVING after — the
// same signal the HTTP readiness gate uses. Evaluated per Check RPC, so
// there is no poller and no push-state to keep in step; any future
// un-latching of the worker's readiness propagates on the next check.
type syncHealthChecker struct {
	worker   syncReadiness
	services map[string]struct{}
}

// newSyncHealthChecker builds a checker that recognises the empty
// service name (whole-process health) plus each named ConnectRPC
// service, mirroring the per-service registrations the previous
// StaticChecker carried.
func newSyncHealthChecker(worker syncReadiness, serviceNames []string) *syncHealthChecker {
	services := make(map[string]struct{}, len(serviceNames))
	for _, name := range serviceNames {
		services[name] = struct{}{}
	}
	return &syncHealthChecker{worker: worker, services: services}
}

// Check implements grpchealth.Checker. Unknown services return
// CodeNotFound per the grpc.health.v1 contract.
func (c *syncHealthChecker) Check(_ context.Context, req *grpchealth.CheckRequest) (*grpchealth.CheckResponse, error) {
	if req.Service != "" {
		if _, ok := c.services[req.Service]; !ok {
			return nil, connect.NewError(connect.CodeNotFound, nil)
		}
	}
	if c.worker.HasCompletedSync() {
		return &grpchealth.CheckResponse{Status: grpchealth.StatusServing}, nil
	}
	return &grpchealth.CheckResponse{Status: grpchealth.StatusNotServing}, nil
}
