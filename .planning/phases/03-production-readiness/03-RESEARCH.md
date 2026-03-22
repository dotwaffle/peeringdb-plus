# Phase 3: Production Readiness - Research

**Researched:** 2026-03-22
**Domain:** OpenTelemetry observability, health endpoints, Fly.io + LiteFS deployment
**Confidence:** HIGH

## Summary

Phase 3 transforms the working PeeringDB Plus application (Phases 1-2) into a production-grade system with three pillars: comprehensive OpenTelemetry observability (traces, metrics, logs), health/readiness endpoints that report sync freshness, and deployment on Fly.io with LiteFS for global edge reads.

The existing codebase already has basic OTel tracing stubs (stdout exporter in `internal/otel/provider.go`), slog structured logging middleware, a `sync_status` table that tracks sync results, a readiness gate that returns 503 until first sync completes, and environment-based configuration. This phase extends all of these into production-ready implementations rather than building from scratch.

**Primary recommendation:** Use the `autoexport` package from OTel contrib for environment-driven exporter selection (traces, metrics, logs all configurable via standard `OTEL_*` env vars). Combine with `otelslog` bridge for dual slog output (stdout + OTel pipeline). Use `otelhttp.NewMiddleware` to wrap the existing HTTP handler stack. Create `Dockerfile.prod` extending the Phase 1 Dockerfile with LiteFS binary. Start with a single Fly.io region per D-15.

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Export telemetry via OTLP protocol -- works with any OTel-compatible backend
- **D-02:** Configurable sampling rate via PDBPLUS_OTEL_SAMPLE_RATE env var, default to always sample (1.0)
- **D-03:** slog output flows through OTel log pipeline AND stdout -- dual output
- **D-04:** Allow disabling tracing, metrics, and logs via OTel individually -- Fly.io may not support all signals
- **D-05:** Track HTTP request metrics (latency/count), sync metrics (duration/status), and Go runtime metrics (goroutines, memory)
- **D-06:** Centralized OTel setup in `internal/otel` package
- **D-07:** Use standard OTel env vars (OTEL_EXPORTER_OTLP_ENDPOINT etc.) -- follow OTel convention, no custom env vars
- **D-08:** Service version via `debug.ReadBuildInfo()` at runtime -- no ldflags build-time injection
- **D-09:** W3C TraceContext propagation via traceparent/tracestate headers -- enables distributed tracing
- **D-10:** OTel resource attributes include Fly.io-specific data: FLY_REGION, FLY_MACHINE_ID, FLY_APP_NAME
- **D-11:** Health endpoint design at Claude's discretion (separate liveness/readiness or combined)
- **D-12:** Sync freshness threshold configurable via PDBPLUS_SYNC_STALE_THRESHOLD env var, default 24 hours
- **D-13:** Health endpoints return detailed JSON with component status (db, sync, otel) -- HTTP status code reflects overall health (200 OK / 503 unhealthy)
- **D-14:** Metrics exported via OTel push only -- no Prometheus /metrics endpoint
- **D-15:** Start with single Fly.io region -- add more regions later
- **D-16:** Machine size: shared-cpu-1x with 512MB RAM
- **D-17:** LiteFS with Consul static leasing for leader election -- more resilient primary failover
- **D-18:** Include fly.toml template in the repo with sensible defaults
- **D-19:** No Litestream backup -- data can always be re-synced from PeeringDB
- **D-20:** LiteFS as Dockerfile entrypoint / process supervisor -- starts LiteFS first, then launches app
- **D-21:** Persistent Fly.io volume mounted at /data for SQLite database
- **D-22:** Manual deploy via `fly deploy` -- no CI/CD pipeline initially
- **D-23:** Separate Dockerfile.prod for LiteFS deployment -- keep Phase 1 Dockerfile for local dev
- **D-24:** Primary/replica role detection via LiteFS lease file (.primary) -- standard LiteFS mechanism
- **D-25:** Fixed machine count per region -- no auto-scaling initially
- **D-26:** App handles write-forwarding internally for /sync endpoint -- checks if primary, returns redirect if not (does NOT rely on LiteFS proxy for this)
- **D-27:** Use Fly.io's built-in Consul at standard address -- no custom Consul configuration

### Claude's Discretion
- Health endpoint path design (separate /healthz + /readyz vs combined /health)
- Exact OTel metric names and histogram buckets
- LiteFS configuration file details (litefs.yml)
- fly.toml region and machine count defaults
- Individual OTel signal disable env var naming

### Deferred Ideas (OUT OF SCOPE)
- None -- discussion stayed within phase scope

</user_constraints>

<phase_requirements>

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| OPS-01 | OpenTelemetry tracing throughout | OTel autoexport + otelhttp middleware + manual spans in sync worker. Existing stub in `internal/otel/provider.go` gets replaced with full TracerProvider + MeterProvider + LoggerProvider. |
| OPS-02 | OpenTelemetry metrics throughout | autoexport NewMetricReader + runtime.Start() for Go runtime metrics + custom sync/HTTP metrics via otel.Meter |
| OPS-03 | OpenTelemetry structured logging (slog) | otelslog bridge creates dual handler: slog -> OTel LoggerProvider AND slog -> stdout JSONHandler via io.MultiWriter or slog handler fan-out |
| OPS-04 | Health/readiness endpoints with sync age check | Extend existing /health endpoint. Add /healthz (liveness) + /readyz (readiness). Readiness checks db connectivity + sync freshness from sync_status table. |
| OPS-05 | Expose last sync timestamp | Already partially done via syncStatus GraphQL query. Health endpoint also includes lastSyncAt. |
| STOR-02 | Deploy on Fly.io with LiteFS for global edge reads | Dockerfile.prod with LiteFS binary, litefs.yml, fly.toml. Replace PDBPLUS_IS_PRIMARY env var with LiteFS .primary file detection. |

</phase_requirements>

## Standard Stack

### Core (New Dependencies for Phase 3)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| go.opentelemetry.io/contrib/exporters/autoexport | v0.67.0 | Environment-driven exporter selection | Reads standard OTEL_*_EXPORTER env vars to select OTLP/console/none. Eliminates custom exporter-selection code. Supports traces, metrics, logs. |
| go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp | latest | OTLP trace exporter (HTTP) | Default transport for autoexport. Reads OTEL_EXPORTER_OTLP_ENDPOINT automatically. |
| go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp | latest | OTLP metric exporter (HTTP) | Default transport for autoexport metrics. |
| go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp | latest | OTLP log exporter (HTTP) | Default transport for autoexport logs. Experimental but functional. |
| go.opentelemetry.io/otel/exporters/stdout/stdoutmetric | latest | Stdout metric exporter | Fallback when no OTLP endpoint configured (dev). |
| go.opentelemetry.io/otel/exporters/stdout/stdoutlog | latest | Stdout log exporter | Fallback when no OTLP endpoint configured (dev). |
| go.opentelemetry.io/otel/sdk/log | latest | OTel Log SDK | Required for LoggerProvider. Experimental but stable enough for production use. |
| go.opentelemetry.io/contrib/bridges/otelslog | v0.17.0 | slog-to-OTel log bridge | Bridges slog.Handler to OTel LoggerProvider. Converts slog records to OTel log records with severity mapping. |
| go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp | v0.67.0 | HTTP server instrumentation | Automatic spans + metrics for all HTTP requests. NewMiddleware wraps existing handler stack. |
| go.opentelemetry.io/contrib/instrumentation/runtime | v0.67.0 | Go runtime metrics | Automatic collection of goroutine count, memory usage, GC stats. Uses runtime/metrics under the hood. |
| go.opentelemetry.io/otel/propagation | (in core) | W3C TraceContext propagation | propagation.TraceContext{} + propagation.Baggage{} for D-09. |

### Already Present (From Phases 1-2)

| Library | Version | Purpose |
|---------|---------|---------|
| go.opentelemetry.io/otel | v1.42.0 | OTel API (tracer, meter) |
| go.opentelemetry.io/otel/sdk | v1.42.0 | TracerProvider SDK |
| go.opentelemetry.io/otel/sdk/trace | v1.42.0 | Trace SDK |
| go.opentelemetry.io/otel/exporters/stdout/stdouttrace | v1.42.0 | Stdout trace exporter (dev fallback) |
| log/slog (stdlib) | Go 1.26 | Structured logging |

### Not Needed

| Library | Why Not |
|---------|---------|
| go.opentelemetry.io/otel/bridge/opencensus | entgo OTel support not used -- manual spans suffice (Pitfall 11) |
| prometheus/client_golang | D-14 says no Prometheus endpoint -- OTel push only |
| go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc | HTTP/protobuf is the default autoexport protocol; gRPC adds a grpc dependency |

**Installation:**
```bash
go get go.opentelemetry.io/contrib/exporters/autoexport@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp@latest
go get go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp@latest
go get go.opentelemetry.io/otel/exporters/stdout/stdoutmetric@latest
go get go.opentelemetry.io/otel/exporters/stdout/stdoutlog@latest
go get go.opentelemetry.io/otel/sdk/log@latest
go get go.opentelemetry.io/contrib/bridges/otelslog@latest
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@latest
go get go.opentelemetry.io/contrib/instrumentation/runtime@latest
```

## Architecture Patterns

### Recommended Project Structure (Phase 3 Changes)

```
internal/
  otel/
    provider.go          # REWRITE: Full TracerProvider + MeterProvider + LoggerProvider init
    metrics.go           # NEW: Custom metric instruments (sync duration, HTTP request counts)
    shutdown.go          # NEW: Graceful shutdown helper for all providers
  litefs/
    primary.go           # NEW: Primary detection via .primary file (replaces PDBPLUS_IS_PRIMARY)
  health/
    handler.go           # NEW: /healthz + /readyz handlers with component checks
  config/
    config.go            # EXTEND: Add PDBPLUS_SYNC_STALE_THRESHOLD, PDBPLUS_OTEL_SAMPLE_RATE
  middleware/
    logging.go           # EXTEND: Add trace/span ID correlation from context
litefs.yml               # NEW: LiteFS configuration
Dockerfile.prod          # NEW: Production Dockerfile with LiteFS
fly.toml                 # NEW: Fly.io deployment config
```

### Pattern 1: Centralized OTel Provider with autoexport

**What:** Single `internal/otel` package initializes all three providers using autoexport, which reads standard `OTEL_*` env vars at runtime.
**When:** Application startup, before any instrumented code runs.
**Why:** D-06 requires centralized setup. D-07 requires standard env vars. autoexport handles both automatically.

```go
// internal/otel/provider.go
package otel

import (
    "context"
    "fmt"
    "log/slog"
    "runtime/debug"

    "go.opentelemetry.io/contrib/exporters/autoexport"
    "go.opentelemetry.io/contrib/instrumentation/runtime"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/propagation"
    "go.opentelemetry.io/otel/sdk/log"
    "go.opentelemetry.io/otel/sdk/metric"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// SetupInput holds configuration for OTel initialization.
type SetupInput struct {
    ServiceName string
    SampleRate  float64 // 0.0 to 1.0, default 1.0 (always sample)
}

// Setup initializes TracerProvider, MeterProvider, LoggerProvider using
// autoexport for environment-driven exporter selection.
// Returns a shutdown function that flushes all providers.
func Setup(ctx context.Context, in SetupInput) (shutdown func(context.Context) error, err error) {
    res := buildResource(ctx, in.ServiceName)

    // TracerProvider
    spanExporter, err := autoexport.NewSpanExporter(ctx)
    if err != nil {
        return nil, fmt.Errorf("creating span exporter: %w", err)
    }
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(spanExporter),
        sdktrace.WithResource(res),
        sdktrace.WithSampler(sdktrace.TraceIDRatioBased(in.SampleRate)),
    )
    otel.SetTracerProvider(tp)

    // MeterProvider
    metricReader, err := autoexport.NewMetricReader(ctx)
    if err != nil {
        return nil, fmt.Errorf("creating metric reader: %w", err)
    }
    mp := metric.NewMeterProvider(
        metric.WithResource(res),
        metric.WithReader(metricReader),
    )
    otel.SetMeterProvider(mp)

    // LoggerProvider
    logExporter, err := autoexport.NewLogExporter(ctx)
    if err != nil {
        return nil, fmt.Errorf("creating log exporter: %w", err)
    }
    lp := log.NewLoggerProvider(
        log.WithResource(res),
        log.WithProcessor(log.NewBatchProcessor(logExporter)),
    )

    // W3C TraceContext propagation per D-09
    otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{},
        propagation.Baggage{},
    ))

    // Go runtime metrics per D-05
    if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
        return nil, fmt.Errorf("starting runtime metrics: %w", err)
    }

    // Return composite shutdown
    return func(ctx context.Context) error {
        // Shutdown in reverse order
        var errs []error
        if e := lp.Shutdown(ctx); e != nil { errs = append(errs, e) }
        if e := mp.Shutdown(ctx); e != nil { errs = append(errs, e) }
        if e := tp.Shutdown(ctx); e != nil { errs = append(errs, e) }
        return errors.Join(errs...)
    }, nil
}
```

### Pattern 2: Dual slog Output (OTel + stdout)

**What:** Create a composite slog handler that writes to both the OTel log pipeline and stdout JSON.
**When:** After OTel LoggerProvider is initialized.
**Why:** D-03 requires dual output so logs are visible even when OTel backend is unavailable.

```go
// internal/otel/logger.go
package otel

import (
    "io"
    "log/slog"

    "go.opentelemetry.io/contrib/bridges/otelslog"
    sdklog "go.opentelemetry.io/otel/sdk/log"
)

// NewDualLogger creates a slog.Logger that writes to both stdout (JSON)
// and the OTel log pipeline per D-03.
func NewDualLogger(stdout io.Writer, logProvider *sdklog.LoggerProvider) *slog.Logger {
    otelHandler := otelslog.NewHandler("peeringdb-plus",
        otelslog.WithLoggerProvider(logProvider),
    )
    stdoutHandler := slog.NewJSONHandler(stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })
    return slog.New(&fanoutHandler{
        handlers: []slog.Handler{stdoutHandler, otelHandler},
    })
}

// fanoutHandler sends log records to multiple slog handlers.
type fanoutHandler struct {
    handlers []slog.Handler
    attrs    []slog.Attr
    group    string
}

func (h *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
    for _, handler := range h.handlers {
        if handler.Enabled(ctx, level) {
            return true
        }
    }
    return false
}

func (h *fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
    for _, handler := range h.handlers {
        if handler.Enabled(ctx, record.Level) {
            if err := handler.Handle(ctx, record); err != nil {
                // Best-effort: log to other handlers even if one fails
                continue
            }
        }
    }
    return nil
}
```

### Pattern 3: LiteFS Primary Detection (File-Based)

**What:** Check `/litefs/.primary` file to determine if this node is the primary.
**When:** Application startup and runtime (sync gating, write forwarding).
**Why:** D-24 specifies LiteFS lease file mechanism. Replaces the current PDBPLUS_IS_PRIMARY env var.

```go
// internal/litefs/primary.go
package litefs

import (
    "errors"
    "os"
)

const primaryFile = "/litefs/.primary"

// IsPrimary returns true if this node holds the LiteFS lease.
// The .primary file exists ONLY on replica nodes (contains primary hostname).
// Its absence means we ARE the primary.
func IsPrimary() bool {
    _, err := os.Stat(primaryFile)
    return errors.Is(err, os.ErrNotExist)
}
```

### Pattern 4: Health Endpoints with Component Checks

**What:** Separate /healthz (liveness) and /readyz (readiness) endpoints returning JSON with per-component status.
**When:** All nodes, always available (even before first sync).
**Why:** D-13 requires detailed JSON. Liveness confirms the process is running. Readiness confirms data is available and fresh.

**Recommendation for Claude's discretion:** Use separate /healthz + /readyz paths. Kubernetes and Fly.io health check conventions distinguish liveness from readiness. /healthz always returns 200 (process alive). /readyz returns 200 only when db is accessible AND sync data is fresh (within PDBPLUS_SYNC_STALE_THRESHOLD).

```go
// Readiness check logic
type ReadinessStatus struct {
    Status     string           `json:"status"`     // "ready" or "not_ready"
    Components map[string]Check `json:"components"`
}

type Check struct {
    Status  string `json:"status"`  // "ok" or "degraded" or "failed"
    Message string `json:"message,omitempty"`
}

// Components to check:
// 1. "db" - can we query SQLite?
// 2. "sync" - is last sync within stale threshold?
// 3. "otel" - is OTel pipeline connected? (informational, not blocking)
```

### Pattern 5: otelhttp Middleware in the Stack

**What:** Insert otelhttp.NewMiddleware into the existing middleware chain between Recovery and Logging.
**When:** All HTTP requests.
**Why:** Automatic span creation and HTTP metrics for every request per OPS-01, OPS-02.

```go
// Updated middleware order:
// 1. Recovery (catch panics, return 500)
// 2. OTel HTTP (tracing spans + metrics) -- NEW
// 3. Logging (slog with request context, now includes trace_id)
// 4. CORS (browser access for GraphQL playground)
// 5. Readiness gate
// 6. Handler

var handler http.Handler = readinessMiddleware(syncWorker, mux)
handler = middleware.CORS(corsInput)(handler)
handler = middleware.Logging(logger)(handler)
handler = otelhttp.NewMiddleware("peeringdb-plus")(handler)
handler = middleware.Recovery(logger)(handler)
```

### Pattern 6: Write Forwarding via Fly-Replay Header

**What:** On replica nodes, /sync endpoint returns a response with `Fly-Replay: leader` header so Fly.io routes the request to the primary.
**When:** Replica receives POST /sync request.
**Why:** D-26 requires app-level write forwarding (not LiteFS proxy).

```go
func syncHandler(w http.ResponseWriter, r *http.Request) {
    if !litefs.IsPrimary() {
        w.Header().Set("Fly-Replay", "leader")
        w.WriteHeader(http.StatusTemporaryRedirect)
        return
    }
    // ... trigger sync on primary
}
```

### Anti-Patterns to Avoid

- **Custom exporter selection logic:** Use autoexport, not hand-rolled if/else chains checking env vars.
- **Prometheus /metrics endpoint:** D-14 explicitly forbids this. OTel push only.
- **Build-time version injection via ldflags:** D-08 says use `debug.ReadBuildInfo()`.
- **Starting runtime metrics before MeterProvider:** Runtime metrics must use the configured MeterProvider, not the global noop.
- **Blocking on OTel shutdown in signal handler:** Use a timeout context (the existing DrainTimeout works).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Exporter selection per env | Custom if/else for OTLP vs stdout vs none | `autoexport.NewSpanExporter` etc. | Handles all standard OTEL_* env vars including protocol selection |
| HTTP span creation | Manual tracer.Start in every handler | `otelhttp.NewMiddleware` | Automatic spans, metrics, status code recording, request/response size |
| Runtime metrics collection | Manual goroutine/memory polling | `runtime.Start()` from instrumentation/runtime | Uses runtime/metrics API, follows OTel semantic conventions |
| slog-to-OTel bridging | Custom slog handler writing to OTel API | `otelslog.NewHandler` | Handles severity mapping, attribute conversion, trace context injection |
| W3C propagation | Parsing traceparent headers manually | `propagation.TraceContext{}` | Standard-compliant, handles both extraction and injection |
| Primary detection | Custom Consul client or env var | Read `/litefs/.primary` file | Standard LiteFS mechanism, no extra dependencies |

## Common Pitfalls

### Pitfall 1: autoexport Defaults to OTLP When No Endpoint Configured

**What goes wrong:** autoexport defaults to OTLP exporter even when OTEL_EXPORTER_OTLP_ENDPOINT is not set. Without an endpoint, the exporter tries to connect to localhost:4318 (HTTP) or localhost:4317 (gRPC) and fails silently, buffering spans until memory fills.
**Why it happens:** The default behavior assumes an OTel Collector is running locally, which is not the case in dev or early Fly.io deployments.
**How to avoid:** Use `WithFallbackSpanExporter` (and metric/log equivalents) to fall back to stdout exporters when the standard env vars are unset. Or set OTEL_TRACES_EXPORTER=console for dev.
**Warning signs:** Memory growth in long-running dev instances. No traces appearing despite instrumentation.

### Pitfall 2: OTel Log SDK Is Experimental

**What goes wrong:** The OTel log SDK (`go.opentelemetry.io/otel/sdk/log`) and the log OTLP exporter are marked experimental. API may change between minor versions.
**Why it happens:** OTel logs signal reached stable status in the spec but Go SDK implementation lags behind.
**How to avoid:** Pin versions. The otelslog bridge (v0.17.0, published 2026-03-06) is functional and widely used. Accept minor API churn. The dual-output pattern (D-03) means logs always appear on stdout even if the OTel pipeline breaks.
**Warning signs:** Build failures after dependency updates touching log packages.

### Pitfall 3: LiteFS .primary File Semantics Are Inverted

**What goes wrong:** Developers assume `.primary` file exists on the primary node. In reality, `.primary` exists ONLY on replica nodes (containing the primary's hostname). Its ABSENCE means you ARE the primary.
**Why it happens:** The filename `.primary` is misleading. LiteFS documentation is clear but the intuition is wrong.
**How to avoid:** The `litefs.IsPrimary()` function must check for `os.ErrNotExist` returning true. Comment the inversion clearly.
**Warning signs:** Sync running on replicas, SQLITE_READONLY errors.

### Pitfall 4: LiteFS Requires FUSE and Root in Docker

**What goes wrong:** LiteFS uses FUSE to intercept SQLite file operations. Docker containers need `--privileged` or specific capabilities. The Dockerfile must install `fuse3` and run LiteFS as root (or with FUSE permissions).
**Why it happens:** FUSE is a kernel-level filesystem. Unprivileged containers cannot mount FUSE filesystems without explicit capabilities.
**How to avoid:** Use `RUN apk add fuse3` in Dockerfile. Do NOT switch to a non-root user. Fly.io machines run with sufficient privileges for FUSE by default.
**Warning signs:** "fuse: mount failed" errors at startup.

### Pitfall 5: LiteFS proxy passthrough Must Include Health Endpoints

**What goes wrong:** The LiteFS proxy intercepts all requests and applies consistency logic (waiting for replication position). Health check endpoints should bypass this -- they must respond immediately regardless of replication state.
**Why it happens:** Default proxy behavior is to manage all requests for consistency.
**How to avoid:** Add health paths to the proxy passthrough list in litefs.yml: `passthrough: ["/healthz", "/readyz"]`.
**Warning signs:** Health checks timing out during replication lag, causing Fly.io to kill machines.

### Pitfall 6: Replacing PDBPLUS_IS_PRIMARY with File-Based Detection

**What goes wrong:** The current codebase uses `PDBPLUS_IS_PRIMARY` env var (config.go line 79). In production with LiteFS, primary role is dynamic (determined by Consul lease). The env var cannot be changed at runtime.
**Why it happens:** Phase 1 used a static env var for simplicity. LiteFS primary can change during failover.
**How to avoid:** Replace `cfg.IsPrimary` with `litefs.IsPrimary()` calls at runtime. The sync scheduler should check before each sync attempt. Schema migration should also check. Keep the env var as a fallback for local dev (where LiteFS is not running).
**Warning signs:** Primary failover doesn't move the sync worker. Two nodes both trying to sync after failover.

### Pitfall 7: Fly.io Volume is Per-Machine, Not Shared

**What goes wrong:** Developers expect the volume mount to be shared across machines. Each Fly.io machine gets its own volume. LiteFS handles replication between them.
**Why it happens:** Fly.io volumes are local storage, not NFS.
**How to avoid:** Understand that `/data` (the volume mount for LiteFS data dir) is unique per machine. LiteFS's data dir (`/var/lib/litefs`) stores replication state on the volume. The FUSE mount (`/litefs`) is where the application accesses the database.
**Warning signs:** Data not appearing on new machines until LiteFS replication catches up.

### Pitfall 8: Sample Rate Must Be Set on TracerProvider, Not Per-Span

**What goes wrong:** Setting sample rate per-span or using always-sample in TracerProvider defeats the purpose of D-02's configurable sampling.
**Why it happens:** OTel Go has multiple sampling strategies. The correct one for a global rate is `TraceIDRatioBased` on the TracerProvider.
**How to avoid:** Use `sdktrace.WithSampler(sdktrace.TraceIDRatioBased(rate))` where rate comes from PDBPLUS_OTEL_SAMPLE_RATE.
**Warning signs:** 100% trace data volume despite setting a lower sample rate.

## Code Examples

### Resource Builder with Fly.io Attributes (D-08, D-10)

```go
// Source: OpenTelemetry Go SDK docs + Fly.io env var conventions
func buildResource(ctx context.Context, serviceName string) *resource.Resource {
    // Get version from build info per D-08
    version := "unknown"
    if info, ok := debug.ReadBuildInfo(); ok {
        version = info.Main.Version
        if version == "(devel)" {
            // Development build -- try vcs.revision
            for _, s := range info.Settings {
                if s.Key == "vcs.revision" && len(s.Value) >= 7 {
                    version = s.Value[:7]
                    break
                }
            }
        }
    }

    attrs := []attribute.KeyValue{
        semconv.ServiceName(serviceName),
        semconv.ServiceVersion(version),
    }

    // Fly.io-specific attributes per D-10
    if region := os.Getenv("FLY_REGION"); region != "" {
        attrs = append(attrs, attribute.String("fly.region", region))
    }
    if machineID := os.Getenv("FLY_MACHINE_ID"); machineID != "" {
        attrs = append(attrs, attribute.String("fly.machine_id", machineID))
    }
    if appName := os.Getenv("FLY_APP_NAME"); appName != "" {
        attrs = append(attrs, attribute.String("fly.app_name", appName))
    }

    res, _ := resource.Merge(
        resource.Default(),
        resource.NewWithAttributes(semconv.SchemaURL, attrs...),
    )
    return res
}
```

### Custom Sync Metrics (D-05)

```go
// internal/otel/metrics.go
package otel

import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/metric"
)

var (
    meter = otel.Meter("peeringdb-plus")

    // Sync metrics per D-05
    SyncDuration metric.Float64Histogram
    SyncStatus   metric.Int64Counter

    // HTTP metrics are handled automatically by otelhttp
)

func InitMetrics() error {
    var err error
    SyncDuration, err = meter.Float64Histogram(
        "pdbplus.sync.duration",
        metric.WithDescription("Duration of sync operations in seconds"),
        metric.WithUnit("s"),
        metric.WithExplicitBucketBoundaries(1, 5, 10, 30, 60, 120, 300),
    )
    if err != nil {
        return err
    }

    SyncStatus, err = meter.Int64Counter(
        "pdbplus.sync.operations",
        metric.WithDescription("Count of sync operations by status"),
        metric.WithUnit("{operation}"),
    )
    return err
}
```

### litefs.yml Configuration

```yaml
fuse:
  dir: "/litefs"

data:
  dir: "/var/lib/litefs"
  compress: true
  retention: "10m"

# LiteFS launches the app as a subprocess per D-20
exec:
  - cmd: "/usr/local/bin/peeringdb-plus"

# Internal replication API
http:
  addr: ":20202"

# HTTP proxy for consistency and write forwarding
proxy:
  addr: ":8080"
  target: "localhost:8081"
  db: "peeringdb-plus.db"
  passthrough:
    - "/healthz"
    - "/readyz"

# Consul-based leader election per D-17, D-27
lease:
  type: "consul"
  candidate: ${FLY_REGION == PRIMARY_REGION}
  advertise-url: "http://${FLY_ALLOC_ID}.vm.${FLY_APP_NAME}.internal:20202"
  consul:
    url: "${FLY_CONSUL_URL}"
    key: "${FLY_APP_NAME}/primary"
```

### fly.toml Configuration

```toml
app = "peeringdb-plus"
primary_region = "iad"
kill_signal = "SIGTERM"
kill_timeout = 30

[build]
  dockerfile = "Dockerfile.prod"

[env]
  PDBPLUS_DB_PATH = "/litefs/peeringdb-plus.db"
  PDBPLUS_LISTEN_ADDR = ":8081"
  PRIMARY_REGION = "iad"

[[mounts]]
  source = "litefs_data"
  destination = "/var/lib/litefs"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = "off"
  auto_start_machines = true
  min_machines_running = 1

  [[http_service.checks]]
    grace_period = "30s"
    interval = "15s"
    method = "GET"
    timeout = "5s"
    path = "/healthz"

[[vm]]
  size = "shared-cpu-1x"
  memory = "512mb"
```

### Dockerfile.prod

```dockerfile
# Build stage (same as Phase 1 Dockerfile)
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /peeringdb-plus ./cmd/peeringdb-plus

# Production stage with LiteFS per D-20, D-23
FROM alpine:3.21

# Install LiteFS dependencies per D-20
RUN apk add --no-cache ca-certificates fuse3

# Copy LiteFS binary (v0.5 tag covers 0.5.x line)
COPY --from=flyio/litefs:0.5 /usr/local/bin/litefs /usr/local/bin/litefs

# Copy application binary
COPY --from=builder /peeringdb-plus /usr/local/bin/peeringdb-plus

# Copy LiteFS configuration
COPY litefs.yml /etc/litefs.yml

LABEL org.opencontainers.image.title="peeringdb-plus" \
      org.opencontainers.image.description="High-performance read-only PeeringDB mirror"

EXPOSE 8080
EXPOSE 20202

# LiteFS as entrypoint / process supervisor per D-20
ENTRYPOINT ["litefs", "mount"]
```

### Config Extensions

New env vars for this phase:

| Env Var | Default | Description |
|---------|---------|-------------|
| PDBPLUS_OTEL_SAMPLE_RATE | 1.0 | Trace sampling rate 0.0-1.0 (D-02) |
| PDBPLUS_SYNC_STALE_THRESHOLD | 24h | Max age before sync considered stale (D-12) |
| OTEL_EXPORTER_OTLP_ENDPOINT | (none) | Standard OTel OTLP endpoint (D-07) |
| OTEL_TRACES_EXPORTER | otlp | Standard: otlp, console, none (D-04) |
| OTEL_METRICS_EXPORTER | otlp | Standard: otlp, console, none (D-04) |
| OTEL_LOGS_EXPORTER | otlp | Standard: otlp, console, none (D-04) |

For D-04 (individual signal disabling): Use the standard `OTEL_TRACES_EXPORTER=none`, `OTEL_METRICS_EXPORTER=none`, `OTEL_LOGS_EXPORTER=none` env vars. No custom env vars needed -- autoexport handles this natively.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Manual exporter setup code | autoexport with env var selection | OTel contrib v0.50+ | Eliminates ~50 lines of custom exporter wiring |
| OpenCensus bridge for entgo | Direct OTel spans via hooks | OTel Go SDK v1.0+ | No bridge dependency needed for custom spans |
| slog + separate OTel logging | otelslog bridge (slog -> OTel) | otelslog v0.17.0 (2026-03) | Single logger for both outputs |
| LiteFS Cloud for backups | Self-managed or Litestream | Oct 2024 (Cloud sunset) | D-19 says no backup needed (re-sync) |
| LiteFS static leasing | Consul leasing | LiteFS v0.5+ | D-17: dynamic failover |

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Build | Yes | 1.26.1 | -- |
| Docker | Dockerfile.prod build | Yes | 29.3.0 | -- |
| golangci-lint | Lint | Yes | 2.11.3 | -- |
| flyctl | Deployment (D-22) | Installed but sandbox-restricted | unknown | Run outside sandbox for deploy |
| Consul | LiteFS leader election | Fly.io managed (`fly consul attach`) | -- | N/A -- Fly.io provides it |
| FUSE | LiteFS runtime | Fly.io machines include FUSE | -- | N/A -- only needed on Fly.io |

**Missing dependencies with no fallback:**
- flyctl cannot run in the current sandbox due to filesystem permissions. The user will run `fly deploy` manually per D-22. This is expected -- we only create the fly.toml and Dockerfile.prod templates.

**Missing dependencies with fallback:**
- None. All code-level work can be done and tested locally without Fly.io or LiteFS.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + enttest |
| Config file | None (standard `go test`) |
| Quick run command | `go test ./internal/otel/... ./internal/health/... ./internal/litefs/... -count=1 -short` |
| Full suite command | `go test ./... -count=1 -race` |

### Phase Requirements to Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| OPS-01 | Traces created for HTTP requests and sync | unit | `go test ./internal/otel/... -run TestSetup -count=1` | No -- Wave 0 |
| OPS-02 | Metrics recorded (sync duration, runtime, HTTP) | unit | `go test ./internal/otel/... -run TestMetrics -count=1` | No -- Wave 0 |
| OPS-03 | slog logs flow to OTel and stdout | unit | `go test ./internal/otel/... -run TestDualLogger -count=1` | No -- Wave 0 |
| OPS-04 | Health endpoint returns component status with sync age | unit | `go test ./internal/health/... -count=1` | No -- Wave 0 |
| OPS-05 | Sync timestamp exposed via health endpoint | unit | `go test ./internal/health/... -run TestSyncTimestamp -count=1` | No -- Wave 0 |
| STOR-02 | LiteFS primary detection works correctly | unit | `go test ./internal/litefs/... -count=1` | No -- Wave 0 |
| STOR-02 | Dockerfile.prod builds successfully | smoke | `docker build -f Dockerfile.prod -t pdbplus-test .` | N/A -- manual |

### Sampling Rate
- **Per task commit:** `go test ./internal/otel/... ./internal/health/... ./internal/litefs/... -count=1`
- **Per wave merge:** `go test ./... -count=1 -race`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/otel/provider_test.go` -- covers OPS-01, OPS-02 (TracerProvider, MeterProvider init)
- [ ] `internal/otel/logger_test.go` -- covers OPS-03 (dual slog handler)
- [ ] `internal/otel/metrics_test.go` -- covers OPS-02 (custom metric instruments)
- [ ] `internal/health/handler_test.go` -- covers OPS-04, OPS-05 (health endpoint responses)
- [ ] `internal/litefs/primary_test.go` -- covers STOR-02 (file-based primary detection)

## Open Questions

1. **LiteFS proxy and the existing PDBPLUS_LISTEN_ADDR**
   - What we know: LiteFS proxy listens on :8080, forwards to app on :8081. The app currently defaults to :8080.
   - What's unclear: Whether the existing readiness middleware (503 before first sync) conflicts with the LiteFS proxy passthrough for /healthz.
   - Recommendation: Change the app's default listen address to :8081 when running under LiteFS (detected by FLY_APP_NAME presence). Keep :8080 default for local dev.

2. **otelhttp metric names colliding with custom metrics**
   - What we know: otelhttp creates automatic HTTP metrics (http.server.request.duration etc.). D-05 also asks for HTTP request metrics.
   - What's unclear: Whether otelhttp's automatic metrics are sufficient for D-05 or if custom HTTP metrics are also needed.
   - Recommendation: otelhttp's automatic metrics cover HTTP latency and count. Custom metrics only needed for sync-specific operations. Do not duplicate what otelhttp provides.

3. **sync_status table writes on replicas**
   - What we know: The sync_status table is written via raw SQL, not through LiteFS-managed transactions. On replicas, the database is read-only.
   - What's unclear: Whether InitStatusTable (CREATE TABLE IF NOT EXISTS) will fail on replicas.
   - Recommendation: Guard InitStatusTable with the primary check, same as schema migration. Replicas only read sync_status.

## Project Constraints (from CLAUDE.md)

Directives from CLAUDE.md that affect this phase:

- **CS-5 (MUST):** Input structs for functions with >2 args. OTel Setup function uses SetupInput struct.
- **CS-6 (SHOULD):** Declare input structs before the function consuming them.
- **ERR-1 (MUST):** Wrap errors with %w and context. All OTel init errors must follow this.
- **CC-2 (MUST):** Tie goroutine lifetime to context.Context. Runtime metrics goroutine must respect shutdown.
- **CTX-1 (MUST):** Context as first parameter. All provider functions take ctx first.
- **CFG-1 (MUST):** Config via env/flags; validate on startup; fail fast. New env vars validated in config.Load().
- **CFG-2 (MUST):** Config immutable after init. Sample rate and stale threshold are immutable after Load.
- **OBS-1 (MUST):** Structured logging via slog. Already established, extended with OTel bridge.
- **OBS-4 (SHOULD):** Use OpenTelemetry for observability. This entire phase.
- **OBS-5 (SHOULD):** Use slog attribute setters. Continue existing pattern.
- **SEC-1 (MUST):** Validate inputs; set explicit I/O timeouts. HTTP timeouts on health endpoints.
- **SEC-2 (MUST):** Never log secrets. No OTEL_EXPORTER_OTLP_HEADERS logging.
- **CI-2 (MUST):** Reproducible builds with -trimpath. Already in Dockerfile.
- **T-1 (MUST):** Table-driven tests. All new test files use table-driven pattern.
- **T-2 (MUST):** Run -race in CI. Full test suite uses -race flag.
- **API-1 (MUST):** Document exported items. All exported types/functions need doc comments.

## Sources

### Primary (HIGH confidence)
- [OpenTelemetry Go Getting Started](https://opentelemetry.io/docs/languages/go/getting-started/) -- TracerProvider, MeterProvider, LoggerProvider setup
- [OpenTelemetry Go Exporters](https://opentelemetry.io/docs/languages/go/exporters/) -- OTLP exporter configuration
- [autoexport package (pkg.go.dev)](https://pkg.go.dev/go.opentelemetry.io/contrib/exporters/autoexport) -- v0.67.0 API reference
- [otelslog package (pkg.go.dev)](https://pkg.go.dev/go.opentelemetry.io/contrib/bridges/otelslog) -- v0.17.0 API reference
- [otelhttp package (pkg.go.dev)](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) -- v0.67.0 API reference
- [runtime instrumentation (pkg.go.dev)](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/runtime) -- Go runtime metrics
- [LiteFS Config Reference (Fly Docs)](https://fly.io/docs/litefs/config/) -- litefs.yml full reference
- [LiteFS Getting Started on Fly.io](https://fly.io/docs/litefs/getting-started-fly/) -- Dockerfile, volume, Consul setup
- [fly.toml Configuration Reference](https://fly.io/docs/reference/configuration/) -- Full deployment config
- [Fly.io fly consul attach](https://fly.io/docs/flyctl/consul-attach/) -- Consul provisioning

### Secondary (MEDIUM confidence)
- [OTel Semantic Conventions for Go runtime](https://opentelemetry.io/docs/specs/semconv/runtime/go-metrics/) -- metric naming conventions
- [LiteFS Architecture (GitHub)](https://github.com/superfly/litefs/blob/main/docs/ARCHITECTURE.md) -- replication internals
- [LiteFS Status Discussion (Fly Community)](https://community.fly.io/t/what-is-the-status-of-litefs/23883) -- maintenance mode confirmation

### Tertiary (LOW confidence)
- None -- all findings verified against official documentation.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all packages verified against pkg.go.dev with published dates
- Architecture: HIGH -- patterns derived from official OTel and LiteFS documentation, applied to existing codebase patterns
- Pitfalls: HIGH -- based on official documentation semantics and known LiteFS behaviors

**Research date:** 2026-03-22
**Valid until:** 2026-04-22 (stable stack, LiteFS in maintenance mode so unlikely to change)
