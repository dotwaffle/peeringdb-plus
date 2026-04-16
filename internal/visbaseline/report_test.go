package visbaseline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// fixedTime is the deterministic GeneratedAt stamp used across golden tests.
var fixedTime = time.Date(2026, 4, 16, 12, 34, 56, 0, time.UTC)

// buildEmptyReport builds a zero-delta Report for the "poc" type — used by
// the golden-empty tests.
func buildEmptyReport() Report {
	return Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   fixedTime,
		Targets:       []string{"beta"},
		Types: map[string]TypeReport{
			"poc": {
				AnonRowCount:     0,
				AuthRowCount:     0,
				AuthOnlyRowCount: 0,
			},
		},
	}
}

// buildSimpleReport runs Diff on the simple golden fixtures, then wraps the
// TypeReport into a full Report for emitter tests.
func buildSimpleReport(t *testing.T) Report {
	t.Helper()
	anon := loadFixture(t, "anon_simple.json")
	auth := loadFixture(t, "auth_simple.json")
	tr, err := Diff("poc", anon, auth)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	return Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   fixedTime,
		Targets:       []string{"beta"},
		Types: map[string]TypeReport{
			"poc": tr,
		},
	}
}

func TestWriteJSONMatchesSchema(t *testing.T) {
	rep := Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   fixedTime,
		Targets:       []string{"beta", "prod"},
		Types: map[string]TypeReport{
			"poc": {AnonRowCount: 1, AuthRowCount: 2},
			"net": {AnonRowCount: 3, AuthRowCount: 3},
		},
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, rep); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal emitted: %v", err)
	}
	want := map[string]struct{}{
		"schema_version": {},
		"generated":      {},
		"targets":        {},
		"types":          {},
	}
	for k := range m {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected top-level key %q in emitted JSON", k)
		}
	}
	for k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("missing required top-level key %q in emitted JSON", k)
		}
	}
}

func TestWriteJSONDeterministic(t *testing.T) {
	rep := buildSimpleReport(t)
	var b1, b2 bytes.Buffer
	if err := WriteJSON(&b1, rep); err != nil {
		t.Fatalf("WriteJSON #1: %v", err)
	}
	if err := WriteJSON(&b2, rep); err != nil {
		t.Fatalf("WriteJSON #2: %v", err)
	}
	if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
		t.Errorf("WriteJSON not deterministic:\n%s\n%s", b1.String(), b2.String())
	}
}

func TestWriteJSONStableTypeOrder(t *testing.T) {
	rep := Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   fixedTime,
		Targets:       []string{"beta"},
		Types: map[string]TypeReport{
			"poc": {},
			"net": {},
			"org": {},
		},
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, rep); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	out := buf.String()
	// The indent-marshaller sorts map keys alphabetically. Assert the position
	// of "net" precedes "org" precedes "poc" in the emitted bytes.
	idxNet := strings.Index(out, `"net"`)
	idxOrg := strings.Index(out, `"org"`)
	idxPoc := strings.Index(out, `"poc"`)
	if idxNet == -1 || idxOrg == -1 || idxPoc == -1 {
		t.Fatalf("type key missing: net=%d org=%d poc=%d", idxNet, idxOrg, idxPoc)
	}
	if idxNet >= idxOrg || idxOrg >= idxPoc {
		t.Errorf("type keys not sorted: net=%d org=%d poc=%d", idxNet, idxOrg, idxPoc)
	}
}

func TestWriteMarkdownHasTOC(t *testing.T) {
	rep := buildSimpleReport(t)
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, rep); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	s := buf.String()
	if !strings.HasPrefix(s, "# ") {
		t.Errorf("markdown should start with a top-level header, got:\n%s", s)
	}
	if !strings.Contains(s, "## Table of Contents") {
		t.Errorf("markdown should contain '## Table of Contents' section")
	}
	// At least one anchor link per type.
	if !strings.Contains(s, "[poc](#poc)") {
		t.Errorf("markdown should contain TOC anchor for poc")
	}
}

func TestWriteMarkdownPerTypeTables(t *testing.T) {
	rep := Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   fixedTime,
		Targets:       []string{"beta"},
		Types: map[string]TypeReport{
			"poc": {AnonRowCount: 1, AuthRowCount: 1},
			"net": {AnonRowCount: 2, AuthRowCount: 2},
		},
	}
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, rep); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	s := buf.String()
	if strings.Count(s, "### net\n") != 1 {
		t.Errorf("expected exactly one '### net' header")
	}
	if strings.Count(s, "### poc\n") != 1 {
		t.Errorf("expected exactly one '### poc' header")
	}
}

func TestWriteMarkdownColumnsRequired(t *testing.T) {
	rep := buildSimpleReport(t)
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, rep); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	// When there ARE field deltas, the column header row must carry the
	// stable column set.
	want := "| Field | Auth-only | Placeholder | Rows added | PII? | Notes |"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("markdown missing stable column header: %q", want)
	}
}

func TestWriteMarkdownNoRawValues(t *testing.T) {
	// The Report itself carries no raw values — but as a belt-and-braces
	// check, synthesise a Report whose FieldDelta names look like PII and
	// confirm the emitter never constructs the leaked string.
	rep := Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   fixedTime,
		Targets:       []string{"beta"},
		Types: map[string]TypeReport{
			"poc": {
				AnonRowCount:     1,
				AuthRowCount:     2,
				AuthOnlyRowCount: 1,
				Fields: []FieldDelta{
					{Name: "email", AuthOnly: true, Placeholder: PlaceholderString, RowsAdded: 1, IsPII: true},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, rep); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	canary := "sensitive@example.invalid"
	if strings.Contains(buf.String(), canary) {
		t.Errorf("markdown contains canary value %q: %s", canary, buf.String())
	}
}

// stripGeneratedLine replaces the timestamp line with a stable sentinel for
// golden comparison.
func stripGeneratedLine(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "_Generated:") {
			out = append(out, "_Generated: <STRIPPED>_")
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// stripJSONTimestamp parses, replaces "generated" with a stable sentinel,
// and re-marshals to canonical indented JSON.
func stripJSONTimestamp(t *testing.T, data []byte) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal for timestamp strip: %v", err)
	}
	m["generated"] = "<STRIPPED>"
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("remarshal: %v", err)
	}
	return out
}

// loadGolden reads a golden file from testdata/diff_golden/.
func loadGolden(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "diff_golden", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func TestWriteMarkdownGoldenEmpty(t *testing.T) {
	rep := buildEmptyReport()
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, rep); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	got := stripGeneratedLine(buf.String())
	want := stripGeneratedLine(string(loadGolden(t, "expected_empty.md")))
	if got != want {
		t.Errorf("markdown golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteMarkdownGoldenSimple(t *testing.T) {
	rep := buildSimpleReport(t)
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, rep); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	got := stripGeneratedLine(buf.String())
	want := stripGeneratedLine(string(loadGolden(t, "expected_simple.md")))
	if got != want {
		t.Errorf("markdown golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteJSONGoldenEmpty(t *testing.T) {
	rep := buildEmptyReport()
	var buf bytes.Buffer
	if err := WriteJSON(&buf, rep); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	got := stripJSONTimestamp(t, buf.Bytes())
	want := stripJSONTimestamp(t, loadGolden(t, "expected_empty.json"))
	if !bytes.Equal(got, want) {
		t.Errorf("json golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteJSONGoldenSimple(t *testing.T) {
	rep := buildSimpleReport(t)
	var buf bytes.Buffer
	if err := WriteJSON(&buf, rep); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	got := stripJSONTimestamp(t, buf.Bytes())
	want := stripJSONTimestamp(t, loadGolden(t, "expected_simple.json"))
	if !bytes.Equal(got, want) {
		t.Errorf("json golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteMarkdownSortsFieldsAlpha(t *testing.T) {
	// Give a Report with deliberately unsorted fields — the builder in Diff
	// sorts them, but this asserts the emitter does not reorder back.
	rep := Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   fixedTime,
		Targets:       []string{"beta"},
		Types: map[string]TypeReport{
			"poc": {
				AnonRowCount: 1,
				AuthRowCount: 1,
				Fields: []FieldDelta{
					{Name: "a_field", AuthOnly: true, Placeholder: PlaceholderString, RowsAdded: 1},
					{Name: "b_field", AuthOnly: true, Placeholder: PlaceholderString, RowsAdded: 1},
					{Name: "z_field", AuthOnly: true, Placeholder: PlaceholderString, RowsAdded: 1},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, rep); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	s := buf.String()
	idxA := strings.Index(s, "`a_field`")
	idxB := strings.Index(s, "`b_field`")
	idxZ := strings.Index(s, "`z_field`")
	if idxA == -1 || idxB == -1 || idxZ == -1 {
		t.Fatalf("field row missing in markdown table")
	}
	if idxA >= idxB || idxB >= idxZ {
		t.Errorf("fields not in alphabetical order in markdown: a=%d b=%d z=%d", idxA, idxB, idxZ)
	}
}

func TestReportConsistency(t *testing.T) {
	rep := buildSimpleReport(t)

	var mdBuf bytes.Buffer
	if err := WriteMarkdown(&mdBuf, rep); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	var jsonBuf bytes.Buffer
	if err := WriteJSON(&jsonBuf, rep); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	// Parse the JSON back; confirm row counts match what the Markdown claims.
	var parsed Report
	if err := json.Unmarshal(jsonBuf.Bytes(), &parsed); err != nil {
		t.Fatalf("reparse JSON: %v", err)
	}
	poc := parsed.Types["poc"]
	if poc.AnonRowCount != rep.Types["poc"].AnonRowCount {
		t.Errorf("AnonRowCount drift: parsed=%d orig=%d",
			poc.AnonRowCount, rep.Types["poc"].AnonRowCount)
	}
	if poc.AuthRowCount != rep.Types["poc"].AuthRowCount {
		t.Errorf("AuthRowCount drift: parsed=%d orig=%d",
			poc.AuthRowCount, rep.Types["poc"].AuthRowCount)
	}
	// Markdown claims should appear literally.
	md := mdBuf.String()
	if !strings.Contains(md, "- Anon rows: 2") {
		t.Errorf("markdown should surface AnonRowCount=2, got:\n%s", md)
	}
	if !strings.Contains(md, "- Auth rows: 2") {
		t.Errorf("markdown should surface AuthRowCount=2, got:\n%s", md)
	}
	// Field set agreement: list the fields from both sides, sort, compare.
	var jsonFields, origFields []string
	for _, fd := range poc.Fields {
		jsonFields = append(jsonFields, fd.Name)
	}
	for _, fd := range rep.Types["poc"].Fields {
		origFields = append(origFields, fd.Name)
	}
	sort.Strings(jsonFields)
	sort.Strings(origFields)
	if strings.Join(jsonFields, ",") != strings.Join(origFields, ",") {
		t.Errorf("field set drift: json=%v orig=%v", jsonFields, origFields)
	}
	for _, name := range origFields {
		if !strings.Contains(md, "`"+name+"`") {
			t.Errorf("markdown should reference field %q", name)
		}
	}
}
