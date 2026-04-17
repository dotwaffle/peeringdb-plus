// Package sync — Plan 59-05 Task 2: single-call-site audit test.
//
// D-08/D-09 + RESEARCH Pitfall 4: the ent privacy bypass
// `privacy.DecisionContext(ctx, privacy.Allow)` must appear in exactly
// ONE production source location: internal/sync/worker.go. A second
// call site in any non-test file would be a silent policy bypass that
// lets a handler goroutine read Users-visibility rows. This test walks
// the source tree and fails the build if the invariant is violated.
//
// Test files (`*_test.go`) are exempt: the VIS-04/VIS-05 tests
// legitimately seed Users-tier rows via the bypass so they can assert
// the policy filters correctly (see internal/sync/policy_test.go,
// internal/sync/worker_test.go).
package sync

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// bypassCallRE matches the production call shape
// `privacy.DecisionContext(..., privacy.Allow...)`.
//
// Design notes:
//   - `(?s)` enables dotall so the non-greedy body `.*?` can cross newlines
//     — handles gofmt-split multi-line calls as well as the canonical
//     single-line form.
//   - The body regex uses `[^;]*?` to forbid statement boundaries so we
//     can't span two unrelated calls; nested parens (e.g.
//     `DecisionContext(context.Background(), privacy.Allow)`) are allowed.
//   - `privacy.Allow\b` (word boundary) prevents matching hypothetical
//     identifiers that start with "Allow" (e.g. `privacy.AllowAll`).
//   - Comments are stripped from the source before matching, so prose
//     that mentions the pattern is not counted as a call site.
var bypassCallRE = regexp.MustCompile(`(?s)privacy\.DecisionContext\([^;]*?privacy\.Allow\b`)

// bypassRefRE matches ANY reference to `privacy.Allow` — not just inside
// a DecisionContext(...) call. This catches aliasing evasions like:
//
//	allow := privacy.Allow
//	ctx = privacy.DecisionContext(ctx, allow)
//
// or package-level:
//
//	var bypass = privacy.Allow
//
// which `bypassCallRE` alone would miss (WR-01 from Phase 59 review).
// `\b` prevents matching `privacy.AllowAll` or similar future identifiers.
var bypassRefRE = regexp.MustCompile(`privacy\.Allow\b`)

// TestSyncBypass_SingleCallSite enforces D-09: exactly one production
// call site for the privacy bypass, located in internal/sync/worker.go.
//
// Scans `internal/`, `cmd/`, `ent/schema/`, and `graph/` — the four
// directories that hold hand-written Go code. Skips `*_test.go` (seeds
// use the bypass legitimately), skips generated code under `ent/`
// (except the hand-maintained `ent/schema/`), and skips `graph/generated.go`
// (gqlgen output — unlikely to contain privacy.Allow, but guarded).
//
// Two regexes run against the stripped source:
//
//   - `bypassCallRE` — the canonical `privacy.DecisionContext(..., privacy.Allow)`
//     shape. Exactly ONE hit expected in `internal/sync/worker.go`.
//   - `bypassRefRE` — ANY `privacy.Allow` reference (including aliasing
//     evasions like `allow := privacy.Allow`). Exactly ONE hit expected
//     in `internal/sync/worker.go`. This closes the WR-01 evasion channel
//     where a maintainer hoists `privacy.Allow` to a local/package
//     variable and calls `DecisionContext(ctx, bypass)` to slip past the
//     narrower `bypassCallRE`.
//
// Comments are stripped before matching so prose that mentions the
// pattern is not counted as a call site (see ent/schema/poc.go godoc).
func TestSyncBypass_SingleCallSite(t *testing.T) {
	t.Parallel()

	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("finding repo root: %v", err)
	}

	type hit struct {
		path string
		line int
		text string
	}
	var callHits []hit
	var refHits []hit

	scanDirs := []string{"internal", "cmd", "ent/schema", "graph"}
	for _, d := range scanDirs {
		base := filepath.Join(root, d)
		err := filepath.WalkDir(base, func(path string, ent fs.DirEntry, werr error) error {
			if werr != nil {
				return werr
			}
			if ent.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// Skip generated ent/ subtree (e.g. ent/poc/*.go) but keep
			// the hand-written ent/schema/*.go files.
			if strings.Contains(path, string(os.PathSeparator)+"ent"+string(os.PathSeparator)) &&
				!strings.Contains(path, string(os.PathSeparator)+"ent"+string(os.PathSeparator)+"schema"+string(os.PathSeparator)) {
				return nil
			}
			// Skip gqlgen-generated graph/generated.go — not hand-written.
			if strings.HasSuffix(path, string(os.PathSeparator)+"graph"+string(os.PathSeparator)+"generated.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			stripped := stripGoComments(string(b))
			recordHit := func(dst *[]hit, m []int) {
				lineNo := 1 + strings.Count(stripped[:m[0]], "\n")
				// Extract the first line of the match for the error message.
				text := stripped[m[0]:m[1]]
				if nl := strings.IndexByte(text, '\n'); nl >= 0 {
					text = text[:nl]
				}
				*dst = append(*dst, hit{
					path: path,
					line: lineNo,
					text: strings.TrimSpace(text),
				})
			}
			for _, m := range bypassCallRE.FindAllStringIndex(stripped, -1) {
				recordHit(&callHits, m)
			}
			for _, m := range bypassRefRE.FindAllStringIndex(stripped, -1) {
				recordHit(&refHits, m)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", base, err)
		}
	}

	const wantRelPath = "internal/sync/worker.go"

	assertSingleHit := func(kind string, hits []hit, help string) {
		t.Helper()
		switch {
		case len(hits) == 0:
			t.Fatalf("expected exactly 1 %s in production code; found 0. Did Plan 59-05 Task 1 run?", kind)
		case len(hits) > 1:
			var msg strings.Builder
			msg.WriteString("expected exactly 1 ")
			msg.WriteString(kind)
			msg.WriteString(" per D-09; found multiple:\n")
			for _, h := range hits {
				msg.WriteString("  ")
				msg.WriteString(h.path)
				msg.WriteString(":")
				msg.WriteString(strconv.Itoa(h.line))
				msg.WriteString("  ")
				msg.WriteString(h.text)
				msg.WriteString("\n")
			}
			msg.WriteString(help)
			t.Fatal(msg.String())
		}

		rel, err := filepath.Rel(root, hits[0].path)
		if err != nil {
			t.Fatalf("filepath.Rel(%q, %q): %v", root, hits[0].path, err)
		}
		// Normalise to forward-slash for cross-platform comparison.
		rel = filepath.ToSlash(rel)
		if rel != wantRelPath {
			t.Fatalf("%s must be in %s; found in %s:%d", kind, wantRelPath, rel, hits[0].line)
		}
	}

	assertSingleHit(
		"privacy.DecisionContext(ctx, privacy.Allow) call site",
		callHits,
		"Only internal/sync/worker.go may call privacy.DecisionContext(ctx, privacy.Allow).\n"+
			"Non-sync tier elevation must use internal/privctx.WithTier instead.",
	)
	assertSingleHit(
		"privacy.Allow reference",
		refHits,
		"Only internal/sync/worker.go may reference privacy.Allow.\n"+
			"Aliasing it to a local/package variable (e.g. `allow := privacy.Allow`) defeats the\n"+
			"single-audit-point invariant (D-09, WR-01). Non-sync tier elevation must use\n"+
			"internal/privctx.WithTier instead.",
	)
}

// findRepoRoot walks up from the current working directory looking for
// go.mod. Returns the directory containing it.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// stripGoComments removes `//` line comments and `/* ... */` block
// comments from src while preserving line numbers (newlines inside block
// comments are kept so that error messages point at the right line).
//
// Honours string and rune literals: a `//` inside a quoted string is
// NOT treated as a comment. Escape sequences inside strings are handled.
//
// This is a conservative scanner good enough for the audit invariant —
// it does not need to parse full Go syntax, just avoid false positives
// where the pattern appears in documentation.
func stripGoComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))

	var (
		inLineComment  bool
		inBlockComment bool
		inString       bool
		inRune         bool
		escaped        bool
	)
	for i := 0; i < len(src); i++ {
		c := src[i]
		var next byte
		if i+1 < len(src) {
			next = src[i+1]
		}

		switch {
		case inLineComment:
			if c == '\n' {
				inLineComment = false
				b.WriteByte(c)
			}
			// otherwise: elide the comment body
			continue
		case inBlockComment:
			if c == '*' && next == '/' {
				inBlockComment = false
				i++ // consume the '/'
				continue
			}
			if c == '\n' {
				// Preserve newlines inside block comments so the line
				// numbering stays aligned with the original source.
				b.WriteByte(c)
			}
			continue
		case inString:
			b.WriteByte(c)
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		case inRune:
			b.WriteByte(c)
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '\'':
				inRune = false
			}
			continue
		}

		// Not in any special mode.
		if c == '/' && next == '/' {
			inLineComment = true
			i++ // consume the second '/'
			continue
		}
		if c == '/' && next == '*' {
			inBlockComment = true
			i++ // consume the '*'
			continue
		}
		if c == '"' {
			inString = true
			b.WriteByte(c)
			continue
		}
		if c == '\'' {
			inRune = true
			b.WriteByte(c)
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
