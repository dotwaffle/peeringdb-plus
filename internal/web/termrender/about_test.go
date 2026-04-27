package termrender

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// neutralPrivacySync is the "anon / public" PrivacySync payload reused by
// the pre-Phase-61 About tests that only exercise freshness + endpoints.
var neutralPrivacySync = templates.PrivacySync{
	AuthMode:              "Anonymous (no key)",
	PublicTier:            "public",
	PublicTierExplanation: "Anonymous callers see Public-only data (default).",
	OverrideActive:        false,
}

func TestRenderAboutPage_WithFreshness(t *testing.T) {
	t.Parallel()

	data := templates.DataFreshness{
		Available:  true,
		LastSyncAt: time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC),
		Age:        5 * time.Minute,
	}

	r := NewRenderer(ModeRich, false)
	var buf bytes.Buffer
	if err := r.RenderAboutPage(&buf, data, neutralPrivacySync); err != nil {
		t.Fatalf("RenderAboutPage() error: %v", err)
	}
	out := buf.String()
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"PeeringDB Plus",
		"Last Sync",
		"2025-06-15 12:30:00 UTC",
		"Data Age",
		"5m0s",
		"API Endpoints",
		"/graphql",
		"/rest/v1/",
		"/api/",
		"/ui/",
		"/peeringdb.v1.*/",
		"?format=json",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderAboutPage_NoFreshness(t *testing.T) {
	t.Parallel()

	data := templates.DataFreshness{Available: false}

	r := NewRenderer(ModeRich, false)
	var buf bytes.Buffer
	if err := r.RenderAboutPage(&buf, data, neutralPrivacySync); err != nil {
		t.Fatalf("RenderAboutPage() error: %v", err)
	}
	out := buf.String()
	stripped := ansiRE.ReplaceAllString(out, "")

	if !strings.Contains(stripped, "PeeringDB Plus") {
		t.Error("output missing project name")
	}
	if !strings.Contains(stripped, "No sync data available") {
		t.Error("output missing 'No sync data available'")
	}
	if strings.Contains(stripped, "Data Age") {
		t.Error("output should not contain 'Data Age' when freshness unavailable")
	}
	if !strings.Contains(stripped, "/graphql") {
		t.Error("output missing API endpoints")
	}
}

func TestRenderAboutPage_PlainMode(t *testing.T) {
	t.Parallel()

	data := templates.DataFreshness{
		Available:  true,
		LastSyncAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Age:        time.Hour,
	}

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer
	if err := r.RenderAboutPage(&buf, data, neutralPrivacySync); err != nil {
		t.Fatalf("RenderAboutPage() error: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "\x1b[") {
		t.Error("Plain mode output should not contain ANSI escape codes")
	}
	if !strings.Contains(out, "PeeringDB Plus") {
		t.Error("output missing project name in plain mode")
	}
}

// TestRenderAboutPage_PrivacySync exercises the Phase 61 OBS-02 Privacy & Sync
// terminal section across the four (auth mode, public tier) combinations.
// The "! " prefix on the Public Tier line is the D-06 override indicator —
// it must survive ANSI stripping so log-tailing operators still see it.
func TestRenderAboutPage_PrivacySync(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                  string
		privacy               templates.PrivacySync
		wantAuthMode          string
		wantPublicTier        string
		wantExplanation       string
		wantOverrideIndicator bool
	}{
		{
			name: "anon_public",
			privacy: templates.PrivacySync{
				AuthMode:              "Anonymous (no key)",
				PublicTier:            "public",
				PublicTierExplanation: "Anonymous callers see Public-only data (default).",
				OverrideActive:        false,
			},
			wantAuthMode:          "Anonymous (no key)",
			wantPublicTier:        "public",
			wantExplanation:       "Public-only data",
			wantOverrideIndicator: false,
		},
		{
			name: "auth_public",
			privacy: templates.PrivacySync{
				AuthMode:              "Authenticated with PeeringDB API key",
				PublicTier:            "public",
				PublicTierExplanation: "Anonymous callers see Public-only data (default).",
				OverrideActive:        false,
			},
			wantAuthMode:          "Authenticated with PeeringDB API key",
			wantPublicTier:        "public",
			wantExplanation:       "Public-only data",
			wantOverrideIndicator: false,
		},
		{
			name: "anon_users",
			privacy: templates.PrivacySync{
				AuthMode:              "Anonymous (no key)",
				PublicTier:            "users",
				PublicTierExplanation: "Anonymous callers see Users-tier data (internal/private deployment — override active).",
				OverrideActive:        true,
			},
			wantAuthMode:          "Anonymous (no key)",
			wantPublicTier:        "users",
			wantExplanation:       "override active",
			wantOverrideIndicator: true,
		},
		{
			name: "auth_users",
			privacy: templates.PrivacySync{
				AuthMode:              "Authenticated with PeeringDB API key",
				PublicTier:            "users",
				PublicTierExplanation: "Anonymous callers see Users-tier data (internal/private deployment — override active).",
				OverrideActive:        true,
			},
			wantAuthMode:          "Authenticated with PeeringDB API key",
			wantPublicTier:        "users",
			wantExplanation:       "override active",
			wantOverrideIndicator: true,
		},
	}

	data := templates.DataFreshness{
		Available:  true,
		LastSyncAt: time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		Age:        time.Minute,
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := NewRenderer(ModeRich, false)
			var buf bytes.Buffer
			if err := r.RenderAboutPage(&buf, data, tc.privacy); err != nil {
				t.Fatalf("RenderAboutPage: %v", err)
			}
			stripped := ansiRE.ReplaceAllString(buf.String(), "")

			if !strings.Contains(stripped, "Privacy & Sync") {
				t.Errorf("output missing 'Privacy & Sync' heading; got:\n%s", stripped)
			}
			if !strings.Contains(stripped, tc.wantAuthMode) {
				t.Errorf("output missing auth mode %q", tc.wantAuthMode)
			}
			if !strings.Contains(stripped, tc.wantPublicTier) {
				t.Errorf("output missing public tier %q", tc.wantPublicTier)
			}
			if !strings.Contains(stripped, tc.wantExplanation) {
				t.Errorf("output missing explanation fragment %q", tc.wantExplanation)
			}
			// "! users" is the override indicator prefix glued to the tier value
			// on the Public Tier line. It's unique — no other line carries it.
			if tc.wantOverrideIndicator {
				if !strings.Contains(stripped, "! users") {
					t.Errorf("override indicator '! users' missing when OverrideActive=true; got:\n%s", stripped)
				}
			} else {
				if strings.Contains(stripped, "! users") {
					t.Errorf("unexpected override indicator '! users' when OverrideActive=false; got:\n%s", stripped)
				}
			}
		})
	}
}
