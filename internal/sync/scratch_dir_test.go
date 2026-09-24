package sync

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
)

// sweepLogMsg is the summary message of sweepStaleScratchFiles.
const sweepLogMsg = "removed stale scratch files"

// newSweepLogger returns a DEBUG JSON logger that writes to a new
// logBuffer.
func newSweepLogger() (*slog.Logger, *logBuffer) {
	buf := &logBuffer{}
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

// writeScratchTestFile creates the file dir/name with size bytes.
func writeScratchTestFile(t *testing.T, dir, name string, size int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// setScratchDir sets the scratch dir of w to dir. The test cleanup
// releases the scratch dir lock of w, which a worker keeps until the
// process exits.
func setScratchDir(t *testing.T, w *Worker, dir string) {
	t.Helper()
	w.config.ScratchDir = dir
	t.Cleanup(func() { _ = w.scratchLock.close() })
}

// dirNames returns the sorted names of the entries in dir.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestOpenScratchDB_CreatesDir verifies that openScratchDB creates a
// missing scratch directory with mode 0700 and puts the file in it.
func TestOpenScratchDB_CreatesDir(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dir := filepath.Join(t.TempDir(), "volume", "scratch")

	s, err := openScratchDB(ctx, dir)
	if err != nil {
		t.Fatalf("openScratchDB: %v", err)
	}
	if got := filepath.Dir(s.path); got != dir {
		t.Errorf("scratch file dir = %q, want %q", got, dir)
	}
	if name := filepath.Base(s.path); !strings.HasPrefix(name, scratchFilePrefix) {
		t.Errorf("scratch file name = %q, want prefix %q", name, scratchFilePrefix)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat scratch dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("scratch dir mode = %v, want 0700", perm)
	}

	closeScratchDB(ctx, s, slog.Default())
	if names := dirNames(t, dir); len(names) != 0 {
		t.Errorf("scratch dir after close = %v, want empty", names)
	}
}

// TestOpenScratchDB_DirWithURISyntax verifies that openScratchDB opens
// the file that it created, with its pragmas, when the directory name
// holds characters that are syntax in a SQLite URI. Without escaping,
// '#' and '?' end the path and SQLite opens a different file, and '%'
// starts an escape.
func TestOpenScratchDB_DirWithURISyntax(t *testing.T) {
	t.Parallel()
	// The subtest names hold no URI syntax, so t.TempDir() does not add
	// it to parent. A regression then writes its file inside parent.
	for tc, name := range map[string]string{
		"hash":    "has#hash",
		"query":   "has?query",
		"percent": "pct%41",
		"space":   "has space",
	} {
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			parent := t.TempDir()
			dir := filepath.Join(parent, name)

			s, err := openScratchDB(ctx, dir)
			if err != nil {
				t.Fatalf("openScratchDB: %v", err)
			}
			if _, err := os.Stat(s.path); err != nil {
				t.Errorf("scratch file at %s: %v", s.path, err)
			}
			var mode string
			if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
				t.Fatalf("read journal_mode: %v", err)
			}
			if mode != "memory" {
				t.Errorf("journal_mode = %q, want memory", mode)
			}

			closeScratchDB(ctx, s, slog.Default())
			if got := dirNames(t, parent); !slices.Equal(got, []string{name}) {
				t.Errorf("parent dir after close = %v, want [%s]", got, name)
			}
			if got := dirNames(t, dir); len(got) != 0 {
				t.Errorf("scratch dir after close = %v, want empty", got)
			}
		})
	}
}

// TestSweepStaleScratchFiles verifies that the sweep removes only the
// regular files with the scratch prefix, reports their count and bytes,
// and logs a WARN only when it removed files.
func TestSweepStaleScratchFiles(t *testing.T) {
	t.Parallel()

	t.Run("removes only matching regular files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeScratchTestFile(t, dir, "pdbplus-sync-scratch-1.db", 100)
		writeScratchTestFile(t, dir, "pdbplus-sync-scratch-2.db-journal", 23)
		writeScratchTestFile(t, dir, "other.db", 7)
		sub := filepath.Join(dir, "pdbplus-sync-scratch-subdir")
		if err := os.Mkdir(sub, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeScratchTestFile(t, sub, "pdbplus-sync-scratch-3.db", 5)
		if err := os.Symlink("other.db", filepath.Join(dir, "pdbplus-sync-scratch-link.db")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		logger, buf := newSweepLogger()

		files, size := sweepStaleScratchFiles(t.Context(), dir, logger)

		if files != 2 || size != 123 {
			t.Errorf("sweep = %d files, %d bytes; want 2 files, 123 bytes", files, size)
		}
		want := []string{"other.db", "pdbplus-sync-scratch-link.db", "pdbplus-sync-scratch-subdir"}
		if got := dirNames(t, dir); !slices.Equal(got, want) {
			t.Errorf("dir after sweep = %v, want %v", got, want)
		}
		if got := dirNames(t, sub); len(got) != 1 {
			t.Errorf("subdir after sweep = %v, want its file kept", got)
		}
		logs := buf.records(t, sweepLogMsg)
		if len(logs) != 1 || logs[0]["level"] != "WARN" || logs[0]["dir"] != dir ||
			logs[0]["count"] != float64(2) || logs[0]["bytes"] != float64(123) {
			t.Errorf("sweep log = %v, want one WARN record with dir, count 2, bytes 123", logs)
		}
	})

	t.Run("nothing to remove logs debug", func(t *testing.T) {
		t.Parallel()
		for name, dir := range map[string]string{
			"empty dir":   t.TempDir(),
			"missing dir": filepath.Join(t.TempDir(), "missing"),
		} {
			logger, buf := newSweepLogger()
			if files, size := sweepStaleScratchFiles(t.Context(), dir, logger); files != 0 || size != 0 {
				t.Errorf("%s: sweep = %d files, %d bytes; want 0, 0", name, files, size)
			}
			logs := buf.records(t, sweepLogMsg)
			if len(logs) != 1 || logs[0]["level"] != "DEBUG" || logs[0]["count"] != float64(0) {
				t.Errorf("%s: sweep log = %v, want one DEBUG record with count 0", name, logs)
			}
		}
	})

	t.Run("unreadable dir logs warn", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "not-a-dir")
		writeScratchTestFile(t, filepath.Dir(dir), filepath.Base(dir), 1)
		logger, buf := newSweepLogger()

		if files, size := sweepStaleScratchFiles(t.Context(), dir, logger); files != 0 || size != 0 {
			t.Errorf("sweep = %d files, %d bytes; want 0, 0", files, size)
		}
		logs := buf.records(t, "failed to sweep stale scratch files")
		if len(logs) != 1 || logs[0]["level"] != "WARN" || logs[0]["dir"] != dir {
			t.Errorf("error log = %v, want one WARN record with dir", logs)
		}
	})
}

// TestSweepStaleScratchFiles_EmptyDirSkipsTempDir verifies that an empty
// scratch dir setting sweeps nothing. The default scratch directory is
// os.TempDir(), which other processes share. Not parallel: it sets
// TMPDIR.
func TestSweepStaleScratchFiles_EmptyDirSkipsTempDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	writeScratchTestFile(t, tmp, "pdbplus-sync-scratch-other-process.db", 10)
	logger, buf := newSweepLogger()

	if files, size := sweepStaleScratchFiles(t.Context(), "", logger); files != 0 || size != 0 {
		t.Errorf("sweep = %d files, %d bytes; want 0, 0", files, size)
	}
	if got := dirNames(t, tmp); len(got) != 1 {
		t.Errorf("temp dir after sweep = %v, want the file kept", got)
	}
	if n := buf.buf.Len(); n != 0 {
		t.Errorf("sweep logged %d bytes, want nothing", n)
	}
}

// TestStartScheduler_SweepsScratchDir verifies that StartScheduler
// removes stale scratch files from the configured directory on a primary
// before the first cycle, and that a replica keeps them.
func TestStartScheduler_SweepsScratchDir(t *testing.T) {
	t.Parallel()

	t.Run("primary", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS != "linux" {
			t.Skip("the startup sweep needs the scratch dir lock, which uses Linux flock")
		}
		f := newFixture(t)
		w, db := newTestWorker(t, f)
		var buf *logBuffer
		w.logger, buf = newSweepLogger()
		dir := t.TempDir()
		setScratchDir(t, w, dir)
		writeScratchTestFile(t, dir, "pdbplus-sync-scratch-crashed.db", 64)
		writeScratchTestFile(t, dir, "keep.txt", 1)

		now := time.Now()
		id, err := RecordSyncStart(t.Context(), db, now.Add(-10*time.Minute), "incremental")
		if err != nil {
			t.Fatalf("record sync start: %v", err)
		}
		if err := RecordSyncComplete(t.Context(), db, id, Status{
			LastSyncAt: now.Add(-10 * time.Minute), Duration: time.Second, Status: "success",
		}); err != nil {
			t.Fatalf("record sync complete: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan struct{})
		go func() {
			w.StartScheduler(ctx, time.Hour)
			close(done)
		}()

		// The sweep sends no completion event, so wait until its log
		// record appears. The deadline only bounds a broken sweep.
		deadline := time.NewTimer(60 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for len(buf.records(t, sweepLogMsg)) == 0 {
			select {
			case <-tick.C:
			case <-done:
				t.Fatal("scheduler exited before the startup scratch sweep")
			case <-deadline.C:
				cancel()
				<-done
				t.Fatal("startup scratch sweep did not run before the deadline")
			}
		}
		cancel()
		<-done

		if got, want := dirNames(t, dir), []string{scratchLockName, "keep.txt"}; !slices.Equal(got, want) {
			t.Errorf("scratch dir after start = %v, want %v", got, want)
		}
		if calls := f.callCount.Load(); calls != 0 {
			t.Errorf("got %d upstream calls, want 0 (no sync cycle is due)", calls)
		}
		if logs := buf.records(t, sweepLogMsg); len(logs) != 1 || logs[0]["level"] != "WARN" || logs[0]["count"] != float64(1) {
			t.Errorf("sweep log = %v, want one WARN record with count 1", logs)
		}
		if w.Running() {
			t.Error("running latch still held after the startup sweep")
		}
	})

	t.Run("replica", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		w, _ := newTestWorker(t, f)
		w.config.IsPrimary = func() bool { return false }
		dir := t.TempDir()
		setScratchDir(t, w, dir)
		writeScratchTestFile(t, dir, "pdbplus-sync-scratch-crashed.db", 64)

		// A done ctx makes a replica scheduler return after its startup
		// steps, so the call is synchronous.
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		w.StartScheduler(ctx, time.Hour)

		if got := dirNames(t, dir); len(got) != 1 {
			t.Errorf("scratch dir on a replica = %v, want the file kept", got)
		}
	})
}

// TestSweepScratchDirAtStartup_SkipsWhileSyncRuns verifies that the
// startup sweep does not remove the live file of a running cycle. The
// cycle holds the running latch.
func TestSweepScratchDirAtStartup_SkipsWhileSyncRuns(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	w, _ := newTestWorker(t, f)
	var buf *logBuffer
	w.logger, buf = newSweepLogger()
	dir := t.TempDir()
	setScratchDir(t, w, dir)
	writeScratchTestFile(t, dir, "pdbplus-sync-scratch-live.db", 64)

	w.running.Store(true)
	w.sweepScratchDirAtStartup(t.Context())

	if !w.running.Load() {
		t.Error("the skipped startup sweep released the latch of the running cycle")
	}
	if got := dirNames(t, dir); !slices.Equal(got, []string{"pdbplus-sync-scratch-live.db"}) {
		t.Errorf("scratch dir = %v, want the live file kept", got)
	}
	if logs := buf.records(t, "sync cycle running, skipping startup scratch sweep"); len(logs) != 1 || logs[0]["level"] != "DEBUG" {
		t.Errorf("skip log = %v, want one DEBUG record", logs)
	}
	if logs := buf.records(t, sweepLogMsg); len(logs) != 0 {
		t.Errorf("sweep log = %v, want none", logs)
	}
}

// TestSync_UsesScratchDir verifies that a sync cycle stages its scratch
// database in the configured directory and removes the file at the end.
// The lock file stays.
func TestSync_UsesScratchDir(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, _ := newTestWorker(t, f)
	dir := filepath.Join(t.TempDir(), "scratch")
	setScratchDir(t, w, dir)

	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}
	// The cycle created the missing directory for its scratch file.
	names := slices.DeleteFunc(dirNames(t, dir), func(n string) bool { return n == scratchLockName })
	if len(names) != 0 {
		t.Errorf("scratch dir after sync = %v, want only the lock file", names)
	}
}
