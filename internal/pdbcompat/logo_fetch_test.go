package pdbcompat

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newLogoTestServer returns a TLS server for the logo fetcher tests and
// the number of requests that it received.
func newLogoTestServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func TestLogoFetcher_Get(t *testing.T) {
	t.Parallel()
	srv, reads := newLogoTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.png":
			_, _ = w.Write([]byte("png"))
		case "/big.png":
			_, _ = w.Write(bytes.Repeat([]byte("x"), logoMaxBytes+1))
		case "/redirect.png":
			http.Redirect(w, r, "/ok.png", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	})
	host := srv.Listener.Addr().String()
	f := newLogoFetcher(srv.Client(), host)
	ctx := t.Context()

	for _, rawURL := range []string{
		"http://" + host + "/ok.png",
		"https://other.invalid/ok.png",
		"https://" + host + ":1/ok.png",
		"%zz",
	} {
		if _, ok := f.get(ctx, rawURL); ok {
			t.Errorf("get(%q): ok, want skipped", rawURL)
		}
	}
	if got := reads.Load(); got != 0 {
		t.Fatalf("skipped URLs sent %d requests, want 0", got)
	}

	for range 2 {
		data, ok := f.get(ctx, "https://"+host+"/ok.png")
		if !ok || string(data) != "png" {
			t.Errorf("get(ok.png) = %q, %v; want png, true", data, ok)
		}
	}
	if got := reads.Load(); got != 1 {
		t.Errorf("two reads of one URL sent %d requests, want 1 (cache)", got)
	}

	// A redirect is not followed, a 404 and a body above the cap fail.
	for _, name := range []string{"/redirect.png", "/missing.png", "/big.png"} {
		if _, ok := f.get(ctx, "https://"+host+name); ok {
			t.Errorf("get(%s): ok, want a failure", name)
		}
	}
	if got := reads.Load(); got != 4 {
		t.Errorf("requests = %d, want 4 (no redirect followed)", got)
	}
}

func TestLogoFetcher_FailureBackoff(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	fail.Store(true)
	srv, reads := newLogoTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("png"))
	})
	f := newLogoFetcher(srv.Client(), srv.Listener.Addr().String())
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	f.now = func() time.Time { return now }
	rawURL := "https://" + srv.Listener.Addr().String() + "/a.png"

	if _, ok := f.get(t.Context(), rawURL); ok {
		t.Fatal("first get: ok, want a failure")
	}
	fail.Store(false)
	now = now.Add(logoFailureBackoff - time.Second)
	if _, ok := f.get(t.Context(), rawURL); ok {
		t.Error("get in the backoff window: ok, want a failure without a request")
	}
	if got := reads.Load(); got != 1 {
		t.Errorf("requests in the backoff window = %d, want 1", got)
	}
	now = now.Add(time.Second)
	if data, ok := f.get(t.Context(), rawURL); !ok || string(data) != "png" {
		t.Errorf("get after the backoff = %q, %v; want png, true", data, ok)
	}
}

func TestLogoFetcher_SharedRead(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	srv, reads := newLogoTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = w.Write([]byte("png"))
	})
	f := newLogoFetcher(srv.Client(), srv.Listener.Addr().String())
	rawURL := "https://" + srv.Listener.Addr().String() + "/a.png"

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if data, ok := f.get(t.Context(), rawURL); !ok || string(data) != "png" {
				t.Errorf("get = %q, %v; want png, true", data, ok)
			}
		})
	}
	// Let the callers join the first read before it completes.
	for reads.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if got := reads.Load(); got != 1 {
		t.Errorf("concurrent reads of one URL sent %d requests, want 1", got)
	}
}

func TestLogoFetcher_Eviction(t *testing.T) {
	t.Parallel()
	f := newLogoFetcher(http.DefaultClient, "media.invalid")
	third := make([]byte, logoCacheBytes/3+1)
	f.store("a", third)
	f.store("b", third)
	if _, ok := f.cached("a"); !ok {
		t.Fatal("a not cached")
	}
	// a is now the most recently used, so c evicts b.
	f.store("c", third)
	if _, ok := f.cached("b"); ok {
		t.Error("b cached, want it evicted as the least recently used")
	}
	for _, u := range []string{"a", "c"} {
		if _, ok := f.cached(u); !ok {
			t.Errorf("%s not cached", u)
		}
	}
	if f.size != 2*len(third) {
		t.Errorf("size = %d, want %d", f.size, 2*len(third))
	}
	f.store("huge", make([]byte, logoCacheBytes+1))
	if _, ok := f.cached("huge"); ok || f.size != 2*len(third) {
		t.Errorf("a file above the cache size was cached (size %d)", f.size)
	}
}
