package litefs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/litefs"
)

func TestIsPrimaryAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T) string
		wantPrimary bool
	}{
		{
			name: "file does not exist means we are primary",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			wantPrimary: true,
		},
		{
			name: "file exists means we are a replica",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, ".primary")
				if err := os.WriteFile(path, []byte("primary-host.internal"), 0o644); err != nil {
					t.Fatalf("writing test file: %v", err)
				}
				return path
			},
			wantPrimary: false,
		},
		{
			name: "empty file exists means we are a replica",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, ".primary")
				if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
					t.Fatalf("writing test file: %v", err)
				}
				return path
			},
			wantPrimary: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := tt.setup(t)
			got := litefs.IsPrimaryAt(path)
			if got != tt.wantPrimary {
				t.Errorf("IsPrimaryAt(%q) = %v, want %v", path, got, tt.wantPrimary)
			}
		})
	}
}

func TestIsPrimaryWithFallback(t *testing.T) {
	// Not parallel: subtests that use t.Setenv cannot be parallel.

	tests := []struct {
		name        string
		setup       func(t *testing.T) (path string, envKey string)
		wantPrimary bool
	}{
		{
			name: "primary file exists means replica",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				path := filepath.Join(dir, ".primary")
				if err := os.WriteFile(path, []byte("primary-host.internal"), 0o644); err != nil {
					t.Fatalf("writing test file: %v", err)
				}
				return path, "TEST_IS_PRIMARY_1"
			},
			wantPrimary: false,
		},
		{
			name: "no file but litefs directory exists means primary",
			setup: func(t *testing.T) (string, string) {
				// Simulate LiteFS directory existing but no .primary file
				dir := t.TempDir() // acts as the litefs mount dir
				path := filepath.Join(dir, ".primary")
				// dir exists but .primary file does not
				return path, "TEST_IS_PRIMARY_2"
			},
			wantPrimary: true,
		},
		{
			name: "no litefs directory falls back to env var true",
			setup: func(t *testing.T) (string, string) {
				// Use a non-existent directory to simulate no LiteFS
				dir := filepath.Join(t.TempDir(), "nonexistent-litefs")
				path := filepath.Join(dir, ".primary")
				envKey := "TEST_IS_PRIMARY_3"
				t.Setenv(envKey, "true")
				return path, envKey
			},
			wantPrimary: true,
		},
		{
			name: "no litefs directory falls back to env var false",
			setup: func(t *testing.T) (string, string) {
				dir := filepath.Join(t.TempDir(), "nonexistent-litefs")
				path := filepath.Join(dir, ".primary")
				envKey := "TEST_IS_PRIMARY_4"
				t.Setenv(envKey, "false")
				return path, envKey
			},
			wantPrimary: false,
		},
		{
			name: "no litefs directory and no env var defaults to true",
			setup: func(t *testing.T) (string, string) {
				dir := filepath.Join(t.TempDir(), "nonexistent-litefs")
				path := filepath.Join(dir, ".primary")
				envKey := "TEST_IS_PRIMARY_5"
				// Don't set the env var — should default to true (assume primary for local dev)
				return path, envKey
			},
			wantPrimary: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, envKey := tt.setup(t)
			got := litefs.IsPrimaryWithFallback(path, envKey)
			if got != tt.wantPrimary {
				t.Errorf("IsPrimaryWithFallback(%q, %q) = %v, want %v", path, envKey, got, tt.wantPrimary)
			}
		})
	}
}
