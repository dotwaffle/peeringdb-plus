// Package main is the entry point for the peeringdb-plus application.
// It wires together config, database, OTel, PeeringDB client, and sync worker,
// then serves HTTP endpoints for health checks and on-demand sync triggers.
package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
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
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/dotwaffle/peeringdb-plus/ent/migrate"
	"github.com/dotwaffle/peeringdb-plus/ent/rest"
	_ "github.com/dotwaffle/peeringdb-plus/ent/runtime" // register schema hooks (OTel mutation tracing)
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
	"github.com/dotwaffle/peeringdb-plus/graph"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/database"
	pdbgql "github.com/dotwaffle/peeringdb-plus/internal/graphql"
	"github.com/dotwaffle/peeringdb-plus/internal/grpcserver"
	"github.com/dotwaffle/peeringdb-plus/internal/health"
	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
	"github.com/dotwaffle/peeringdb-plus/internal/litefs"
	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/privfield"
	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/web"
	webtemplates "github.com/dotwaffle/peeringdb-plus/internal/web/templates"
	"github.com/dotwaffle/peeringdb-plus/internal/web/termrender"
)

// maxRequestBodySize is the maximum allowed request body for POST endpoints (1 MB).
// GraphQL queries rarely exceed 10 KB; 1 MB is generous per SRVR-04.
const maxRequestBodySize = 1 << 20

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

func main() {
	// Load config from environment per D-33, CFG-1.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize OpenTelemetry per D-06, D-07.
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
	defer otelOut.Shutdown(ctx) //nolint:errcheck // best-effort flush at exit

	// Set up dual slog logger (stdout + OTel pipeline) per D-03, OBS-1.
	logger := pdbotel.NewDualLogger(os.Stdout, otelOut.LogProvider)
	slog.SetDefault(logger)

	// Initialize custom sync metrics per D-05.
	if err := pdbotel.InitMetrics(); err != nil {
		logger.Error("failed to init metrics", slog.Any("error", err))
		os.Exit(1)
	}

	// Phase 66 (OBS-05): peak heap / RSS observable gauges for SEED-001 watch.
	if err := pdbotel.InitMemoryGauges(); err != nil {
		logger.Error("failed to init memory gauges", slog.Any("error", err))
		os.Exit(1)
	}

	// Open database per D-28, D-34.
	entClient, db, err := database.Open(cfg.DBPath)
	if err != nil {
		logger.Error("failed to open database", slog.Any("error", err))
		os.Exit(1)
	}
	defer entClient.Close()

	// Detect primary status via LiteFS lease file with env fallback per D-24.
	isPrimary := litefs.IsPrimaryWithFallback(litefs.PrimaryFile, "PDBPLUS_IS_PRIMARY")

	// Live primary detection function for dynamic role changes without restart.
	isPrimaryFn := func() bool {
		return litefs.IsPrimaryWithFallback(litefs.PrimaryFile, "PDBPLUS_IS_PRIMARY")
	}

	// Auto-migrate schema on primary per D-43.
	//
	// WithDropColumn(true): enables ALTER TABLE DROP COLUMN for v1.15 Phase 63
	// schema cleanup (ixpfx.notes, organization.fac_count, organization.net_count)
	// and any future hygiene drops. Per D-04. ent defaults to additive-only
	// migrations for safety; this flag opts in to destructive DDL.
	//
	// WithDropIndex(true): symmetric handling of stale indexes per the ent
	// docs recommendation. None of the Phase 63 target columns are indexed,
	// but enabling both together is idiomatic.
	if isPrimary {
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
	if isPrimary {
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

	// Initialize sync freshness gauge per D-09.
	if err := pdbotel.InitFreshnessGauge(func(ctx context.Context) (time.Time, bool) {
		status, err := pdbsync.GetLastStatus(ctx, db)
		if err != nil || status == nil || status.Status != "success" {
			return time.Time{}, false
		}
		return status.LastSyncAt, true
	}); err != nil {
		logger.Error("failed to init freshness gauge", slog.Any("error", err))
		os.Exit(1)
	}

	// Cached object counts for metrics gauge (PERF-02).
	// Updated by sync worker after each successful sync.
	var objectCountCache atomic.Pointer[map[string]int64]
	initialCounts := make(map[string]int64)
	objectCountCache.Store(&initialCounts)

	// Initialize per-type object count gauges for business metrics dashboard.
	// Reads from atomic cache instead of live COUNT queries per PERF-02.
	if err := pdbotel.InitObjectCountGauges(func() map[string]int64 {
		return *objectCountCache.Load()
	}); err != nil {
		logger.Error("failed to init object count gauges", slog.Any("error", err))
		os.Exit(1)
	}

	// Create PeeringDB client per D-04, D-09.
	var clientOpts []peeringdb.ClientOption
	if cfg.PeeringDBAPIKey != "" {
		clientOpts = append(clientOpts, peeringdb.WithAPIKey(cfg.PeeringDBAPIKey))
		logger.Info("PeeringDB API key configured", slog.String("api_key", "[set]"))
	} else {
		logger.Info("PeeringDB API key not configured, using unauthenticated access",
			slog.String("api_key", "[not set]"))
	}

	// SEC-04: make the disabled-sync-auth state loud at boot so operators see it
	// in deploy logs rather than discovering it via a curl probe.
	if cfg.SyncToken == "" {
		logger.Warn("sync endpoint is unauthenticated — set PDBPLUS_SYNC_TOKEN to require authentication",
			slog.String("endpoint", "/sync"),
			slog.String("action", "set PDBPLUS_SYNC_TOKEN"))
	}

	// Phase 61 (SYNC-04, OBS-01): classify sync mode + public tier at startup.
	// Emitted after config parse / OTel init / dual-logger install, before any
	// handler registration, so a failure to start the server does not swallow
	// the classification record.
	logStartupClassification(logger, cfg)

	pdbClient := peeringdb.NewClient(cfg.PeeringDBBaseURL, logger, clientOpts...)

	// PERF-07: Caching middleware holds the current ETag behind an atomic
	// pointer. Constructed once here so the OnSyncComplete callback below
	// can capture it. The initial ETag is seeded from any existing
	// sync_status row so warm restarts serve cacheable GETs immediately;
	// cold starts leave the pointer nil (Middleware skips caching headers
	// until the first sync completes, matching pre-PERF-07 behavior).
	//
	// /ui/about is opted out of caching because it renders wall-clock-
	// relative text ("N minutes ago") that would freeze at cache-creation
	// time under the sync-time-keyed ETag. See internal/web/about.go and
	// internal/web/templates/about.templ.
	cachingState := middleware.NewCachingState(cfg.SyncInterval, "/ui/about")
	if t, err := pdbsync.GetLastSuccessfulSyncTime(ctx, db); err == nil && !t.IsZero() {
		cachingState.UpdateETag(t)
	}

	// Create sync worker.
	syncWorker := pdbsync.NewWorker(pdbClient, entClient, db, pdbsync.WorkerConfig{
		IsPrimary: isPrimaryFn,
		SyncMode:  cfg.SyncMode,
		OnSyncComplete: func(counts map[string]int, syncTime time.Time) {
			m := make(map[string]int64, len(counts))
			for k, v := range counts {
				m[k] = int64(v)
			}
			objectCountCache.Store(&m)
			// PERF-07: swap the cached ETag using the exact completion
			// timestamp the worker persisted to sync_status. One SHA-256
			// per sync, zero per request.
			cachingState.UpdateETag(syncTime)
		},
		SyncMemoryLimit: cfg.SyncMemoryLimit,
		HeapWarnBytes:   cfg.HeapWarnBytes,
		RSSWarnBytes:    cfg.RSSWarnBytes,
	}, logger)

	// Start scheduler on all instances per D-22, D-29.
	// The scheduler gates sync on live IsPrimary() checks per tick.
	go syncWorker.StartScheduler(ctx, cfg.SyncInterval)

	// Create GraphQL resolver with ent client and raw DB for sync_status queries.
	resolver := graph.NewResolver(entClient, db)

	// Create GraphQL handler with complexity/depth limits per D-04.
	gqlHandler := pdbgql.NewHandler(resolver)

	// Set up HTTP server.
	mux := http.NewServeMux()

	// POST /sync: on-demand sync trigger per D-23.
	// Write forwarding: replicas replay to primary via fly-replay header on Fly.io.
	syncHandler := newSyncHandler(ctx, SyncHandlerInput{
		IsPrimaryFn: isPrimaryFn,
		SyncToken:   cfg.SyncToken,
		DefaultMode: cfg.SyncMode,
		SyncFn: func(syncCtx context.Context, mode config.SyncMode) {
			syncWorker.SyncWithRetry(syncCtx, mode) //nolint:errcheck,gosec // fire-and-forget
		},
	})
	mux.HandleFunc("POST /sync", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		syncHandler(w, r)
	})

	// GET /healthz: liveness probe (always 200, not gated by readiness).
	mux.HandleFunc("GET /healthz", health.LivenessHandler())

	// GET /readyz: readiness probe (checks DB connectivity and sync freshness).
	// SEC-08: detailed error strings flow to logger via slog.LogAttrs; the wire
	// body carries only the generic {"status":"ok"|"unhealthy"} shape.
	mux.HandleFunc("GET /readyz", health.ReadinessHandler(health.ReadinessInput{
		DB:             db,
		StaleThreshold: cfg.SyncStaleThreshold,
		Logger:         logger,
	}))

	// GET /graphql: serve GraphiQL playground per D-17, D-18, D-21.
	// POST /graphql: handle GraphQL queries per D-18.
	// POST body limited to maxRequestBodySize per SRVR-04.
	playgroundHandler := pdbgql.PlaygroundHandler("/graphql")
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			playgroundHandler.ServeHTTP(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		gqlHandler.ServeHTTP(w, r)
	})

	// Mount entrest-generated REST API at /rest/v1/ per D-01, D-04.
	// Read-only (OperationRead + OperationList) configured via entrest annotations.
	restSrv, err := rest.NewServer(entClient, &rest.ServerConfig{
		BasePath: "/rest/v1",
	})
	if err != nil {
		logger.Error("failed to create REST server", slog.Any("error", err))
		os.Exit(1)
	}
	restCORS := middleware.CORS(middleware.CORSInput{AllowedOrigins: cfg.CORSOrigins})
	mux.Handle("/rest/v1/", restCORS(restErrorMiddleware(restFieldRedactMiddleware(restSrv.Handler()))))
	logger.Info("REST API mounted", slog.String("prefix", "/rest/v1/"))

	// Mount PeeringDB compatibility API at /api/ per D-27, D-28.
	// Readiness gating applies automatically (not in bypass list) per D-29.
	compatHandler := pdbcompat.NewHandler(entClient, cfg.ResponseMemoryLimit)
	compatHandler.Register(mux)
	logger.Info("PeeringDB compat API mounted", slog.String("prefix", "/api/"))

	// Mount web UI at /ui/ and /static/ prefixes.
	// authMode is captured here (not re-read by the handler) so /ui/about
	// reflects the process-start configuration, matching the "diagnostic
	// snapshot" semantics of the rest of the page (Phase 61 OBS-02).
	authMode := "anonymous"
	if cfg.PeeringDBAPIKey != "" {
		authMode = "authenticated"
	}
	webHandler := web.NewHandler(web.NewHandlerInput{
		Client:     entClient,
		DB:         db,
		AuthMode:   authMode,
		PublicTier: cfg.PublicTier,
	})
	webHandler.Register(mux)
	logger.Info("Web UI mounted", slog.String("prefix", "/ui/"))

	// Create OTel interceptor for ConnectRPC services per OBS-01.
	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithoutServerPeerAttributes(),
		otelconnect.WithoutTraceEvents(), // Suppress per-message events (critical for streaming RPCs per STRM-06).
	)
	if err != nil {
		logger.Error("failed to create otel interceptor", slog.Any("error", err))
		os.Exit(1)
	}
	handlerOpts := connect.WithInterceptors(otelInterceptor)

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

	// Register all 13 ConnectRPC services on the mux per API-04.
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

	// gRPC server reflection for grpcurl/grpcui discovery per OBS-03.
	reflector := grpcreflect.NewStaticReflector(serviceNames...)
	mux.Handle(grpcreflect.NewHandlerV1(reflector, handlerOpts))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector, handlerOpts))
	logger.Info("gRPC reflection enabled")

	// gRPC health check tied to sync readiness per OBS-04.
	healthChecker := grpchealth.NewStaticChecker(serviceNames...)
	mux.Handle(grpchealth.NewHandler(healthChecker, handlerOpts))
	logger.Info("gRPC health check enabled")

	// Update gRPC health status when first sync completes.
	// StaticChecker defaults to SERVING; set to NOT_SERVING until sync done.
	if !syncWorker.HasCompletedSync() {
		healthChecker.SetStatus("", grpchealth.StatusNotServing)
		for _, name := range serviceNames {
			healthChecker.SetStatus(name, grpchealth.StatusNotServing)
		}
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if syncWorker.HasCompletedSync() {
						healthChecker.SetStatus("", grpchealth.StatusServing)
						for _, name := range serviceNames {
							healthChecker.SetStatus(name, grpchealth.StatusServing)
						}
						logger.Info("gRPC health status set to SERVING")
						return
					}
				}
			}
		}()
	}

	// GET /: content negotiation for terminal, browser, and API clients (NAV-04).
	// Terminal clients (curl, wget, HTTPie) receive help text.
	// Browsers (Accept: text/html) redirect to /ui/.
	// API clients (Accept: application/json) get JSON discovery.
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
			w.Header().Set("Vary", "User-Agent, Accept")
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
			w.Header().Set("Vary", "User-Agent, Accept")
			fmt.Fprint(w, `{"name":"peeringdb-plus","version":"0.1.0","graphql":"/graphql","rest":"/rest/v1/","api":"/api/","connectrpc":"/peeringdb.v1.","ui":"/ui/","healthz":"/healthz","readyz":"/readyz"}`)
			return

		default:
			accept := r.Header.Get("Accept")
			if strings.Contains(accept, "text/html") {
				http.Redirect(w, r, "/ui/", http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"name":"peeringdb-plus","version":"0.1.0","graphql":"/graphql","rest":"/rest/v1/","api":"/api/","connectrpc":"/peeringdb.v1.","ui":"/ui/","healthz":"/healthz","readyz":"/readyz"}`)
		}
	})

	// Build middleware stack (outermost first):
	// Recovery -> MaxBytesBody -> CORS -> OTel HTTP -> Logging -> PrivacyTier -> Readiness -> SecurityHeaders -> CSP -> Caching -> Gzip -> mux
	//
	// MaxBytesBody caps every non-gRPC request body at maxRequestBodySize (1 MB)
	// per SEC-09. Per-route http.MaxBytesReader wraps at /sync and /graphql stay
	// as belt-and-suspenders — innermost wins, so they remain effective and the
	// redundancy is intentional. ConnectRPC/gRPC paths bypass via the middleware's
	// hardcoded skip list; streaming RPCs would break if the body were capped.
	//
	// SecurityHeaders (SEC-10) sits between Readiness and CSP so HSTS/XCTO fire
	// on every response — including the Readiness 503 syncing page — and XFO
	// stays scoped to browser paths. The wrap order is regression-locked by
	// TestMiddlewareChain_Order in middleware_chain_test.go.
	handler := buildMiddlewareChain(mux, chainConfig{
		Logger:      logger,
		CORSOrigins: cfg.CORSOrigins,
		CSPInput: middleware.CSPInput{
			UIPolicy:      "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; img-src 'self' data: https://*.basemaps.cartocdn.com; connect-src 'self'; font-src 'self' https://cdn.jsdelivr.net",
			GraphQLPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data:; connect-src 'self'",
			EnforcingMode: cfg.CSPEnforce,
		},
		CachingState: cachingState,
		SyncWorker:   syncWorker,
		MaxBodyBytes: maxRequestBodySize,
		HSTSMaxAge:   180 * 24 * time.Hour,
		DefaultTier:  cfg.PublicTier,
	})

	// Enable HTTP/1.1 + h2c (HTTP/2 cleartext) for gRPC support.
	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	server := buildServer(cfg.ListenAddr, handler, &protocols)

	// Graceful shutdown on SIGINT/SIGTERM.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("starting server",
			slog.String("addr", cfg.ListenAddr),
			slog.Bool("is_primary", isPrimary),
			slog.Bool("h2c", true),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	sig := <-sigChan
	logger.Info("shutting down", slog.String("signal", sig.String()))
	cancel() // Stop scheduler and background syncs.

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.DrainTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", slog.Any("error", err))
	}
}

// syncReadiness reports whether at least one sync has completed.
type syncReadiness interface {
	HasCompletedSync() bool
}

// restErrorMiddleware wraps entrest error responses in RFC 9457 Problem Details format.
// It intercepts non-2xx responses and rewrites the body as application/problem+json.
func restErrorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &restErrorWriter{ResponseWriter: w, r: r}
		next.ServeHTTP(rw, r)
	})
}

// restErrorWriter captures non-2xx status codes from entrest and converts them
// to RFC 9457 Problem Details responses.
type restErrorWriter struct {
	http.ResponseWriter
	r           *http.Request
	wroteHeader bool
}

// WriteHeader intercepts non-2xx status codes and writes RFC 9457 problem detail.
func (w *restErrorWriter) WriteHeader(code int) {
	if code >= 400 && !w.wroteHeader {
		w.wroteHeader = true
		httperr.WriteProblem(w.ResponseWriter, httperr.WriteProblemInput{
			Status:   code,
			Instance: w.r.URL.Path,
		})
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write passes through for 2xx responses or suppresses body for error responses
// (already written by WriteHeader).
func (w *restErrorWriter) Write(b []byte) (int, error) {
	if w.wroteHeader {
		return len(b), nil // discard entrest's error body
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter for middleware-aware interface detection.
func (w *restErrorWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// restListWrapperKey is the JSON key under which entrest's PagedResponse
// serialises the items slice. Confirmed at planning time by grepping:
//
//	$ grep -rn 'json:"content"' ent/rest/
//	ent/rest/list.go:153:    Content    []*T `json:"content"`      // Paged data.
//
// If a future entrest upgrade changes this tag, this constant MUST move
// in lock-step or restFieldRedactMiddleware silently stops redacting list
// responses — a privacy leak. The wave-2 E2E list sub-test catches the
// regression.
const restListWrapperKey = "content"

// restFieldRedactMiddleware removes the `ixf_ixp_member_list_url` key
// from /rest/v1/ix-lans* JSON responses when the caller's ctx tier
// does not admit the field (per internal/privfield.Redact).
//
// entrest has no native per-field conditional-omission hook (verified
// against lrstanley/entrest annotation reference, Phase 64 RESEARCH.md
// Finding #1). This middleware is the workaround: it buffers the
// response body on the ixlan paths, parses the JSON, walks the ixlan
// object(s), and re-emits with the field deleted when privfield.Redact
// says omit.
//
// Scope: only /rest/v1/ix-lans (prefix match). Detail responses are
// single objects; list responses wrap entries in {restListWrapperKey:[…]}.
// Non-ixlan REST paths and non-JSON bodies pass through unchanged.
//
// Ordering: this middleware MUST be wrapped INSIDE restErrorMiddleware
// so that problem+json error bodies pass through without being mis-parsed
// as data payloads.
//
// Phase 64 VIS-08 / VIS-09.
func restFieldRedactMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rest/v1/ix-lans") {
			next.ServeHTTP(w, r)
			return
		}
		rw := &restFieldRedactWriter{ResponseWriter: w, ctx: r.Context()}
		next.ServeHTTP(rw, r)
		rw.flush()
	})
}

// restFieldRedactWriter buffers an ixlan REST response so that the body
// can be parsed + rewritten before reaching the client. Implements
// http.Flusher and Unwrap() per CLAUDE.md §Middleware.
type restFieldRedactWriter struct {
	http.ResponseWriter
	ctx    context.Context
	status int
	buf    bytes.Buffer
}

// WriteHeader captures the status code — the real header write is
// deferred until flush() after body rewrite.
func (w *restFieldRedactWriter) WriteHeader(code int) {
	w.status = code
}

// Write buffers the body so we can rewrite the JSON before flushing.
func (w *restFieldRedactWriter) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

// Unwrap returns the underlying ResponseWriter for middleware-aware
// interface detection (matches restErrorWriter pattern).
func (w *restFieldRedactWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush is a no-op during buffering; real flushing happens in flush().
// Required for CLAUDE.md §Middleware Flusher contract — REST is
// non-streaming so this never fires mid-response in practice.
func (w *restFieldRedactWriter) Flush() {}

// flush writes the buffered body to the underlying ResponseWriter,
// rewriting ixlan JSON payloads to drop the URL key when the caller's
// tier does not admit it.
func (w *restFieldRedactWriter) flush() {
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	body := w.buf.Bytes()
	contentType := w.Header().Get("Content-Type")

	// Pass through non-JSON (e.g. application/problem+json error bodies
	// from restErrorMiddleware when wrapped inside-out, or empty 204s).
	if !strings.HasPrefix(contentType, "application/json") || len(body) == 0 {
		w.ResponseWriter.WriteHeader(status)
		_, _ = w.ResponseWriter.Write(body)
		return
	}

	rewritten, err := redactIxlanJSON(w.ctx, body)
	if err != nil {
		// Parse failed — pass through unchanged. A legitimate parse
		// error shouldn't happen on a 2xx entrest response; if it does,
		// corrupting the body would be worse than letting it through.
		// The E2E test in Plan 64-03 Task 5 will catch any real leak.
		w.ResponseWriter.Header().Del("Content-Length")
		w.ResponseWriter.WriteHeader(status)
		_, _ = w.ResponseWriter.Write(body)
		return
	}

	// Clear Content-Length — Go's http server will compute a fresh
	// length or use chunked encoding as appropriate.
	w.ResponseWriter.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
	_, _ = w.ResponseWriter.Write(rewritten)
}

// redactIxlanJSON parses body as JSON and applies Redact to any ixlan
// object (detail shape) or list of ixlan objects (under restListWrapperKey).
// Returns the re-encoded body.
func redactIxlanJSON(ctx context.Context, body []byte) ([]byte, error) {
	var top map[string]any
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}
	// List shape: {page, total_count, last_page, is_last_page, content:[…]}
	if wrapped, ok := top[restListWrapperKey].([]any); ok {
		for _, entry := range wrapped {
			obj, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			redactIxlanObject(ctx, obj)
		}
		return json.Marshal(top)
	}
	// Detail shape: single ixlan object at the top level.
	redactIxlanObject(ctx, top)
	return json.Marshal(top)
}

// redactIxlanObject drops the ixf_ixp_member_list_url key in-place when
// privfield.Redact says omit. The _visible companion is left alone
// (D-05: always emitted).
func redactIxlanObject(ctx context.Context, obj map[string]any) {
	visible, _ := obj["ixf_ixp_member_list_url_visible"].(string)
	url, _ := obj["ixf_ixp_member_list_url"].(string)
	_, omit := privfield.Redact(ctx, visible, url)
	if omit {
		delete(obj, "ixf_ixp_member_list_url")
	}
}

// buildServer constructs the production http.Server with all timeouts
// deliberately set. WriteTimeout is explicitly 0 because StreamEntities in
// internal/grpcserver/generic.go already enforces cfg.StreamTimeout per
// stream via context.WithTimeout; a server-wide WriteTimeout would race
// with it and silently truncate streams (see PITFALLS.md §CP-2).
//
// ReadHeaderTimeout=10s mitigates slowloris header-stall attacks;
// ReadTimeout=30s mitigates slowloris body-stall attacks (SEC-05);
// IdleTimeout=120s caps keep-alive idle connections.
// Go 1.26 net/http godoc: "A zero or negative value means there will be
// no timeout" — WriteTimeout:0 is safe for long-lived h2c streams.
//
// TestServer_NoWriteTimeoutOnStreamingPaths regression-locks every field;
// any drift fails CI.
func buildServer(addr string, handler http.Handler, protocols *http.Protocols) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout intentionally 0 — see buildServer doc comment.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
}

// chainConfig bundles the inputs for buildMiddlewareChain. It is a plain
// data struct rather than a fluent builder — the chain is locked and
// every field is required at startup.
type chainConfig struct {
	Logger       *slog.Logger
	CORSOrigins  string // comma-separated list of allowed CORS origins
	CSPInput     middleware.CSPInput
	CachingState *middleware.CachingState
	SyncWorker   syncReadiness
	MaxBodyBytes int64
	HSTSMaxAge   time.Duration
	// DefaultTier is the resolved PDBPLUS_PUBLIC_TIER value stamped on
	// every inbound request context by middleware.PrivacyTier. Consumed
	// downstream by the ent privacy policies on visibility-bearing
	// entities (see ent/schema/poc.go). Zero value is TierPublic — the
	// safest default if unset.
	DefaultTier privctx.Tier
}

// buildMiddlewareChain wraps the innermost handler in the full production
// middleware stack, returning the outermost handler. The chain order is:
//
//	Recovery -> MaxBytesBody -> CORS -> OTel HTTP -> Logging ->
//	PrivacyTier -> Readiness -> SecurityHeaders -> CSP -> Caching ->
//	Gzip -> innermost
//
// The code below wraps innermost-first (Gzip is wrapped first so it sits
// closest to the handler; Recovery is wrapped last so it sits outermost).
// This ordering is regression-locked by TestMiddlewareChain_Order, which
// source-scans this function body and asserts the literal wrap order.
//
// SecurityHeaders (SEC-10) sits between Readiness and CSP so HSTS/XCTO fire
// on every response, including the Readiness 503 syncing page, and XFO
// stays scoped to browser paths via middleware.isBrowserPath. The 180-day
// HSTSMaxAge is passed through chainConfig so v1.14 can flip the default
// without touching this helper.
//
// PrivacyTier (phase 59, D-05) sits between Logging and Readiness in
// request flow so every request ctx — including the Readiness 503 path —
// carries the resolved PDBPLUS_PUBLIC_TIER before any handler or ent
// query reads it. Placing it inside Logging (rather than outside) keeps
// Recovery/Logging free of tier coupling while still stamping the ctx
// before any downstream observation of the request.
func buildMiddlewareChain(inner http.Handler, cc chainConfig) http.Handler {
	h := middleware.Compression()(inner)
	h = cc.CachingState.Middleware()(h)
	h = middleware.CSP(cc.CSPInput)(h)
	h = middleware.SecurityHeaders(middleware.SecurityHeadersInput{
		HSTSMaxAge:            cc.HSTSMaxAge,
		HSTSIncludeSubDomains: true,
		FrameOptions:          "DENY",
		ContentTypeOptions:    true,
	})(h)
	h = readinessMiddleware(cc.SyncWorker, h)
	h = middleware.PrivacyTier(middleware.PrivacyTierInput{DefaultTier: cc.DefaultTier})(h)
	h = middleware.Logging(cc.Logger)(h)
	h = otelhttp.NewMiddleware("peeringdb-plus")(h)
	h = middleware.CORS(middleware.CORSInput{AllowedOrigins: cc.CORSOrigins})(h)
	h = middleware.MaxBytesBody(middleware.MaxBytesBodyInput{MaxBytes: cc.MaxBodyBytes})(h)
	h = middleware.Recovery(cc.Logger)(h)
	return h
}

// logStartupClassification emits the v1.14 Phase 61 sync-mode classification
// lines (SYNC-04, OBS-01). Called once from main() after config parse and
// after slog.SetDefault, before the HTTP listener starts.
//
//   - slog.Info "sync mode" (always): auth = "authenticated" | "anonymous",
//     public_tier = "public" | "users". Exactly those two attrs, in that order.
//   - slog.Warn "public tier override active" (only when tier == TierUsers):
//     public_tier = "users", env = "PDBPLUS_PUBLIC_TIER". Exactly those two attrs.
//
// Attribute shapes are a wire contract — Grafana Loki filters and external
// parsers key off the literal strings. Do not rename without a coordinated
// dashboard update.
//
// Per 61-CONTEXT.md D-10/D-11 the tests in startup_logging_test.go capture
// slog records and assert the attrs directly; changing attr keys or values
// is a breaking change to the operator contract.
func logStartupClassification(logger *slog.Logger, cfg *config.Config) {
	auth := "anonymous"
	if cfg.PeeringDBAPIKey != "" {
		auth = "authenticated"
	}
	publicTier := "public"
	if cfg.PublicTier == privctx.TierUsers {
		publicTier = "users"
	}
	logger.Info("sync mode",
		slog.String("auth", auth),
		slog.String("public_tier", publicTier),
	)
	if cfg.PublicTier == privctx.TierUsers {
		logger.Warn("public tier override active",
			slog.String("public_tier", "users"),
			slog.String("env", "PDBPLUS_PUBLIC_TIER"),
		)
	}
}

// readinessMiddleware returns 503 for all routes except infrastructure paths
// until the first sync has completed per D-30.
// Browser requests receive a styled HTML syncing page instead of JSON.
func readinessMiddleware(sr syncReadiness, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Infrastructure, static, and gRPC health paths bypass readiness.
		// Static assets must be served for the syncing page to render correctly.
		// gRPC health check manages its own NOT_SERVING/SERVING state.
		if r.URL.Path == "/sync" || r.URL.Path == "/healthz" ||
			r.URL.Path == "/readyz" || r.URL.Path == "/" ||
			r.URL.Path == "/favicon.ico" ||
			strings.HasPrefix(r.URL.Path, "/static/") ||
			strings.HasPrefix(r.URL.Path, "/grpc.health.v1.Health/") {
			next.ServeHTTP(w, r)
			return
		}
		if !sr.HasCompletedSync() {
			accept := r.Header.Get("Accept")
			if strings.Contains(accept, "text/html") {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				webtemplates.SyncingPage().Render(r.Context(), w) //nolint:errcheck,gosec // best-effort render
				return
			}

			// Terminal clients (curl, wget, HTTPie) get styled text output.
			mode := termrender.Detect(termrender.DetectInput{
				UserAgent: r.UserAgent(),
				Accept:    accept,
				Query:     r.URL.Query(),
			})
			if mode == termrender.ModeRich || mode == termrender.ModePlain {
				noColor := termrender.HasNoColor(termrender.DetectInput{Query: r.URL.Query()})
				renderer := termrender.NewRenderer(mode, noColor)
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				renderer.RenderError(w, http.StatusServiceUnavailable, "Service Unavailable", "PeeringDB data sync has not yet completed.\nPlease try again in a few moments.") //nolint:errcheck,gosec // best-effort render
				return
			}

			// API/JSON fallback for non-terminal, non-browser clients.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":"sync not yet completed"}`)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SyncHandlerInput holds dependencies for the sync handler.
type SyncHandlerInput struct {
	IsPrimaryFn func() bool
	SyncToken   string
	DefaultMode config.SyncMode
	SyncFn      func(ctx context.Context, mode config.SyncMode)
}

// newSyncHandler creates the POST /sync handler with fly-replay write forwarding.
// On Fly.io replicas (FLY_REGION set, not primary), it returns a fly-replay header
// routing to PRIMARY_REGION. In local dev (FLY_REGION empty, not primary), it
// returns 503 since there is no Fly proxy to replay the request.
func newSyncHandler(appCtx context.Context, in SyncHandlerInput) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !in.IsPrimaryFn() {
			// On Fly.io, replay to primary region.
			if flyRegion := os.Getenv("FLY_REGION"); flyRegion != "" {
				primaryRegion := os.Getenv("PRIMARY_REGION")
				w.Header().Set("fly-replay", "region="+primaryRegion)
				w.WriteHeader(http.StatusTemporaryRedirect)
				return
			}
			// Not on Fly.io (local dev) -- non-primary cannot handle sync.
			http.Error(w, "not primary", http.StatusServiceUnavailable)
			return
		}
		got := r.Header.Get("X-Sync-Token")
		if in.SyncToken == "" ||
			len(got) != len(in.SyncToken) ||
			subtle.ConstantTimeCompare([]byte(got), []byte(in.SyncToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mode := in.DefaultMode
		if qm := r.URL.Query().Get("mode"); qm != "" {
			switch config.SyncMode(qm) {
			case config.SyncModeFull, config.SyncModeIncremental:
				mode = config.SyncMode(qm)
			default:
				http.Error(w, fmt.Sprintf("invalid mode %q: must be full or incremental", qm), http.StatusBadRequest)
				return
			}
		}
		// Use application root ctx, NOT r.Context() -- request context
		// is cancelled when the response is sent, which would kill the sync.
		go in.SyncFn(appCtx, mode)
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"status":"accepted"}`)
	}
}
