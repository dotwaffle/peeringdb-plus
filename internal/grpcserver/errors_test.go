package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/dotwaffle/peeringdb-plus/ent"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// logRecorder is a slog.Handler that keeps every record it receives.
type logRecorder struct {
	mu      sync.Mutex
	records []slog.Record
}

func (r *logRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *logRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec.Clone())
	return nil
}

func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }

func (r *logRecorder) WithGroup(string) slog.Handler { return r }

// takeLevels returns the levels of the records received since the last
// call that carry the attribute op=wantOp, and clears all records. The
// filter skips any record that another goroutine logs.
func (r *logRecorder) takeLevels(wantOp string) []slog.Level {
	r.mu.Lock()
	defer r.mu.Unlock()
	var levels []slog.Level
	for _, rec := range r.records {
		rec.Attrs(func(a slog.Attr) bool {
			if a.Key == "op" && a.Value.String() == wantOp {
				levels = append(levels, rec.Level)
				return false
			}
			return true
		})
	}
	r.records = nil
	return levels
}

// captureDefaultLogs sets the default logger to a logRecorder until the test
// ends. The default logger is process-wide, so the caller must not use
// t.Parallel. Go starts the parallel phase of top-level tests only after all
// sequential top-level tests have finished, so no other test logs to the
// recorder.
//
// slog.SetDefault also sends the output of the log package to the new
// handler. A later slog.SetDefault with the original handler does not undo
// that change, so the cleanup also restores the log package output and
// flags. Without this, later tests log into the recorder and their log
// lines are lost.
func captureDefaultLogs(t *testing.T) *logRecorder {
	t.Helper()
	rec := &logRecorder{}
	prev := slog.Default()
	prevOut, prevFlags := log.Writer(), log.Flags()
	slog.SetDefault(slog.New(rec))
	t.Cleanup(func() {
		slog.SetDefault(prev)
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return rec
}

// TestQueryError verifies how queryError classifies a query error. A server
// failure gets CodeInternal with the generic message, an ERROR log, and the
// cause on the span. A query that failed because the request context ended
// gets CodeCanceled or CodeDeadlineExceeded, a DEBUG log only, and no error
// on the span. The test changes the default logger, so it does not run in
// parallel.
func TestQueryError(t *testing.T) {
	logs := captureDefaultLogs(t)

	liveCtx := func(t *testing.T) context.Context { return t.Context() }
	canceledCtx := func(t *testing.T) context.Context {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		return ctx
	}
	expiredCtx := func(t *testing.T) context.Context {
		ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
		t.Cleanup(cancel)
		return ctx
	}

	tests := []struct {
		name        string
		ctx         func(*testing.T) context.Context
		err         error
		wantCode    connect.Code
		wantMsg     string
		wantLevel   slog.Level
		wantSpanErr bool
	}{
		{
			name:        "database failure",
			ctx:         liveCtx,
			err:         errors.New("SQL logic error: no such table: networks (1)"),
			wantCode:    connect.CodeInternal,
			wantMsg:     errInternal.Error(),
			wantLevel:   slog.LevelError,
			wantSpanErr: true,
		},
		{
			name:      "client canceled",
			ctx:       canceledCtx,
			err:       context.Canceled,
			wantCode:  connect.CodeCanceled,
			wantMsg:   context.Canceled.Error(),
			wantLevel: slog.LevelDebug,
		},
		{
			name:      "deadline passed",
			ctx:       expiredCtx,
			err:       context.DeadlineExceeded,
			wantCode:  connect.CodeDeadlineExceeded,
			wantMsg:   context.DeadlineExceeded.Error(),
			wantLevel: slog.LevelDebug,
		},
		{
			name:      "wrapped cancel on live context",
			ctx:       liveCtx,
			err:       fmt.Errorf("ent: query networks: %w", context.Canceled),
			wantCode:  connect.CodeCanceled,
			wantMsg:   context.Canceled.Error(),
			wantLevel: slog.LevelDebug,
		},
		{
			name:      "ended context with driver text",
			ctx:       canceledCtx,
			err:       errors.New("SQL logic error: interrupted (9)"),
			wantCode:  connect.CodeCanceled,
			wantMsg:   context.Canceled.Error(),
			wantLevel: slog.LevelDebug,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
			t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

			const op = "list networks"
			ctx, span := tp.Tracer("test").Start(tt.ctx(t), "rpc")
			got := queryError(ctx, op, tt.err)
			span.End()

			if got.Code() != tt.wantCode {
				t.Errorf("code = %v, want %v", got.Code(), tt.wantCode)
			}
			if got.Message() != tt.wantMsg {
				t.Errorf("message = %q, want %q", got.Message(), tt.wantMsg)
			}

			if levels := logs.takeLevels(op); len(levels) != 1 || levels[0] != tt.wantLevel {
				t.Errorf("log levels = %v, want [%v]", levels, tt.wantLevel)
			}

			ended := spans.Ended()
			if len(ended) != 1 {
				t.Fatalf("got %d ended spans, want 1", len(ended))
			}
			st, events := ended[0].Status(), ended[0].Events()
			if !tt.wantSpanErr {
				if st.Code != codes.Unset || len(events) != 0 {
					t.Errorf("span status = %+v, events = %+v; want Unset and no events", st, events)
				}
				return
			}
			if st.Code != codes.Error || st.Description != op {
				t.Errorf("span status = %+v, want Error %q", st, op)
			}
			var recorded bool
			for _, ev := range events {
				for _, kv := range ev.Attributes {
					if kv.Key == "exception.message" && kv.Value.AsString() == tt.err.Error() {
						recorded = true
					}
				}
			}
			if !recorded {
				t.Errorf("span has no exception event with the cause; events %+v", events)
			}
		})
	}
}

// closedClientError closes entClient and returns the text of the error that
// a query on it now gets, so a test can check that the text does not reach
// the client.
func closedClientError(t *testing.T, entClient *ent.Client) string {
	t.Helper()
	if err := entClient.Close(); err != nil {
		t.Fatalf("close ent client: %v", err)
	}
	_, err := entClient.Network.Query().Count(t.Context())
	if err == nil {
		t.Fatal("query on closed client succeeded; cannot force a database error")
	}
	return err.Error()
}

// checkGenericInternal fails the test unless err is a CodeInternal
// *connect.Error with the generic message and without dbText.
func checkGenericInternal(t *testing.T, err error, dbText string) {
	t.Helper()
	connectErr, ok := errors.AsType[*connect.Error](err)
	if !ok {
		t.Fatalf("got error %v (%T), want *connect.Error", err, err)
	}
	if connectErr.Code() != connect.CodeInternal {
		t.Errorf("code = %v, want %v", connectErr.Code(), connect.CodeInternal)
	}
	if connectErr.Message() != errInternal.Error() {
		t.Errorf("message = %q, want %q", connectErr.Message(), errInternal.Error())
	}
	if strings.Contains(connectErr.Error(), dbText) {
		t.Errorf("error %q contains database error text %q", connectErr.Error(), dbText)
	}
}

// TestQueryError_NoDatabaseTextOnWire forces a database error (the ent
// client is closed) and verifies that each RPC path returns CodeInternal
// with the generic message, and that the driver error text does not reach
// the client.
func TestQueryError_NoDatabaseTextOnWire(t *testing.T) {
	t.Parallel()
	entClient := testutil.SetupClient(t)
	rpcClient := setupStreamTestServer(t, entClient)
	dbText := closedClientError(t, entClient)

	sinceID := int64(0)
	tests := []struct {
		name string
		call func(context.Context) error
	}{
		{name: "Get", call: func(ctx context.Context) error {
			_, err := rpcClient.GetNetwork(ctx, &pb.GetNetworkRequest{Id: 1})
			return err
		}},
		{name: "List", call: func(ctx context.Context) error {
			_, err := rpcClient.ListNetworks(ctx, &pb.ListNetworksRequest{})
			return err
		}},
		// A full stream fails on the COUNT preflight.
		{name: "StreamCount", call: func(ctx context.Context) error {
			return drainNetworkStream(ctx, rpcClient, &pb.StreamNetworksRequest{})
		}},
		// A delta stream skips COUNT and fails on the first batch query.
		{name: "StreamBatch", call: func(ctx context.Context) error {
			return drainNetworkStream(ctx, rpcClient, &pb.StreamNetworksRequest{SinceId: &sinceID})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checkGenericInternal(t, tt.call(t.Context()), dbText)
		})
	}
}

// TestQueryError_GetAllServices checks the Get handler of each of the 13
// services, because each has its own error path. List and Stream share
// generic code, which TestQueryError_NoDatabaseTextOnWire covers. A handler
// that returns the ent error as-is fails here too: connect would send it as
// CodeUnknown with the driver text.
func TestQueryError_GetAllServices(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	dbText := closedClientError(t, c)

	tests := []struct {
		name string
		get  func(context.Context) error
	}{
		{name: "Campus", get: func(ctx context.Context) error {
			_, err := (&CampusService{Client: c}).GetCampus(ctx, &pb.GetCampusRequest{Id: 1})
			return err
		}},
		{name: "Carrier", get: func(ctx context.Context) error {
			_, err := (&CarrierService{Client: c}).GetCarrier(ctx, &pb.GetCarrierRequest{Id: 1})
			return err
		}},
		{name: "CarrierFacility", get: func(ctx context.Context) error {
			_, err := (&CarrierFacilityService{Client: c}).GetCarrierFacility(ctx, &pb.GetCarrierFacilityRequest{Id: 1})
			return err
		}},
		{name: "Facility", get: func(ctx context.Context) error {
			_, err := (&FacilityService{Client: c}).GetFacility(ctx, &pb.GetFacilityRequest{Id: 1})
			return err
		}},
		{name: "InternetExchange", get: func(ctx context.Context) error {
			_, err := (&InternetExchangeService{Client: c}).GetInternetExchange(ctx, &pb.GetInternetExchangeRequest{Id: 1})
			return err
		}},
		{name: "IxFacility", get: func(ctx context.Context) error {
			_, err := (&IxFacilityService{Client: c}).GetIxFacility(ctx, &pb.GetIxFacilityRequest{Id: 1})
			return err
		}},
		{name: "IxLan", get: func(ctx context.Context) error {
			_, err := (&IxLanService{Client: c}).GetIxLan(ctx, &pb.GetIxLanRequest{Id: 1})
			return err
		}},
		{name: "IxPrefix", get: func(ctx context.Context) error {
			_, err := (&IxPrefixService{Client: c}).GetIxPrefix(ctx, &pb.GetIxPrefixRequest{Id: 1})
			return err
		}},
		{name: "Network", get: func(ctx context.Context) error {
			_, err := (&NetworkService{Client: c}).GetNetwork(ctx, &pb.GetNetworkRequest{Id: 1})
			return err
		}},
		{name: "NetworkFacility", get: func(ctx context.Context) error {
			_, err := (&NetworkFacilityService{Client: c}).GetNetworkFacility(ctx, &pb.GetNetworkFacilityRequest{Id: 1})
			return err
		}},
		{name: "NetworkIxLan", get: func(ctx context.Context) error {
			_, err := (&NetworkIxLanService{Client: c}).GetNetworkIxLan(ctx, &pb.GetNetworkIxLanRequest{Id: 1})
			return err
		}},
		{name: "Organization", get: func(ctx context.Context) error {
			_, err := (&OrganizationService{Client: c}).GetOrganization(ctx, &pb.GetOrganizationRequest{Id: 1})
			return err
		}},
		{name: "Poc", get: func(ctx context.Context) error {
			_, err := (&PocService{Client: c}).GetPoc(ctx, &pb.GetPocRequest{Id: 1})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checkGenericInternal(t, tt.get(t.Context()), dbText)
		})
	}
}

// TestQueryError_ContextEnd verifies that a request that ends because its
// context ended gets the context code, not CodeInternal, on each path. Get
// and List are called directly with a canceled context, because a client
// does not send a call whose context is already canceled. The streams run
// over the wire with a 1ns stream timeout: a full stream stops on the COUNT
// preflight query, and a delta stream stops on the context check before
// its first batch.
//
// connect sends a raw context error with the matching code too, so the
// stream server also checks that the handler itself returns a
// *connect.Error. Interceptors such as otelconnect see the handler error
// before connect converts it, and mark the span as failed for a raw error.
func TestQueryError_ContextEnd(t *testing.T) {
	t.Parallel()
	entClient := testutil.SetupClient(t)
	svc := &NetworkService{Client: entClient, StreamTimeout: time.Nanosecond}
	rpcClient := serveNetworkService(t, svc,
		connect.WithInterceptors(connectErrorCheck{t: t}))

	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	sinceID := int64(0)
	tests := []struct {
		name     string
		call     func(context.Context) error
		wantCode connect.Code
	}{
		{name: "Get", wantCode: connect.CodeCanceled, call: func(context.Context) error {
			_, err := svc.GetNetwork(canceled, &pb.GetNetworkRequest{Id: 1})
			return err
		}},
		{name: "List", wantCode: connect.CodeCanceled, call: func(context.Context) error {
			_, err := svc.ListNetworks(canceled, &pb.ListNetworksRequest{})
			return err
		}},
		{name: "StreamFull", wantCode: connect.CodeDeadlineExceeded, call: func(ctx context.Context) error {
			return drainNetworkStream(ctx, rpcClient, &pb.StreamNetworksRequest{})
		}},
		{name: "StreamDelta", wantCode: connect.CodeDeadlineExceeded, call: func(ctx context.Context) error {
			return drainNetworkStream(ctx, rpcClient, &pb.StreamNetworksRequest{SinceId: &sinceID})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.call(t.Context())
			if got := connect.CodeOf(err); got != tt.wantCode {
				t.Errorf("code = %v (err %v), want %v", got, err, tt.wantCode)
			}
		})
	}
}

// connectErrorCheck is an interceptor that fails t when a streaming
// handler returns an error that is not a *connect.Error.
type connectErrorCheck struct{ t *testing.T }

func (c connectErrorCheck) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc { return next }

func (c connectErrorCheck) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (c connectErrorCheck) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		err := next(ctx, conn)
		if _, ok := errors.AsType[*connect.Error](err); err != nil && !ok {
			c.t.Errorf("%s returned %v (%T), want *connect.Error",
				conn.Spec().Procedure, err, err)
		}
		return err
	}
}

// serveNetworkService mounts svc on an HTTP/2 TLS test server and returns a
// client for it.
func serveNetworkService(t *testing.T, svc *NetworkService, opts ...connect.HandlerOption) peeringdbv1connect.NetworkServiceClient {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewNetworkServiceHandler(svc, opts...))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewNetworkServiceClient(srv.Client(), srv.URL)
}

// drainNetworkStream reads a StreamNetworks response to the end and returns
// the stream error.
func drainNetworkStream(ctx context.Context, c peeringdbv1connect.NetworkServiceClient, req *pb.StreamNetworksRequest) error {
	stream, err := c.StreamNetworks(ctx, req)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	for stream.Receive() {
	}
	return stream.Err()
}
