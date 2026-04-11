package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

// weakETagRE is the canonical regex for a W/"<32 hex chars>" weak ETag
// produced by computeETag. Hoisted to package scope so every test compiles
// it once.
var weakETagRE = regexp.MustCompile(`^W/"[0-9a-f]{32}"$`)

func TestCaching(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	syncInterval := time.Hour

	// Precompute the ETag for fixedTime so tests can reference it.
	probeState := middleware.NewCachingState(syncInterval)
	probeState.UpdateETag(fixedTime)
	probeMW := probeState.Middleware()
	probe := httptest.NewRequest(http.MethodGet, "/api/net", nil)
	probeRec := httptest.NewRecorder()
	probeMW(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(probeRec, probe)
	currentETag := probeRec.Header().Get("ETag")

	tests := []struct {
		name          string
		method        string
		callUpdate    bool // if false, do NOT call UpdateETag (pre-sync state)
		syncTime      time.Time
		ifNoneMatch   string
		wantStatus    int
		wantCacheCtrl string
		wantETag      bool
		wantETagValue string
		wantCalled    bool
		wantEmptyBody bool
	}{
		{
			name:          "GET with valid sync time sets Cache-Control and ETag",
			method:        http.MethodGet,
			callUpdate:    true,
			syncTime:      fixedTime,
			wantStatus:    http.StatusOK,
			wantCacheCtrl: "public, max-age=3720",
			wantETag:      true,
			wantCalled:    true,
		},
		{
			name:          "HEAD with valid sync time sets Cache-Control and ETag",
			method:        http.MethodHead,
			callUpdate:    true,
			syncTime:      fixedTime,
			wantStatus:    http.StatusOK,
			wantCacheCtrl: "public, max-age=3720",
			wantETag:      true,
			wantCalled:    true,
		},
		{
			name:       "POST request has no caching headers",
			method:     http.MethodPost,
			callUpdate: true,
			syncTime:   fixedTime,
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "GET before first UpdateETag has no caching headers",
			method:     http.MethodGet,
			callUpdate: false,
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:          "GET with matching If-None-Match returns 304",
			method:        http.MethodGet,
			callUpdate:    true,
			syncTime:      fixedTime,
			ifNoneMatch:   currentETag,
			wantStatus:    http.StatusNotModified,
			wantETag:      true,
			wantETagValue: currentETag,
			wantCalled:    false,
			wantEmptyBody: true,
		},
		{
			name:          "GET with If-None-Match wildcard returns 304",
			method:        http.MethodGet,
			callUpdate:    true,
			syncTime:      fixedTime,
			ifNoneMatch:   "*",
			wantStatus:    http.StatusNotModified,
			wantETag:      true,
			wantCalled:    false,
			wantEmptyBody: true,
		},
		{
			name:          "GET with non-matching If-None-Match returns normal response",
			method:        http.MethodGet,
			callUpdate:    true,
			syncTime:      fixedTime,
			ifNoneMatch:   `W/"0000000000000000000000000000dead"`,
			wantStatus:    http.StatusOK,
			wantCacheCtrl: "public, max-age=3720",
			wantETag:      true,
			wantCalled:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			called := false
			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			state := middleware.NewCachingState(syncInterval)
			if tc.callUpdate {
				state.UpdateETag(tc.syncTime)
			}
			handler := state.Middleware()(inner)

			req := httptest.NewRequest(tc.method, "/api/net", nil)
			if tc.ifNoneMatch != "" {
				req.Header.Set("If-None-Match", tc.ifNoneMatch)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if called != tc.wantCalled {
				t.Errorf("inner handler called = %v, want %v", called, tc.wantCalled)
			}
			if tc.wantCacheCtrl != "" {
				got := rec.Header().Get("Cache-Control")
				if got != tc.wantCacheCtrl {
					t.Errorf("Cache-Control = %q, want %q", got, tc.wantCacheCtrl)
				}
			} else if tc.method == http.MethodPost || !tc.callUpdate {
				got := rec.Header().Get("Cache-Control")
				if got != "" {
					t.Errorf("Cache-Control = %q, want empty for %s / pre-sync", got, tc.method)
				}
			}
			if tc.wantETag {
				got := rec.Header().Get("ETag")
				if got == "" {
					t.Error("ETag header missing, want non-empty")
				}
				if tc.wantETagValue != "" && got != tc.wantETagValue {
					t.Errorf("ETag = %q, want %q", got, tc.wantETagValue)
				}
			}
			if tc.wantEmptyBody {
				if rec.Body.Len() != 0 {
					t.Errorf("body length = %d, want 0 for 304", rec.Body.Len())
				}
			}
		})
	}
}

func TestCachingETagFormat(t *testing.T) {
	t.Parallel()

	state := middleware.NewCachingState(time.Hour)
	state.UpdateETag(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	state.Middleware()(inner).ServeHTTP(rec, req)

	etag := rec.Header().Get("ETag")
	if !weakETagRE.MatchString(etag) {
		t.Errorf("ETag = %q, want format W/\"<32 hex chars>\"", etag)
	}
}

func TestCachingETagChangesWithSyncTime(t *testing.T) {
	t.Parallel()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	time1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	time2 := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)

	// Single CachingState across the two UpdateETag calls — validates
	// that the atomic swap surfaces the new value on the next GET.
	state := middleware.NewCachingState(time.Hour)
	mw := state.Middleware()(inner)

	state.UpdateETag(time1)
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	mw.ServeHTTP(rec1, req1)

	state.UpdateETag(time2)
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	mw.ServeHTTP(rec2, req2)

	etag1 := rec1.Header().Get("ETag")
	etag2 := rec2.Header().Get("ETag")

	if etag1 == etag2 {
		t.Errorf("ETags should differ for different sync times, both = %q", etag1)
	}
}

// TestCaching_ETagAtomicSwap proves that calling UpdateETag with a new sync
// timestamp between two GETs changes the ETag returned to subsequent requests.
// This is the PERF-07 core contract: one UpdateETag call -> next Load observes
// the new value. Regression-locks the atomic.Pointer swap semantics.
func TestCaching_ETagAtomicSwap(t *testing.T) {
	t.Parallel()

	state := middleware.NewCachingState(time.Hour)
	mw := state.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	time1 := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	time2 := time.Date(2026, 2, 1, 13, 0, 0, 0, time.UTC)

	state.UpdateETag(time1)
	recA := httptest.NewRecorder()
	mw.ServeHTTP(recA, httptest.NewRequest(http.MethodGet, "/api/net", nil))
	etagA := recA.Header().Get("ETag")

	state.UpdateETag(time2)
	recB := httptest.NewRecorder()
	mw.ServeHTTP(recB, httptest.NewRequest(http.MethodGet, "/api/net", nil))
	etagB := recB.Header().Get("ETag")

	if !weakETagRE.MatchString(etagA) {
		t.Errorf("etagA = %q, want weak ETag format", etagA)
	}
	if !weakETagRE.MatchString(etagB) {
		t.Errorf("etagB = %q, want weak ETag format", etagB)
	}
	if etagA == etagB {
		t.Errorf("ETag did not swap after UpdateETag: A=%q B=%q", etagA, etagB)
	}
}

// TestCaching_ETagStableBetweenSyncs proves that 100 consecutive GETs between
// UpdateETag calls return byte-identical ETag headers. This is the cheap proxy
// for "no per-request recomputation" — if computeETag were still being called
// on every GET, the ETag value would still be identical (same syncTime input),
// but the runtime cost wouldn't. The byte-identity assertion is necessary but
// not sufficient; the stronger regression lock is the acceptance criterion
// that awk-slices the Middleware body and asserts zero sha256/computeETag
// references.
func TestCaching_ETagStableBetweenSyncs(t *testing.T) {
	t.Parallel()

	state := middleware.NewCachingState(time.Hour)
	state.UpdateETag(time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC))
	mw := state.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var first string
	const iterations = 100
	for i := range iterations {
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/net", nil))
		got := rec.Header().Get("ETag")
		if i == 0 {
			first = got
			if !weakETagRE.MatchString(first) {
				t.Fatalf("iter 0: ETag = %q, want weak ETag format", first)
			}
			continue
		}
		if got != first {
			t.Errorf("iter %d: ETag = %q, want %q (stable between syncs)", i, got, first)
		}
	}
}

// TestCaching_IfNoneMatch304 is a dedicated regression lock for the 304 Not
// Modified contract under the new atomic API. Mirrors the subtest inside
// TestCaching but stands alone so a future refactor that accidentally breaks
// the conditional path fails loudly with a named test.
func TestCaching_IfNoneMatch304(t *testing.T) {
	t.Parallel()

	state := middleware.NewCachingState(time.Hour)
	state.UpdateETag(time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC))

	var innerCalled atomic.Bool
	mw := state.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		innerCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))

	// First GET captures the current ETag.
	rec1 := httptest.NewRecorder()
	mw.ServeHTTP(rec1, httptest.NewRequest(http.MethodGet, "/api/net", nil))
	etag := rec1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("first GET: ETag header missing")
	}
	if !innerCalled.Load() {
		t.Error("first GET: inner handler should have been called")
	}

	// Reset the inner-call flag, then re-request with If-None-Match: <etag>.
	innerCalled.Store(false)
	req := httptest.NewRequest(http.MethodGet, "/api/net", nil)
	req.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	mw.ServeHTTP(rec2, req)

	if rec2.Code != http.StatusNotModified {
		t.Errorf("status = %d, want %d", rec2.Code, http.StatusNotModified)
	}
	if rec2.Body.Len() != 0 {
		t.Errorf("body length = %d, want 0 on 304", rec2.Body.Len())
	}
	if innerCalled.Load() {
		t.Error("inner handler called on 304 — must short-circuit before next.ServeHTTP")
	}
	if got := rec2.Header().Get("ETag"); got != etag {
		t.Errorf("304 ETag = %q, want %q (must still emit header)", got, etag)
	}
}

// TestCaching_ConcurrentReadDuringUpdate runs 8 reader goroutines hammering
// the middleware while a 9th goroutine alternates between two UpdateETag
// values in a tight loop. Under -race this catches any future drift that
// removes the atomic.Pointer wrapper. Every observed ETag MUST be one of the
// two known values — never a torn read, never empty.
func TestCaching_ConcurrentReadDuringUpdate(t *testing.T) {
	t.Parallel()

	state := middleware.NewCachingState(time.Hour)
	time1 := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	time2 := time.Date(2026, 2, 1, 13, 0, 0, 0, time.UTC)

	// Pre-compute both expected ETag values via a throwaway state so the
	// test knows the exact strings to compare against. computeETag is
	// unexported; we derive both values the same way the middleware does:
	// one UpdateETag call, one probe GET.
	expect := func(ts time.Time) string {
		s := middleware.NewCachingState(time.Hour)
		s.UpdateETag(ts)
		rec := httptest.NewRecorder()
		s.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		return rec.Header().Get("ETag")
	}
	etag1 := expect(time1)
	etag2 := expect(time2)
	if etag1 == etag2 || etag1 == "" || etag2 == "" {
		t.Fatalf("probe etags invalid: etag1=%q etag2=%q", etag1, etag2)
	}

	state.UpdateETag(time1)
	mw := state.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer goroutine: flips between time1 and time2 as fast as it can.
	wg.Go(func() {
		flip := true
		for {
			select {
			case <-stop:
				return
			default:
			}
			if flip {
				state.UpdateETag(time2)
			} else {
				state.UpdateETag(time1)
			}
			flip = !flip
		}
	})

	// Reader goroutines: hammer the middleware and check the observed ETag
	// is always one of the two known values.
	const readers = 8
	readErrs := make(chan string, readers)
	for range readers {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				rec := httptest.NewRecorder()
				mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/net", nil))
				got := rec.Header().Get("ETag")
				if got != etag1 && got != etag2 {
					select {
					case readErrs <- got:
					default:
					}
					return
				}
			}
		})
	}

	// Run for 100ms then signal stop.
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
	close(readErrs)

	for bad := range readErrs {
		t.Errorf("reader observed unexpected ETag = %q (not etag1=%q and not etag2=%q)", bad, etag1, etag2)
	}
}

// TestCaching_SkipPath verifies that paths passed to NewCachingState via the
// variadic skipPaths argument bypass the sync-time-keyed ETag entirely:
// they must receive Cache-Control: no-store, must NOT emit an ETag header,
// must reach the inner handler on every GET, and must NOT short-circuit to
// 304 even when the client sends If-None-Match: *.
//
// The regression this locks in: /ui/about contains wall-clock-relative
// rendering ("N minutes ago") that would freeze at cache-creation time
// under the sync-time ETag, misleading users for up to a full sync interval.
func TestCaching_SkipPath(t *testing.T) {
	t.Parallel()

	state := middleware.NewCachingState(time.Hour, "/ui/about")
	state.UpdateETag(time.Date(2026, 4, 11, 12, 27, 46, 0, time.UTC))

	var called atomic.Bool
	mw := state.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))

	// 1. GET on skipped path: no ETag, Cache-Control: no-store, handler runs.
	called.Store(false)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ui/about", nil))
	if !called.Load() {
		t.Error("inner handler not called on skipped path")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Errorf("ETag = %q, want empty on skipped path", got)
	}

	// 2. Skipped path must NOT 304 even with If-None-Match: * (the wildcard
	//    that matches any ETag). This is the core semantic: the skip list
	//    overrides the 304 short-circuit.
	called.Store(false)
	req := httptest.NewRequest(http.MethodGet, "/ui/about", nil)
	req.Header.Set("If-None-Match", "*")
	rec2 := httptest.NewRecorder()
	mw.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Errorf("status with If-None-Match:* = %d, want 200 on skipped path", rec2.Code)
	}
	if !called.Load() {
		t.Error("inner handler not called on skipped path with If-None-Match: *")
	}

	// 3. A non-skipped path on the same middleware still receives the normal
	//    ETag + Cache-Control treatment — the skip list is path-specific, not
	//    a global kill switch.
	called.Store(false)
	rec3 := httptest.NewRecorder()
	mw.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/ui/asn/13335", nil))
	if got := rec3.Header().Get("ETag"); got == "" {
		t.Error("non-skipped path: ETag header missing, want non-empty")
	}
	if got := rec3.Header().Get("Cache-Control"); got != "public, max-age=3720" {
		t.Errorf("non-skipped path: Cache-Control = %q, want %q", got, "public, max-age=3720")
	}
}
