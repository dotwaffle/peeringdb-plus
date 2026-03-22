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

	// TracerProvider with configurable sampling per D-02.
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

	// MeterProvider for metrics.
	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating metric reader: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(metricReader),
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
// build info per D-08, and Fly.io environment attributes per D-10.
func buildResource(ctx context.Context, serviceName string) *resource.Resource {
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
