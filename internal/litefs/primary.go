// Package litefs provides utilities for detecting the role of the current
// node in a LiteFS cluster (primary vs replica).
package litefs

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
)

// PrimaryFile is the standard LiteFS lease file path.
// This file exists ONLY on replica nodes and contains the primary's hostname.
// Its ABSENCE means this node IS the primary.
//
// The semantics are inverted from what the filename suggests:
//   - .primary file EXISTS  -> this node is a REPLICA (file contains the primary's address)
//   - .primary file ABSENT  -> this node IS the PRIMARY (we hold the lease)
//
// See: https://fly.io/docs/litefs/primary/
const PrimaryFile = "/litefs/.primary"

// IsPrimaryAt reports whether this node is the LiteFS primary by checking
// the given path for the lease file. Returns true when the file does NOT
// exist (absence = primary), false when it does exist (presence = replica).
//
// This is a low-level primitive with no fallback handling; it is exported
// so the package's external test can exercise the lease-file semantics
// against arbitrary paths. Production code calls IsPrimaryWithFallback,
// which adds the fail-safe-to-replica path for ambiguous stat errors and
// the no-LiteFS env-var fallback.
func IsPrimaryAt(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}

// IsPrimaryWithFallback detects primary status with a fallback for
// environments where LiteFS is not running (e.g., local development).
//
// Detection order:
//  1. If the primary file at path exists, this is a replica (return false).
//  2. If stat fails with anything other than "does not exist" (e.g. a flaky
//     LiteFS FUSE mount returning EIO/EACCES/ENOTDIR), fail SAFE toward
//     replica (return false) and log a WARN. A misclassified replica that
//     declines primary duties is harmless; a misclassified primary runs
//     destructive Schema.Create DDL (WithDropColumn/WithDropIndex).
//  3. If the file is genuinely absent and the parent directory of path exists
//     (LiteFS is mounted), this is the primary (return true).
//  4. If neither exists (no LiteFS), fall back to the environment variable
//     identified by envKey parsed as a boolean. Defaults to true if the env
//     var is unset, matching the common local-dev expectation (single node = primary).
func IsPrimaryWithFallback(path string, envKey string) bool {
	// Check if the .primary file itself exists (replica indicator).
	_, err := os.Stat(path)
	if err == nil {
		// File exists — we are a replica.
		return false
	}
	if !errors.Is(err, os.ErrNotExist) {
		// Ambiguous stat error (not a clean "does not exist"): a transient
		// FUSE fault must not be read as primary, because the startup path
		// would then run destructive DDL on a node that is really a replica.
		// Fail safe to replica.
		slog.Warn("litefs: ambiguous .primary stat, failing safe to replica",
			slog.String("path", path),
			slog.Any("error", err),
		)
		return false
	}

	// File is genuinely absent. Check if the parent directory exists (LiteFS
	// mount is present).
	dir := filepath.Dir(path)
	if info, dirErr := os.Stat(dir); dirErr == nil && info.IsDir() {
		// LiteFS directory exists but .primary file does not — we are primary.
		return true
	}

	// No LiteFS at all — fall back to environment variable.
	v := os.Getenv(envKey)
	if v == "" {
		// Default to true: in local dev without LiteFS, assume primary.
		return true
	}
	b, parseErr := strconv.ParseBool(v)
	if parseErr != nil {
		// ValidateEnvFallback rejects unparseable values at startup, so
		// this branch is unreachable in practice. If it ever fires, fail
		// SAFE to replica — the previous "default to primary for safety"
		// had it exactly backwards: a misclassified primary runs
		// destructive Schema.Create DDL, a declining replica is harmless.
		slog.Warn("litefs: unparseable env fallback, failing safe to replica",
			slog.String("key", envKey),
			slog.String("value", v),
			slog.Any("error", parseErr),
		)
		return false
	}
	return b
}

// ValidateEnvFallback checks that the environment fallback variable, if
// set, parses as a boolean. Call once at startup so an operator typo in
// PDBPLUS_IS_PRIMARY fails fast with a clear message instead of being
// silently coerced into a role at every scheduler tick. Unset is valid
// (the documented "default primary" local-dev behaviour).
func ValidateEnvFallback(envKey string) error {
	v := os.Getenv(envKey)
	if v == "" {
		return nil
	}
	if _, err := strconv.ParseBool(v); err != nil {
		return fmt.Errorf("%s=%q is not a boolean (use true/false/1/0): %w", envKey, v, err)
	}
	return nil
}
