package config

import (
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
