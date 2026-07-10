package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// TestRedactIxlanJSON_FastPath verifies the redact writer skips the
// re-marshal when nothing is gated out, returning the original bytes
// (audit P6). The input uses a non-alphabetical key order; json.Marshal
// sorts keys, so byte-equality to the input proves no marshal happened.
func TestRedactIxlanJSON_FastPath(t *testing.T) {
	t.Parallel()
	publicCtx := privctx.WithTier(context.Background(), privctx.TierPublic)
	usersCtx := privctx.WithTier(context.Background(), privctx.TierUsers)

	mk := func(visible string) []byte {
		return []byte(`{"ixf_ixp_member_list_url":"http://x/","ixf_ixp_member_list_url_visible":"` + visible + `","id":1}`)
	}

	t.Run("public field returns original bytes", func(t *testing.T) {
		t.Parallel()
		in := mk("Public")
		out, err := redactIxlanJSON(publicCtx, in)
		if err != nil {
			t.Fatalf("redact: %v", err)
		}
		if !bytes.Equal(out, in) {
			t.Errorf("expected original bytes (fast-path); got re-marshaled %s", out)
		}
	})

	t.Run("users field at users tier returns original bytes", func(t *testing.T) {
		t.Parallel()
		in := mk("Users")
		out, err := redactIxlanJSON(usersCtx, in)
		if err != nil {
			t.Fatalf("redact: %v", err)
		}
		if !bytes.Equal(out, in) {
			t.Errorf("expected original bytes (admitted for users tier); got %s", out)
		}
	})

	t.Run("users field for anonymous is redacted and re-marshaled", func(t *testing.T) {
		t.Parallel()
		in := mk("Users")
		out, err := redactIxlanJSON(publicCtx, in)
		if err != nil {
			t.Fatalf("redact: %v", err)
		}
		if bytes.Equal(out, in) {
			t.Fatal("expected redaction (re-marshal), got original bytes")
		}
		var m map[string]any
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, ok := m["ixf_ixp_member_list_url"]; ok {
			t.Error("gated url should be removed for anonymous caller")
		}
		if _, ok := m["ixf_ixp_member_list_url_visible"]; !ok {
			t.Error("_visible companion must still be emitted")
		}
	})
}

// flushRecorder is an http.ResponseWriter that records whether Flush was
// called, for the REST writer Flusher-contract test.
type flushRecorder struct {
	http.ResponseWriter
	flushed bool
}

func (f *flushRecorder) Flush() { f.flushed = true }

// plainWriter implements http.ResponseWriter but NOT http.Flusher.
type plainWriter struct{ h http.Header }

func (p plainWriter) Header() http.Header         { return p.h }
func (p plainWriter) Write(b []byte) (int, error) { return len(b), nil }
func (p plainWriter) WriteHeader(int)             {}

// TestRESTWriters_FlushContract verifies the http.Flusher contract on the
// two REST response-writer wrappers (audit M5/M7): the pass-through
// restErrorWriter delegates Flush to the underlying writer, while the
// buffering restFieldRedactWriter must NOT delegate — flushing mid-response
// would commit headers before its deferred body rewrite.
func TestRESTWriters_FlushContract(t *testing.T) {
	t.Parallel()

	t.Run("restErrorWriter delegates", func(t *testing.T) {
		t.Parallel()
		rec := &flushRecorder{ResponseWriter: httptest.NewRecorder()}
		w := &restErrorWriter{ResponseWriter: rec, r: httptest.NewRequest(http.MethodGet, "/rest/v1/x", nil)}
		w.Flush()
		if !rec.flushed {
			t.Error("restErrorWriter.Flush should delegate to the underlying Flusher")
		}
	})

	t.Run("restErrorWriter tolerates non-Flusher underlying", func(t *testing.T) {
		t.Parallel()
		w := &restErrorWriter{ResponseWriter: plainWriter{h: http.Header{}}, r: httptest.NewRequest(http.MethodGet, "/", nil)}
		w.Flush() // must not panic
	})

	t.Run("restFieldRedactWriter does not delegate", func(t *testing.T) {
		t.Parallel()
		rec := &flushRecorder{ResponseWriter: httptest.NewRecorder()}
		w := &restFieldRedactWriter{ResponseWriter: rec, ctx: context.Background()}
		w.Flush()
		if rec.flushed {
			t.Error("restFieldRedactWriter.Flush must be a no-op (it buffers; delegating would commit headers early)")
		}
	})
}
