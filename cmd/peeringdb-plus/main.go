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
	"sync/atomic"
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
	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
	"github.com/dotwaffle/peeringdb-plus/internal/litefs"
	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
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
	if isPrimary {
		if err := entClient.Schema.Create(ctx); err != nil {
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
	pdbClient := peeringdb.NewClient(cfg.PeeringDBBaseURL, logger, clientOpts...)

	// Create sync worker.
	syncWorker := pdbsync.NewWorker(pdbClient, entClient, db, pdbsync.WorkerConfig{
		IncludeDeleted: cfg.IncludeDeleted,
		IsPrimary:      isPrimaryFn,
		SyncMode:       cfg.SyncMode,
		OnSyncComplete: func(counts map[string]int) {
			m := make(map[string]int64, len(counts))
			for k, v := range counts {
				m[k] = int64(v)
			}
			objectCountCache.Store(&m)
		},
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
			syncWorker.SyncWithRetry(syncCtx, mode) //nolint:errcheck // fire-and-forget
		},
	})
	mux.HandleFunc("POST /sync", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		syncHandler(w, r)
	})

	// GET /healthz: liveness probe (always 200, not gated by readiness).
	mux.HandleFunc("GET /healthz", health.LivenessHandler())

	// GET /readyz: readiness probe (checks DB connectivity and sync freshness).
	mux.HandleFunc("GET /readyz", health.ReadinessHandler(health.ReadinessInput{
		DB:             db,
		StaleThreshold: cfg.SyncStaleThreshold,
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
	mux.Handle("/rest/v1/", restCORS(restErrorMiddleware(restSrv.Handler())))
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
	// Recovery -> CORS -> OTel HTTP -> Logging -> Readiness -> CSP -> Caching -> Gzip -> mux
	compressionMiddleware := middleware.Compression()
	handler := compressionMiddleware(mux)
	cachingMiddleware := middleware.Caching(middleware.CachingInput{
		SyncTimeFn: func() time.Time {
			t, _ := pdbsync.GetLastSuccessfulSyncTime(context.Background(), db)
			return t
		},
		SyncInterval: cfg.SyncInterval,
	})
	handler = cachingMiddleware(handler)
	handler = middleware.CSP(middleware.CSPInput{
		UIPolicy:      "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; img-src 'self' data: https://*.basemaps.cartocdn.com; connect-src 'self'; font-src 'self' https://cdn.jsdelivr.net",
		GraphQLPolicy: "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data:; connect-src 'self'",
	})(handler)
	handler = readinessMiddleware(syncWorker, handler)
	handler = middleware.Logging(logger)(handler)
	handler = otelhttp.NewMiddleware("peeringdb-plus")(handler)
	handler = middleware.CORS(middleware.CORSInput{AllowedOrigins: cfg.CORSOrigins})(handler)
	handler = middleware.Recovery(logger)(handler)

	// Enable HTTP/1.1 + h2c (HTTP/2 cleartext) for gRPC support.
	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		Protocols:         &protocols,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
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
				webtemplates.SyncingPage().Render(r.Context(), w) //nolint:errcheck // best-effort render
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
				renderer.RenderError(w, http.StatusServiceUnavailable, "Service Unavailable", "PeeringDB data sync has not yet completed.\nPlease try again in a few moments.") //nolint:errcheck // best-effort render
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
