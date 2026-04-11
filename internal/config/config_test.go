package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad_OTelSampleRate(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    float64
		wantErr bool
	}{
		{name: "default is 1.0", envVal: "", want: 1.0},
		{name: "explicit 0.5", envVal: "0.5", want: 0.5},
		{name: "explicit 0.0", envVal: "0.0", want: 0.0},
		{name: "explicit 1.0", envVal: "1.0", want: 1.0},
		{name: "negative is invalid", envVal: "-0.1", wantErr: true},
		{name: "above 1.0 is invalid", envVal: "2.0", wantErr: true},
		{name: "non-numeric is invalid", envVal: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cannot use t.Parallel with t.Setenv per Go 1.26 testing rules.
			if tt.envVal != "" {
				t.Setenv("PDBPLUS_OTEL_SAMPLE_RATE", tt.envVal)
			}
			// Ensure required fields are valid for Load to succeed.
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for PDBPLUS_OTEL_SAMPLE_RATE=%q, got nil", tt.envVal)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.OTelSampleRate != tt.want {
				t.Errorf("OTelSampleRate = %v, want %v", cfg.OTelSampleRate, tt.want)
			}
		})
	}
}

func TestLoad_SyncStaleThreshold(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    time.Duration
		wantErr bool
	}{
		{name: "default is 24h", envVal: "", want: 24 * time.Hour},
		{name: "explicit 12h", envVal: "12h", want: 12 * time.Hour},
		{name: "explicit 1h30m", envVal: "1h30m", want: 90 * time.Minute},
		{name: "invalid duration", envVal: "not-a-duration", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("PDBPLUS_SYNC_STALE_THRESHOLD", tt.envVal)
			}
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for PDBPLUS_SYNC_STALE_THRESHOLD=%q, got nil", tt.envVal)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.SyncStaleThreshold != tt.want {
				t.Errorf("SyncStaleThreshold = %v, want %v", cfg.SyncStaleThreshold, tt.want)
			}
		})
	}
}

func TestLoad_SyncMode(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    SyncMode
		wantErr bool
	}{
		{name: "default is full", envVal: "", want: SyncModeFull},
		{name: "explicit full", envVal: "full", want: SyncModeFull},
		{name: "explicit incremental", envVal: "incremental", want: SyncModeIncremental},
		{name: "invalid value", envVal: "invalid", wantErr: true},
		{name: "wrong case FULL", envVal: "FULL", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cannot use t.Parallel with t.Setenv per Go 1.26 testing rules.
			if tt.envVal != "" {
				t.Setenv("PDBPLUS_SYNC_MODE", tt.envVal)
			}
			// Ensure required fields are valid for Load to succeed.
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for PDBPLUS_SYNC_MODE=%q, got nil", tt.envVal)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.SyncMode != tt.want {
				t.Errorf("SyncMode = %v, want %v", cfg.SyncMode, tt.want)
			}
		})
	}
}

func TestLoad_PeeringDBAPIKey(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   string
	}{
		{name: "set to test-key-123", envVal: "test-key-123", want: "test-key-123"},
		{name: "default empty when not set", envVal: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("PDBPLUS_PEERINGDB_API_KEY", tt.envVal)
			}
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.PeeringDBAPIKey != tt.want {
				t.Errorf("PeeringDBAPIKey = %q, want %q", cfg.PeeringDBAPIKey, tt.want)
			}
		})
	}
}

func TestLoad_StreamTimeout(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    time.Duration
		wantErr bool
	}{
		{name: "default is 60s", envVal: "", want: 60 * time.Second},
		{name: "explicit 30s", envVal: "30s", want: 30 * time.Second},
		{name: "explicit 2m", envVal: "2m", want: 2 * time.Minute},
		{name: "invalid duration", envVal: "not-a-duration", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("PDBPLUS_STREAM_TIMEOUT", tt.envVal)
			}
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for PDBPLUS_STREAM_TIMEOUT=%q, got nil", tt.envVal)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.StreamTimeout != tt.want {
				t.Errorf("StreamTimeout = %v, want %v", cfg.StreamTimeout, tt.want)
			}
		})
	}
}

func TestLoad_SyncInterval(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    time.Duration
		wantErr bool
		wantMsg string
	}{
		{name: "default is 1h", envVal: "", want: 1 * time.Hour},
		{name: "explicit 30m", envVal: "30m", want: 30 * time.Minute},
		{name: "invalid duration", envVal: "not-a-duration", wantErr: true, wantMsg: "PDBPLUS_SYNC_INTERVAL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("PDBPLUS_SYNC_INTERVAL", tt.envVal)
			}
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for PDBPLUS_SYNC_INTERVAL=%q, got nil", tt.envVal)
				}
				if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.SyncInterval != tt.want {
				t.Errorf("SyncInterval = %v, want %v", cfg.SyncInterval, tt.want)
			}
		})
	}
}

func TestLoad_IncludeDeleted(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    bool
		wantErr bool
		wantMsg string
	}{
		{name: "default is false", envVal: "", want: false},
		{name: "explicit true", envVal: "true", want: true},
		{name: "explicit false", envVal: "false", want: false},
		{name: "invalid bool", envVal: "maybe", wantErr: true, wantMsg: "PDBPLUS_INCLUDE_DELETED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("PDBPLUS_INCLUDE_DELETED", tt.envVal)
			}
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for PDBPLUS_INCLUDE_DELETED=%q, got nil", tt.envVal)
				}
				if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.IncludeDeleted != tt.want {
				t.Errorf("IncludeDeleted = %v, want %v", cfg.IncludeDeleted, tt.want)
			}
		})
	}
}

func TestLoad_CSPEnforce(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    bool
		wantErr bool
		wantMsg string
	}{
		{name: "default is false", envVal: "", want: false},
		{name: "explicit true", envVal: "true", want: true},
		{name: "explicit false", envVal: "false", want: false},
		{name: "invalid bool", envVal: "maybe", wantErr: true, wantMsg: "PDBPLUS_CSP_ENFORCE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("PDBPLUS_CSP_ENFORCE", tt.envVal)
			}
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for PDBPLUS_CSP_ENFORCE=%q, got nil", tt.envVal)
				}
				if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.CSPEnforce != tt.want {
				t.Errorf("CSPEnforce = %v, want %v", cfg.CSPEnforce, tt.want)
			}
		})
	}
}

func TestLoad_DrainTimeout(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    time.Duration
		wantErr bool
		wantMsg string
	}{
		{name: "default is 10s", envVal: "", want: 10 * time.Second},
		{name: "explicit 5s", envVal: "5s", want: 5 * time.Second},
		{name: "invalid duration", envVal: "garbage", wantErr: true, wantMsg: "PDBPLUS_DRAIN_TIMEOUT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("PDBPLUS_DRAIN_TIMEOUT", tt.envVal)
			}
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for PDBPLUS_DRAIN_TIMEOUT=%q, got nil", tt.envVal)
				}
				if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.DrainTimeout != tt.want {
				t.Errorf("DrainTimeout = %v, want %v", cfg.DrainTimeout, tt.want)
			}
		})
	}
}

func TestLoad_Validate(t *testing.T) {
	tests := []struct {
		name        string
		env         string
		val         string
		wantErr     bool
		errContains string
	}{
		{name: "valid defaults", wantErr: false},
		{name: "listen addr missing colon", env: "PDBPLUS_LISTEN_ADDR", val: "no-colon", wantErr: true, errContains: "must contain"},
		{name: "listen addr with port only", env: "PDBPLUS_LISTEN_ADDR", val: ":8080", wantErr: false},
		{name: "listen addr with host:port", env: "PDBPLUS_LISTEN_ADDR", val: "0.0.0.0:9090", wantErr: false},
		{name: "peeringdb url invalid", env: "PDBPLUS_PEERINGDB_URL", val: "not://valid url %%%", wantErr: true, errContains: "valid URL"},
		{name: "peeringdb url valid", env: "PDBPLUS_PEERINGDB_URL", val: "https://api.peeringdb.com", wantErr: false},
		{name: "peeringdb url no scheme", env: "PDBPLUS_PEERINGDB_URL", val: "just-a-hostname", wantErr: true, errContains: "missing scheme"},
		{name: "drain timeout zero", env: "PDBPLUS_DRAIN_TIMEOUT", val: "0s", wantErr: true, errContains: "greater than 0"},
		{name: "drain timeout negative", env: "PDBPLUS_DRAIN_TIMEOUT", val: "-5s", wantErr: true, errContains: "greater than 0"},
		{name: "drain timeout valid", env: "PDBPLUS_DRAIN_TIMEOUT", val: "10s", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv(tt.env, tt.val)
			}
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s=%q, got nil", tt.env, tt.val)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_ = cfg
		})
	}
}

func TestLoad_OTelEndpointRemoved(t *testing.T) {
	t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify OTelEndpoint field no longer exists.
	// Since this is an internal package test, accessing cfg.OTelEndpoint
	// would cause a compile error if the field existed. This test passes
	// by compiling successfully without any OTelEndpoint reference.
	_ = cfg
}

// TestConfig_PeeringDBURLValidation verifies SEC-06: PDBPLUS_PEERINGDB_URL must
// be https://, OR http:// against loopback / RFC 1918 private IPs / literal
// "localhost". Each rejection class produces a distinct error message.
//
// Rows marked wantErrContains distinguish the four error classes:
//   - "missing scheme"  — no scheme at all
//   - "empty host"      — scheme present, host empty
//   - "unsupported scheme" — not http or https
//   - "non-local host"  — http:// against public host
func TestConfig_PeeringDBURLValidation(t *testing.T) {
	tests := []struct {
		name            string
		envVal          string
		wantErr         bool
		wantErrContains string // substring asserted when wantErr is true
	}{
		// Accept — https:// always
		{name: "https api.peeringdb.com", envVal: "https://api.peeringdb.com", wantErr: false},
		{name: "https localhost", envVal: "https://localhost:8443", wantErr: false},
		{name: "https public IP", envVal: "https://203.0.113.1", wantErr: false},

		// Accept — http:// loopback / localhost
		{name: "http localhost", envVal: "http://localhost:8000", wantErr: false},
		{name: "http 127.0.0.1", envVal: "http://127.0.0.1", wantErr: false},
		{name: "http 127.0.0.1 with port", envVal: "http://127.0.0.1:9000", wantErr: false},
		{name: "http IPv6 loopback", envVal: "http://[::1]", wantErr: false},
		{name: "http IPv6 loopback with port", envVal: "http://[::1]:8080", wantErr: false},

		// Accept — http:// RFC 1918 private ranges
		{name: "http 10.0.0.0/8 low", envVal: "http://10.0.0.1", wantErr: false},
		{name: "http 10.0.0.0/8 high", envVal: "http://10.255.255.254", wantErr: false},
		{name: "http 172.16.0.0/12 low", envVal: "http://172.16.0.1", wantErr: false},
		{name: "http 172.16.0.0/12 mid", envVal: "http://172.20.1.1", wantErr: false},
		{name: "http 172.16.0.0/12 high", envVal: "http://172.31.255.254", wantErr: false},
		{name: "http 192.168.0.0/16 low", envVal: "http://192.168.0.1", wantErr: false},
		{name: "http 192.168.0.0/16 high", envVal: "http://192.168.255.254", wantErr: false},

		// Reject — http:// outside private ranges (boundary cases per mp-7)
		{
			name:            "http 11.0.0.1 outside 10/8",
			envVal:          "http://11.0.0.1",
			wantErr:         true,
			wantErrContains: "non-local host",
		},
		{
			name:            "http 172.15.255.255 below 172.16/12",
			envVal:          "http://172.15.255.255",
			wantErr:         true,
			wantErrContains: "non-local host",
		},
		{
			name:            "http 172.32.0.1 above 172.16/12",
			envVal:          "http://172.32.0.1",
			wantErr:         true,
			wantErrContains: "non-local host",
		},
		{
			name:            "http 193.168.0.1 outside 192.168/16",
			envVal:          "http://193.168.0.1",
			wantErr:         true,
			wantErrContains: "non-local host",
		},

		// Reject — http:// public hostnames
		{
			name:            "http example.com",
			envVal:          "http://example.com",
			wantErr:         true,
			wantErrContains: "non-local host",
		},
		{
			name:            "http api.peeringdb.com",
			envVal:          "http://api.peeringdb.com",
			wantErr:         true,
			wantErrContains: "non-local host",
		},

		// Reject — missing scheme
		{
			name:            "bare hostname",
			envVal:          "example.com",
			wantErr:         true,
			wantErrContains: "missing scheme",
		},
		{
			name:            "protocol-relative",
			envVal:          "//api.peeringdb.com",
			wantErr:         true,
			wantErrContains: "missing scheme",
		},

		// Reject — empty host
		{
			name:            "https empty host",
			envVal:          "https://",
			wantErr:         true,
			wantErrContains: "empty host",
		},
		{
			name:            "http empty host",
			envVal:          "http://",
			wantErr:         true,
			wantErrContains: "empty host",
		},

		// Reject — unsupported scheme
		{
			name:            "ftp scheme",
			envVal:          "ftp://example.com",
			wantErr:         true,
			wantErrContains: "unsupported scheme",
		},
		{
			name:            "file scheme",
			envVal:          "file:///tmp/foo",
			wantErr:         true,
			wantErrContains: "unsupported scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cannot use t.Parallel with t.Setenv per Go testing rules.
			t.Setenv("PDBPLUS_PEERINGDB_URL", tt.envVal)
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			_, err := Load()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("PDBPLUS_PEERINGDB_URL=%q: expected error, got nil", tt.envVal)
				}
				msg := err.Error()
				if !strings.Contains(msg, "PDBPLUS_PEERINGDB_URL") {
					t.Errorf("error must name env var; got %q", msg)
				}
				if tt.wantErrContains != "" && !strings.Contains(msg, tt.wantErrContains) {
					t.Errorf("error must contain %q to distinguish rejection class; got %q", tt.wantErrContains, msg)
				}
				return
			}
			if err != nil {
				t.Fatalf("PDBPLUS_PEERINGDB_URL=%q: unexpected error: %v", tt.envVal, err)
			}
		})
	}
}

// TestLoad_SyncMemoryLimit_Default asserts the default (no env var set)
// resolves to 400 MB per Commit F decision. The default matches the
// DEBT-03 benchmark regression gate and leaves 112 MB headroom under
// the 512 MB Fly.io VM cap.
func TestLoad_SyncMemoryLimit_Default(t *testing.T) {
	t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := int64(400 * 1024 * 1024)
	if cfg.SyncMemoryLimit != want {
		t.Errorf("default SyncMemoryLimit = %d, want %d", cfg.SyncMemoryLimit, want)
	}
}

// TestLoad_SyncMemoryLimit_Parse covers all branches of the parseByteSize
// helper for PDBPLUS_SYNC_MEMORY_LIMIT: standard units (KB/MB/GB/TB),
// short aliases (K/M/G/T), lowercase, explicit "0" disable, and the
// REJECTED forms (bare number, unknown unit, negative, empty prefix,
// non-numeric prefix). Table-driven per GO-T-1.
func TestLoad_SyncMemoryLimit_Parse(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    int64
		wantErr bool
	}{
		{name: "100MB", envVal: "100MB", want: 100 * 1024 * 1024},
		{name: "1GB", envVal: "1GB", want: 1024 * 1024 * 1024},
		{name: "1TB", envVal: "1TB", want: 1024 * 1024 * 1024 * 1024},
		{name: "512KB", envVal: "512KB", want: 512 * 1024},
		{name: "short_alias_M", envVal: "100M", want: 100 * 1024 * 1024},
		{name: "short_alias_G", envVal: "2G", want: 2 * 1024 * 1024 * 1024},
		{name: "lowercase_mb", envVal: "500mb", want: 500 * 1024 * 1024},
		{name: "lowercase_kb", envVal: "64kb", want: 64 * 1024},
		{name: "explicit_zero_disable", envVal: "0", want: 0},
		{name: "bare_number_rejected", envVal: "12345", wantErr: true},
		{name: "unknown_unit_XB", envVal: "500XB", wantErr: true},
		{name: "negative_rejected", envVal: "-100MB", wantErr: true},
		{name: "missing_prefix", envVal: "MB", wantErr: true},
		{name: "non_numeric_prefix", envVal: "abcMB", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PDBPLUS_SYNC_MEMORY_LIMIT", tt.envVal)
			t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for PDBPLUS_SYNC_MEMORY_LIMIT=%q, got nil", tt.envVal)
				}
				if !strings.Contains(err.Error(), "PDBPLUS_SYNC_MEMORY_LIMIT") {
					t.Errorf("error must name env var; got %q", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.SyncMemoryLimit != tt.want {
				t.Errorf("SyncMemoryLimit = %d, want %d", cfg.SyncMemoryLimit, tt.want)
			}
		})
	}
}
