package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunRedactRequiresInDir asserts -redact without -in fails fast.
func TestRunRedactRequiresInDir(t *testing.T) {
	t.Parallel()
	cfg := runConfig{outDir: t.TempDir()}
	err := runRedact(cfg, discardLogger())
	if err == nil {
		t.Fatal("expected error when -in is empty")
	}
	if !strings.Contains(err.Error(), "-in") {
		t.Errorf("error = %v, want mention of -in", err)
	}
}

// TestRunRedactRequiresOutDir asserts -redact without -out fails fast.
func TestRunRedactRequiresOutDir(t *testing.T) {
	t.Parallel()
	cfg := runConfig{inDir: t.TempDir()}
	err := runRedact(cfg, discardLogger())
	if err == nil {
		t.Fatal("expected error when -out is empty")
	}
	if !strings.Contains(err.Error(), "-out") {
		t.Errorf("error = %v, want mention of -out", err)
	}
}

// TestRunRedactOutMustEndInAuth asserts -out with a non-auth leaf fails.
func TestRunRedactOutMustEndInAuth(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfg := runConfig{
		inDir:  tmp,
		outDir: filepath.Join(tmp, "wrong"),
	}
	err := runRedact(cfg, discardLogger())
	if err == nil {
		t.Fatal("expected error for -out not ending in /auth/")
	}
	if !strings.Contains(err.Error(), "/auth/") && !strings.Contains(err.Error(), "auth") {
		t.Errorf("error = %v, want mention of auth path requirement", err)
	}
}

// TestRunDiffRequiresOutDir asserts -diff without -out fails fast.
func TestRunDiffRequiresOutDir(t *testing.T) {
	t.Parallel()
	cfg := runConfig{}
	err := runDiff(cfg, discardLogger())
	if err == nil {
		t.Fatal("expected error when -out is empty")
	}
	if !strings.Contains(err.Error(), "-out") {
		t.Errorf("error = %v, want mention of -out", err)
	}
}

// TestRunRedactHappyPathDerivesAnonDir: end-to-end through the CLI shim.
func TestRunRedactHappyPathDerivesAnonDir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// Layout: tmp/baseline/{anon,auth}/api/poc/page-1.json (anon side
	// pre-populated); separate raw-auth staging dir at tmp/raw/auth/...
	baseline := filepath.Join(tmp, "baseline")
	rawAuth := filepath.Join(tmp, "raw", "auth")

	anonDir := filepath.Join(baseline, "anon", "api", "poc")
	if err := os.MkdirAll(anonDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(anonDir, "page-1.json"),
		[]byte(`{"meta":{},"data":[{"id":1,"name_long":"A"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rawDir := filepath.Join(rawAuth, "api", "poc")
	if err := os.MkdirAll(rawDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rawDir, "page-1.json"),
		[]byte(`{"meta":{},"data":[{"id":1,"name_long":"A","email":"leak@x.invalid"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := runConfig{
		inDir:  rawAuth,
		outDir: filepath.Join(baseline, "auth"),
	}
	if err := runRedact(cfg, discardLogger()); err != nil {
		t.Fatalf("runRedact: %v", err)
	}
	out := filepath.Join(baseline, "auth", "api", "poc", "page-1.json")
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read %s: %v", out, err)
	}
	if strings.Contains(string(b), "leak@x.invalid") {
		t.Errorf("PII leak in CLI-driven redact:\n%s", b)
	}
}

// TestRunDiffHappyPath: end-to-end through the CLI shim.
func TestRunDiffHappyPath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	anonDir := filepath.Join(tmp, "anon", "api", "poc")
	authDir := filepath.Join(tmp, "auth", "api", "poc")
	for _, d := range []string{anonDir, authDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(anonDir, "page-1.json"),
		[]byte(`{"meta":{},"data":[{"id":1,"visible":"Public"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "page-1.json"),
		[]byte(`{"meta":{},"data":[{"id":1,"visible":"Public","email":"<auth-only:string>"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := runConfig{outDir: tmp}
	if err := runDiff(cfg, discardLogger()); err != nil {
		t.Fatalf("runDiff: %v", err)
	}
	for _, name := range []string{"DIFF.md", "diff.json"} {
		if _, err := os.Stat(filepath.Join(tmp, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}
