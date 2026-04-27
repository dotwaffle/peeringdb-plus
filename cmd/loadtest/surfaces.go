//go:build loadtest

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Surface identifies one of the five API surfaces exposed by
// peeringdb-plus.
type Surface string

// Five concrete surface values map 1:1 to the surfaces documented in
// docs/API.md.
const (
	SurfacePdbCompat  Surface = "pdbcompat"
	SurfaceEntRest    Surface = "entrest"
	SurfaceGraphQL    Surface = "graphql"
	SurfaceConnectRPC Surface = "connectrpc"
	SurfaceWebUI      Surface = "webui"
)

// loadtestUserAgent is the default UA. The Web UI surface overrides
// this on a per-endpoint basis to coax the content-negotiation
// middleware into emitting HTML.
const (
	loadtestUserAgent = "peeringdb-plus-loadtest/0.1"
	browserUserAgent  = "Mozilla/5.0 (compatible; pdbplus-loadtest)"
)

// Endpoint is one row in the registry inventory.
//
// EntityType is the short PeeringDB type name (peeringdb.TypeOrg, ...)
// or empty for surface-wide endpoints (e.g. /ui/about).
type Endpoint struct {
	Surface    Surface
	EntityType string
	Shape      string
	Method     string
	Path       string
	Body       []byte
	Header     http.Header
}

// Result captures the outcome of a single Hit.
type Result struct {
	Endpoint Endpoint
	Status   int
	Latency  time.Duration
	Err      error
}

// Hit executes one request against base+ep.Path and returns a Result.
// Network errors are folded into Result.Err; non-2xx is recorded as
// Status only — callers categorise success via the helper Result.OK.
func Hit(ctx context.Context, client *http.Client, base, authToken string, ep Endpoint) Result {
	url := base + ep.Path
	res := Result{Endpoint: ep}

	var body io.Reader
	if len(ep.Body) > 0 {
		body = bytes.NewReader(ep.Body)
	}

	req, err := http.NewRequestWithContext(ctx, ep.Method, url, body)
	if err != nil {
		res.Err = fmt.Errorf("build request %s %s: %w", ep.Method, url, err)
		return res
	}

	// Default headers — overridden per-endpoint where needed.
	req.Header.Set("User-Agent", loadtestUserAgent)
	if ep.Surface == SurfaceWebUI {
		req.Header.Set("User-Agent", browserUserAgent)
	}
	for k, vs := range ep.Header {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	start := time.Now()
	resp, err := client.Do(req)
	res.Latency = time.Since(start)
	if err != nil {
		res.Err = fmt.Errorf("do %s %s: %w", ep.Method, url, err)
		return res
	}
	defer resp.Body.Close()

	// Drain the body so the connection is reusable (mirror the
	// project's own internal/peeringdb/client.go pattern).
	_, _ = io.Copy(io.Discard, resp.Body)
	res.Status = resp.StatusCode
	return res
}

// OK reports whether a Result represents a successful request: no
// transport error and a 2xx status.
func (r Result) OK() bool {
	return r.Err == nil && r.Status >= 200 && r.Status < 300
}
