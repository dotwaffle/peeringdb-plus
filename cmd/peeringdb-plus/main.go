// Package main is the entry point for the peeringdb-plus application.
// It wires together config, database, OTel, PeeringDB client, and sync worker,
// then serves HTTP endpoints for health checks and on-demand sync triggers.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/otelconnect"
	"github.com/KimMachineGun/automemlimit/memlimit"

	"github.com/dotwaffle/peeringdb-plus/ent/migrate"
	"github.com/dotwaffle/peeringdb-plus/ent/rest"
	_ "github.com/dotwaffle/peeringdb-plus/ent/runtime" // register ent schema runtime config (field defaults/validators, privacy policy)
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
	"github.com/dotwaffle/peeringdb-plus/graph"
	"github.com/dotwaffle/peeringdb-plus/internal/buildinfo"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/database"
	pdbgql "github.com/dotwaffle/peeringdb-plus/internal/graphql"
	"github.com/dotwaffle/peeringdb-plus/internal/grpcserver"
	"github.com/dotwaffle/peeringdb-plus/internal/health"
	"github.com/dotwaffle/peeringdb-plus/internal/litefs"
	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/web"
	"github.com/dotwaffle/peeringdb-plus/internal/web/termrender"
)

// maxRequestBodySize is the maximum allowed request body for POST endpoints (1 MB).
// GraphQL queries rarely exceed 10 KB; 1 MB is generous.
const maxRequestBodySize = 1 << 20

// connectHandlerOpts builds the handler options shared by all 13 ConnectRPC
// service registrations: OTel tracing plus an inbound message-size cap.
// WithReadMaxBytes bounds each received message (raw AND decompressed) at the
// connect protocol layer. The HTTP-level MaxBytesBody middleware skips
// ConnectRPC paths so server-to-client streaming is not truncated, which
// would otherwise leave unary request bodies unbounded (gzip-bomb OOM).
// Request messages here are small filter/pagination structs; 1 MB is ample.
func connectHandlerOpts(interceptor connect.Interceptor) connect.HandlerOption {
	return connect.WithHandlerOptions(
		connect.WithInterceptors(interceptor),
		connect.WithReadMaxBytes(maxRequestBodySize),
	)
}

func init() {
	// Best-effort memory limit configuration from cgroup/system.
	_, _ = memlimit.SetGoMemLimitWithOpts(
		memlimit.WithProvider(
			memlimit.ApplyFallback(
				memlimit.FromCgroup,
				memlimit.FromSystem,
			),
		),
	)
}

// discoveryBody builds the JSON service-discovery payload returned to API
// clients on GET /. The version is passed in (rather than read inside) so the
// body can be unit-tested without build-time ldflags injection.
func discoveryBody(version string) string {
	return fmt.Sprintf(
		`{"name":"peeringdb-plus","version":%q,"graphql":"/graphql","rest":"/rest/v1/","api":"/api/","connectrpc":"/peeringdb.v1.","ui":"/ui/","healthz":"/healthz","readyz":"/readyz"}`,
		version,
	)
}

func main() {
	// Load config from environment.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", slog.Any("error", err))
		os.Exit(1)
	}

	// exitCode is delivered to the OS by the deferred os.Exit below. The
	// defer is registered FIRST so it runs LAST — after the OTel flush and
	// entClient.Close defers — letting a server failure exit non-zero
	// without dropping the buffered telemetry that explains it. (An os.Exit
	// inside the serve goroutine would skip every deferred cleanup.)
	exitCode := 0
	defer func() {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	// Initialize OpenTelemetry.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	otelOut, err := pdbotel.Setup(ctx, pdbotel.SetupInput{
		ServiceName: "peeringdb-plus",
		SampleRate:  cfg.OTelSampleRate,
	})
	if err != nil {
		slog.Error("failed to init otel", slog.Any("error", err))
		os.Exit(1) //nolint:gocritic // exitAfterDefer: cancel() deferred above is trivial at this stage
	}
	defer func() {
		// Flush OTel on a context detached from the cancelled root. By the
		// time this defer runs, the signal handler has called cancel() and the
		// root ctx is Done; the OTLP exporters honor cancellation, so the final
		// buffered batch — including the shutdown-time log records — would be
		// dropped instead of flushed. WithoutCancel keeps any context values
		// while shedding the cancellation; the drain timeout bounds the flush.
		flushCtx, flushCancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.DrainTimeout)
		defer flushCancel()
		_ = otelOut.Shutdown(flushCtx) // best-effort flush at exit
	}()

	// Set up dual slog logger (stdout + OTel pipeline).
	logger := pdbotel.NewDualLogger(os.Stdout, otelOut.LogProvider)
	slog.SetDefault(logger)

	// Sync/transport/response instruments need no Init call — they are
	// bound at internal/otel package init and delegate to the provider
	// installed by Setup() above. Observable gauges still register here
	// because they need runtime callbacks.

	// Initialize peak heap / RSS observable gauges for runtime memory visibility.
	if err := pdbotel.InitMemoryGauges(); err != nil {
		logger.Error("failed to init memory gauges", slog.Any("error", err))
		os.Exit(1)
	}

	// Open database.
	entClient, db, err := database.Open(cfg.DBPath, cfg.OTelSQL)
	if err != nil {
		logger.Error("failed to open database", slog.Any("error", err))
		os.Exit(1)
	}
	defer entClient.Close()

	// Detect primary status via LiteFS lease file with env fallback.
	// Validate the fallback var up front: a typo'd PDBPLUS_IS_PRIMARY
	// must fail startup, not be silently coerced into a role (the
	// primary role runs destructive DDL).
	if err := litefs.ValidateEnvFallback("PDBPLUS_IS_PRIMARY"); err != nil {
		logger.Error("invalid primary-role configuration", slog.Any("error", err))
		os.Exit(1)
	}
	isPrimary := litefs.IsPrimaryWithFallback(litefs.PrimaryFile, "PDBPLUS_IS_PRIMARY")
	policy := newStartupPolicy(isPrimary)

	// Live primary detection function for dynamic role changes without restart.
	isPrimaryFn := func() bool {
		return litefs.IsPrimaryWithFallback(litefs.PrimaryFile, "PDBPLUS_IS_PRIMARY")
	}

	// Auto-migrate schema on primary.
	//
	// WithDropColumn(true): enables ALTER TABLE DROP COLUMN for schema
	// cleanup (ixpfx.notes, organization.fac_count, organization.net_count)
	// and any future hygiene drops. ent defaults to additive-only
	// migrations for safety; this flag opts in to destructive DDL.
	//
	// WithDropIndex(true): symmetric handling of stale indexes per the ent
	// docs recommendation. None of the current target columns are indexed,
	// but enabling both together is idiomatic.
	if policy.ShouldMigrateSchema {
		if err := entClient.Schema.Create(
			ctx,
			migrate.WithDropColumn(true),
			migrate.WithDropIndex(true),
		); err != nil {
			logger.Error("failed to migrate schema", slog.Any("error", err))
			os.Exit(1)
		}
	}

	// Initialize sync_status table on primary (raw SQL, outside ent schema management).
	if policy.ShouldInitSyncStatus {
		if err := pdbsync.InitStatusTable(ctx, db); err != nil {
			logger.Error("failed to init sync_status table", slog.Any("error", err))
			os.Exit(1)
		}
		// Transition any stale "running" rows to "failed" so /ui/about
		// and /readyz stop reporting phantom in-flight syncs left behind
		// by a previous process that was killed mid-cycle (typically
		// during rolling deploys). Non-fatal on error — it's cosmetic.
		if reaped, err := pdbsync.ReapStaleRunningRows(ctx, db); err != nil {
			logger.LogAttrs(ctx, slog.LevelError, "failed to reap stale running rows",
				slog.Any("error", err))
		} else if reaped > 0 {
			logger.LogAttrs(ctx, slog.LevelInfo, "reaped stale running rows",
				slog.Int("count", reaped))
		}
	}

	// Initialize sync freshness gauge. The observable callback fires once per
	// OTel metric-export interval (default 60s) and reads sync_status live on
	// each call. The read is a single-row, primary-key-ordered lookup against
	// the local (LiteFS-replicated) SQLite file with no network hop, so it is
	// far too cheap to be worth caching. Reading per call rather than from a
	// cache is deliberate: on a replica — which never runs the sync worker —
	// it reflects real replication lag (e.g. during a deploy) instead of a
	// value frozen at boot.
	if err := pdbotel.InitFreshnessGauge(func(ctx context.Context) (time.Time, bool) {
		return freshnessFromDB(ctx, db)
	}); err != nil {
		logger.Error("failed to init freshness gauge", slog.Any("error", err))
		os.Exit(1)
	}

	// Cached object counts for metrics gauge.
	// Updated by sync worker after each successful sync via OnSyncComplete.
	//
	// Synchronously seed the cache from a one-shot
	// Count(ctx) per entity table at startup so pdbplus_data_type_count
	// reports correct values within 30s of process start. Without this seed
	// the gauge holds zeros until the first sync cycle completes
	// (~15 min default), which renders the "Total Objects", "Objects by
	// Type", and "Object Counts Over Time" Grafana panels as flat-zero
	// for the entire pre-first-sync window after every deploy.
	//
	// Failure mode: if InitialObjectCounts errors (e.g. LiteFS not yet
	// mounted on a replica boot race), log + exit. The cost is ~1-2s on a
	// primed DB; replicas already cold-sync in 5-45s so the extra latency
	// is noise on top of hydration.
	var objectCountCache atomic.Pointer[map[string]int64]
	seededCounts, err := seedObjectCountCache(ctx, db, logger)
	if err != nil {
		logger.Error("failed to seed initial object counts", slog.Any("error", err))
		os.Exit(1)
	}
	objectCountCache.Store(&seededCounts)
	logger.LogAttrs(ctx, slog.LevelInfo, "seeded initial object counts",
		slog.Int("type_count", len(seededCounts)))

	// Initialize per-type object count gauges for business metrics dashboard.
	// Reads from atomic cache instead of live COUNT queries.
	if err := pdbotel.InitObjectCountGauges(func() map[string]int64 {
		return *objectCountCache.Load()
	}); err != nil {
		logger.Error("failed to init object count gauges", slog.Any("error", err))
		os.Exit(1)
	}

	// Create PeeringDB client.
	// WithRPS comes BEFORE WithAPIKey so the auth
	// path can override the unauth RPS to the upstream-fixed 60/min quota
	// inside NewClient (see internal/peeringdb/client.go option apply order).
	var clientOpts []peeringdb.ClientOption
	clientOpts = append(clientOpts, peeringdb.WithRPS(cfg.PeeringDBRPS))
	if cfg.PeeringDBAPIKey != "" {
		clientOpts = append(clientOpts, peeringdb.WithAPIKey(cfg.PeeringDBAPIKey))
		logger.Info("PeeringDB API key configured", slog.String("api_key", "[set]"))
	} else {
		logger.Info("PeeringDB API key not configured, using unauthenticated access",
			slog.String("api_key", "[not set]"),
			slog.Float64("rps", cfg.PeeringDBRPS))
	}

	// Make the disabled-sync state loud at boot so operators see it in
	// deploy logs rather than discovering it via 401s from a curl probe.
	// Empty token is fail-closed: newSyncHandler rejects EVERY request
	// (regression-locked by TestSyncHandler_TokenCompare's disabled-mode
	// rows) — the endpoint is disabled, not open.
	if cfg.SyncToken == "" {
		logger.Warn("PDBPLUS_SYNC_TOKEN not set — POST /sync is disabled (all requests rejected with 401)",
			slog.String("endpoint", "/sync"),
			slog.String("action", "set PDBPLUS_SYNC_TOKEN to enable on-demand sync"))
	}

	// Classify sync mode + public tier at startup.
	// Emitted after config parse / OTel init / dual-logger install, before any
	// handler registration, so a failure to start the server does not swallow
	// the classification record.
	logStartupClassification(logger, cfg)

	pdbClient := peeringdb.NewClient(cfg.PeeringDBBaseURL, logger, clientOpts...)

	// Caching middleware holds the current ETag behind an atomic
	// pointer. Constructed once here so the OnSyncComplete callback below
	// can capture it. The initial ETag is seeded from any existing
	// sync_status row so warm restarts serve cacheable GETs immediately;
	// cold starts leave the pointer nil (Middleware skips caching headers
	// until the first sync completes, matching the prior atomic-ETag behavior).
	//
	// /ui/about is opted out of caching because it renders wall-clock-
	// relative text ("N minutes ago") that would freeze at cache-creation
	// time under the sync-time-keyed ETag. See internal/web/about.go and
	// internal/web/templates/about.templ.
	// /healthz and /readyz are opted out alongside /ui/about: a shared
	// cache pinning a stale health verdict (or a 304 short-circuit that
	// skips the readiness probes entirely) defeats their purpose.
	cachingState := middleware.NewCachingState(cfg.SyncInterval, "/ui/about", "/healthz", "/readyz")
	if t, err := pdbsync.GetLastSuccessfulSyncTime(ctx, db); err == nil && !t.IsZero() {
		cachingState.UpdateETag(t)
	}

	// Create sync worker.
	syncWorker := pdbsync.NewWorker(pdbClient, entClient, db, pdbsync.WorkerConfig{
		IsPrimary: isPrimaryFn,
		SyncMode:  cfg.SyncMode,
		OnSyncComplete: func(ctx context.Context, syncTime time.Time) {
			// Refresh the gauge cache from live
			// row counts instead of the per-cycle upsert deltas the
			// callback used to receive. The old shape under-counted
			// every type after incremental syncs (delta != total) and
			// always under-counted Poc by however many visible="Users"
			// rows existed (the upsert-side count was raw but the
			// gauge cache was previously primed from a TierPublic
			// Count(ctx) at startup, so the two values flipped between
			// "filtered" and "raw" — hence "doubling-halving").
			//
			// InitialObjectCounts now uses raw SQL
			// (UNION ALL across the 13 tables) which bypasses ent's
			// Privacy policy entirely — no Poc.Policy filtering — so
			// visible="Users" Pocs are included symmetrically with the
			// OnSyncComplete writer's privacy.DecisionContext bypass.
			//
			// Failure mode: log+skip the cache update. Keeping the
			// previous value renders the dashboard with a stale (but
			// correct) total rather than a flat-zero, which would
			// trigger ops alerts that aren't actually fires.
			counts, err := pdbsync.InitialObjectCounts(ctx, db)
			if err != nil {
				logger.LogAttrs(ctx, slog.LevelWarn,
					"failed to refresh object counts after sync",
					slog.Any("error", err))
			} else {
				objectCountCache.Store(&counts)
			}
			// Swap the cached ETag using the exact completion
			// timestamp the worker persisted to sync_status. One SHA-256
			// per sync, zero per request. Kept outside the err branch
			// because ETag freshness is decoupled from gauge cache —
			// even if InitialObjectCounts fails the sync itself
			// succeeded.
			cachingState.UpdateETag(syncTime)
		},
		SyncMemoryLimit:               cfg.SyncMemoryLimit,
		HeapWarnBytes:                 cfg.HeapWarnBytes,
		RSSWarnBytes:                  cfg.RSSWarnBytes,
		FKBackfillMaxRequestsPerCycle: cfg.FKBackfillMaxRequestsPerCycle,
		FKBackfillTimeout:             cfg.FKBackfillTimeout,
		SyncTimeout:                   cfg.SyncTimeout,
		FullSyncInterval:              cfg.FullSyncInterval,
	}, logger)

	// Pre-warm the 5 zero-rate counters so dashboard
	// panels render `0` instead of `No data` on a freshly-deployed healthy
	// fleet that hasn't fired any sync errors / fallbacks / role transitions /
	// deletes yet. Without this, OTel cumulative counters only export a
	// series after the first non-zero .Add() — which for some metrics
	// (e.g. role-transitions on a single-primary fleet) may be never.
	//
	// MUST run AFTER otel Setup() installs the real MeterProvider (the
	// package-init instruments delegate to it; pre-warming a no-op is
	// useless) and BEFORE StartScheduler spawns the sync goroutine
	// (preserves the "all observability set up before background work
	// starts" startup ordering established by the Init* calls above).
	//
	// Total baseline series introduced: 4 per-type × 13 types + 1 direction × 2 = 54.
	pdbotel.PrewarmCounters(ctx)

	// Start scheduler on all instances.
	// The scheduler gates sync on live IsPrimary() checks per tick.
	go syncWorker.StartScheduler(ctx, cfg.SyncInterval)

	// Create GraphQL resolver with ent client and raw DB for sync_status queries.
	resolver := graph.NewResolver(entClient, db)

	// Create GraphQL handler with complexity/depth limits.
	gqlHandler := pdbgql.NewHandler(resolver)

	// Set up HTTP server.
	mux := http.NewServeMux()

	// POST /sync: on-demand sync trigger.
	// Write forwarding: replicas replay to primary via fly-replay header on Fly.io.
	syncHandler := newSyncHandler(ctx, SyncHandlerInput{
		IsPrimaryFn: isPrimaryFn,
		SyncToken:   cfg.SyncToken,
		DefaultMode: cfg.SyncMode,
		// Route on-demand syncs through the same demotion monitor the
		// scheduler gets via internal/sync's runSyncCycle, so a node
		// demoted mid-cycle aborts instead of burning upstream quota.
		SyncFn: func(syncCtx context.Context, mode config.SyncMode) {
			runSyncWithDemotionMonitor(syncCtx, mode, monitoredSyncInput{
				IsPrimary: isPrimaryFn,
				Logger:    logger,
				Sync:      syncWorker.SyncWithRetry,
			})
		},
		SyncRunning: syncWorker.Running,
	})
	mux.HandleFunc("POST /sync", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		syncHandler(w, r)
	})

	// GET /healthz: liveness probe (always 200, not gated by readiness).
	mux.HandleFunc("GET /healthz", health.LivenessHandler())

	// GET /readyz: readiness probe (checks DB connectivity and sync freshness).
	// Detailed error strings flow to logger via slog.LogAttrs; the wire
	// body carries only the generic {"status":"ok"|"unhealthy"} shape.
	mux.HandleFunc("GET /readyz", health.ReadinessHandler(health.ReadinessInput{
		DB:             db,
		StaleThreshold: cfg.SyncStaleThreshold,
		Logger:         logger,
	}))

	// GET /graphql: serve GraphiQL playground.
	// POST /graphql: handle GraphQL queries.
	// POST body limited to maxRequestBodySize.
	playgroundHandler := pdbgql.PlaygroundHandler("/graphql")
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			playgroundHandler.ServeHTTP(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		gqlHandler.ServeHTTP(w, r)
	})

	// Mount entrest-generated REST API at /rest/v1/.
	// Read-only (OperationRead + OperationList) configured via entrest annotations.
	restSrv, err := rest.NewServer(entClient, &rest.ServerConfig{
		BasePath: "/rest/v1",
	})
	if err != nil {
		logger.Error("failed to create REST server", slog.Any("error", err))
		os.Exit(1)
	}
	// Rewrite the served OpenAPI spec's error responses to describe the RFC
	// 9457 application/problem+json bodies middleware.RESTError actually
	// emits. The entrest-generated spec documents its native ErrorResponse
	// shape ({error,type,code,timestamp} as application/json), which this
	// server never puts on the wire — clients generated from the unpatched
	// spec would fail to deserialize every error. rest.Server.Spec serves
	// the package-level rest.OpenAPI bytes, so patching the var once at
	// startup fixes /rest/v1/openapi.json for the process lifetime.
	patchedSpec, err := patchOpenAPIErrorResponses(rest.OpenAPI)
	if err != nil {
		logger.Error("failed to patch OpenAPI error responses", slog.Any("error", err))
		os.Exit(1)
	}
	rest.OpenAPI = patchedSpec
	// CORS is applied once by the outer middleware chain (buildHandlerChain);
	// an inner wrap here double-appended Vary: Origin on every REST response.
	mux.Handle("/rest/v1/", middleware.RESTError(middleware.RESTFieldRedact(restSrv.Handler())))
	logger.Info("REST API mounted", slog.String("prefix", "/rest/v1/"))

	// Mount PeeringDB compatibility API at /api/.
	// Readiness gating applies automatically (not in bypass list).
	compatHandler := pdbcompat.NewHandler(entClient, cfg.ResponseMemoryLimit)
	compatHandler.Register(mux)
	logger.Info("PeeringDB compat API mounted", slog.String("prefix", "/api/"))

	// Mount web UI at /ui/ and /static/ prefixes.
	// authMode is captured here (not re-read by the handler) so /ui/about
	// reflects the process-start configuration, matching the "diagnostic
	// snapshot" semantics of the rest of the page.
	authMode := "anonymous"
	if cfg.PeeringDBAPIKey != "" {
		authMode = "authenticated"
	}
	webHandler := web.NewHandler(web.NewHandlerInput{
		Client:     entClient,
		DB:         db,
		AuthMode:   authMode,
		PublicTier: cfg.PublicTier,
		Version:    buildinfo.Version(),
		Region:     strings.TrimSpace(os.Getenv("FLY_REGION")),
	})
	webHandler.Register(mux)
	logger.Info("Web UI mounted", slog.String("prefix", "/ui/"))

	// Create OTel interceptor for ConnectRPC services.
	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithoutServerPeerAttributes(),
		otelconnect.WithoutTraceEvents(), // Suppress per-message events (critical for streaming RPCs).
	)
	if err != nil {
		logger.Error("failed to create otel interceptor", slog.Any("error", err))
		os.Exit(1)
	}
	handlerOpts := connectHandlerOpts(otelInterceptor)

	// Service names for reflection and health checking.
	serviceNames := []string{
		peeringdbv1connect.CampusServiceName,
		peeringdbv1connect.CarrierServiceName,
		peeringdbv1connect.CarrierFacilityServiceName,
		peeringdbv1connect.FacilityServiceName,
		peeringdbv1connect.InternetExchangeServiceName,
		peeringdbv1connect.IxFacilityServiceName,
		peeringdbv1connect.IxLanServiceName,
		peeringdbv1connect.IxPrefixServiceName,
		peeringdbv1connect.NetworkServiceName,
		peeringdbv1connect.NetworkFacilityServiceName,
		peeringdbv1connect.NetworkIxLanServiceName,
		peeringdbv1connect.OrganizationServiceName,
		peeringdbv1connect.PocServiceName,
	}

	// Register all 13 ConnectRPC services on the mux.
	registerService := func(path string, handler http.Handler) {
		mux.Handle(path, handler)
	}
	registerService(peeringdbv1connect.NewCampusServiceHandler(&grpcserver.CampusService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewCarrierServiceHandler(&grpcserver.CarrierService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewCarrierFacilityServiceHandler(&grpcserver.CarrierFacilityService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewFacilityServiceHandler(&grpcserver.FacilityService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewInternetExchangeServiceHandler(&grpcserver.InternetExchangeService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewIxFacilityServiceHandler(&grpcserver.IxFacilityService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewIxLanServiceHandler(&grpcserver.IxLanService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewIxPrefixServiceHandler(&grpcserver.IxPrefixService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewNetworkServiceHandler(&grpcserver.NetworkService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewNetworkFacilityServiceHandler(&grpcserver.NetworkFacilityService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewNetworkIxLanServiceHandler(&grpcserver.NetworkIxLanService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewOrganizationServiceHandler(&grpcserver.OrganizationService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	registerService(peeringdbv1connect.NewPocServiceHandler(&grpcserver.PocService{Client: entClient, StreamTimeout: cfg.StreamTimeout}, handlerOpts))
	logger.Info("ConnectRPC services mounted", slog.Int("count", len(serviceNames)))

	// gRPC server reflection for grpcurl/grpcui discovery.
	reflector := grpcreflect.NewStaticReflector(serviceNames...)
	mux.Handle(grpcreflect.NewHandlerV1(reflector, handlerOpts))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector, handlerOpts))
	logger.Info("gRPC reflection enabled")

	// gRPC health check tied to sync readiness. syncHealthChecker
	// evaluates the worker's live sync state on every Check RPC — the
	// same source /readyz's middleware gate reads — so health flips to
	// SERVING the instant the first sync lands (primary) or replicated
	// sync history is observed (replica heartbeat), with no polling
	// goroutine, and tracks any future state transition symmetrically.
	// This replaced a StaticChecker fed by a one-shot 1s ticker, which
	// burned a goroutine until first sync and could never return to
	// NOT_SERVING once flipped.
	mux.Handle(grpchealth.NewHandler(newSyncHealthChecker(syncWorker, serviceNames), handlerOpts))
	logger.Info("gRPC health check enabled")

	// GET /: content negotiation for terminal, browser, and API clients.
	// Terminal clients (curl, wget, HTTPie) receive help text.
	// Browsers (Accept: text/html) redirect to /ui/.
	// API clients (Accept: application/json) get JSON discovery. The version
	// comes from internal/buildinfo (injected via -ldflags from `git describe`
	// in Dockerfile.prod — Go's debug.ReadBuildInfo records only the commit,
	// never the tag, so it must be injected). Built once per process.
	discoveryJSON := discoveryBody(buildinfo.Version())
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		mode := termrender.Detect(termrender.DetectInput{
			Query:     r.URL.Query(),
			Accept:    r.Header.Get("Accept"),
			UserAgent: r.Header.Get("User-Agent"),
		})
		noColor := termrender.HasNoColor(termrender.DetectInput{Query: r.URL.Query()})

		switch mode { //nolint:exhaustive // default case handles remaining modes
		case termrender.ModeRich, termrender.ModePlain:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			// Add, not Set: gzhttp already added Vary: Accept-Encoding.
			w.Header().Add("Vary", "User-Agent, Accept")
			renderer := termrender.NewRenderer(mode, noColor)
			var freshness time.Time
			status, err := pdbsync.GetLastStatus(r.Context(), db)
			if err == nil && status != nil && status.Status == "success" {
				freshness = status.LastSyncAt
			}
			if err := renderer.RenderHelp(w, freshness); err != nil {
				slog.Error("render terminal help", slog.Any("error", err))
			}
			return

		case termrender.ModeJSON:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Add("Vary", "User-Agent, Accept")
			fmt.Fprint(w, discoveryJSON)
			return

		default:
			// The branch taken depends on Accept and (via Detect above)
			// User-Agent, so caches must key on both here too.
			w.Header().Add("Vary", "User-Agent, Accept")
			accept := r.Header.Get("Accept")
			if strings.Contains(accept, "text/html") {
				http.Redirect(w, r, "/ui/", http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, discoveryJSON)
		}
	})

	// Build middleware stack (outermost first):
	// Recovery -> MaxBytesBody -> CORS -> OTel HTTP -> Logging -> PrivacyTier -> Readiness -> SecurityHeaders -> CSP -> Caching -> Gzip -> RouteTag -> mux
	//
	// MaxBytesBody caps every non-gRPC request body at maxRequestBodySize (1 MB).
	// Per-route http.MaxBytesReader wraps at /sync and /graphql stay
	// as belt-and-suspenders — innermost wins, so they remain effective and the
	// redundancy is intentional. ConnectRPC/gRPC paths bypass via the middleware's
	// hardcoded skip list; streaming RPCs would break if the body were capped.
	//
	// SecurityHeaders sits between Readiness and CSP so HSTS/XCTO fire
	// on every response — including the Readiness 503 syncing page — and XFO
	// stays scoped to browser paths. The wrap order is regression-locked by
	// TestMiddlewareChain_Order in middleware_chain_test.go.
	handler := buildMiddlewareChain(mux, chainConfig{
		Logger:      logger,
		CORSOrigins: cfg.CORSOrigins,
		CSPInput: middleware.CSPInput{
			// script-src is 'self' only: all UI behaviour lives in
			// /static/{theme-init,ui,map-init}.js and Tailwind/Leaflet are
			// self-hosted, so no inline scripts or CDN script hosts remain.
			// style-src keeps 'unsafe-inline' (layout <style> blocks,
			// Leaflet's inline style attributes) plus jsdelivr for the
			// flag-icons stylesheet; img-src includes jsdelivr because
			// that stylesheet resolves its flag SVGs relative to itself.
			UIPolicy:      "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data: https://*.basemaps.cartocdn.com https://cdn.jsdelivr.net; connect-src 'self'; font-src 'self' https://cdn.jsdelivr.net",
			GraphQLPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data:; connect-src 'self'",
			EnforcingMode: cfg.CSPEnforce,
		},
		CachingState: cachingState,
		SyncWorker:   syncWorker,
		MaxBodyBytes: maxRequestBodySize,
		HSTSMaxAge:   365 * 24 * time.Hour,
		DefaultTier:  cfg.PublicTier,
	})

	// Enable HTTP/1.1 + h2c (HTTP/2 cleartext) for gRPC support.
	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	server := buildServer(cfg.ListenAddr, handler, &protocols)

	// Graceful shutdown on SIGINT/SIGTERM, or on server failure. The serve
	// goroutine reports failure via serveErr instead of calling os.Exit so
	// main's normal return path — and therefore the deferred OTel flush and
	// DB close — runs either way (see awaitShutdown).
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	serveErr := make(chan error, 1)

	go func() {
		logger.Info("starting server",
			slog.String("addr", cfg.ListenAddr),
			slog.Bool("is_primary", isPrimary),
			slog.Bool("h2c", true),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	exitCode = awaitShutdown(awaitShutdownInput{
		SigChan:  sigChan,
		ServeErr: serveErr,
		Logger:   logger,
	})
	cancel() // Stop scheduler and background syncs.

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.DrainTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", slog.Any("error", err))
	}
}
