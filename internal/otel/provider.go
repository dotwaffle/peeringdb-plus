// Package otel initializes the OpenTelemetry trace, metric, and log pipelines.
// It uses autoexport for environment-driven configuration of exporters,
// supporting all standard OTEL_* env vars per D-07.
package otel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
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
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(in.SampleRate)),
	)
	otel.SetTracerProvider(tp)

	// MeterProvider for metrics. Views drop HTTP body-size instruments (low
	// debugging value, high cardinality) and override rpc.server.duration
	// bucket boundaries to a 5-boundary set. Metric resource omits
	// fly.machine_id to prevent per-VM metric fan-out; traces and logs keep
	// it for per-VM debugging.
	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating metric reader: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(metricRes),
		sdkmetric.WithReader(metricReader),
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
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{Name: "rpc.server.duration"},
				sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
					Boundaries: []float64{0.01, 0.05, 0.25, 1, 5},
					NoMinMax:   false,
				}},
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
// attribution via fly.machine_id.
func buildResource(ctx context.Context, serviceName string) *resource.Resource {
	return buildResourceFiltered(ctx, serviceName, true)
}

// buildMetricResource is like buildResource but omits fly.machine_id to
// prevent per-VM metric fan-out. Use for MeterProvider only;
// TracerProvider/LoggerProvider keep the full resource for per-VM
// debugging.
func buildMetricResource(ctx context.Context, serviceName string) *resource.Resource {
	return buildResourceFiltered(ctx, serviceName, false)
}

// buildResourceFiltered builds the OTel resource, optionally including
// the fly.machine_id attribute. Shared implementation backing
// buildResource (trace/log) and buildMetricResource (metrics).
func buildResourceFiltered(_ context.Context, serviceName string, includeMachineID bool) *resource.Resource {
	version := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		version = info.Main.Version
		if version == "(devel)" || version == "" {
			// Try VCS revision from build settings.
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

	// Fly.io-specific resource attributes per D-10.
	for _, env := range []struct {
		key  string
		attr string
	}{
		{"FLY_REGION", "fly.region"},
		{"FLY_MACHINE_ID", "fly.machine_id"},
		{"FLY_APP_NAME", "fly.app_name"},
	} {
		if !includeMachineID && env.attr == "fly.machine_id" {
			continue
		}
		if v := os.Getenv(env.key); v != "" {
			attrs = append(attrs, attribute.String(env.attr, v))
		}
	}

	res, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, attrs...),
	)
	return res
}
