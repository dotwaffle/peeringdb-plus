//go:build linux

package sync

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"syscall"
)

// lockExclusive takes the exclusive lock of dir without blocking. It
// converts a shared lock that this process holds. It returns
// errScratchDirInUse when another process holds a lock on dir.
func (l *scratchDirLock) lockExclusive(dir string) error {
	return l.flock(dir, syscall.LOCK_EX)
}

// lockShared takes the shared lock of dir without blocking. It converts
// an exclusive lock that this process holds, and does nothing when this
// process holds the shared lock. It returns errScratchDirInUse when
// another process holds the exclusive lock.
func (l *scratchDirLock) lockShared(dir string) error {
	return l.flock(dir, syscall.LOCK_SH)
}

// flock applies how (LOCK_EX or LOCK_SH) to the lock file of dir with
// LOCK_NB. It opens the lock file when this process holds no lock.
//
// Someone can remove or replace the lock file or dir while this process
// holds the lock. The lock on the old file does not protect the files in
// dir then. Thus flock closes the held file when it is not the file at
// the lock path, and opens the lock path again. The open also creates a
// missing dir.
//
// After an error, this process holds no lock: a conversion that fails
// releases the old lock (see flock(2)). Thus flock closes the file, and
// the next call opens it again.
func (l *scratchDirLock) flock(dir string, how int) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil && !isFileAt(l.file, filepath.Join(dir, scratchLockName)) {
		_ = l.file.Close()
		l.file = nil
	}
	if l.file == nil {
		f, err := openScratchLockFile(dir)
		if err != nil {
			return err
		}
		l.file = f
	}
	if err := flockFile(l.file, how|syscall.LOCK_NB); err != nil {
		_ = l.file.Close()
		l.file = nil
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return errScratchDirInUse
		}
		return fmt.Errorf("lock %s: %w", filepath.Join(dir, scratchLockName), err)
	}
	return nil
}

// openScratchLockFile opens the lock file of dir. It creates dir with
// mode 0700 and the file with mode 0600 when they are missing. With
// O_NOFOLLOW, the open fails when the name is a symbolic link, so the
// process does not create or lock a file outside dir. With O_NONBLOCK,
// the open does not wait for a writer when the name is a FIFO. The
// function accepts only a regular file.
func openScratchLockFile(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create scratch dir %s: %w", dir, err)
	}
	// The path derives from the operator-set PDBPLUS_SCRATCH_DIR. Cleaning
	// it also satisfies gosec G304's static analysis.
	path := filepath.Clean(filepath.Join(dir, scratchLockName))
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open scratch lock file: %w", err)
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat scratch lock file: %w", err)
	}
	if !fi.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("scratch lock file %s is not a regular file", path)
	}
	return f, nil
}

// isFileAt reports whether f is the file at path. It returns false when
// path is missing or names another file. It does not follow a symbolic
// link at path.
func isFileAt(f *os.File, path string) bool {
	held, err := f.Stat()
	if err != nil {
		return false
	}
	cur, err := os.Lstat(path)
	return err == nil && os.SameFile(held, cur)
}

// dirFreeBytes returns the free space of the file system of dir, in
// bytes, that a process without root privileges can use (see statfs(2)).
func dirFreeBytes(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", dir, err)
	}
	free, err := freeBytes(st.Bavail, int64(st.Frsize), int64(st.Bsize))
	if err != nil {
		return 0, fmt.Errorf("statfs %s: %w", dir, err)
	}
	return free, nil
}

// freeBytes returns the size in bytes of bavail units of statfs(2).
// Bavail counts units of the fragment size frsize (see statvfs(3)). Linux
// sets frsize to the block size bsize when a file system does not set
// it, and freeBytes does the same. The size fields are signed on some
// systems, so a size of 0 or less is an error, not a very large free
// space. A product that does not fit in 64 bits returns math.MaxUint64.
func freeBytes(bavail uint64, frsize, bsize int64) (uint64, error) {
	unit := frsize
	if unit <= 0 {
		unit = bsize
	}
	if unit <= 0 {
		return 0, fmt.Errorf("fragment size %d, block size %d", frsize, bsize)
	}
	hi, lo := bits.Mul64(bavail, uint64(unit))
	if hi != 0 {
		return math.MaxUint64, nil
	}
	return lo, nil
}

// flockFile calls flock(2) with how on the file descriptor of f.
func flockFile(f *os.File, how int) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var ferr error
	if err := rc.Control(func(fd uintptr) {
		ferr = syscall.Flock(int(fd), how)
	}); err != nil {
		return err
	}
	return ferr
}
