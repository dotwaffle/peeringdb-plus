package main

import (
	"io"
	"log/slog"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRunCaptureFailFastNoAPIKey(t *testing.T) {
	t.Parallel()
	cfg := runConfig{capture: true, target: "beta", mode: "auth"}
	err := runCapture(cfg, discardLogger())
	if err == nil {
		t.Fatal("expected error when -mode=auth without API key, got nil")
	}
}

func TestRunCaptureProdAuthDowngradeFails(t *testing.T) {
	t.Parallel()
	cfg := runConfig{
		capture: true, target: "prod", mode: "auth",
		apiKey: "something", prodAuth: false,
	}
	err := runCapture(cfg, discardLogger())
	if err == nil {
		t.Fatal("expected error when prod+auth without -prod-auth, got nil")
	}
}

func TestRunCaptureUnknownTarget(t *testing.T) {
	t.Parallel()
	cfg := runConfig{capture: true, target: "staging", mode: "anon"}
	err := runCapture(cfg, discardLogger())
	if err == nil {
		t.Fatal("expected error for unknown target, got nil")
	}
}

func TestRunCaptureUnknownMode(t *testing.T) {
	t.Parallel()
	cfg := runConfig{capture: true, target: "beta", mode: "both-of-them"}
	err := runCapture(cfg, discardLogger())
	if err == nil {
		t.Fatal("expected error for unknown mode, got nil")
	}
}

func TestParseModes(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"anon": {"anon"},
		"auth": {"auth"},
		"both": {"anon", "auth"},
	}
	for in, want := range cases {
		got, err := parseModes(in)
		if err != nil {
			t.Errorf("parseModes(%q) unexpected error: %v", in, err)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("parseModes(%q) len = %d, want %d", in, len(got), len(want))
		}
	}
}

func TestResolveTypesProdDefault(t *testing.T) {
	t.Parallel()
	got := resolveTypes("", "prod")
	if len(got) != 3 {
		t.Fatalf("prod default types count = %d, want 3 (poc,org,net)", len(got))
	}
}

func TestResolveTypesBetaDefault(t *testing.T) {
	t.Parallel()
	got := resolveTypes("", "beta")
	if len(got) != 13 {
		t.Fatalf("beta default types count = %d, want 13", len(got))
	}
}

func TestResolveTypesExplicitList(t *testing.T) {
	t.Parallel()
	got := resolveTypes("poc, org , net", "beta")
	if len(got) != 3 {
		t.Fatalf("explicit types count = %d, want 3", len(got))
	}
	want := map[string]bool{"poc": true, "org": true, "net": true}
	for _, v := range got {
		if !want[v] {
			t.Errorf("unexpected type %q in %v", v, got)
		}
	}
}

func TestTargetBaseURLBeta(t *testing.T) {
	t.Parallel()
	got, err := targetBaseURL("beta")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://beta.peeringdb.com" {
		t.Errorf("beta = %q, want https://beta.peeringdb.com", got)
	}
}

func TestTargetBaseURLProd(t *testing.T) {
	t.Parallel()
	got, err := targetBaseURL("prod")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://www.peeringdb.com" {
		t.Errorf("prod = %q, want https://www.peeringdb.com", got)
	}
}

func TestRemoveString(t *testing.T) {
	t.Parallel()
	got := removeString([]string{"anon", "auth"}, "auth")
	if len(got) != 1 || got[0] != "anon" {
		t.Errorf("removeString = %v, want [anon]", got)
	}
	got = removeString([]string{"anon"}, "auth")
	if len(got) != 1 || got[0] != "anon" {
		t.Errorf("removeString on missing element = %v, want [anon]", got)
	}
}
