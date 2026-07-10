package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
)

// RESTError wraps entrest error responses in RFC 9457 Problem
// Details format. It buffers non-2xx entrest bodies, then rewrites them as
// application/problem+json — preserving entrest's client-actionable error
// message (e.g. "per_page 0 is out of bounds, must be >= 1") as the problem
// Detail for 4xx responses. 5xx detail is dropped: those messages can carry
// SQL/driver internals that must not reach anonymous callers.
func RESTError(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &restErrorWriter{ResponseWriter: w, r: r}
		next.ServeHTTP(rw, r)
		rw.finish()
	})
}

// restErrorWriter captures non-2xx status codes and bodies from entrest and
// converts them to RFC 9457 Problem Details responses in finish().
type restErrorWriter struct {
	http.ResponseWriter
	r           *http.Request
	wroteHeader bool // true once a >=400 status has been intercepted
	status      int
	errBody     bytes.Buffer
}

// WriteHeader intercepts non-2xx status codes; the problem+json response is
// deferred to finish() so entrest's error body can be captured first.
func (w *restErrorWriter) WriteHeader(code int) {
	if code >= 400 && !w.wroteHeader {
		w.wroteHeader = true
		w.status = code
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write passes through for 2xx responses, or buffers entrest's error body
// so finish() can extract the client-actionable detail from it.
func (w *restErrorWriter) Write(b []byte) (int, error) {
	if w.wroteHeader {
		return w.errBody.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// finish emits the problem+json response for an intercepted error. No-op on
// the 2xx pass-through path. Called by RESTError after the inner
// handler returns.
func (w *restErrorWriter) finish() {
	if !w.wroteHeader {
		return
	}
	// The header map is shared with the underlying writer, so any
	// Content-Length entrest set for the discarded JSON body is stale;
	// WriteProblem sets its own Content-Type.
	w.ResponseWriter.Header().Del("Content-Length")
	httperr.WriteProblem(w.ResponseWriter, httperr.WriteProblemInput{
		Status:   w.status,
		Detail:   restErrorDetail(w.status, w.errBody.Bytes()),
		Instance: w.r.URL.Path,
	})
}

// restErrorDetail extracts the client-actionable message from an entrest
// error body ({"error": "...", ...}). Only 4xx messages are returned: they
// describe the caller's mistake (bad filter method, out-of-bounds page).
// 5xx messages may embed SQL/driver internals and are dropped. A body that
// fails to parse yields an empty detail (generic problem shape).
func restErrorDetail(status int, body []byte) string {
	if status >= http.StatusInternalServerError {
		return ""
	}
	var er struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &er); err != nil {
		return ""
	}
	return er.Error
}

// Unwrap returns the underlying ResponseWriter for middleware-aware interface detection.
func (w *restErrorWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Flush forwards to the underlying writer per the http.Flusher contract
// for middleware-aware response writers — but only
// on the 2xx pass-through path. Once an error has been intercepted, nothing
// has reached the underlying writer yet (the body is buffered for finish());
// forwarding Flush would commit an implicit 200 before finish() writes the
// real status and problem body.
func (w *restErrorWriter) Flush() {
	if w.wroteHeader {
		return
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
