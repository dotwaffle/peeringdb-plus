package sync

import (
	"context"
	"errors"
	"log/slog"
	"os"
	stdsync "sync"
)

// scratchDirForCycle returns the directory of the scratch database of the
// next cycle: ScratchDir, or "" for os.TempDir(). The caller holds the
// running latch.
//
// A cycle stages in a set ScratchDir only while this process holds its
// shared lock (see scratchDirLock). The startup sweep can fail to take
// the lock, or run on a replica that later becomes the primary, so this
// process takes the lock here when it holds none. When it cannot take
// the lock, the cycle stages in os.TempDir(). A lock error never fails
// the cycle.
func (w *Worker) scratchDirForCycle(ctx context.Context) string {
	dir := w.config.ScratchDir
	if dir == "" {
		return ""
	}
	if err := w.scratchLock.lockShared(dir); err != nil {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "failed to lock scratch dir, staging in the temp dir",
			slog.String("dir", dir),
			slog.Any("error", err),
		)
		return ""
	}
	return dir
}

// scratchLockName is the name of the lock file in a set scratch
// directory. The name does not start with scratchFilePrefix, so
// sweepStaleScratchFiles never removes the file.
const scratchLockName = ".pdbplus-scratch.lock"

// errScratchDirInUse reports that another process holds a lock on the
// scratch directory that blocks the requested lock.
var errScratchDirInUse = errors.New("scratch dir in use by another process")

// scratchDirLock is the flock of this process on the lock file of a set
// scratch directory. Each process that stages a cycle in the directory
// holds a shared lock (see scratchDirForCycle). The startup sweep needs
// the exclusive lock (see sweepScratchDirAtStartup). Thus the sweep of
// one process never runs while another process uses the directory.
//
// The lock cannot protect a process that stages in the directory without
// the lock, for example a process that stages in os.TempDir() when the
// directory is the temp dir. Thus the directory must not be a shared
// temp dir.
//
// After the process gets a lock, it keeps the lock file open until it
// exits, and the exit releases the lock. Only tests call close. When
// someone removes or replaces the lock file or the directory, the next
// lock call opens the new lock file and creates a missing directory.
//
// The lock uses the Linux flock call (see scratch_dir_linux.go). On other
// systems, the process takes no lock and never sweeps (see
// scratch_dir_other.go).
type scratchDirLock struct {
	mu   stdsync.Mutex
	file *os.File // open lock file, nil when this process holds no lock
}

// close releases the lock and closes the lock file. Tests call it in
// t.Cleanup.
func (l *scratchDirLock) close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
