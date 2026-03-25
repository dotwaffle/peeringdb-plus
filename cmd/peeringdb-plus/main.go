// Package main is the entry point for the peeringdb-plus application.
// It wires together config, database, OTel, PeeringDB client, and sync worker,
// then serves HTTP endpoints for health checks and on-demand sync triggers.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"connectrpc.com/grpcreflect"
	"connectrpc.com/otelconnect"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	_ "github.com/dotwaffle/peeringdb-plus/ent/runtime" // register schema hooks (OTel mutation tracing)
	"github.com/dotwaffle/peeringdb-plus/ent/rest"
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
	"github.com/dotwaffle/peeringdb-plus/graph"
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
	webtemplates "github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

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
		slog.Error("failed to load config", slog.String("error", err.Error()))
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
		slog.Error("failed to init otel", slog.String("error", err.Error()))
		os.Exit(1) //nolint:gocritic // exitAfterDefer: cancel() deferred above is trivial at this stage
	}
	defer otelOut.Shutdown(ctx) //nolint:errcheck // best-effort flush at exit

	// Set up dual slog logger (stdout + OTel pipeline) per D-03, OBS-1.
	logger := pdbotel.NewDualLogger(os.Stdout, otelOut.LogProvider)
	slog.SetDefault(logger)

	// Initialize custom sync metrics per D-05.
	if err := pdbotel.InitMetrics(); err != nil {
		logger.Error("failed to init metrics", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Open database per D-28, D-34.
	entClient, db, err := database.Open(cfg.DBPath)
	if err != nil {
		logger.Error("failed to open database", slog.String("error", err.Error()))
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
	if isPrimary {
		if err := entClient.Schema.Create(ctx); err != nil {
			logger.Error("failed to migrate schema", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}

	// Initialize sync_status table on primary (raw SQL, outside ent schema management).
	if isPrimary {
		if err := pdbsync.InitStatusTable(ctx, db); err != nil {
			logger.Error("failed to init sync_status table", slog.String("error", err.Error()))
			os.Exit(1)
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
		logger.Error("failed to init freshness gauge", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Initialize per-type object count gauges for business metrics dashboard.
	if err := pdbotel.InitObjectCountGauges(entClient); err != nil {
		logger.Error("failed to init object count gauges", slog.String("error", err.Error()))
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
	pdbClient := peeringdb.NewClient(cfg.PeeringDBBaseURL, logger, clientOpts...)

	// Create sync worker.
	syncWorker := pdbsync.NewWorker(pdbClient, entClient, db, pdbsync.WorkerConfig{
		IncludeDeleted: cfg.IncludeDeleted,
		IsPrimary:      isPrimaryFn,
		SyncMode:       cfg.SyncMode,
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
	mux.HandleFunc("POST /sync", newSyncHandler(ctx, SyncHandlerInput{
		IsPrimaryFn: isPrimaryFn,
		SyncToken:   cfg.SyncToken,
		DefaultMode: cfg.SyncMode,
		SyncFn: func(syncCtx context.Context, mode config.SyncMode) {
			syncWorker.SyncWithRetry(syncCtx, mode) //nolint:errcheck // fire-and-forget
		},
	}))

	// GET /healthz: liveness probe (always 200, not gated by readiness).
	mux.HandleFunc("GET /healthz", health.LivenessHandler())

	// GET /readyz: readiness probe (checks DB connectivity and sync freshness).
	mux.HandleFunc("GET /readyz", health.ReadinessHandler(health.ReadinessInput{
		DB:             db,
		StaleThreshold: cfg.SyncStaleThreshold,
	}))

	// GET /graphql: serve GraphiQL playground per D-17, D-18, D-21.
	// POST /graphql: handle GraphQL queries per D-18.
	playgroundHandler := pdbgql.PlaygroundHandler("/graphql")
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			playgroundHandler.ServeHTTP(w, r)
			return
		}
		gqlHandler.ServeHTTP(w, r)
	})

	// Mount entrest-generated REST API at /rest/v1/ per D-01, D-04.
	// Read-only (OperationRead + OperationList) configured via entrest annotations.
	restSrv, err := rest.NewServer(entClient, &rest.ServerConfig{
		BasePath: "/rest/v1",
	})
	if err != nil {
		logger.Error("failed to create REST server", slog.String("error", err.Error()))
		os.Exit(1)
	}
	restCORS := middleware.CORS(middleware.CORSInput{AllowedOrigins: cfg.CORSOrigins})
	mux.Handle("/rest/v1/", restCORS(restSrv.Handler()))
	logger.Info("REST API mounted", slog.String("prefix", "/rest/v1/"))

	// Mount PeeringDB compatibility API at /api/ per D-27, D-28.
	// Readiness gating applies automatically (not in bypass list) per D-29.
	compatHandler := pdbcompat.NewHandler(entClient)
	compatHandler.Register(mux)
	logger.Info("PeeringDB compat API mounted", slog.String("prefix", "/api/"))

	// Mount web UI at /ui/ and /static/ prefixes.
	webHandler := web.NewHandler(entClient, db)
	webHandler.Register(mux)
	logger.Info("Web UI mounted", slog.String("prefix", "/ui/"))

	// Create OTel interceptor for ConnectRPC services per OBS-01.
	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithoutServerPeerAttributes(),
		otelconnect.WithoutTraceEvents(), // Suppress per-message events (critical for streaming RPCs per STRM-06).
	)
	if err != nil {
		logger.Error("failed to create otel interceptor", slog.String("error", err.Error()))
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

	// GET /: content negotiation per user decision.
	// Browsers (Accept: text/html) redirect to /ui/.
	// API clients (Accept: application/json) get JSON discovery.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "text/html") {
			http.Redirect(w, r, "/ui/", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"peeringdb-plus","version":"0.1.0","graphql":"/graphql","rest":"/rest/v1/","api":"/api/","connectrpc":"/peeringdb.v1.","ui":"/ui/","healthz":"/healthz","readyz":"/readyz"}`)
	})

	// Build middleware stack (outermost first):
	// Recovery -> OTel HTTP -> Logging -> CORS -> Readiness -> mux
	handler := readinessMiddleware(syncWorker, mux)
	handler = middleware.CORS(middleware.CORSInput{AllowedOrigins: cfg.CORSOrigins})(handler)
	handler = middleware.Logging(logger)(handler)
	handler = otelhttp.NewMiddleware("peeringdb-plus")(handler)
	handler = middleware.Recovery(logger)(handler)

	// Enable HTTP/1.1 + h2c (HTTP/2 cleartext) for gRPC support.
	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	server := &http.Server{
		Addr:      cfg.ListenAddr,
		Handler:   handler,
		Protocols: &protocols,
	}

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
			logger.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	sig := <-sigChan
	logger.Info("shutting down", slog.String("signal", sig.String()))
	cancel() // Stop scheduler and background syncs.

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.DrainTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", slog.String("error", err.Error()))
	}
}

// syncReadiness reports whether at least one sync has completed.
type syncReadiness interface {
	HasCompletedSync() bool
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
				webtemplates.SyncingPage().Render(r.Context(), w) //nolint:errcheck // best-effort render
				return
			}
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
		if in.SyncToken == "" || r.Header.Get("X-Sync-Token") != in.SyncToken {
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
