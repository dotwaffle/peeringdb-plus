package visbaseline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeBaselinePage writes a page JSON under <root>/<mode>/api/<type>/page-N.json.
// Unlike redactcli_test.writePage it accepts a separate root so both single-
// target and multi-target trees can be constructed with the same helper.
func writeBaselinePage(t *testing.T, root, mode, typeName string, page int, payload string) {
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

// TestBuildReportSingleTarget: build DIFF.md + diff.json for a single
// target dir. The report surfaces the auth-only email + phone fields and
// does NOT leak PII values (which are already placeholdered by redaction
// upstream — this test checks the report contract, not the redactor).
func TestBuildReportSingleTarget(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	anonPayload := `{"meta":{},"data":[{"id":1,"visible":"Public"},{"id":2,"visible":"Public"}]}`
	authPayload := `{"meta":{},"data":[
		{"id":1,"visible":"Public","email":"<auth-only:string>","phone":"<auth-only:string>"},
		{"id":2,"visible":"Public","email":"<auth-only:string>","phone":"<auth-only:string>"}
	]}`
	writeBaselinePage(t, tmp, "anon", "poc", 1, anonPayload)
	writeBaselinePage(t, tmp, "auth", "poc", 1, authPayload)

	cfg := BuildReportConfig{
		BaselineRoot: tmp,
		OutDir:       tmp,
		GeneratedAt:  time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC),
		Logger:       discardLogger(),
	}
	if err := BuildReport(context.Background(), cfg); err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	md, err := os.ReadFile(filepath.Join(tmp, "DIFF.md"))
	if err != nil {
		t.Fatalf("read DIFF.md: %v", err)
	}
	jsonBytes, err := os.ReadFile(filepath.Join(tmp, "diff.json"))
	if err != nil {
		t.Fatalf("read diff.json: %v", err)
	}

	if !strings.Contains(string(md), "### poc") {
		t.Errorf("DIFF.md missing '### poc' section:\n%s", md)
	}
	if !strings.Contains(string(md), "`email`") {
		t.Errorf("DIFF.md should reference email field:\n%s", md)
	}

	var rep Report
	if err := json.Unmarshal(jsonBytes, &rep); err != nil {
		t.Fatalf("unmarshal diff.json: %v", err)
	}
	tr, ok := rep.Types["poc"]
	if !ok {
		t.Fatalf("diff.json missing 'poc' type: %v", rep.Types)
	}
	if tr.AnonRowCount != 2 || tr.AuthRowCount != 2 {
		t.Errorf("row counts: anon=%d auth=%d, want 2/2", tr.AnonRowCount, tr.AuthRowCount)
	}
	seenEmail := false
	for _, fd := range tr.Fields {
		if fd.Name == "email" {
			seenEmail = true
			if !fd.AuthOnly {
				t.Errorf("email should be AuthOnly")
			}
			if !fd.IsPII {
				t.Errorf("email should be IsPII")
			}
		}
	}
	if !seenEmail {
		t.Errorf("diff.json 'poc' should contain an email field delta: %v", tr.Fields)
	}
}

// TestBuildReportMultiTarget: root with beta/+prod/ subdirs emits unified
// DIFF.md + diff.json with "{target}/{type}" keys AND per-target
// DIFF-{target}.md files.
func TestBuildReportMultiTarget(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	for _, target := range []string{"beta", "prod"} {
		root := filepath.Join(tmp, target)
		writeBaselinePage(t, root, "anon", "poc", 1,
			`{"meta":{},"data":[{"id":1,"visible":"Public"}]}`)
		writeBaselinePage(t, root, "auth", "poc", 1,
			`{"meta":{},"data":[{"id":1,"visible":"Public","email":"<auth-only:string>"}]}`)
	}

	cfg := BuildReportConfig{
		BaselineRoot: tmp,
		OutDir:       tmp,
		GeneratedAt:  time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC),
		Logger:       discardLogger(),
	}
	if err := BuildReport(context.Background(), cfg); err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	// Unified artifacts.
	md, err := os.ReadFile(filepath.Join(tmp, "DIFF.md"))
	if err != nil {
		t.Fatalf("read unified DIFF.md: %v", err)
	}
	jsonBytes, err := os.ReadFile(filepath.Join(tmp, "diff.json"))
	if err != nil {
		t.Fatalf("read unified diff.json: %v", err)
	}
	var rep Report
	if err := json.Unmarshal(jsonBytes, &rep); err != nil {
		t.Fatalf("unmarshal unified: %v", err)
	}
	if _, ok := rep.Types["beta/poc"]; !ok {
		t.Errorf("unified diff.json missing 'beta/poc': got %v", rep.Types)
	}
	if _, ok := rep.Types["prod/poc"]; !ok {
		t.Errorf("unified diff.json missing 'prod/poc': got %v", rep.Types)
	}
	if !strings.Contains(string(md), "`beta/poc`") && !strings.Contains(string(md), "### beta/poc") {
		t.Errorf("unified DIFF.md should reference beta/poc:\n%s", md)
	}

	// Per-target markdowns.
	for _, target := range []string{"beta", "prod"} {
		p := filepath.Join(tmp, "DIFF-"+target+".md")
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if !strings.Contains(string(b), "### poc") {
			t.Errorf("%s missing '### poc' section:\n%s", p, b)
		}
	}
}

// TestBuildReportFailFastEmptyOutDir asserts GO-CFG-1 rejects empty -out.
func TestBuildReportFailFastEmptyOutDir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	writeBaselinePage(t, tmp, "anon", "poc", 1, `{"meta":{},"data":[]}`)
	writeBaselinePage(t, tmp, "auth", "poc", 1, `{"meta":{},"data":[]}`)
	err := BuildReport(context.Background(), BuildReportConfig{
		BaselineRoot: tmp, OutDir: "",
	})
	if err == nil {
		t.Fatal("expected error for empty OutDir")
	}
}

// TestBuildReportFailFastOutDirRoot asserts "/" is rejected.
func TestBuildReportFailFastOutDirRoot(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	writeBaselinePage(t, tmp, "anon", "poc", 1, `{"meta":{},"data":[]}`)
	writeBaselinePage(t, tmp, "auth", "poc", 1, `{"meta":{},"data":[]}`)
	err := BuildReport(context.Background(), BuildReportConfig{
		BaselineRoot: tmp, OutDir: "/",
	})
	if err == nil {
		t.Fatal("expected error for root OutDir")
	}
	if !strings.Contains(err.Error(), "filesystem root") {
		t.Errorf("error = %v, want 'filesystem root'", err)
	}
}

// TestBuildReportFailFastOutDirDot asserts "." is rejected.
func TestBuildReportFailFastOutDirDot(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	writeBaselinePage(t, tmp, "anon", "poc", 1, `{"meta":{},"data":[]}`)
	writeBaselinePage(t, tmp, "auth", "poc", 1, `{"meta":{},"data":[]}`)
	err := BuildReport(context.Background(), BuildReportConfig{
		BaselineRoot: tmp, OutDir: ".",
	})
	if err == nil {
		t.Fatal("expected error for OutDir='.'")
	}
	if !strings.Contains(err.Error(), "current working directory") {
		t.Errorf("error = %v, want 'current working directory'", err)
	}
}

// TestBuildReportFailFastNoSubdirs: an empty baseline root (no anon/auth
// and no target subdirs) must fail rather than produce an empty report.
func TestBuildReportFailFastNoSubdirs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	empty := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	err := BuildReport(context.Background(), BuildReportConfig{
		BaselineRoot: empty, OutDir: tmp, Logger: discardLogger(),
	})
	if err == nil {
		t.Fatal("expected error for empty baseline root")
	}
}

// TestBuildReportAmbiguousShape: a root that has BOTH direct anon/auth AND
// per-target subdirs with anon/auth is rejected.
func TestBuildReportAmbiguousShape(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	writeBaselinePage(t, tmp, "anon", "poc", 1, `{"meta":{},"data":[]}`)
	writeBaselinePage(t, tmp, "auth", "poc", 1, `{"meta":{},"data":[]}`)
	writeBaselinePage(t, filepath.Join(tmp, "beta"), "anon", "poc", 1, `{"meta":{},"data":[]}`)
	writeBaselinePage(t, filepath.Join(tmp, "beta"), "auth", "poc", 1, `{"meta":{},"data":[]}`)

	err := BuildReport(context.Background(), BuildReportConfig{
		BaselineRoot: tmp,
		OutDir:       filepath.Join(tmp, "out"),
		Logger:       discardLogger(),
	})
	if err == nil {
		t.Fatal("expected error for ambiguous layout")
	}
	if !strings.Contains(err.Error(), "both") {
		t.Errorf("error = %v, want 'both' (ambiguous)", err)
	}
}

// TestBuildReportTwoPagesConcatenated: two pages per type contribute to the
// merged envelope so row counts match the total across pages.
func TestBuildReportTwoPagesConcatenated(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	writeBaselinePage(t, tmp, "anon", "poc", 1, `{"meta":{},"data":[{"id":1}]}`)
	writeBaselinePage(t, tmp, "anon", "poc", 2, `{"meta":{},"data":[{"id":2}]}`)
	writeBaselinePage(t, tmp, "auth", "poc", 1, `{"meta":{},"data":[{"id":1}]}`)
	writeBaselinePage(t, tmp, "auth", "poc", 2, `{"meta":{},"data":[{"id":2}]}`)

	cfg := BuildReportConfig{
		BaselineRoot: tmp, OutDir: tmp,
		GeneratedAt: time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC),
		Logger:      discardLogger(),
	}
	if err := BuildReport(context.Background(), cfg); err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(tmp, "diff.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rep Report
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	tr := rep.Types["poc"]
	if tr.AnonRowCount != 2 || tr.AuthRowCount != 2 {
		t.Errorf("row counts: anon=%d auth=%d, want 2/2 (concatenated across pages)",
			tr.AnonRowCount, tr.AuthRowCount)
	}
}

// TestBuildReportSingleTargetReportContainsTarget asserts the single-target
// report stamps the target in Report.Targets (derived from BaselineRoot's
// basename).
func TestBuildReportSingleTargetReportContainsTarget(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	betaDir := filepath.Join(tmp, "beta")
	writeBaselinePage(t, betaDir, "anon", "poc", 1, `{"meta":{},"data":[]}`)
	writeBaselinePage(t, betaDir, "auth", "poc", 1, `{"meta":{},"data":[]}`)

	outDir := filepath.Join(tmp, "out")
	cfg := BuildReportConfig{
		BaselineRoot: betaDir, OutDir: outDir,
		GeneratedAt: time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC),
		Logger:      discardLogger(),
	}
	if err := BuildReport(context.Background(), cfg); err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(outDir, "diff.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rep Report
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Targets) != 1 || rep.Targets[0] != "beta" {
		t.Errorf("Targets = %v, want [beta]", rep.Targets)
	}
}

// TestDetectShapeSingle asserts the detector returns shapeSingle for a bare
// anon+auth layout.
func TestDetectShapeSingle(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	for _, m := range []string{"anon", "auth"} {
		if err := os.MkdirAll(filepath.Join(tmp, m), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	s, targets, err := detectShape(tmp)
	if err != nil {
		t.Fatalf("detectShape: %v", err)
	}
	if s != shapeSingle {
		t.Errorf("shape = %v, want shapeSingle", s)
	}
	if len(targets) != 0 {
		t.Errorf("targets = %v, want empty", targets)
	}
}

// TestDetectShapeMulti asserts the detector returns shapeMulti for target
// subdirs each with anon+auth.
func TestDetectShapeMulti(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	for _, target := range []string{"beta", "prod"} {
		for _, m := range []string{"anon", "auth"} {
			if err := os.MkdirAll(filepath.Join(tmp, target, m), 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}
	s, targets, err := detectShape(tmp)
	if err != nil {
		t.Fatalf("detectShape: %v", err)
	}
	if s != shapeMulti {
		t.Errorf("shape = %v, want shapeMulti", s)
	}
	if len(targets) != 2 || targets[0] != "beta" || targets[1] != "prod" {
		t.Errorf("targets = %v, want [beta prod]", targets)
	}
}
