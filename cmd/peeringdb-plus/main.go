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
	"syscall"
	"time"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/dotwaffle/peeringdb-plus/ent/rest"
	"github.com/dotwaffle/peeringdb-plus/graph"
	"github.com/dotwaffle/peeringdb-plus/graph/dataloader"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/database"
	pdbgql "github.com/dotwaffle/peeringdb-plus/internal/graphql"
	"github.com/dotwaffle/peeringdb-plus/internal/health"
	"github.com/dotwaffle/peeringdb-plus/internal/litefs"
	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
)

func init() {
	memlimit.SetGoMemLimitWithOpts(
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
		os.Exit(1)
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
		status, err := pdbsync.GetLastSyncStatus(ctx, db)
		if err != nil || status == nil || status.Status != "success" {
			return time.Time{}, false
		}
		return status.LastSyncAt, true
	}); err != nil {
		logger.Error("failed to init freshness gauge", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Create PeeringDB client per D-04, D-09.
	pdbClient := peeringdb.NewClient(cfg.PeeringDBBaseURL, logger)

	// Create sync worker.
	syncWorker := pdbsync.NewWorker(pdbClient, entClient, db, pdbsync.WorkerConfig{
		IncludeDeleted: cfg.IncludeDeleted,
		IsPrimary:      isPrimary,
		SyncMode:       cfg.SyncMode,
	}, logger)

	// Start scheduler on primary per D-22, D-29.
	if isPrimary {
		go syncWorker.StartScheduler(ctx, cfg.SyncInterval)
	}

	// Create GraphQL resolver with ent client and raw DB for sync_status queries.
	resolver := graph.NewResolver(entClient, db)

	// Create GraphQL handler with complexity/depth limits per D-04.
	gqlHandler := pdbgql.NewHandler(resolver)

	// Wrap GraphQL handler with DataLoader middleware per D-13.
	gqlWithLoader := dataloader.Middleware(entClient, gqlHandler)

	// Set up HTTP server.
	mux := http.NewServeMux()

	// POST /sync: on-demand sync trigger per D-23.
	// Write forwarding: replicas redirect to primary via Fly-Replay per D-26.
	mux.HandleFunc("POST /sync", func(w http.ResponseWriter, r *http.Request) {
		if !isPrimary {
			w.Header().Set("Fly-Replay", "leader")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		if cfg.SyncToken == "" || r.Header.Get("X-Sync-Token") != cfg.SyncToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse optional mode override from query param.
		mode := cfg.SyncMode
		if modeParam := r.URL.Query().Get("mode"); modeParam != "" {
			switch config.SyncMode(modeParam) {
			case config.SyncModeFull, config.SyncModeIncremental:
				mode = config.SyncMode(modeParam)
			default:
				http.Error(w, fmt.Sprintf("invalid mode %q: must be 'full' or 'incremental'", modeParam), http.StatusBadRequest)
				return
			}
		}

		// Use application root ctx, NOT r.Context() -- request context
		// is cancelled when the response is sent, which would kill the sync.
		go syncWorker.SyncWithRetry(ctx, mode) //nolint:errcheck // fire-and-forget
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"status":"accepted"}`)
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
	playgroundHandler := pdbgql.PlaygroundHandler("/graphql")
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			playgroundHandler.ServeHTTP(w, r)
			return
		}
		gqlWithLoader.ServeHTTP(w, r)
	})

	// Mount entrest-generated REST API at /rest/v1/ per D-01, D-04.
	// Read-only (OperationRead + OperationList) configured via entrest annotations.
	restSrv, err := rest.NewServer(entClient, &rest.ServerConfig{})
	if err != nil {
		logger.Error("failed to create REST server", slog.String("error", err.Error()))
		os.Exit(1)
	}
	restHandler := http.StripPrefix("/rest/v1", restSrv.Handler())
	restCORS := middleware.CORS(middleware.CORSInput{AllowedOrigins: cfg.CORSOrigins})
	mux.Handle("/rest/v1/", restCORS(restHandler))
	logger.Info("REST API mounted", slog.String("prefix", "/rest/v1/"))

	// Mount PeeringDB compatibility API at /api/ per D-27, D-28.
	// Readiness gating applies automatically (not in bypass list) per D-29.
	compatHandler := pdbcompat.NewHandler(entClient)
	compatHandler.Register(mux)
	logger.Info("PeeringDB compat API mounted", slog.String("prefix", "/api/"))

	// GET /: root discovery endpoint per D-28.
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"peeringdb-plus","version":"0.1.0","graphql":"/graphql","rest":"/rest/v1/","api":"/api/","healthz":"/healthz","readyz":"/readyz"}`)
	})

	// Build middleware stack (outermost first):
	// Recovery -> OTel HTTP -> Logging -> CORS -> Readiness -> mux
	var handler http.Handler = readinessMiddleware(syncWorker, mux)
	handler = middleware.CORS(middleware.CORSInput{AllowedOrigins: cfg.CORSOrigins})(handler)
	handler = middleware.Logging(logger)(handler)
	handler = otelhttp.NewMiddleware("peeringdb-plus")(handler)
	handler = middleware.Recovery(logger)(handler)

	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("starting server",
			slog.String("addr", cfg.ListenAddr),
			slog.Bool("is_primary", isPrimary),
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
func readinessMiddleware(sr syncReadiness, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Infrastructure paths are not gated by readiness.
		if r.URL.Path == "/sync" || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/" {
			next.ServeHTTP(w, r)
			return
		}
		if !sr.HasCompletedSync() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":"sync not yet completed"}`)
			return
		}
		next.ServeHTTP(w, r)
	})
}
