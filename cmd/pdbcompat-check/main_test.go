package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenStub creates a minimal golden file directory for tests.
// It writes a valid PeeringDB-shaped golden file for the given type
// so that checkType can complete the structural comparison.
func goldenStub(t *testing.T, typeName string) string {
	t.Helper()
	dir := t.TempDir()
	typeDir := filepath.Join(dir, typeName)
	if err := os.MkdirAll(typeDir, 0o755); err != nil {
		t.Fatalf("create golden dir: %v", err)
	}
	// Minimal valid PeeringDB response envelope.
	golden := `{"meta":{},"data":[{"id":1}]}`
	if err := os.WriteFile(filepath.Join(typeDir, "list.json"), []byte(golden), 0o644); err != nil {
		t.Fatalf("write golden file: %v", err)
	}
	return dir
}

func TestCheckTypeAuthHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		apiKey     string
		wantHeader string // expected Authorization header value, empty means absent
	}{
		{
			name:       "auth header sent when apiKey is non-empty",
			apiKey:     "test-key-123",
			wantHeader: "Api-Key test-key-123",
		},
		{
			name:       "no auth header when apiKey is empty",
			apiKey:     "",
			wantHeader: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotAuth string
			var gotAuthPresent bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				gotAuthPresent = r.Header.Get("Authorization") != ""
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"meta":{},"data":[{"id":1}]}`))
			}))
			defer server.Close()

			goldenDir := goldenStub(t, "org")
			client := &http.Client{}

			_, err := checkType(t.Context(), client, server.URL, goldenDir, "org", tt.apiKey)
			if err != nil {
				t.Fatalf("checkType returned error: %v", err)
			}

			if tt.wantHeader == "" {
				if gotAuthPresent {
					t.Errorf("expected no Authorization header, got %q", gotAuth)
				}
			} else {
				if gotAuth != tt.wantHeader {
					t.Errorf("Authorization header = %q, want %q", gotAuth, tt.wantHeader)
				}
			}
		})
	}
}

func TestCheckTypeAuthErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		wantErr    string
	}{
		{
			name:       "401 returns API key error",
			statusCode: http.StatusUnauthorized,
			wantErr:    "API key may be invalid",
		},
		{
			name:       "403 returns API key error",
			statusCode: http.StatusForbidden,
			wantErr:    "API key may be invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"detail":"Authentication credentials were not provided."}`))
			}))
			defer server.Close()

			goldenDir := goldenStub(t, "org")
			client := &http.Client{}

			_, err := checkType(t.Context(), client, server.URL, goldenDir, "org", "bad-key")
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestAPIKeyFlagPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		flagVal string
		envVal  string
		wantKey string
	}{
		{
			name:    "flag value used when provided",
			flagVal: "flag-key-abc",
			envVal:  "",
			wantKey: "flag-key-abc",
		},
		{
			name:    "env var used as fallback when flag is empty",
			flagVal: "",
			envVal:  "env-key-xyz",
			wantKey: "env-key-xyz",
		},
		{
			name:    "flag takes precedence over env var",
			flagVal: "flag-key-abc",
			envVal:  "env-key-xyz",
			wantKey: "flag-key-abc",
		},
		{
			name:    "empty when neither flag nor env var set",
			flagVal: "",
			envVal:  "",
			wantKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveAPIKey(tt.flagVal, tt.envVal)
			if got != tt.wantKey {
				t.Errorf("resolveAPIKey(%q, %q) = %q, want %q", tt.flagVal, tt.envVal, got, tt.wantKey)
			}
		})
	}
}

// TestAPIKeyEnvVarFallback verifies that the actual env var is read
// when the flag value is empty. This test is not parallel because it
// uses t.Setenv.
func TestAPIKeyEnvVarFallback(t *testing.T) {
	t.Setenv("PDBPLUS_PEERINGDB_API_KEY", "env-key-from-os")

	got := resolveAPIKey("", os.Getenv("PDBPLUS_PEERINGDB_API_KEY"))
	if got != "env-key-from-os" {
		t.Errorf("apiKey = %q, want %q", got, "env-key-from-os")
	}
}

// TestRunSubcommandDispatch exercises the error paths of the subcommand
// shell: missing subcommand, unknown subcommand, and per-mode flag
// isolation (a capture-only flag must be rejected by the check mode).
func TestRunSubcommandDispatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		argv    []string
		wantErr string // substring of the returned error; empty = nil error
	}{
		{
			name:    "no arguments",
			argv:    nil,
			wantErr: "missing subcommand",
		},
		{
			name:    "unknown subcommand",
			argv:    []string{"observe"},
			wantErr: `unknown subcommand "observe"`,
		},
		{
			name:    "capture flag rejected by check mode",
			argv:    []string{"check", "-target=prod"},
			wantErr: "-target",
		},
		{
			name:    "check flag rejected by diff mode",
			argv:    []string{"diff", "-url=http://example.invalid"},
			wantErr: "-url",
		},
		{
			name: "help flag succeeds",
			argv: []string{"--help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr strings.Builder
			err := run(tt.argv, &stdout, &stderr)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("run(%v) = %v, want nil", tt.argv, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("run(%v) = nil, want error containing %q", tt.argv, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("run(%v) error = %q, want it to contain %q", tt.argv, err.Error(), tt.wantErr)
			}
		})
	}
}
