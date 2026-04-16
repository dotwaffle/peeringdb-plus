package visbaseline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BuildReportConfig parameterises a BuildReport run. Grouped per GO-CS-5.
//
// BaselineRoot is resolved in one of two shapes:
//
//  1. Single-target: BaselineRoot directly contains "anon/" and "auth/"
//     subdirs (e.g. testdata/visibility-baseline/beta). BuildReport emits
//     a single DIFF.md + diff.json at OutDir covering the type-level deltas
//     for that target, with top-level keys namespaced by type name only.
//
//  2. Multi-target: BaselineRoot contains per-target subdirs each with
//     "anon/" + "auth/" (e.g. testdata/visibility-baseline/ containing
//     beta/ and prod/). BuildReport emits a unified DIFF.md + diff.json
//     at OutDir whose type keys are namespaced "{target}/{type}" AND emits
//     per-target auxiliary DIFF-{target}.md files for reviewers who only
//     want one target at a time.
//
// The caller picks the shape by choosing BaselineRoot; BuildReport
// auto-detects which one is present. An ambiguous tree (both direct
// anon/auth subdirs AND per-target subdirs) is rejected as a user error.
type BuildReportConfig struct {
	BaselineRoot string
	OutDir       string
	GeneratedAt  time.Time // optional; defaults to time.Now().UTC()
	Logger       *slog.Logger
}

// BuildReport walks BaselineRoot, runs Diff per type, and emits DIFF.md +
// diff.json at OutDir. See BuildReportConfig for the dual-shape contract.
//
// GO-CFG-1 fail-fast validation:
//   - BaselineRoot must be non-empty and a readable directory.
//   - OutDir must be non-empty and must NOT be the filesystem root.
//   - Single-target shape requires both anon/ and auth/ subdirs present.
//   - Multi-target shape requires at least one target subdir with both
//     anon/ and auth/ present.
func BuildReport(ctx context.Context, cfg BuildReportConfig) error {
	if err := validateBuildReportConfig(cfg); err != nil {
		return err
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	generatedAt := cfg.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}

	shape, targets, err := detectShape(cfg.BaselineRoot)
	if err != nil {
		return err
	}

	switch shape {
	case shapeSingle:
		return buildSingleTargetReport(ctx, cfg, logger, generatedAt)
	case shapeMulti:
		return buildMultiTargetReport(ctx, cfg, logger, generatedAt, targets)
	default:
		return fmt.Errorf("BuildReport: unreachable shape %v", shape)
	}
}

// validateBuildReportConfig implements the GO-CFG-1 fail-fast checks. It does
// NOT read from disk beyond stat — that is detectShape's responsibility.
func validateBuildReportConfig(cfg BuildReportConfig) error {
	if cfg.BaselineRoot == "" {
		return errors.New("BuildReport: BaselineRoot required")
	}
	if cfg.OutDir == "" {
		return errors.New("BuildReport: OutDir required")
	}
	// Reject filesystem root: on POSIX filepath.Dir("/") == "/", so
	// filepath.Dir(Clean(path)) == Clean(path) identifies a root-like value.
	// Bare "." is also rejected — writing DIFF.md into the CWD is almost
	// certainly an operator mistake and would scatter outputs into the repo
	// root alongside unrelated files.
	clean := filepath.Clean(cfg.OutDir)
	if clean == "." {
		return fmt.Errorf("BuildReport: OutDir %q resolves to current working directory; specify a dedicated output dir", cfg.OutDir)
	}
	if filepath.Dir(clean) == clean {
		return fmt.Errorf("BuildReport: OutDir %q is filesystem root; refusing to write there", cfg.OutDir)
	}
	if fi, err := os.Stat(cfg.BaselineRoot); err != nil || !fi.IsDir() {
		return fmt.Errorf("BuildReport: BaselineRoot %q not a readable directory: %w", cfg.BaselineRoot, err)
	}
	return nil
}

// shape enumerates the detected BaselineRoot layout.
type shape int

const (
	shapeUnknown shape = iota
	shapeSingle
	shapeMulti
)

// detectShape inspects BaselineRoot and reports whether it contains direct
// anon/ + auth/ subdirs (shapeSingle) or per-target subdirs each with
// anon/ + auth/ (shapeMulti). Ambiguous trees (both forms present) are
// rejected.
func detectShape(root string) (shape, []string, error) {
	hasAnon := isDir(filepath.Join(root, "anon"))
	hasAuth := isDir(filepath.Join(root, "auth"))

	entries, err := os.ReadDir(root)
	if err != nil {
		return shapeUnknown, nil, fmt.Errorf("read %s: %w", root, err)
	}
	var targets []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "anon" || name == "auth" {
			continue
		}
		sub := filepath.Join(root, name)
		if isDir(filepath.Join(sub, "anon")) && isDir(filepath.Join(sub, "auth")) {
			targets = append(targets, name)
		}
	}
	sort.Strings(targets)

	singleFormLikely := hasAnon && hasAuth
	multiFormLikely := len(targets) > 0
	switch {
	case singleFormLikely && multiFormLikely:
		return shapeUnknown, nil, fmt.Errorf("BuildReport: BaselineRoot %q contains both direct anon/auth subdirs AND per-target subdirs (%v); pick one layout", root, targets)
	case singleFormLikely:
		return shapeSingle, nil, nil
	case multiFormLikely:
		return shapeMulti, targets, nil
	default:
		return shapeUnknown, nil, fmt.Errorf("BuildReport: BaselineRoot %q has neither direct anon/+auth/ subdirs nor any per-target subdir with both; cannot build a diff", root)
	}
}

// isDir is a small Stat wrapper used by shape detection.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// buildSingleTargetReport runs Diff across all types in BaselineRoot and
// writes DIFF.md + diff.json under OutDir.
func buildSingleTargetReport(ctx context.Context, cfg BuildReportConfig, logger *slog.Logger, generatedAt time.Time) error {
	target := filepath.Base(filepath.Clean(cfg.BaselineRoot))
	anonDir := filepath.Join(cfg.BaselineRoot, "anon")
	authDir := filepath.Join(cfg.BaselineRoot, "auth")
	types, err := listTypes(anonDir, authDir)
	if err != nil {
		return err
	}

	rep := Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   generatedAt,
		Targets:       []string{target},
		Types:         make(map[string]TypeReport, len(types)),
	}
	for _, typeName := range types {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		tr, err := diffType(anonDir, authDir, typeName)
		if err != nil {
			return err
		}
		rep.Types[typeName] = tr
	}
	if err := os.MkdirAll(cfg.OutDir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", cfg.OutDir, err)
	}
	if err := writeReportArtifacts(cfg.OutDir, "DIFF.md", "diff.json", rep); err != nil {
		return err
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "diff report written",
		slog.String("target", target),
		slog.Int("types", len(rep.Types)),
		slog.String("out_dir", cfg.OutDir),
	)
	return nil
}

// buildMultiTargetReport runs Diff across all targets+types and writes a
// unified DIFF.md+diff.json plus per-target DIFF-{target}.md files.
func buildMultiTargetReport(ctx context.Context, cfg BuildReportConfig, logger *slog.Logger, generatedAt time.Time, targets []string) error {
	unified := Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   generatedAt,
		Targets:       targets,
		Types:         map[string]TypeReport{},
	}

	// Per-target reports are accumulated alongside the unified one so we can
	// emit DIFF-{target}.md files without re-walking.
	perTarget := make(map[string]Report, len(targets))
	for _, target := range targets {
		perTarget[target] = Report{
			SchemaVersion: ReportSchemaVersion,
			GeneratedAt:   generatedAt,
			Targets:       []string{target},
			Types:         map[string]TypeReport{},
		}
	}

	for _, target := range targets {
		anonDir := filepath.Join(cfg.BaselineRoot, target, "anon")
		authDir := filepath.Join(cfg.BaselineRoot, target, "auth")
		types, err := listTypes(anonDir, authDir)
		if err != nil {
			return err
		}
		for _, typeName := range types {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			tr, err := diffType(anonDir, authDir, typeName)
			if err != nil {
				return err
			}
			unified.Types[target+"/"+typeName] = tr
			rep := perTarget[target]
			rep.Types[typeName] = tr
			perTarget[target] = rep
		}
	}

	if err := os.MkdirAll(cfg.OutDir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", cfg.OutDir, err)
	}
	if err := writeReportArtifacts(cfg.OutDir, "DIFF.md", "diff.json", unified); err != nil {
		return err
	}
	for _, target := range targets {
		name := "DIFF-" + target + ".md"
		if err := writeMarkdownOnly(cfg.OutDir, name, perTarget[target]); err != nil {
			return err
		}
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "unified diff report written",
		slog.Int("targets", len(targets)),
		slog.Int("entries", len(unified.Types)),
		slog.String("out_dir", cfg.OutDir),
	)
	return nil
}

// listTypes returns the intersection of types present under both anon/api
// and auth/api subtrees. Both sides must be directories. A missing api/
// subdir on either side is an error.
func listTypes(anonDir, authDir string) ([]string, error) {
	anonTypes, err := typeNamesUnder(filepath.Join(anonDir, "api"))
	if err != nil {
		return nil, err
	}
	authTypes, err := typeNamesUnder(filepath.Join(authDir, "api"))
	if err != nil {
		return nil, err
	}
	anonSet := make(map[string]struct{}, len(anonTypes))
	for _, t := range anonTypes {
		anonSet[t] = struct{}{}
	}
	var both []string
	for _, t := range authTypes {
		if _, ok := anonSet[t]; ok {
			both = append(both, t)
		}
	}
	sort.Strings(both)
	if len(both) == 0 {
		return nil, fmt.Errorf("listTypes: no types common to %s and %s", anonDir, authDir)
	}
	return both, nil
}

// typeNamesUnder returns the sorted subdir names of path (treated as
// ".../api"). Missing path is an error because the caller already asserted
// the parent exists.
func typeNamesUnder(apiDir string) ([]string, error) {
	entries, err := os.ReadDir(apiDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", apiDir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// diffType runs Diff for a single type. It concatenates all page-N.json
// files on each side into one envelope's data array before passing to Diff,
// so per-type aggregate counts reflect every captured page.
func diffType(anonDir, authDir, typeName string) (TypeReport, error) {
	anonBytes, err := loadConcatenatedPages(filepath.Join(anonDir, "api", typeName))
	if err != nil {
		return TypeReport{}, fmt.Errorf("anon %s: %w", typeName, err)
	}
	authBytes, err := loadConcatenatedPages(filepath.Join(authDir, "api", typeName))
	if err != nil {
		return TypeReport{}, fmt.Errorf("auth %s: %w", typeName, err)
	}
	return Diff(typeName, anonBytes, authBytes)
}

// loadConcatenatedPages reads all page-N.json envelopes under dir and merges
// their data arrays into one envelope {meta:{},data:[...]}. The merged
// envelope is what Diff consumes. Pages are loaded in ascending page order.
// Types whose capture produced only page-1 contribute that single page.
func loadConcatenatedPages(dir string) ([]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	type pageFile struct {
		page int
		path string
	}
	var files []pageFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		_, page, ok := parsePagePath(filepath.Join(dir, e.Name()))
		if !ok {
			continue
		}
		files = append(files, pageFile{page: page, path: filepath.Join(dir, e.Name())})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].page < files[j].page })
	if len(files) == 0 {
		return nil, fmt.Errorf("no page-N.json files in %s", dir)
	}

	merged := struct {
		Meta any              `json:"meta"`
		Data []map[string]any `json:"data"`
	}{
		Meta: map[string]any{},
		Data: []map[string]any{},
	}

	for _, pf := range files {
		// pf.path is composed from dir (BuildReportConfig caller path) +
		// vetted page-N.json basename; CLI tool by design reads caller-
		// supplied paths.
		raw, err := os.ReadFile(pf.path) //nolint:gosec // G304: path derived from CLI caller.
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", pf.path, err)
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			// Don't embed raw bytes; auth pages in the staging tree can carry
			// PII until redaction. Basename is enough to locate the bad file.
			if strings.Contains(pf.path, string(filepath.Separator)+"auth"+string(filepath.Separator)) {
				return nil, fmt.Errorf("unmarshal auth file %s: %w", filepath.Base(pf.path), err)
			}
			return nil, fmt.Errorf("unmarshal %s: %w", pf.path, err)
		}
		merged.Data = append(merged.Data, env.Data...)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&merged); err != nil {
		return nil, fmt.Errorf("remarshal merged envelope: %w", err)
	}
	return buf.Bytes(), nil
}

// writeReportArtifacts writes DIFF.md + diff.json to outDir. It uses the
// existing WriteMarkdown and WriteJSON emitters.
func writeReportArtifacts(outDir, mdName, jsonName string, rep Report) error {
	if err := writeMarkdownOnly(outDir, mdName, rep); err != nil {
		return err
	}
	jsonPath := filepath.Join(outDir, jsonName)
	f, err := os.OpenFile(jsonPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", jsonPath, err)
	}
	defer f.Close()
	if err := WriteJSON(f, rep); err != nil {
		return fmt.Errorf("write %s: %w", jsonPath, err)
	}
	return nil
}

// writeMarkdownOnly writes a DIFF-style Markdown to outDir/name.
func writeMarkdownOnly(outDir, name string, rep Report) error {
	p := filepath.Join(outDir, name)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", p, err)
	}
	defer f.Close()
	if err := WriteMarkdown(f, rep); err != nil {
		return fmt.Errorf("write %s: %w", p, err)
	}
	return nil
}
