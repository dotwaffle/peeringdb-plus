package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"entgo.io/ent/dialect"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
	"github.com/dotwaffle/peeringdb-plus/internal/grpcserver"
)

// TestConnectOTelOpts_SpansWithoutMetrics verifies that the production
// otelconnect options keep RPC tracing but record no RPC metrics, even when
// the interceptor has a real MeterProvider. The control case runs the same
// harness without the production options, to prove that the harness sees
// the rpc.server.* instruments when the interceptor records them.
func TestConnectOTelOpts_SpansWithoutMetrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		opts        []otelconnect.Option
		wantMetrics bool
	}{
		{name: "production", opts: connectOTelOpts(), wantMetrics: false},
		{name: "control", opts: nil, wantMetrics: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
			spans := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
			t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

			// The test providers go first so that the options under test
			// can override them, as they override the global providers in
			// production.
			opts := append([]otelconnect.Option{
				otelconnect.WithMeterProvider(mp),
				otelconnect.WithTracerProvider(tp),
			}, tt.opts...)
			interceptor, err := otelconnect.NewInterceptor(opts...)
			if err != nil {
				t.Fatalf("create otel interceptor: %v", err)
			}

			client := enttest.Open(t, dialect.SQLite, fmt.Sprintf(
				"file:connect_otel_%s?mode=memory&cache=shared&_pragma=foreign_keys(1)", tt.name))
			t.Cleanup(func() { _ = client.Close() })

			mux := http.NewServeMux()
			mux.Handle(peeringdbv1connect.NewNetworkServiceHandler(
				&grpcserver.NetworkService{Client: client},
				connectHandlerOpts(interceptor),
			))
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			rpc := peeringdbv1connect.NewNetworkServiceClient(srv.Client(), srv.URL)
			_, err = rpc.GetNetwork(t.Context(), &pb.GetNetworkRequest{Id: 1})
			if got := connect.CodeOf(err); got != connect.CodeNotFound {
				t.Fatalf("GetNetwork on empty DB: code %v (err %v), want %v",
					got, err, connect.CodeNotFound)
			}

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(t.Context(), &rm); err != nil {
				t.Fatalf("collect metrics: %v", err)
			}
			var names []string
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					names = append(names, m.Name)
				}
			}
			if tt.wantMetrics {
				if !slices.Contains(names, "rpc.server.duration") {
					t.Errorf("control: rpc.server.duration not recorded; got %v", names)
				}
			} else if len(names) > 0 {
				t.Errorf("interceptor recorded metrics %v, want none", names)
			}

			const wantSpan = "peeringdb.v1.NetworkService/GetNetwork"
			var spanNames []string
			for _, s := range spans.Ended() {
				spanNames = append(spanNames, s.Name())
			}
			if !slices.Contains(spanNames, wantSpan) {
				t.Errorf("span %q not recorded; got %v", wantSpan, spanNames)
			}
		})
	}
}
