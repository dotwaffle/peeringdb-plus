// Package otel initializes the OpenTelemetry trace, metric, and log pipelines.
// It uses autoexport for environment-driven configuration of exporters,
// supporting all standard OTEL_* env vars per D-07.
package otel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/dotwaffle/peeringdb-plus/internal/buildinfo"
)

// SetupInput holds configuration for initializing the OTel pipeline.
// SampleRate controls trace sampling (0.0 to 1.0) per D-02.
type SetupInput struct {
	ServiceName string
	SampleRate  float64
}

// SetupOutput holds the OTel shutdown function and LoggerProvider
// for creating the dual slog handler.
type SetupOutput struct {
	// Shutdown flushes all providers and must be called before program exit.
	Shutdown func(context.Context) error

	// LogProvider is exposed so the caller can create a dual slog handler
	// that bridges slog records into the OTel log pipeline.
	LogProvider *sdklog.LoggerProvider
}

// Setup initializes TracerProvider, MeterProvider, and LoggerProvider using
// autoexport for environment-driven exporter selection per D-06, D-07.
// Individual signals can be disabled via OTEL_*_EXPORTER=none per D-04.
func Setup(ctx context.Context, in SetupInput) (*SetupOutput, error) {
	res := buildResource(ctx, in.ServiceName)
	metricRes := buildMetricResource(ctx, in.ServiceName)

	// TracerProvider with configurable sampling per D-02.
	// Batching is explicitly enabled per PERF-08; defaults to 5s/512 items,
	// tuneable via OTEL_BSP_SCHEDULE_DELAY and OTEL_BSP_MAX_EXPORT_BATCH_SIZE.
	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating span exporter: %w", err)
	}
	// Per CONTEXT.md D-02 / Phase 77 OBS-07: per-route sampler dispatches
	// on URL path prefix so /healthz + /readyz drop to 1% (Fly health
	// probes dominate trace volume) while /api/* /rest/v1/* /peeringdb.v1.*
	// stay at full sampling. Wrapped in sdktrace.ParentBased so children
	// inherit the parent decision — preserves cross-service trace continuity.
	// Ratios mirror .planning/phases/77-telemetry-audit/AUDIT.md
	// § Recommended sampling matrix; update both together.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(NewPerRouteSampler(PerRouteSamplerInput{
			DefaultRatio: in.SampleRate,
			Routes: map[string]float64{
				"/healthz":                0.01,
				"/readyz":                 0.01,
				"/grpc.health.v1.Health/": 0.01,
				"/api/":                   1.0,
				"/rest/v1/":               1.0,
				"/peeringdb.v1.":          1.0,
				"/graphql":                1.0,
				"/ui/":                    0.5,
				"/static/":                0.01,
				"/favicon.ico":            0.01,
			},
		}))),
	)
	otel.SetTracerProvider(tp)

	// MeterProvider for metrics. Views aggressively trim cardinality to fit
	// inside Grafana Cloud's hosted Prometheus quota. Three categories of
	// trim, in declaration order below:
	//
	//   1. Drop HTTP body-size instruments — low debugging value vs. the
	//      cardinality cost (otelhttp emits one series per route × method
	//      × status_code combination for both request and response sides).
	//
	//   2. Drop the entire otelconnect rpc.server.* family. Five
	//      instruments (request.size, response.size, duration,
	//      requests_per_rpc, responses_per_rpc) × ~50 RPC procedures ×
	//      status code = ~2500 series per machine before resource attrs.
	//      ConnectRPC traffic shape is already visible in our existing
	//      pdbplus.* business metrics and the otelhttp duration histogram
	//      at the transport layer; the rpc.server.* family is duplicate
	//      signal at high cost. Replaces the prior single-instrument
	//      duration override (which only coarsened buckets, not the
	//      cardinality blow-up).
	//
	//   3. Replace otelhttp's default 16-boundary duration histogram with a
	//      5-boundary set (10ms / 50ms / 250ms / 1s / 5s) and strip the
	//      method/scheme/server.address/network.protocol.* attribute axes,
	//      keeping only http.route + http.response.status_code +
	//      network.protocol.version. Buckets mirror the rpc.server.duration
	//      set we used previously so SLO panels stay aligned across surfaces.
	//
	// Metric resource omits service.instance.id (buildMetricResource) to
	// prevent per-VM fan-out across the same axes; traces and logs keep it
	// for per-VM debugging.
	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating metric reader: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(metricRes),
		sdkmetric.WithReader(metricReader),
		// 1. HTTP body-size drops.
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{Name: "http.server.request.body.size"},
				sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
			),
		),
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{Name: "http.server.response.body.size"},
				sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
			),
		),
		// 2. rpc.server.* family drops — five instruments enumerated
		//    individually (no wildcard support in SDK Views).
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{Name: "rpc.server.duration"},
				sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
			),
		),
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{Name: "rpc.server.request.size"},
				sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
			),
		),
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{Name: "rpc.server.response.size"},
				sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
			),
		),
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{Name: "rpc.server.requests_per_rpc"},
				sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
			),
		),
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{Name: "rpc.server.responses_per_rpc"},
				sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
			),
		),
		// 3. otelhttp duration: coarsen buckets + allow-list attribute keys
		//    so http.request.method drops out of the label set.
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{Name: "http.server.request.duration"},
				sdkmetric.Stream{
					Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
						Boundaries: []float64{0.01, 0.05, 0.25, 1, 5},
						NoMinMax:   false,
					},
					AttributeFilter: attribute.NewAllowKeysFilter(
						"http.route",
						"http.response.status_code",
						"network.protocol.version",
					),
				},
			),
		),
	)
	otel.SetMeterProvider(mp)

	// LoggerProvider for OTel log pipeline per D-03.
	logExporter, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
	)

	// W3C TraceContext propagation per D-09.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Go runtime metrics (goroutines, memory, GC) per D-05.
	if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
		return nil, fmt.Errorf("starting runtime metrics: %w", err)
	}

	return &SetupOutput{
		LogProvider: lp,
		Shutdown: func(ctx context.Context) error {
			// Shutdown in reverse initialization order.
			return errors.Join(
				lp.Shutdown(ctx),
				mp.Shutdown(ctx),
				tp.Shutdown(ctx),
			)
		},
	}, nil
}

// buildResource creates an OTel resource with service name, version from
// build info per D-08, and Fly.io environment attributes per D-10. Used
// by TracerProvider and LoggerProvider so traces and logs keep per-VM
// attribution via service.instance.id.
func buildResource(ctx context.Context, serviceName string) *resource.Resource {
	return buildResourceFiltered(ctx, serviceName, true)
}

// buildMetricResource is like buildResource but omits service.instance.id
// to prevent per-VM metric fan-out. Use for MeterProvider only;
// TracerProvider/LoggerProvider keep the full resource for per-VM
// debugging.
func buildMetricResource(ctx context.Context, serviceName string) *resource.Resource {
	return buildResourceFiltered(ctx, serviceName, false)
}

// buildResourceFiltered builds the OTel resource, optionally including the
// per-VM service.instance.id attribute. Shared implementation backing
// buildResource (trace/log) and buildMetricResource (metrics).
//
// Naming follows GC-allowlisted OTel semconv keys (service.*, cloud.*) so
// Grafana Cloud's hosted OTLP receiver promotes them to Prometheus labels.
// Custom keys outside that allowlist (e.g. legacy fly.*) are silently
// dropped on the metrics path; only fly.app_name is kept on the trace/log
// resource for human grep against historical telemetry.
//
// Why the metric resource keeps service.namespace + cloud.region:
// the operator wants primary-vs-replica + per-region health breakdowns;
// 2-cardinality and 8-cardinality respectively, well within budget.
//
// Why the metric resource strips service.instance.id:
// per-VM = high cardinality, low value — the operator does not want 8
// machine-id-prefixed series per metric. This is the same per-VM strip
// that previously gated fly.machine_id; the includeInstanceID flag name
// matches the new attr key.
//
// Why cloud.provider + cloud.platform are unconditional:
// they are 1-cardinality semconv resource attrs that GC allowlists for
// free. Emitting them on every signal lets dashboards filter by provider
// without coupling to a Fly-specific env var.
func buildResourceFiltered(_ context.Context, serviceName string, includeInstanceID bool) *resource.Resource {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(buildinfo.Version()),
	}

	// Always-on: cloud provider / platform constants (1-cardinality, GC-allowlisted).
	attrs = append(attrs,
		attribute.String(string(semconv.CloudProviderKey), "fly_io"),
		attribute.String(string(semconv.CloudPlatformKey), "fly_io_apps"),
	)
	// Env-driven: only emit when the env var is set (avoids empty-string attrs in local dev).
	if v := os.Getenv("FLY_REGION"); v != "" {
		attrs = append(attrs, semconv.CloudRegion(v))
	}
	if v := os.Getenv("FLY_PROCESS_GROUP"); v != "" {
		attrs = append(attrs, semconv.ServiceNamespace(v))
	}
	if v := os.Getenv("FLY_APP_NAME"); v != "" {
		// Custom key kept for human grep against historical logs/traces;
		// GC drops this on the metrics path but harmless on traces/logs.
		attrs = append(attrs, attribute.String("fly.app_name", v))
	}
	// Per-VM identity: stripped from metric resource to prevent per-VM
	// metric fan-out (8 machines × N metrics × M label combos). Traces
	// and logs keep it for per-VM debugging — that's the includeInstanceID
	// gate.
	if includeInstanceID {
		if v := os.Getenv("FLY_MACHINE_ID"); v != "" {
			attrs = append(attrs, semconv.ServiceInstanceID(v))
		}
	}

	res, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
	return res
}
