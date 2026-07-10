package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/privfield"
)

// RESTFieldRedact removes the `ixf_ixp_member_list_url` key
// from /rest/v1/ JSON responses when the caller's ctx tier does not
// admit the field (per internal/privfield.Redact).
//
// entrest has no native per-field conditional-omission hook (verified
// against the lrstanley/entrest annotation reference and local behavior notes
// Finding #1). This middleware is the workaround: it buffers the
// response body, parses the JSON, and re-emits with the field deleted
// wherever privfield.Redact says omit.
//
// Scope: ALL /rest/v1/ paths, with the gated object located by a
// recursive walk rather than by response shape. entrest eager-loads the
// ixlan edge unconditionally (ent/rest/eagerload.go), so ixlan objects
// appear nested under edges.ix_lans / edges.ix_lan on internet-exchange,
// ix-prefix, and network-ix-lan responses — scoping redaction to the
// flat /rest/v1/ix-lans* paths leaked the gated URL through every
// embedding endpoint (2026-06-10 audit). Non-JSON bodies pass through
// unchanged.
//
// Single exemption: the exact path /rest/v1/openapi.json. The spec is a
// static document patched once at startup — it describes the
// `ixf_ixp_member_list_url` SCHEMA but can never contain the `_visible`
// companion as a data key, so the buffer+parse+walk over its ~1 MB body
// is pure overhead on every fetch. This is an exact-path skip of a
// provably-static document, NOT the entity path-scoping that caused the
// 2026-06-10 leak.
//
// Ordering: this middleware MUST be wrapped INSIDE RESTError
// so that problem+json error bodies pass through without being mis-parsed
// as data payloads.
//
// Required for privacy-redaction correctness.
func RESTFieldRedact(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// openapi.json exemption: static spec, can never carry the
		// _visible companion — see the doc comment above.
		if !strings.HasPrefix(r.URL.Path, "/rest/v1/") || r.URL.Path == "/rest/v1/openapi.json" {
			next.ServeHTTP(w, r)
			return
		}
		rw := &restFieldRedactWriter{ResponseWriter: w, ctx: r.Context()}
		next.ServeHTTP(rw, r)
		rw.flush()
	})
}

// restFieldRedactWriter buffers an ixlan REST response so that the body
// can be parsed + rewritten before reaching the client. Implements
// http.Flusher and Unwrap() per the middleware writer conventions.
type restFieldRedactWriter struct {
	http.ResponseWriter
	ctx    context.Context
	status int
	buf    bytes.Buffer
}

// WriteHeader captures the status code — the real header write is
// deferred until flush() after body rewrite.
func (w *restFieldRedactWriter) WriteHeader(code int) {
	w.status = code
}

// Write buffers the body so we can rewrite the JSON before flushing.
func (w *restFieldRedactWriter) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

// Unwrap returns the underlying ResponseWriter for middleware-aware
// interface detection (matches restErrorWriter pattern).
func (w *restFieldRedactWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush is intentionally a no-op. Unlike the pass-through restErrorWriter,
// this writer buffers the entire body so flush() can rewrite the JSON
// after the handler returns. Forwarding Flush() to the underlying writer
// mid-response would commit headers (an implicit 200) before flush() sends
// the real status and the redacted body, corrupting the response. REST
// responses are non-streaming, so nothing calls Flush() here in practice;
// the method exists only to satisfy http.Flusher for middleware interface
// detection.
func (w *restFieldRedactWriter) Flush() {}

// flush writes the buffered body to the underlying ResponseWriter,
// rewriting ixlan JSON payloads to drop the URL key when the caller's
// tier does not admit it.
func (w *restFieldRedactWriter) flush() {
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	body := w.buf.Bytes()
	contentType := w.Header().Get("Content-Type")

	// Pass through non-JSON (e.g. application/problem+json error bodies
	// from RESTError when wrapped inside-out, or empty 204s).
	if !strings.HasPrefix(contentType, "application/json") || len(body) == 0 {
		w.ResponseWriter.WriteHeader(status)
		_, _ = w.ResponseWriter.Write(body)
		return
	}

	rewritten, err := redactIxlanJSON(w.ctx, body)
	if err != nil {
		// Parse failed — pass through unchanged. A legitimate parse
		// error shouldn't happen on a 2xx entrest response; if it does,
		// corrupting the body would be worse than letting it through.
		// The field-level E2E test will catch any real leak.
		w.ResponseWriter.Header().Del("Content-Length")
		w.ResponseWriter.WriteHeader(status)
		_, _ = w.ResponseWriter.Write(body)
		return
	}

	// Clear Content-Length — Go's http server will compute a fresh
	// length or use chunked encoding as appropriate.
	w.ResponseWriter.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
	_, _ = w.ResponseWriter.Write(rewritten)
}

// redactIxlanJSON parses body as JSON, recursively redacts every embedded
// ixlan object, and returns the re-encoded body.
func redactIxlanJSON(ctx context.Context, body []byte) ([]byte, error) {
	var top any
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}
	if !redactGatedFields(ctx, top) {
		// Nothing was gated out (the common case: public tier, or a row
		// whose URL is admitted). Return the original bytes and skip the
		// re-marshal — the parsed value is byte-for-byte equivalent.
		return body, nil
	}
	return json.Marshal(top)
}

// redactGatedFields walks an unmarshalled JSON value and drops the
// ixf_ixp_member_list_url key in-place from every object carrying the
// _visible companion whenever privfield.Redact says omit, reporting
// whether anything was removed. Identifying gated objects by the
// companion key (rather than by response shape or URL path) covers
// detail objects, list entries, and ixlans eager-loaded under edges.*
// on other entity types alike. The _visible companion itself is left
// alone (always emitted, upstream parity).
func redactGatedFields(ctx context.Context, v any) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		if visible, ok := t["ixf_ixp_member_list_url_visible"].(string); ok {
			url, _ := t["ixf_ixp_member_list_url"].(string)
			if _, omit := privfield.Redact(ctx, visible, url); omit {
				if _, present := t["ixf_ixp_member_list_url"]; present {
					delete(t, "ixf_ixp_member_list_url")
					changed = true
				}
			}
		}
		for _, child := range t {
			if redactGatedFields(ctx, child) {
				changed = true
			}
		}
	case []any:
		for _, child := range t {
			if redactGatedFields(ctx, child) {
				changed = true
			}
		}
	}
	return changed
}
