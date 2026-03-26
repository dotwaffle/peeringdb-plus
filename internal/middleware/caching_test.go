package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

func TestCaching(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	syncInterval := time.Hour

	// Precompute the ETag for fixedTime so tests can reference it.
	cachingMW := middleware.Caching(middleware.CachingInput{
		SyncTimeFn:   func() time.Time { return fixedTime },
		SyncInterval: syncInterval,
	})
	// Make a GET request to capture the ETag value.
	probe := httptest.NewRequest(http.MethodGet, "/api/net", nil)
	probeRec := httptest.NewRecorder()
	cachingMW(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(probeRec, probe)
	currentETag := probeRec.Header().Get("ETag")

	tests := []struct {
		name            string
		method          string
		syncTime        time.Time
		ifNoneMatch     string
		wantStatus      int
		wantCacheCtrl   string
		wantETag        bool
		wantETagValue   string
		wantCalled      bool
		wantEmptyBody   bool
	}{
		{
			name:          "GET with valid sync time sets Cache-Control and ETag",
			method:        http.MethodGet,
			syncTime:      fixedTime,
			wantStatus:    http.StatusOK,
			wantCacheCtrl: "public, max-age=3720",
			wantETag:      true,
			wantCalled:    true,
		},
		{
			name:          "HEAD with valid sync time sets Cache-Control and ETag",
			method:        http.MethodHead,
			syncTime:      fixedTime,
			wantStatus:    http.StatusOK,
			wantCacheCtrl: "public, max-age=3720",
			wantETag:      true,
			wantCalled:    true,
		},
		{
			name:       "POST request has no caching headers",
			method:     http.MethodPost,
			syncTime:   fixedTime,
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "GET with zero sync time has no caching headers",
			method:     http.MethodGet,
			syncTime:   time.Time{},
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:          "GET with matching If-None-Match returns 304",
			method:        http.MethodGet,
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

			mw := middleware.Caching(middleware.CachingInput{
				SyncTimeFn:   func() time.Time { return tc.syncTime },
				SyncInterval: syncInterval,
			})
			handler := mw(inner)

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
			} else if tc.method == http.MethodPost || tc.syncTime.IsZero() {
				got := rec.Header().Get("Cache-Control")
				if got != "" {
					t.Errorf("Cache-Control = %q, want empty for %s", got, tc.method)
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

	mw := middleware.Caching(middleware.CachingInput{
		SyncTimeFn:   func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		SyncInterval: time.Hour,
	})
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mw(inner).ServeHTTP(rec, req)

	etag := rec.Header().Get("ETag")
	// Weak ETag format: W/"<32 hex chars>"
	re := regexp.MustCompile(`^W/"[0-9a-f]{32}"$`)
	if !re.MatchString(etag) {
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

	mw1 := middleware.Caching(middleware.CachingInput{
		SyncTimeFn:   func() time.Time { return time1 },
		SyncInterval: time.Hour,
	})
	mw2 := middleware.Caching(middleware.CachingInput{
		SyncTimeFn:   func() time.Time { return time2 },
		SyncInterval: time.Hour,
	})

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	mw1(inner).ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	mw2(inner).ServeHTTP(rec2, req2)

	etag1 := rec1.Header().Get("ETag")
	etag2 := rec2.Header().Get("ETag")

	if etag1 == etag2 {
		t.Errorf("ETags should differ for different sync times, both = %q", etag1)
	}
}
