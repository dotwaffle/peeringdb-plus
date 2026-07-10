package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
)

// Recovery returns middleware that recovers from panics in downstream handlers.
// Logs the panic with stack trace via slog and returns an RFC 9457
// problem-detail 500 to the client.
//
// http.ErrAbortHandler is re-panicked, not swallowed: it is net/http's
// sanctioned way for a handler to abort a response (http.ServeContent
// uses it when a client disconnects mid-body), and the server suppresses
// its stack trace by contract. Recovering it would log a scary
// pseudo-panic for every torn-down client connection and attempt a 500
// write on a dead ResponseWriter.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if err, ok := rec.(error); ok && err == http.ErrAbortHandler { //nolint:errorlint // ErrAbortHandler is compared by identity per net/http convention
					panic(rec)
				}
				logger.Error("panic recovered",
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)
				// Best-effort: if the handler already wrote a body, this
				// header write is a no-op ("superfluous WriteHeader" in
				// the server log) and the truncated response stands.
				httperr.WriteProblem(w, httperr.WriteProblemInput{
					Status:   http.StatusInternalServerError,
					Detail:   "internal server error",
					Instance: r.URL.Path,
				})
			}()
			next.ServeHTTP(w, r)
		})
	}
}
