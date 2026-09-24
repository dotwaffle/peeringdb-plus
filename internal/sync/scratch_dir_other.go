//go:build !linux

package sync

import (
	"errors"
	"fmt"
	"math"
)

// errScratchLockUnsupported is the error of lockExclusive on this
// platform.
var errScratchLockUnsupported = fmt.Errorf("scratch dir lock needs Linux: %w", errors.ErrUnsupported)

// lockExclusive always returns an error on this platform, because the
// scratch dir lock uses the Linux flock call. Thus the startup sweep
// never runs here, and the stale scratch files stay in the directory.
//
// The function returns a variable, not a new error. Thus staticcheck
// cannot prove that the error is never nil, and does not report the
// nil check of the caller as SA4023.
func (l *scratchDirLock) lockExclusive(string) error {
	return errScratchLockUnsupported
}

// lockShared takes no lock on this platform and returns nil, so a cycle
// stages in the set directory without a lock. No process on this
// platform sweeps the directory (see lockExclusive), so the file of a
// cycle needs no lock.
func (l *scratchDirLock) lockShared(string) error {
	return nil
}

// dirFreeBytes disables the free-space guard on this platform, because
// the guard uses the Linux statfs call. It returns the largest value, so
// a cycle always stages in the set directory.
func dirFreeBytes(string) (uint64, error) {
	return math.MaxUint64, nil
}
