package grpcserver

import (
	"context"
	"errors"
	"log/slog"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// errInternal is the only text a client receives for a CodeInternal error.
var errInternal = errors.New("internal error")

// queryError converts a failed database query into the error that a handler
// returns. The query error text (usually ent or SQLite error text with
// table, column, or query detail) never goes to the wire. op names the
// failed step, for example "get network 42" or "count networks".
//
// If the request context ended (the client canceled the call, the client
// deadline passed, or the stream timeout fired), the query failed only
// because of that. queryError then returns the matching code from
// contextError and logs at DEBUG. It does not record the error on the span,
// because the server did not fail.
//
// Any other error is a server-side failure. queryError logs it at ERROR,
// records it on the active span with Error status, and returns
// CodeInternal with the generic message "internal error".
//
// Only query failures need this. CodeNotFound and CodeInvalidArgument
// messages describe the caller's own input and are safe to send.
func queryError(ctx context.Context, op string, err error) *connect.Error {
	attrs := []any{slog.String("op", op), slog.Any("error", err)}
	if info, ok := connect.CallInfoForHandlerContext(ctx); ok {
		attrs = append(attrs, slog.String("procedure", info.Spec().Procedure))
	}

	if ctxErr := contextError(ctx, err); ctxErr != nil {
		slog.DebugContext(ctx, "connectrpc request ended by context", attrs...)
		return ctxErr
	}

	slog.ErrorContext(ctx, "connectrpc internal error", attrs...)

	// otelconnect later sets the span status from the wire message, so
	// the exception event from RecordError is what keeps the cause on
	// the trace.
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, op)

	return connect.NewError(connect.CodeInternal, errInternal)
}

// contextError returns CodeDeadlineExceeded or CodeCanceled if ctx has
// ended or err is a context error (errors.Is, so wrapped errors match). It
// returns nil otherwise, and err can be nil. ctx is checked first: after
// the context ends, a query can fail with driver text that does not wrap
// the context error. The message is the fixed text of the context error,
// never the text of err.
func contextError(ctx context.Context, err error) *connect.Error {
	cause := ctx.Err()
	if cause == nil {
		cause = err
	}
	switch {
	case errors.Is(cause, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, context.DeadlineExceeded)
	case errors.Is(cause, context.Canceled):
		return connect.NewError(connect.CodeCanceled, context.Canceled)
	default:
		return nil
	}
}
