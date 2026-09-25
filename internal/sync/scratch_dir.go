package sync

import (
	"context"
	"errors"
	"log/slog"
	"os"
	stdsync "sync"
)

// scratchDirMinFreeBytes is the free space that a set scratch dir must
// have before a cycle stages in it. With less free space, the cycle
// stages in os.TempDir().
//
// A full cycle stages about 100 MB. In production, the dir is on the
// primary volume, and LiteFS writes the database and its LTX files to
// the same volume. After a full cycle stages its file, at least 400 MiB
// stay free for these LiteFS writes. Fly.io extends the volume only when
// it is 80 percent full. Thus the free space can be low for some time,
// and the scratch file must not take the space that LiteFS needs then.
const scratchDirMinFreeBytes = 512 << 20

// DirFreeBytes returns the free space of the file system of dir, in
// bytes, as the free-space guard of the scratch dir reads it. On a
// system other than Linux it returns math.MaxUint64: the free space is
// not known there.
func DirFreeBytes(dir string) (uint64, error) {
	return dirFreeBytes(dir)
}

// scratchDirForCycle returns the directory of the scratch database of the
// next cycle: ScratchDir, or "" for os.TempDir(). The caller holds the
// running latch.
//
// When this process has not swept the directory yet, it sweeps it first
// (see sweepScratchDir).
//
// A cycle stages in a set ScratchDir only while this process holds its
// shared lock (see scratchDirLock). The startup sweep can fail to take
// the lock, so this process takes the lock here when it holds none. The dir must also have
// scratchDirMinFreeBytes of free space (see Worker.scratchFreeBytes).
// When the lock or the free space check fails, the cycle stages in
// os.TempDir() and the function logs a WARN. These checks never fail the
// cycle.
func (w *Worker) scratchDirForCycle(ctx context.Context) string {
	dir := w.config.ScratchDir
	if dir == "" {
		return ""
	}
	if !w.scratchSwept {
		w.sweepScratchDir(ctx)
	}
	if err := w.scratchLock.lockShared(dir); err != nil {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "failed to lock scratch dir, staging in the temp dir",
			slog.String("dir", dir),
			slog.Any("error", err),
		)
		return ""
	}
	free, err := w.scratchFreeBytes(dir)
	if err != nil {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "failed to read free space of scratch dir, staging in the temp dir",
			slog.String("dir", dir),
			slog.Uint64("reserve_bytes", scratchDirMinFreeBytes),
			slog.Any("error", err),
		)
		return ""
	}
	if free < scratchDirMinFreeBytes {
		w.logger.LogAttrs(ctx, slog.LevelWarn, "scratch dir low on free space, staging in the temp dir",
			slog.String("dir", dir),
			slog.Uint64("free_bytes", free),
			slog.Uint64("reserve_bytes", scratchDirMinFreeBytes),
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
