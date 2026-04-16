package visbaseline

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePage writes a page JSON file under root/{mode}/api/{type}/page-N.json.
func writePage(t *testing.T, root, mode, typeName string, page int, payload string) {
	t.Helper()
	dir := filepath.Join(root, mode, "api", typeName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, "page-"+itoa(page)+".json")
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func itoa(n int) string {
	// Keep dependencies minimal in testdata helpers — stdlib strconv is fine
	// but json.MarshalIndent-style formatting is not needed here.
	switch n {
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	}
	// Fall back: for arbitrary n, format via fmt.
	return formatInt(n)
}

// formatInt is a tiny wrapper around strconv.Itoa to keep the import graph of
// the test file honest when golangci-lint complains about the strconv import
// existing solely for the itoa helper above.
func formatInt(n int) string {
	b := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestRedactDirBasic: capture two pages of one type, redact, confirm PII is
// replaced and the redacted files exist at the correct path.
func TestRedactDirBasic(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	rawAuth := filepath.Join(tmp, "auth")
	anon := filepath.Join(tmp, "anon")
	dst := filepath.Join(tmp, "redacted")

	writePage(t, tmp, "anon", "poc", 1,
		`{"meta":{},"data":[{"id":1,"name_long":"Alpha"},{"id":2,"name_long":"Bravo"}]}`)
	writePage(t, tmp, "auth", "poc", 1,
		`{"meta":{},"data":[{"id":1,"name_long":"Alpha","email":"x@alpha.invalid","phone":"+1-555-0001","name":"Admin Alpha"},{"id":2,"name_long":"Bravo","email":"y@bravo.invalid","phone":"+1-555-0002","name":"Admin Bravo"}]}`)

	cfg := RedactDirConfig{AuthSrc: rawAuth, AnonDir: anon, Dst: dst, Logger: discardLogger()}
	if err := RedactDir(context.Background(), cfg); err != nil {
		t.Fatalf("RedactDir: %v", err)
	}

	out := filepath.Join(dst, "api", "poc", "page-1.json")
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read %s: %v", out, err)
	}
	s := string(got)
	for _, leak := range []string{"x@alpha.invalid", "+1-555-0001", "Admin Alpha", "y@bravo.invalid", "+1-555-0002", "Admin Bravo"} {
		if strings.Contains(s, leak) {
			t.Errorf("redacted output contains PII %q:\n%s", leak, s)
		}
	}
	if !strings.Contains(s, PlaceholderString) {
		t.Errorf("redacted output missing placeholder %q:\n%s", PlaceholderString, s)
	}
	// Verify row count and ids preserved.
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(got, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Data) != 2 {
		t.Errorf("row count: got %d, want 2", len(env.Data))
	}
}

// TestRedactDirMultipleTypesAndPages: walk over two types × two pages and
// ensure every pair produces output.
func TestRedactDirMultipleTypesAndPages(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dst := filepath.Join(tmp, "redacted")

	for _, ty := range []string{"poc", "net"} {
		for p := 1; p <= 2; p++ {
			anonBody := `{"meta":{},"data":[{"id":1,"name_long":"X"}]}`
			authBody := `{"meta":{},"data":[{"id":1,"name_long":"X","email":"leak@x.invalid"}]}`
			writePage(t, tmp, "anon", ty, p, anonBody)
			writePage(t, tmp, "auth", ty, p, authBody)
		}
	}

	cfg := RedactDirConfig{
		AuthSrc: filepath.Join(tmp, "auth"),
		AnonDir: filepath.Join(tmp, "anon"),
		Dst:     dst, Logger: discardLogger(),
	}
	if err := RedactDir(context.Background(), cfg); err != nil {
		t.Fatalf("RedactDir: %v", err)
	}

	for _, ty := range []string{"poc", "net"} {
		for p := 1; p <= 2; p++ {
			out := filepath.Join(dst, "api", ty, "page-"+itoa(p)+".json")
			b, err := os.ReadFile(out)
			if err != nil {
				t.Fatalf("missing output %s: %v", out, err)
			}
			if strings.Contains(string(b), "leak@x.invalid") {
				t.Errorf("PII leaked in %s:\n%s", out, b)
			}
		}
	}
}

// TestRedactDirMissingAnonPairFails: an auth page without a matching anon
// page must halt with an error — skipping would over-disclose.
func TestRedactDirMissingAnonPairFails(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	writePage(t, tmp, "auth", "poc", 1, `{"meta":{},"data":[{"id":1,"email":"x@x.invalid"}]}`)
	// Anon dir exists but has no matching type/page.
	if err := os.MkdirAll(filepath.Join(tmp, "anon"), 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := RedactDirConfig{
		AuthSrc: filepath.Join(tmp, "auth"),
		AnonDir: filepath.Join(tmp, "anon"),
		Dst:     filepath.Join(tmp, "redacted"),
		Logger:  discardLogger(),
	}
	err := RedactDir(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for missing anon pair, got nil")
	}
	if !strings.Contains(err.Error(), "read anon") {
		t.Errorf("error = %v, want wrapping 'read anon'", err)
	}
}

// TestRedactDirRequiredArgs: missing AuthSrc/AnonDir/Dst all fail-fast.
func TestRedactDirRequiredArgs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	ok := filepath.Join(tmp, "ok")
	if err := os.MkdirAll(ok, 0o700); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		cfg  RedactDirConfig
		want string
	}{
		{"missing AuthSrc", RedactDirConfig{AnonDir: ok, Dst: ok}, "AuthSrc"},
		{"missing AnonDir", RedactDirConfig{AuthSrc: ok, Dst: ok}, "AnonDir"},
		{"missing Dst", RedactDirConfig{AuthSrc: ok, AnonDir: ok}, "Dst"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := RedactDir(context.Background(), tc.cfg)
			if err == nil {
				t.Fatalf("%s: want error, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s: err %q should mention %q", tc.name, err.Error(), tc.want)
			}
		})
	}
}

// TestRedactDirNonDirectoryArgs: AuthSrc/AnonDir that point at non-dirs fail.
func TestRedactDirNonDirectoryArgs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	notADir := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(notADir, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "ok")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	err := RedactDir(context.Background(), RedactDirConfig{
		AuthSrc: notADir, AnonDir: dir, Dst: dir,
	})
	if err == nil {
		t.Fatal("expected error for AuthSrc not a directory")
	}
}

// TestRedactDirNoPagesFound: an empty AuthSrc tree returns an error rather
// than silently succeeding.
func TestRedactDirNoPagesFound(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	for _, d := range []string{"auth", "anon"} {
		if err := os.MkdirAll(filepath.Join(tmp, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	err := RedactDir(context.Background(), RedactDirConfig{
		AuthSrc: filepath.Join(tmp, "auth"),
		AnonDir: filepath.Join(tmp, "anon"),
		Dst:     filepath.Join(tmp, "redacted"),
		Logger:  discardLogger(),
	})
	if err == nil {
		t.Fatal("expected error when no page-N.json files found")
	}
	if !strings.Contains(err.Error(), "no page-N.json files") {
		t.Errorf("error = %v, want 'no page-N.json files'", err)
	}
}

// TestRedactDirContextCancelled asserts early cancellation is honoured.
func TestRedactDirContextCancelled(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	writePage(t, tmp, "anon", "poc", 1, `{"meta":{},"data":[{"id":1}]}`)
	writePage(t, tmp, "auth", "poc", 1, `{"meta":{},"data":[{"id":1}]}`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RedactDir(ctx, RedactDirConfig{
		AuthSrc: filepath.Join(tmp, "auth"),
		AnonDir: filepath.Join(tmp, "anon"),
		Dst:     filepath.Join(tmp, "redacted"),
		Logger:  discardLogger(),
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

// TestParsePagePath covers the filename parser.
func TestParsePagePath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path     string
		wantType string
		wantPage int
		wantOK   bool
	}{
		{filepath.Join("root", "poc", "page-1.json"), "poc", 1, true},
		{filepath.Join("root", "net", "page-42.json"), "net", 42, true},
		{filepath.Join("root", "poc", "notapage.json"), "", 0, false},
		{filepath.Join("root", "poc", "page-abc.json"), "", 0, false},
		{filepath.Join("root", "poc", "page-0.json"), "", 0, false},
	}
	for _, c := range cases {
		ty, p, ok := parsePagePath(c.path)
		if ty != c.wantType || p != c.wantPage || ok != c.wantOK {
			t.Errorf("parsePagePath(%s) = (%q, %d, %t), want (%q, %d, %t)",
				c.path, ty, p, ok, c.wantType, c.wantPage, c.wantOK)
		}
	}
}
