// Package litefs provides utilities for detecting the role of the current
// node in a LiteFS cluster (primary vs replica).
package litefs

import (
	"errors"
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

// IsPrimary reports whether this node is the LiteFS primary using the
// standard lease file path. Returns true when the .primary file is absent
// (meaning this node holds the lease and is the primary).
func IsPrimary() bool {
	return IsPrimaryAt(PrimaryFile)
}

// IsPrimaryAt reports whether this node is the LiteFS primary by checking
// the given path for the lease file. Returns true when the file does NOT
// exist (absence = primary), false when it does exist (presence = replica).
func IsPrimaryAt(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}

// IsPrimaryWithFallback detects primary status with a fallback for
// environments where LiteFS is not running (e.g., local development).
//
// Detection order:
//  1. If the primary file at path exists, this is a replica (return false).
//  2. If the parent directory of path exists (LiteFS is mounted), this is
//     the primary (return true).
//  3. If neither exists (no LiteFS), fall back to the environment variable
//     identified by envKey parsed as a boolean. Defaults to true if the env
//     var is unset, matching the common local-dev expectation (single node = primary).
func IsPrimaryWithFallback(path string, envKey string) bool {
	// Check if the .primary file itself exists (replica indicator).
	_, err := os.Stat(path)
	if err == nil {
		// File exists — we are a replica.
		return false
	}

	// Check if the parent directory exists (LiteFS mount is present).
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
		// Unparseable value — default to primary for safety.
		return true
	}
	return b
}
