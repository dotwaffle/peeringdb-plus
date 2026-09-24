//go:build linux

package sync

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
)

// holdScratchLock opens the lock file of dir on a new open file
// description and applies how (LOCK_SH or LOCK_EX) to it. A flock lock
// belongs to the open file description, so this lock conflicts with the
// lock of a Worker in the same process, as the lock of another process
// does. Close the returned file to release the lock. The test cleanup
// also closes it.
func holdScratchLock(t *testing.T, dir string, how int) *os.File {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, scratchLockName), os.O_RDONLY|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB); err != nil {
		t.Fatalf("flock lock file: %v", err)
	}
	return f
}

// canLockScratchDir reports whether a new open file description gets
// the lock how on the lock file of dir. The lock is released before
// canLockScratchDir returns.
func canLockScratchDir(t *testing.T, dir string, how int) bool {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, scratchLockName))
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer func() { _ = f.Close() }()
	err = syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false
	}
	if err != nil {
		t.Fatalf("flock lock file: %v", err)
	}
	return true
}

// TestSweepScratchDirAtStartup_Lock verifies that the startup sweep runs
// only when the worker gets the exclusive lock of the scratch dir, and
// that the worker then keeps a shared lock. When another process holds a
// lock, the sweep keeps the stale-named file, which can be the live file
// of that process.
func TestSweepScratchDirAtStartup_Lock(t *testing.T) {
	t.Parallel()
	const stale = "pdbplus-sync-scratch-other.db"

	tests := []struct {
		name  string
		other int // lock of the other process, 0 for none
		// wantSwept is true when the sweep removes the stale file.
		wantSwept bool
		// wantShared is true when the worker holds a shared lock after
		// the other process releases its lock.
		wantShared bool
	}{
		{name: "no other lock", other: 0, wantSwept: true, wantShared: true},
		{name: "other holds shared lock", other: syscall.LOCK_SH, wantSwept: false, wantShared: true},
		{name: "other holds exclusive lock", other: syscall.LOCK_EX, wantSwept: false, wantShared: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			w, _ := newTestWorker(t, f)
			var buf *logBuffer
			w.logger, buf = newSweepLogger()
			dir := t.TempDir()
			setScratchDir(t, w, dir)
			writeScratchTestFile(t, dir, stale, 64)
			var other *os.File
			if tt.other != 0 {
				other = holdScratchLock(t, dir, tt.other)
			}

			w.sweepScratchDirAtStartup(t.Context())

			want := []string{scratchLockName}
			inUse := buf.records(t, "scratch dir in use by another process, skipping stale file sweep")
			if tt.wantSwept {
				if len(inUse) != 0 {
					t.Errorf("in-use log = %v, want none", inUse)
				}
			} else {
				want = append(want, stale)
				if len(inUse) != 1 || inUse[0]["level"] != "WARN" || inUse[0]["dir"] != dir {
					t.Errorf("in-use log = %v, want one WARN record with dir", inUse)
				}
				if logs := buf.records(t, sweepLogMsg); len(logs) != 0 {
					t.Errorf("sweep log = %v, want none", logs)
				}
			}
			if got := dirNames(t, dir); !slices.Equal(got, want) {
				t.Errorf("scratch dir after the startup sweep = %v, want %v", got, want)
			}
			if w.Running() {
				t.Error("running latch still held after the startup sweep")
			}

			if other != nil {
				_ = other.Close()
			}
			// The worker keeps no exclusive lock after the sweep.
			if !canLockScratchDir(t, dir, syscall.LOCK_SH) {
				t.Error("another process cannot take a shared lock, want the worker to hold no exclusive lock")
			}
			if got := !canLockScratchDir(t, dir, syscall.LOCK_EX); got != tt.wantShared {
				t.Errorf("worker holds a shared lock = %v, want %v", got, tt.wantShared)
			}
		})
	}
}

// TestSweepScratchDirAtStartup_LockError verifies that the startup sweep
// is skipped with a WARN when the lock file cannot be opened.
func TestSweepScratchDirAtStartup_LockError(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	w, _ := newTestWorker(t, f)
	var buf *logBuffer
	w.logger, buf = newSweepLogger()
	// A regular file in place of the directory makes the lock fail.
	dir := filepath.Join(t.TempDir(), "not-a-dir")
	writeScratchTestFile(t, filepath.Dir(dir), filepath.Base(dir), 1)
	setScratchDir(t, w, dir)

	w.sweepScratchDirAtStartup(t.Context())

	logs := buf.records(t, "failed to lock scratch dir, skipping stale file sweep")
	if len(logs) != 1 || logs[0]["level"] != "WARN" || logs[0]["dir"] != dir || logs[0]["error"] == nil {
		t.Errorf("lock error log = %v, want one WARN record with dir and error", logs)
	}
	if logs := buf.records(t, sweepLogMsg); len(logs) != 0 {
		t.Errorf("sweep log = %v, want none", logs)
	}
	if w.Running() {
		t.Error("running latch still held after the startup sweep")
	}
}

// TestScratchDirForCycle_Lock verifies that a cycle stages in the set
// scratch dir only with a shared lock of the worker, and stages in the
// temp dir with a WARN when another process holds the exclusive lock.
func TestScratchDirForCycle_Lock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		other int // lock of the other process, 0 for none
		// wantDir is true when the cycle uses the set dir, false when it
		// uses the temp dir.
		wantDir bool
	}{
		{name: "no other lock", other: 0, wantDir: true},
		{name: "other holds shared lock", other: syscall.LOCK_SH, wantDir: true},
		{name: "other holds exclusive lock", other: syscall.LOCK_EX, wantDir: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			w, _ := newTestWorker(t, f)
			var buf *logBuffer
			w.logger, buf = newSweepLogger()
			dir := t.TempDir()
			setScratchDir(t, w, dir)
			var other *os.File
			if tt.other != 0 {
				other = holdScratchLock(t, dir, tt.other)
			}

			got := w.scratchDirForCycle(t.Context())

			logs := buf.records(t, "failed to lock scratch dir, staging in the temp dir")
			if tt.wantDir {
				if got != dir {
					t.Errorf("scratchDirForCycle = %q, want %q", got, dir)
				}
				if len(logs) != 0 {
					t.Errorf("lock log = %v, want none", logs)
				}
			} else {
				if got != "" {
					t.Errorf("scratchDirForCycle = %q, want \"\" (the temp dir)", got)
				}
				if len(logs) != 1 || logs[0]["level"] != "WARN" || logs[0]["dir"] != dir || logs[0]["error"] == nil {
					t.Errorf("lock log = %v, want one WARN record with dir and error", logs)
				}
			}
			if other != nil {
				_ = other.Close()
			}
			if held := !canLockScratchDir(t, dir, syscall.LOCK_EX); held != tt.wantDir {
				t.Errorf("worker holds a shared lock = %v, want %v", held, tt.wantDir)
			}
		})
	}
}

// TestSweepScratchDirAtStartup_LockFileFIFO verifies that the startup
// sweep does not wait when the lock file name is a FIFO. An open(2) of a
// FIFO without O_NONBLOCK waits for a writer, and the sweep holds the
// running latch. The sweep is skipped with a WARN and releases the
// latch.
func TestSweepScratchDirAtStartup_LockFileFIFO(t *testing.T) {
	t.Parallel()
	const stale = "pdbplus-sync-scratch-stale.db"
	f := newFixture(t)
	w, _ := newTestWorker(t, f)
	var buf *logBuffer
	w.logger, buf = newSweepLogger()
	dir := t.TempDir()
	setScratchDir(t, w, dir)
	fifo := filepath.Join(dir, scratchLockName)
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	writeScratchTestFile(t, dir, stale, 64)

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.sweepScratchDirAtStartup(t.Context())
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// A writer lets the blocked open return, so the goroutine ends.
		if wf, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
			_ = wf.Close()
		}
		<-done
		t.Fatal("startup sweep blocked on the FIFO lock file")
	}

	logs := buf.records(t, "failed to lock scratch dir, skipping stale file sweep")
	if len(logs) != 1 || logs[0]["level"] != "WARN" || logs[0]["dir"] != dir || logs[0]["error"] == nil {
		t.Errorf("lock error log = %v, want one WARN record with dir and error", logs)
	}
	if got, want := dirNames(t, dir), []string{scratchLockName, stale}; !slices.Equal(got, want) {
		t.Errorf("scratch dir after the startup sweep = %v, want %v", got, want)
	}
	if w.Running() {
		t.Error("running latch still held after the startup sweep")
	}
}

// TestScratchDirForCycle_LockFileRemoved verifies that a cycle takes the
// lock again when someone removed the scratch dir after the worker took
// its lock. The lock on the removed lock file does not protect the new
// dir. The cycle creates the dir and the lock file again and takes the
// shared lock on the new lock file. When another process holds the
// exclusive lock on the new lock file, the cycle stages in the temp dir.
func TestScratchDirForCycle_LockFileRemoved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		other int // lock of the other process on the new lock file, 0 for none
		// wantDir is true when the cycle uses the set dir, false when it
		// uses the temp dir.
		wantDir bool
	}{
		{name: "dir removed", other: 0, wantDir: true},
		{name: "dir created again, other holds exclusive lock", other: syscall.LOCK_EX, wantDir: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			w, _ := newTestWorker(t, f)
			w.logger, _ = newSweepLogger()
			dir := filepath.Join(t.TempDir(), "scratch")
			setScratchDir(t, w, dir)
			if got := w.scratchDirForCycle(t.Context()); got != dir {
				t.Fatalf("first scratchDirForCycle = %q, want %q", got, dir)
			}
			if err := os.RemoveAll(dir); err != nil {
				t.Fatalf("remove scratch dir: %v", err)
			}
			var other *os.File
			if tt.other != 0 {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatalf("create scratch dir: %v", err)
				}
				other = holdScratchLock(t, dir, tt.other)
			}

			got := w.scratchDirForCycle(t.Context())

			want := ""
			if tt.wantDir {
				want = dir
			}
			if got != want {
				t.Errorf("scratchDirForCycle after the remove = %q, want %q", got, want)
			}
			if names := dirNames(t, dir); !slices.Equal(names, []string{scratchLockName}) {
				t.Errorf("scratch dir = %v, want only the lock file", names)
			}
			if other != nil {
				_ = other.Close()
			}
			if held := !canLockScratchDir(t, dir, syscall.LOCK_EX); held != tt.wantDir {
				t.Errorf("worker holds a shared lock on the new lock file = %v, want %v", held, tt.wantDir)
			}
		})
	}
}

// TestSync_ScratchDirLockedByOtherProcess verifies that a cycle succeeds
// when another process holds the exclusive lock of the scratch dir. The
// cycle stages in the temp dir and leaves no file in the scratch dir.
func TestSync_ScratchDirLockedByOtherProcess(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, _ := newTestWorker(t, f)
	var buf *logBuffer
	w.logger, buf = newSweepLogger()
	dir := t.TempDir()
	setScratchDir(t, w, dir)
	holdScratchLock(t, dir, syscall.LOCK_EX)

	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := dirNames(t, dir); !slices.Equal(got, []string{scratchLockName}) {
		t.Errorf("scratch dir after sync = %v, want only the lock file", got)
	}
	if logs := buf.records(t, "failed to lock scratch dir, staging in the temp dir"); len(logs) != 1 {
		t.Errorf("lock log = %v, want one record", logs)
	}
}
