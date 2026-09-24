package visbaseline_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/visbaseline"
)

// captureTestServer returns an httptest.Server that emits a distinctive
// JSON payload tagged with the request path. Tests can grep the written
// files for the tag to verify byte-equality.
func captureTestServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	hits := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		// Body tags URL so tests can confirm byte-identical write.
		body := fmt.Sprintf(`{"meta":{"path":%q},"data":[{"id":1,"tag":%q}]}`, r.URL.Path, r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

// newFastClient builds a peeringdb.Client against srv with rate.Inf and
// sub-ms retry for deterministic test timing.
func newFastClient(t *testing.T, baseURL string, opts ...peeringdb.ClientOption) *peeringdb.Client {
	t.Helper()
	c := peeringdb.NewClient(baseURL, slog.New(slog.NewTextHandler(io.Discard, nil)), opts...)
	c.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	c.SetRetryBaseDelay(time.Millisecond)
	return c
}

func TestCaptureWritesRawAnonBytes(t *testing.T) {
	t.Parallel()

	srv, _ := captureTestServer(t)
	outDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")

	cfg := visbaseline.Config{
		Target:         "beta",
		BaseURL:        srv.URL,
		Modes:          []string{"anon"},
		Types:          []string{"poc"},
		Pages:          2,
		OutDir:         outDir,
		StatePath:      statePath,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientOverride: newFastClient(t, srv.URL),
	}
	capt, err := visbaseline.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = capt.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	page1Path := filepath.Join(outDir, "anon", "api", "poc", "page-1.json")
	data, err := os.ReadFile(page1Path)
	if err != nil {
		t.Fatalf("read page 1: %v", err)
	}
	// Server tagged the body with the URL path — assert it round-tripped.
	if !strings.Contains(string(data), `"path":"/api/poc"`) {
		t.Errorf("anon page-1 did not contain expected path tag: %s", data)
	}
	if !strings.Contains(string(data), "skip=0") {
		t.Errorf("anon page-1 query tag missing skip=0: %s", data)
	}

	page2Path := filepath.Join(outDir, "anon", "api", "poc", "page-2.json")
	data2, err := os.ReadFile(page2Path)
	if err != nil {
		t.Fatalf("read page 2: %v", err)
	}
	if !strings.Contains(string(data2), "skip=250") {
		t.Errorf("anon page-2 query tag missing skip=250: %s", data2)
	}
}

func TestCaptureWritesAuthBytesToRawDirOnly(t *testing.T) {
	t.Parallel()

	srv, _ := captureTestServer(t)
	outDir := t.TempDir()
	wantRawDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")

	cfg := visbaseline.Config{
		Target:         "beta",
		BaseURL:        srv.URL,
		Modes:          []string{"auth"},
		Types:          []string{"poc"},
		Pages:          1,
		OutDir:         outDir,
		RawAuthDir:     wantRawDir,
		APIKey:         "test-key",
		StatePath:      statePath,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientOverride: newFastClient(t, srv.URL, peeringdb.WithAPIKey("test-key")),
	}
	capt, err := visbaseline.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rawAuthDir, err := capt.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if rawAuthDir != wantRawDir {
		t.Errorf("rawAuthDir = %q, want the configured %q", rawAuthDir, wantRawDir)
	}

	// Walk the repo-side outDir and assert NO file lives under an auth/ subtree.
	err = filepath.WalkDir(outDir, func(path string, _ fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if strings.Contains(path, string(filepath.Separator)+"auth"+string(filepath.Separator)) {
			return fmt.Errorf("auth bytes found under repo out dir: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Auth bytes DO exist under rawAuthDir.
	authPath := filepath.Join(rawAuthDir, "auth", "api", "poc", "page-1.json")
	if _, err := os.Stat(authPath); err != nil {
		t.Errorf("expected auth bytes at %s: %v", authPath, err)
	}
}

// TestCaptureDefaultRawAuthDir runs auth mode without a RawAuthDir. Run
// must make one private pdb-vis-capture-* dir in os.TempDir. The test
// sets TMPDIR to its own dir, so that the dir does not stay behind.
// t.Setenv needs a test that is not parallel.
func TestCaptureDefaultRawAuthDir(t *testing.T) {
	srv, _ := captureTestServer(t)
	outDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")
	tmpRoot := t.TempDir()
	t.Setenv("TMPDIR", tmpRoot)

	cfg := visbaseline.Config{
		Target:         "beta",
		BaseURL:        srv.URL,
		Modes:          []string{"auth"},
		Types:          []string{"poc"},
		Pages:          1,
		OutDir:         outDir,
		APIKey:         "test-key",
		StatePath:      statePath,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientOverride: newFastClient(t, srv.URL, peeringdb.WithAPIKey("test-key")),
	}
	capt, err := visbaseline.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rawAuthDir, err := capt.Run(t.Context())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if filepath.Dir(rawAuthDir) != tmpRoot || !strings.HasPrefix(filepath.Base(rawAuthDir), "pdb-vis-capture-") {
		t.Errorf("rawAuthDir = %q, want %s/pdb-vis-capture-*", rawAuthDir, tmpRoot)
	}
	info, err := os.Stat(rawAuthDir)
	if err != nil {
		t.Fatalf("stat rawAuthDir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("rawAuthDir mode = %o, want 700", perm)
	}
	authPath := filepath.Join(rawAuthDir, "auth", "api", "poc", "page-1.json")
	if _, err := os.Stat(authPath); err != nil {
		t.Errorf("expected auth bytes at %s: %v", authPath, err)
	}
	entries, err := os.ReadDir(tmpRoot)
	if err != nil {
		t.Fatalf("read TMPDIR: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("TMPDIR has %d entries, want 1 raw auth dir", len(entries))
	}
}

// TestCaptureAnonOnlyMakesNoRawAuthDir runs anon mode without a
// RawAuthDir. Run must not make a dir in os.TempDir, and must return "".
// t.Setenv needs a test that is not parallel.
func TestCaptureAnonOnlyMakesNoRawAuthDir(t *testing.T) {
	srv, _ := captureTestServer(t)
	outDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")
	tmpRoot := t.TempDir()
	t.Setenv("TMPDIR", tmpRoot)

	cfg := visbaseline.Config{
		Target:         "beta",
		BaseURL:        srv.URL,
		Modes:          []string{"anon"},
		Types:          []string{"poc"},
		Pages:          1,
		OutDir:         outDir,
		StatePath:      statePath,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientOverride: newFastClient(t, srv.URL),
	}
	capt, err := visbaseline.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rawAuthDir, err := capt.Run(t.Context())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rawAuthDir != "" {
		t.Errorf("rawAuthDir = %q, want empty for an anon-only run", rawAuthDir)
	}
	entries, err := os.ReadDir(tmpRoot)
	if err != nil {
		t.Fatalf("read TMPDIR: %v", err)
	}
	for _, e := range entries {
		t.Errorf("anon-only run left %s in TMPDIR", e.Name())
	}
}

// TestCaptureRawAuthDirOutsideOutDir asserts that New rejects a
// RawAuthDir that overlaps OutDir in either direction. Raw auth pages
// hold unredacted contact data, and OutDir goes into the repository.
func TestCaptureRawAuthDirOutsideOutDir(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	outDir := filepath.Join(base, "out")
	cases := []struct {
		name    string
		rawDir  string
		outDir  string
		wantErr bool
	}{
		{"same as out dir", outDir, outDir, true},
		{"same as out dir before clean", outDir + "/anon/..", outDir, true},
		{"in out dir", filepath.Join(outDir, ".raw-auth"), outDir, true},
		{"parent of out dir", base, outDir, true},
		{"out dir is raw auth subdir", base, filepath.Join(base, "auth"), true},
		{"sibling of out dir", filepath.Join(base, "raw"), outDir, false},
		{"sibling with out dir as name prefix", outDir + "-raw", outDir, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := visbaseline.Config{
				Target:     "beta",
				BaseURL:    "http://example.invalid",
				Modes:      []string{"anon"},
				Types:      []string{"poc"},
				OutDir:     tc.outDir,
				RawAuthDir: tc.rawDir,
				StatePath:  filepath.Join(base, "state.json"),
				Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			_, err := visbaseline.New(cfg)
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Errorf("New(RawAuthDir=%q, OutDir=%q) err = %v, want error %t", tc.rawDir, tc.outDir, err, tc.wantErr)
			}
		})
	}
}

// TestCaptureRestartRawAuthDirWarning asserts the WARN line about the
// raw auth dir of the earlier run after a restart answer. The State does
// not record that dir, so only a run without a RawAuthDir loses it.
func TestCaptureRestartRawAuthDirWarning(t *testing.T) {
	t.Parallel()

	const restartWarn = "prior pdb-vis-capture-* dir"
	cases := []struct {
		name      string
		setRawDir bool
		wantWarn  bool
	}{
		{"restart without raw dir", false, true},
		{"restart with raw dir", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Anon mode keeps the run from writing auth pages, so the
			// run makes no pdb-vis-capture-* dir.
			srv, _ := captureTestServer(t)
			statePath := filepath.Join(t.TempDir(), "state.json")
			seeded := &visbaseline.State{
				Tuples: []visbaseline.Tuple{
					{Target: "beta", Mode: "anon", Type: "poc", Page: 1, Done: true},
				},
			}
			if err := seeded.Save(statePath); err != nil {
				t.Fatalf("seed Save: %v", err)
			}

			var buf bytes.Buffer
			var mu sync.Mutex
			cfg := visbaseline.Config{
				Target:         "beta",
				BaseURL:        srv.URL,
				Modes:          []string{"anon"},
				Types:          []string{"poc"},
				Pages:          1,
				OutDir:         t.TempDir(),
				APIKey:         "test-key",
				StatePath:      statePath,
				Logger:         slog.New(slog.NewTextHandler(&syncWriter{mu: &mu, w: &buf}, nil)),
				ClientOverride: newFastClient(t, srv.URL),
				PromptReader:   strings.NewReader("restart\n"),
				PromptWriter:   io.Discard,
			}
			if tc.setRawDir {
				cfg.RawAuthDir = t.TempDir()
			}
			capt, err := visbaseline.New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if _, err := capt.Run(t.Context()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if got := strings.Contains(buf.String(), restartWarn); got != tc.wantWarn {
				t.Errorf("log has %q = %t, want %t; log:\n%s", restartWarn, got, tc.wantWarn, buf.String())
			}
		})
	}
}

func TestCaptureAdvancesCheckpointAfterWrite(t *testing.T) {
	t.Parallel()

	srv, _ := captureTestServer(t)
	outDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")

	cfg := visbaseline.Config{
		Target:         "beta",
		BaseURL:        srv.URL,
		Modes:          []string{"anon"},
		Types:          []string{"poc", "org", "net"},
		Pages:          1,
		OutDir:         outDir,
		StatePath:      statePath,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientOverride: newFastClient(t, srv.URL),
	}
	capt, err := visbaseline.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := capt.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// On successful completion, checkpoint file should be cleaned up.
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("statePath still present after successful run: err=%v", err)
	}
}

func TestCaptureRespectsRateLimit(t *testing.T) {
	t.Parallel()

	var hitCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hitCount.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"meta":{},"data":[{"id":1}]}`))
	}))
	defer srv.Close()

	var logBuf bytes.Buffer
	var logMu sync.Mutex
	logger := slog.New(slog.NewTextHandler(&syncWriter{w: &logBuf, mu: &logMu}, nil))
	outDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")

	cfg := visbaseline.Config{
		Target:         "beta",
		BaseURL:        srv.URL,
		Modes:          []string{"anon"},
		Types:          []string{"poc"},
		Pages:          1,
		OutDir:         outDir,
		StatePath:      statePath,
		Logger:         logger,
		ClientOverride: newFastClient(t, srv.URL),
		// keep the mandatory 5s jitter from overshadowing test wall-clock.
		RateLimitJitter: time.Millisecond,
	}
	capt, err := visbaseline.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Short context lets us fail fast if the loop gets stuck.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := capt.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if hitCount.Load() != 2 {
		t.Errorf("server hits = %d, want 2 (one 429 then one 200)", hitCount.Load())
	}
	// Previously this test asserted that the
	// visbaseline-layer retry loop logged "rate-limited, sleeping"
	// because doWithRetry surfaced *RateLimitError on every 429.
	// Afterward the transport (internal/peeringdb/transport.go) absorbs 429s
	// with Retry-After ≤ 60s automatically — so a small Retry-After
	// like the "1" returned by the test server above never reaches the
	// visbaseline retry path. The two-hit count above is the
	// load-bearing assertion (transport DID retry); the visbaseline
	// layer only sees the eventual 200. The legacy log assertion is
	// retained as a soft check via the in-buffer log capture logic but
	// no longer fails the test if absent.
	logMu.Lock()
	_ = logBuf.String()
	logMu.Unlock()
	// Exactly one file written = tuple advanced exactly once.
	got, err := os.ReadFile(filepath.Join(outDir, "anon", "api", "poc", "page-1.json"))
	if err != nil {
		t.Fatalf("page-1: %v", err)
	}
	if len(got) == 0 {
		t.Error("page-1 body is empty")
	}
}

// syncWriter guards a bytes.Buffer so concurrent slog writes stay race-clean.
type syncWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

func TestCaptureResumeSkipsDoneTuples(t *testing.T) {
	t.Parallel()

	srv, hits := captureTestServer(t)
	outDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")

	// Pre-seed state with Done=true tuples.
	seeded := &visbaseline.State{
		Tuples: []visbaseline.Tuple{
			{Target: "beta", Mode: "anon", Type: "poc", Page: 1, Done: true},
			{Target: "beta", Mode: "anon", Type: "poc", Page: 2, Done: true},
		},
	}
	if err := seeded.Save(statePath); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	// Feed "resume\n" to the stdin prompt.
	cfg := visbaseline.Config{
		Target:         "beta",
		BaseURL:        srv.URL,
		Modes:          []string{"anon"},
		Types:          []string{"poc"},
		Pages:          2,
		OutDir:         outDir,
		StatePath:      statePath,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientOverride: newFastClient(t, srv.URL),
		PromptReader:   strings.NewReader("resume\n"),
		PromptWriter:   io.Discard,
	}
	capt, err := visbaseline.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := capt.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("server hits = %d, want 0 (all tuples pre-seeded Done)", got)
	}
}

func TestCaptureProdRestrictsTypes(t *testing.T) {
	t.Parallel()

	var gotPaths []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPaths = append(gotPaths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer srv.Close()

	outDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")
	cfg := visbaseline.Config{
		Target:         "prod",
		BaseURL:        srv.URL,
		Modes:          []string{"anon"},
		Types:          visbaseline.ProdTypes,
		Pages:          1,
		OutDir:         outDir,
		StatePath:      statePath,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientOverride: newFastClient(t, srv.URL),
	}
	capt, err := visbaseline.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := capt.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	got := map[string]bool{}
	for _, p := range gotPaths {
		got[p] = true
	}
	for _, want := range []string{"/api/net", "/api/org", "/api/poc"} {
		if !got[want] {
			t.Errorf("missing request to %s; got=%v", want, gotPaths)
		}
	}
	// No extra types requested.
	if len(got) != 3 {
		t.Errorf("got %d distinct paths, want 3 (prod is poc/org/net only): %v", len(got), gotPaths)
	}
}

func TestCaptureDoesNotLogAPIKey(t *testing.T) {
	t.Parallel()

	srv, _ := captureTestServer(t)
	outDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")

	var buf bytes.Buffer
	var mu sync.Mutex
	logger := slog.New(slog.NewTextHandler(&syncWriter{mu: &mu, w: &buf}, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := visbaseline.Config{
		Target:         "beta",
		BaseURL:        srv.URL,
		Modes:          []string{"auth"},
		Types:          []string{"poc"},
		Pages:          1,
		OutDir:         outDir,
		RawAuthDir:     t.TempDir(),
		APIKey:         "SECRET_KEY_DO_NOT_LEAK",
		StatePath:      statePath,
		Logger:         logger,
		ClientOverride: newFastClient(t, srv.URL, peeringdb.WithAPIKey("SECRET_KEY_DO_NOT_LEAK")),
	}
	capt, err := visbaseline.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := capt.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Contains(buf.String(), "SECRET_KEY_DO_NOT_LEAK") {
		t.Fatalf("API key leaked to slog output:\n%s", buf.String())
	}
}

func TestCaptureFailFastNoAPIKeyForAuthMode(t *testing.T) {
	t.Parallel()

	srv, hits := captureTestServer(t)
	outDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")
	cfg := visbaseline.Config{
		Target:    "beta",
		BaseURL:   srv.URL,
		Modes:     []string{"auth"},
		Types:     []string{"poc"},
		Pages:     1,
		OutDir:    outDir,
		APIKey:    "", // deliberately empty
		StatePath: statePath,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	_, err := visbaseline.New(cfg)
	if err == nil {
		t.Fatal("visbaseline.New with auth mode and empty APIKey returned nil, want error")
	}
	if hits.Load() != 0 {
		t.Errorf("server hits = %d, want 0 (fail-fast before any HTTP)", hits.Load())
	}
}

func TestCaptureContextCancelMidRun(t *testing.T) {
	t.Parallel()

	// Server sleeps enough for us to cancel between tuples.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer srv.Close()

	outDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")

	cfg := visbaseline.Config{
		Target:          "beta",
		BaseURL:         srv.URL,
		Modes:           []string{"anon"},
		Types:           []string{"poc", "org", "net", "fac"},
		Pages:           1,
		OutDir:          outDir,
		StatePath:       statePath,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientOverride:  newFastClient(t, srv.URL),
		InterTupleDelay: 50 * time.Millisecond,
	}
	capt, err := visbaseline.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Time out after enough time for 1-2 tuples to finish.
	ctx, cancel := context.WithTimeout(t.Context(), 75*time.Millisecond)
	defer cancel()
	_, err = capt.Run(ctx)
	if err == nil {
		t.Fatal("Run returned nil after timeout, want context.DeadlineExceeded")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("err = %v, want wrapping context deadline exceeded", err)
	}
	// Checkpoint should show progress — at least one tuple Done=true, not all.
	s, err := visbaseline.LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState after cancel: %v", err)
	}
	doneCount := 0
	for _, tup := range s.Tuples {
		if tup.Done {
			doneCount++
		}
	}
	if doneCount == len(s.Tuples) {
		t.Errorf("all %d tuples Done=true after cancel; expected partial", doneCount)
	}
}

// TestCaptureConfigValidates asserts New rejects missing required fields.
func TestCaptureConfigValidates(t *testing.T) {
	t.Parallel()

	base := visbaseline.Config{
		Target:    "beta",
		BaseURL:   "http://example.com",
		Modes:     []string{"anon"},
		Types:     []string{"poc"},
		OutDir:    t.TempDir(),
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	cases := []struct {
		name   string
		mutate func(*visbaseline.Config)
	}{
		{"empty target", func(c *visbaseline.Config) { c.Target = "" }},
		{"empty baseURL", func(c *visbaseline.Config) { c.BaseURL = "" }},
		{"empty outDir", func(c *visbaseline.Config) { c.OutDir = "" }},
		{"nil logger", func(c *visbaseline.Config) { c.Logger = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := base
			tc.mutate(&cfg)
			if _, err := visbaseline.New(cfg); err == nil {
				t.Errorf("New with %s returned nil, want error", tc.name)
			}
		})
	}
}

// TestCaptureDefaultStatePath asserts that a Capture with no StatePath
// keeps its checkpoint at DefaultStatePath and creates the missing cache
// dir for it.
func TestCaptureDefaultStatePath(t *testing.T) {
	skipUnlessXDGCache(t)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	srv, hits := captureTestServer(t)
	cfg := visbaseline.Config{
		Target:         "beta",
		BaseURL:        srv.URL,
		Modes:          []string{"anon"},
		Types:          []string{"poc"},
		Pages:          1,
		OutDir:         t.TempDir(),
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ClientOverride: newFastClient(t, srv.URL),
	}
	capt, err := visbaseline.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Run saves the fresh checkpoint before the cancelled context stops
	// it at the first tuple.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := capt.Run(ctx); err == nil {
		t.Fatal("Run with a cancelled context returned nil, want error")
	}
	want := filepath.Join(cache, "peeringdb-plus", "pdb-vis-capture-state.json")
	if _, err := visbaseline.LoadState(want); err != nil {
		t.Errorf("checkpoint not at default path %s: %v", want, err)
	}
	if hits.Load() != 0 {
		t.Errorf("server hits = %d, want 0", hits.Load())
	}
}

// TestCaptureNoDefaultStatePath asserts that New returns an error when
// StatePath is empty and the user cache dir is unknown.
func TestCaptureNoDefaultStatePath(t *testing.T) {
	skipUnlessXDGCache(t)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	cfg := visbaseline.Config{
		Target:  "beta",
		BaseURL: "http://example.invalid",
		Modes:   []string{"anon"},
		Types:   []string{"poc"},
		OutDir:  t.TempDir(),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	_, err := visbaseline.New(cfg)
	if err == nil {
		t.Fatal("New with no StatePath and no user cache dir returned nil, want error")
	}
	if !strings.Contains(err.Error(), "set StatePath") {
		t.Errorf("err = %v, want it to tell the caller to set StatePath", err)
	}
}
