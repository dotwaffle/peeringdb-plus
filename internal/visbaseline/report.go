package visbaseline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// WriteJSON serialises the report as JSON with stable (alphabetical) map key
// ordering. The "generated" field is taken from rep.GeneratedAt; callers
// that want deterministic output across runs should stamp a fixed time.
//
// WriteJSON normalises nil Fields slices to []FieldDelta{} so they marshal
// as "[]" rather than "null". Empty-array is semantically "no deltas
// observed"; the schema stays stable for downstream JSON consumers that
// expect arrays. See 57-03-PLAN.md Task 2 note.
//
// HTML escaping is disabled so that placeholder strings such as
// "<auth-only:string>" remain greppable in diff.json and committed fixtures
// — json.Encoder's default escapes "<" and ">" to "\u003c"/"\u003e" which
// would defeat the CI grep audit for placeholder sentinels. Matches the
// encoder configuration in internal/visbaseline/redact.go.
func WriteJSON(w io.Writer, rep Report) error {
	if rep.SchemaVersion == 0 {
		rep.SchemaVersion = ReportSchemaVersion
	}
	for k, tr := range rep.Types {
		if tr.Fields == nil {
			tr.Fields = make([]FieldDelta, 0)
			rep.Types[k] = tr
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&rep); err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	// json.Encoder.Encode appends a trailing newline — keep exactly one
	// trailing newline by stripping any encoder-added newline then writing
	// our own.
	out := bytes.TrimRight(buf.Bytes(), "\n")
	if _, err := w.Write(out); err != nil {
		return fmt.Errorf("write report json: %w", err)
	}
	if _, err := w.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("write report newline: %w", err)
	}
	return nil
}

// WriteMarkdown emits one table per PeeringDB type, preceded by a TOC.
// Types are sorted alphabetically; field deltas within each type are emitted
// in their existing slice order (Diff already sorts alphabetically).
func WriteMarkdown(w io.Writer, rep Report) error {
	var b strings.Builder

	schemaVersion := rep.SchemaVersion
	if schemaVersion == 0 {
		schemaVersion = ReportSchemaVersion
	}

	fmt.Fprintf(&b, "# PeeringDB Visibility Baseline Diff\n\n")
	fmt.Fprintf(&b, "_Generated: %s_\n\n", rep.GeneratedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "_Schema version: %d_\n\n", schemaVersion)
	fmt.Fprintf(&b, "_Targets: %s_\n\n", strings.Join(rep.Targets, ", "))

	typeNames := make([]string, 0, len(rep.Types))
	for t := range rep.Types {
		typeNames = append(typeNames, t)
	}
	sort.Strings(typeNames)

	fmt.Fprintf(&b, "## Table of Contents\n\n")
	for _, t := range typeNames {
		fmt.Fprintf(&b, "- [%s](#%s)\n", t, t)
	}
	b.WriteString("\n")

	for _, t := range typeNames {
		tr := rep.Types[t]
		fmt.Fprintf(&b, "### %s\n\n", t)
		fmt.Fprintf(&b, "- Anon rows: %d\n", tr.AnonRowCount)
		fmt.Fprintf(&b, "- Auth rows: %d\n", tr.AuthRowCount)
		fmt.Fprintf(&b, "- Auth-only rows: %d\n", tr.AuthOnlyRowCount)
		if len(tr.VisibleValuesAnon) > 0 || len(tr.VisibleValuesAuth) > 0 {
			fmt.Fprintf(&b, "- `visible` values (anon): %s\n", bracketList(tr.VisibleValuesAnon))
			fmt.Fprintf(&b, "- `visible` values (auth): %s\n", bracketList(tr.VisibleValuesAuth))
		}
		b.WriteString("\n")

		if len(tr.Fields) == 0 {
			b.WriteString("No field-level deltas.\n\n")
			continue
		}

		b.WriteString("| Field | Auth-only | Placeholder | Rows added | PII? | Notes |\n")
		b.WriteString("|-------|-----------|-------------|------------|------|-------|\n")
		for _, fd := range tr.Fields {
			authOnly := "no"
			if fd.AuthOnly {
				authOnly = "yes"
			}
			isPII := "no"
			if fd.IsPII {
				isPII = "yes"
			}
			notes := ""
			if fd.ValueSetDrift {
				notes = "value set differs across modes"
			}
			fmt.Fprintf(&b, "| `%s` | %s | `%s` | %d | %s | %s |\n",
				fd.Name, authOnly, fd.Placeholder, fd.RowsAdded, isPII, notes)
		}
		b.WriteString("\n")
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write markdown: %w", err)
	}
	return nil
}

// bracketList renders a sorted string slice as a comma-separated backticked
// list. An empty slice renders as _(empty)_.
func bracketList(xs []string) string {
	if len(xs) == 0 {
		return "_(empty)_"
	}
	quoted := make([]string, len(xs))
	for i, x := range xs {
		quoted[i] = fmt.Sprintf("`%s`", x)
	}
	return strings.Join(quoted, ", ")
}
