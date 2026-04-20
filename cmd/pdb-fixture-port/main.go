package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"
)

// Process exit codes. run() returns one of these; main() passes the
// result to os.Exit so tests can drive run() without terminating the
// test process.
const (
	exitOK         = 0
	exitDrift      = 1
	exitUsage      = 2
	exitInternal   = 3
	exitFetchError = 4
)

// upstreamPath is the canonical location of the ground-truth file in
// peeringdb/peeringdb. Embedded in the output header so future readers
// can navigate straight to the source.
const upstreamPath = "src/peeringdb_server/management/commands/pdb_api_test.py"

// poCCap bounds the ordering-fixture PoC at a reviewable size. Plan
// 72-01 must produce ≥5; ≤12 keeps fixtures.go short enough to eyeball
// in a PR. Plans 72-02/03 replace this cap with per-category filters.
const poCCap = 12

// allCategoryOrder is the ordered list of (varName, parser) tuples
// emitted by `--category all`. Order is alphabetical-by-var-name to
// keep the on-disk file stable across category additions.
//
// As Go identifiers, the var names sort: In, Limit, Ordering,
// Status, Traversal, Unicode. The category strings below match that
// ordering after titleCase rewrites them.
var allCategoryOrder = []string{"in", "limit", "ordering", "status", "traversal", "unicode"}

// Fixture mirrors internal/testutil/parity.Fixture. Declared locally
// to avoid importing the target package from the codegen tool (keeps
// the tool independent of the runtime layering it generates).
type Fixture struct {
	Entity   string
	ID       int
	Fields   map[string]string
	Upstream string // e.g. "pdb_api_test.py:1479"
}

// FieldKeys returns the sorted keys of f.Fields. Used inside the
// template so map iteration order is deterministic on every run.
func (f Fixture) FieldKeys() []string {
	keys := make([]string, 0, len(f.Fields))
	for k := range f.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// renderData is the template input for fixtures.go.
type renderData struct {
	UpstreamCommit string // git SHA of peeringdb/peeringdb (or "local" when --upstream-file)
	UpstreamPath   string
	UpstreamHash   string // sha256 hex of the upstream bytes
	Category       string // "ordering" | "status" | "limit" | "all"
	Ported         string // YYYY-MM-DD
	Sections       []section
}

// section carries one var declaration's worth of fixtures plus its
// per-section preamble comment (used by `--category limit` to flag
// the synthesised bulk per Plan 72-02 D-02).
type section struct {
	Category string // "ordering" | "status" | "limit" — for downstream tooling
	VarName  string // e.g. "OrderingFixtures"
	Title    string // e.g. "Ordering"
	Preamble string // optional comment line emitted above the var
	Fixtures []Fixture
}

// entityGoName maps Python Django model names to the short PeeringDB
// type namespace used in fixtures.Entity. Unknown names (User, Group,
// EmailAddress, ...) are ignored by the parser.
var entityGoName = map[string]string{
	"Network":                  "net",
	"Organization":             "org",
	"InternetExchange":         "ix",
	"IXLan":                    "ixlan",
	"IXLanPrefix":              "ixpfx",
	"Facility":                 "fac",
	"NetworkIXLan":             "netixlan",
	"NetworkFacility":          "netfac",
	"NetworkContact":           "poc",
	"Carrier":                  "carrier",
	"CarrierFacility":          "carrierfac",
	"Campus":                   "campus",
	"InternetExchangeFacility": "ixfac",
}

// entityOffset keeps synthesised IDs from colliding across entity
// types in the emitted fixtures. Stable — must not be renumbered
// without regenerating and reviewing fixtures.go.
var entityOffset = map[string]int{
	"campus":     10000,
	"carrier":    11000,
	"carrierfac": 12000,
	"fac":        13000,
	"ix":         14000,
	"ixfac":      15000,
	"ixlan":      16000,
	"ixpfx":      17000,
	"net":        18000,
	"netfac":     19000,
	"netixlan":   20000,
	"org":        21000,
	"poc":        22000,
}

// createLinePat matches `X.objects.create(` with X captured — the
// upstream DSL start marker. The trailing `(` anchors the match so a
// stray reference like `Network.objects.filter(` is not caught.
var createLinePat = regexp.MustCompile(`\b([A-Z][A-Za-z]+)\.objects\.create\(`)

// fieldPat captures `key=value` pairs within a fixture block. Value
// is captured verbatim; the parser tracks paren depth explicitly so
// this regex only separates key from the raw value expression.
var fieldPat = regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*(.*?)\s*$`)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// runOptions captures the CLI flag surface. Extracted to a struct so
// tests can exercise run() with typed configurations.
type runOptions struct {
	UpstreamFile   string
	UpstreamRef    string
	UpstreamCommit string // when --upstream-file is set, override the "local" sentinel with this SHA
	Out            string
	Category       string
	Check          bool
	Pinned         string
	Date           string
	Append         bool
}

// run is the testable entry point. Returns a process exit code.
// stdout and stderr are injected so tests can capture output without
// temporarily reassigning the os globals.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("pdb-fixture-port", flag.ContinueOnError)
	fs.SetOutput(stderr)
	opts := &runOptions{}
	fs.StringVar(&opts.UpstreamFile, "upstream-file", "", "local path to pdb_api_test.py (overrides --upstream-ref)")
	fs.StringVar(&opts.UpstreamRef, "upstream-ref", "master", "git ref to fetch via `gh api` when --upstream-file is empty")
	fs.StringVar(&opts.UpstreamCommit, "upstream-commit", "", "override the 'local' sentinel commit SHA recorded in fixtures.go header (only used with --upstream-file)")
	fs.StringVar(&opts.Out, "out", "internal/testutil/parity/fixtures.go", "output file path")
	fs.StringVar(&opts.Category, "category", "ordering", "fixture category: ordering | status | limit | all")
	fs.BoolVar(&opts.Check, "check", false, "advisory drift-check mode; does not write")
	fs.StringVar(&opts.Pinned, "pinned", "", "expected sha256 of upstream file for --check")
	fs.StringVar(&opts.Date, "date", time.Now().UTC().Format("2006-01-02"), "ported-on date stamp (UTC)")
	fs.BoolVar(&opts.Append, "append", false, "rewrite only the requested category in --out, preserving other category blocks byte-identically")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "pdb-fixture-port: port peeringdb/peeringdb pdb_api_test.py fixtures to Go.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Usage: pdb-fixture-port [--upstream-file path | --upstream-ref ref] [--out path] [--category ordering|status|limit|all] [--append] [--check --pinned SHA256]")
		fmt.Fprintln(stderr, "")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}

	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	srcBytes, commitSHA, err := resolveUpstream(opts, logger)
	if err != nil {
		fmt.Fprintf(stderr, "pdb-fixture-port: resolve upstream: %v\n", err)
		return exitFetchError
	}

	hash := sha256Hex(srcBytes)

	if opts.Check {
		return doCheck(opts, hash, stdout, stderr)
	}

	sections, err := buildSections(srcBytes, opts.Category)
	if err != nil {
		fmt.Fprintf(stderr, "pdb-fixture-port: parse %q: %v\n", opts.Category, err)
		return exitInternal
	}

	// Determinism: sort each section by (Entity, ID). Matches the
	// pattern used by cmd/pdb-compat-allowlist so two runs with the
	// same upstream bytes produce byte-identical output.
	for i := range sections {
		sortFixtures(sections[i].Fixtures)
	}

	data := renderData{
		UpstreamCommit: commitSHA,
		UpstreamPath:   upstreamPath,
		UpstreamHash:   hash,
		Category:       opts.Category,
		Ported:         opts.Date,
		Sections:       sections,
	}

	src, err := renderTemplate(data)
	if err != nil {
		fmt.Fprintf(stderr, "pdb-fixture-port: render: %v\n", err)
		return exitInternal
	}
	formatted, err := format.Source(src)
	if err != nil {
		// Persist raw output for debugging; mirror cmd/pdb-compat-allowlist.
		_ = os.WriteFile(opts.Out+".broken", src, 0o600)
		fmt.Fprintf(stderr, "pdb-fixture-port: gofmt output: %v (raw at %s.broken)\n", err, opts.Out)
		return exitInternal
	}

	finalBytes := formatted
	if opts.Append {
		// Append mode: rewrite ONLY the requested category's var
		// block in the existing --out file, preserving other vars
		// byte-identically (and the existing header). Falls back to
		// a full write when --out doesn't yet exist.
		merged, mergeErr := appendCategory(opts.Out, formatted, opts.Category)
		if mergeErr != nil {
			fmt.Fprintf(stderr, "pdb-fixture-port: --append merge: %v\n", mergeErr)
			return exitInternal
		}
		finalBytes = merged
	}

	if err := writeAtomic(opts.Out, finalBytes); err != nil {
		fmt.Fprintf(stderr, "pdb-fixture-port: write %s: %v\n", opts.Out, err)
		return exitInternal
	}

	totalFixtures := 0
	for _, s := range sections {
		totalFixtures += len(s.Fixtures)
	}
	logger.Info("fixtures emitted",
		slog.String("out", opts.Out),
		slog.String("category", opts.Category),
		slog.Int("count", totalFixtures),
		slog.String("upstream_commit", commitSHA),
		slog.String("upstream_sha256", hash),
	)
	return exitOK
}

// sortFixtures applies the determinism contract: sort by (Entity, ID)
// in-place. Extracted so each per-category parser need not re-implement
// the pattern, and so future sections inherit the same invariant.
func sortFixtures(fxs []Fixture) {
	sort.Slice(fxs, func(i, j int) bool {
		if fxs[i].Entity != fxs[j].Entity {
			return fxs[i].Entity < fxs[j].Entity
		}
		return fxs[i].ID < fxs[j].ID
	})
}

// buildSections expands --category into the list of (varName,
// fixtures, comment) sections to emit. For singular categories
// (`ordering`/`status`/`limit`/`unicode`/`in`/`traversal`) returns
// one section; for `all` returns six sections in
// alphabetical-by-var-name order.
func buildSections(srcBytes []byte, category string) ([]section, error) {
	switch category {
	case "ordering":
		return []section{newSection("ordering", parseOrdering(srcBytes))}, nil
	case "status":
		return []section{newSection("status", parseStatus(srcBytes))}, nil
	case "limit":
		return []section{newSection("limit", parseLimit(srcBytes))}, nil
	case "unicode":
		return []section{newSection("unicode", parseUnicode(srcBytes))}, nil
	case "in":
		return []section{newSection("in", parseIn(srcBytes))}, nil
	case "traversal":
		return []section{newSection("traversal", parseTraversal(srcBytes))}, nil
	case "all":
		out := make([]section, 0, len(allCategoryOrder))
		for _, cat := range allCategoryOrder {
			subs, err := buildSections(srcBytes, cat)
			if err != nil {
				return nil, err
			}
			out = append(out, subs...)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown category %q (supported: ordering|status|limit|unicode|in|traversal|all)", category)
	}
}

// newSection assembles a section{} with the per-category varName,
// title, and provenance preamble (synthesised marker for limit).
func newSection(category string, fixtures []Fixture) section {
	s := section{
		Category: category,
		VarName:  titleCase(category) + "Fixtures",
		Title:    titleCase(category),
		Fixtures: fixtures,
	}
	switch category {
	case "limit":
		// Plan 72-02 D-02 path: limit bulk Network rows are
		// synthesised because upstream pdb_api_test.py does not
		// include a literal 260-row block. The provenance
		// preamble points at the authoritative behaviour citation.
		s.Preamble = "// synthesised per Plan 72-02 D-02: covers LIMIT-01 unlimited boundary at upstream rest.py:494-497"
	case "unicode":
		// Plan 72-03 synthesis path: 6 entities × 4 folded fields ×
		// 9 sample inputs matrix targets the Phase 69 unifold pipeline.
		// Per-entry citations point at upstream substring matches when
		// available; rest.py:576 (unidecode call site) is the
		// authoritative behaviour anchor.
		s.Preamble = "// synthesised per Plan 72-03: covers UNICODE-01/02 fold matrix at upstream rest.py:576"
	case "in":
		// Plan 72-03 synthesis path: 5001 contiguous Network rows
		// (IDs 100000..105000) for IN-01 large-list boundary +
		// sentinel for IN-02 empty-__in path.
		s.Preamble = "// synthesised per Plan 72-03: covers IN-01 large-list boundary + IN-02 empty-__in sentinel at upstream rest.py:IN-01-json_each"
	case "traversal":
		// Plan 72-03 synthesis path: ring topology rooted at one
		// Organization (id=200001). 1-hop + 2-hop fixtures cite
		// pdb_api_test.py:5081 / :2340 respectively. Silent-ignore
		// fixture (TRAVERSAL-04) covers the 3+-hop cap.
		s.Preamble = "// synthesised per Plan 72-03: ring topology covers TRAVERSAL-01..04 at upstream pdb_api_test.py:5081 (1-hop) + :2340 (2-hop)"
	}
	return s
}

// doCheck compares the observed upstream-file sha256 to the expected
// value. Per D-03 the comparison is advisory: exit 1 on mismatch so a
// scheduled CI job surfaces the drift, but do NOT gate PR merges.
func doCheck(opts *runOptions, observed string, stdout, stderr io.Writer) int {
	// When --pinned is empty, fall back to reading the header of the
	// current --out file so `--check` works without the caller looking
	// up the prior SHA by hand.
	expected := opts.Pinned
	if expected == "" {
		headerHash, err := readHeaderHash(opts.Out)
		if err != nil {
			fmt.Fprintf(stderr, "pdb-fixture-port: --check needs --pinned or a readable --out file: %v\n", err)
			return exitUsage
		}
		expected = headerHash
	}
	if expected != observed {
		fmt.Fprintf(stderr, "pdb-fixture-port: upstream drift — expected sha256=%s, observed sha256=%s\n", expected, observed)
		return exitDrift
	}
	fmt.Fprintf(stdout, "pdb-fixture-port: upstream matches pinned sha256=%s\n", observed)
	return exitOK
}

// resolveUpstream returns (srcBytes, commitSHA, err). If UpstreamFile
// is set, reads locally and returns commitSHA == "local" (tool user
// is responsible for ensuring the local bytes match the intended
// ref). Otherwise shells out to `gh api` to fetch the ref's contents
// plus commit SHA. The sandbox's github.com allowlist covers this.
func resolveUpstream(opts *runOptions, logger *slog.Logger) ([]byte, string, error) {
	if opts.UpstreamFile != "" {
		b, err := os.ReadFile(opts.UpstreamFile)
		if err != nil {
			return nil, "", fmt.Errorf("read --upstream-file %s: %w", opts.UpstreamFile, err)
		}
		// --upstream-commit lets the operator pin the recorded SHA
		// when the local file is known to match a specific upstream
		// ref (e.g. mirroring a downloaded snapshot). Defaults to
		// "local" sentinel when not provided.
		commitSHA := "local"
		if opts.UpstreamCommit != "" {
			commitSHA = opts.UpstreamCommit
		}
		return b, commitSHA, nil
	}

	// `gh api … --jq .content` returns base64-encoded content. gh
	// wraps long base64 bodies with newlines; strip before decoding.
	contentB64, err := runGhAPI("repos/peeringdb/peeringdb/contents/" + upstreamPath + "?ref=" + opts.UpstreamRef, "--jq", ".content")
	if err != nil {
		return nil, "", fmt.Errorf("gh api contents: %w", err)
	}
	cleaned := strings.ReplaceAll(strings.TrimSpace(contentB64), "\n", "")
	srcBytes, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, "", fmt.Errorf("decode upstream base64: %w", err)
	}

	shaBytes, err := runGhAPI("repos/peeringdb/peeringdb/commits/"+opts.UpstreamRef, "--jq", ".sha")
	if err != nil {
		return nil, "", fmt.Errorf("gh api commits: %w", err)
	}
	commitSHA := strings.TrimSpace(shaBytes)
	logger.Info("upstream fetched",
		slog.String("ref", opts.UpstreamRef),
		slog.String("commit", commitSHA),
		slog.Int("bytes", len(srcBytes)),
	)
	return srcBytes, commitSHA, nil
}

// runGhAPI shells out to `gh api <path> <extra...>` and returns
// stdout. Kept as a single helper so tests can swap it out if needed.
func runGhAPI(path string, extra ...string) (string, error) {
	args := append([]string{"api", path}, extra...)
	// #nosec G204 — arguments constructed from fixed path + internal flags.
	cmd := exec.Command("gh", args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
	}
	return out.String(), nil
}

// parseOrdering scans the upstream Python file for Django fixture
// blocks and extracts a curated slice relevant to the ordering
// category (rows created via `X.objects.create(...)` with keyword
// arguments — `updated`/`created` pairs drive the (-updated, -created)
// default ordering assertion).
//
// The parser operates line-by-line with paren-depth tracking. This is
// lightweight but deliberate: upstream uses a consistent multi-line
// indent style, so a regex-plus-state-machine approach captures the
// 90%-case without pulling in a full Python parser.
//
// Every extracted row carries an Upstream citation of form
// "pdb_api_test.py:<line>". Per plan must_haves, ≥5 entries are
// required; the parser synthesises a deterministic ID from the upstream
// line number when a pk=... isn't provided (Django auto-allocates).
func parseOrdering(srcBytes []byte) []Fixture {
	scanner := bufio.NewScanner(bytes.NewReader(srcBytes))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // tolerate long lines
	lineNum := 0

	var out []Fixture
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		m := createLinePat.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		entity, ok := entityGoName[m[1]]
		if !ok {
			// Non-PeeringDB model (User, Group, EmailAddress, ...)
			continue
		}
		startLine := lineNum
		block, endLine := readFixtureBlock(scanner, line, lineNum)
		lineNum = endLine
		fields := extractFields(block)
		if len(fields) == 0 {
			continue
		}
		id := synthID(entity, startLine, fields)
		// Synthesise `updated` and `created` timestamps for ordering
		// tests. Upstream Django auto-populates these via handleref
		// (not declared in the source), but the PARITY-01 ordering
		// assertion requires stable, differentiated timestamps to
		// verify `(-updated, -created)` default ordering. Derive the
		// timestamps from the upstream source line so they stay stable
		// across tool reruns and maintain an intuitive "lower line =
		// older row" ordering. The base epoch (2024-01-01T00:00:00Z)
		// is arbitrary but documented here so future maintainers don't
		// re-derive. Plans 72-02/03 may overlay real timestamps when
		// upstream fixtures include them.
		if _, hasCreated := fields["created"]; !hasCreated {
			fields["created"] = fmt.Sprintf("%q", orderingCreatedAt(startLine).Format(time.RFC3339))
		}
		if _, hasUpdated := fields["updated"]; !hasUpdated {
			fields["updated"] = fmt.Sprintf("%q", orderingUpdatedAt(startLine).Format(time.RFC3339))
		}
		out = append(out, Fixture{
			Entity:   entity,
			ID:       id,
			Fields:   fields,
			Upstream: lineCitation(startLine),
		})
		if len(out) >= poCCap {
			// Stop scanning once the PoC cap is hit. Keeps fixtures.go
			// short enough to eyeball in a PR; 72-02/03 remove the cap.
			break
		}
	}
	return out
}

// orderingBaseEpoch anchors synthesised ordering timestamps. Pinned
// here (not in CONTEXT.md) because future maintainers reading only
// fixtures.go need to trace back the source; the constant is also
// exercised by main_test.go so moving it triggers a visible diff.
var orderingBaseEpoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// orderingCreatedAt returns a synthesised `created` timestamp for an
// ordering fixture sourced from the given upstream line number. One
// hour per line keeps timestamps well-separated on the wire so
// ordering assertions are unambiguous even under clock skew.
func orderingCreatedAt(line int) time.Time {
	return orderingBaseEpoch.Add(time.Duration(line) * time.Hour)
}

// orderingUpdatedAt returns a synthesised `updated` timestamp. Offset
// +24h from `created` so the `(-updated, -created)` compound ordering
// has two moving parts that can diverge per row (plan 72-01 PoC —
// plans 72-02/03 extend with updates-newer-than-created variants).
func orderingUpdatedAt(line int) time.Time {
	return orderingCreatedAt(line).Add(24 * time.Hour)
}

// readFixtureBlock continues reading from scanner until the paren
// depth (opened by the create() on firstLine) returns to zero.
// Returns the joined block string and the absolute line number of the
// closing paren.
func readFixtureBlock(scanner *bufio.Scanner, firstLine string, startLine int) (string, int) {
	depth := parenDelta(firstLine)
	var b strings.Builder
	b.WriteString(firstLine)
	b.WriteString("\n")
	line := startLine
	for depth > 0 && scanner.Scan() {
		line++
		cur := scanner.Text()
		b.WriteString(cur)
		b.WriteString("\n")
		depth += parenDelta(cur)
	}
	return b.String(), line
}

// parenDelta returns (#'(' - #')') ignoring characters inside Python
// string literals. Upstream uses both "…" and '…' quote styles, so the
// scanner tracks both and honours backslash escapes.
func parenDelta(s string) int {
	depth := 0
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && (inSingle || inDouble):
			// Escape consumes next byte; skip depth logic.
			if i+1 < len(s) {
				i++
			}
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == '(' && !inSingle && !inDouble:
			depth++
		case c == ')' && !inSingle && !inDouble:
			depth--
		}
	}
	return depth
}

// extractFields scans a fixture block for key=value lines and returns
// a map. Values are captured verbatim (including quotes) so the
// emitted Go literal can carry the original expression; callers that
// need typed values can re-parse per-column.
//
// Lines starting with `**` (Python kwargs-splat — e.g.
// `**self.make_data_net()`) are skipped because their resolved fields
// live in a helper function we don't evaluate.
//
// NOTE on backward compatibility: this parser folds same-line trailing
// kwargs into the leading kwarg's value (e.g. `status="ok",
// name="X"` becomes `status: "ok", name=\"X\"`). Plan 72-01 OrderingFixtures
// were generated with this behaviour and the byte-identical-to-72-01
// constraint requires preserving it. parseStatus / parseLimit use
// extractFieldsSharp instead — they need per-kwarg fidelity.
func extractFields(block string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(block, "\n")
	for i, ln := range lines {
		if i == 0 {
			// Skip the `X.objects.create(` header line.
			continue
		}
		trim := strings.TrimSpace(ln)
		if trim == "" || trim == ")" || strings.HasPrefix(trim, "#") {
			continue
		}
		trim = strings.TrimSuffix(trim, ",")
		if strings.HasPrefix(trim, "**") {
			continue
		}
		m := fieldPat.FindStringSubmatch(trim)
		if m == nil {
			continue
		}
		key, val := m[1], m[2]
		val = strings.TrimSuffix(val, ",")
		// Skip values that continue across lines (open paren/bracket
		// without its close). Conservative — ordering-PoC doesn't need
		// multi-line values.
		if parenDelta(val) != 0 {
			continue
		}
		out[key] = val
	}
	return out
}

// extractFieldsSharp is a per-kwarg-fidelity extractor used by
// parseStatus / parseLimit. It splits every fixture-block line on
// top-level commas (commas not inside strings or nested parens) and
// applies fieldPat to each fragment. Each kwarg ends up as a
// separate map entry, so `status="pending", name="X"` produces
// `{"status": "\"pending\"", "name": "\"X\""}` rather than the
// folded form extractFields produces.
//
// Same skip rules as extractFields: blank lines, comment lines,
// lone `)`, kwargs-splat `**...`, and values that span across
// lines (paren-delta != 0). Per-kwarg fragments are evaluated
// independently — folding never happens.
//
// Unlike extractFields, the header line is NOT blindly skipped:
// upstream contains many one-line `Foo.objects.create(status="...",
// name="...")` blocks (e.g. line 6252 of the 99e92c72 ref). For
// those, the first (and only) line carries all kwargs between the
// outermost parens. The implementation strips the `Foo.objects.
// create(` prefix and trailing `)` from the first line before
// splitting; multi-line blocks have their first-line kwargs (after
// the `(`) parsed too.
func extractFieldsSharp(block string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(block, "\n")
	for i, ln := range lines {
		trim := strings.TrimSpace(ln)
		if i == 0 {
			// Strip the `X.objects.create(` prefix; if the block is
			// single-line, also strip the trailing `)`. Anything left
			// is one or more kwargs.
			openIdx := strings.Index(trim, ".objects.create(")
			if openIdx < 0 {
				continue
			}
			trim = trim[openIdx+len(".objects.create("):]
			// If the closing `)` is on this same line, drop it (and
			// anything after — typically nothing, but defensively
			// handle `)  # comment` or assignment trailers).
			if endIdx := strings.LastIndex(trim, ")"); endIdx >= 0 && parenDelta(trim[:endIdx]) == 0 {
				trim = trim[:endIdx]
			}
		}
		if trim == "" || trim == ")" || strings.HasPrefix(trim, "#") {
			continue
		}
		trim = strings.TrimSuffix(trim, ",")
		// Split on top-level commas to separate same-line kwargs.
		for _, frag := range splitTopLevelCommas(trim) {
			frag = strings.TrimSpace(frag)
			if frag == "" || strings.HasPrefix(frag, "**") {
				continue
			}
			m := fieldPat.FindStringSubmatch(frag)
			if m == nil {
				continue
			}
			key, val := m[1], strings.TrimSpace(m[2])
			val = strings.TrimSuffix(val, ",")
			if parenDelta(val) != 0 {
				continue
			}
			out[key] = val
		}
	}
	return out
}

// splitTopLevelCommas splits s at commas that are not inside
// quoted strings or nested parentheses/brackets/braces. Used by
// extractFieldsSharp to break `status="ok", name="X"` into two
// kwarg fragments.
func splitTopLevelCommas(s string) []string {
	var out []string
	depth := 0
	inSingle, inDouble := false, false
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && (inSingle || inDouble):
			if i+1 < len(s) {
				i++
			}
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case (c == '(' || c == '[' || c == '{') && !inSingle && !inDouble:
			depth++
		case (c == ')' || c == ']' || c == '}') && !inSingle && !inDouble:
			depth--
		case c == ',' && depth == 0 && !inSingle && !inDouble:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// synthID produces a deterministic ID from the entity name + upstream
// line number + a stable field subset. Upstream Django fixtures rarely
// declare explicit pks — they auto-allocate — so we synthesise an ID
// that is unique per (Entity, upstream source location) and small
// enough to stay readable in the committed file.
//
// Algorithm: take the low 12 bits of sha256(entity|line|name|asn) and
// add a per-entity offset so cross-entity collisions can't happen.
// The actual numeric value is irrelevant for parity tests — what
// matters is STABILITY across tool reruns.
func synthID(entity string, line int, fields map[string]string) int {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%d|%s|%s", entity, line, fields["name"], fields["asn"])
	sum := h.Sum(nil)
	low12 := (int(sum[0])<<8 | int(sum[1])) & 0x0FFF
	offset, ok := entityOffset[entity]
	if !ok {
		offset = 30000
	}
	return offset + low12
}

// sha256Hex returns the hex-encoded sha256 of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// writeAtomic writes b to path via a temp file + rename. This matches
// the atomicity invariant used by cmd/pdb-compat-allowlist so a SIGINT
// mid-write can't leave a partially-formatted file on disk.
func writeAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".pdb-fixture-port-*")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s → %s: %w", tmpPath, path, err)
	}
	return nil
}

// readHeaderHash reads the first ~20 lines of path and extracts the
// `// UpstreamHash: sha256:<hex>` value. Used by --check when
// --pinned is empty.
func readHeaderHash(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 — path is an operator-supplied flag
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for i := 0; i < 20 && sc.Scan(); i++ {
		line := sc.Text()
		if strings.HasPrefix(line, "// UpstreamHash: sha256:") {
			return strings.TrimPrefix(line, "// UpstreamHash: sha256:"), nil
		}
	}
	return "", fmt.Errorf("no UpstreamHash header in %s", path)
}

// titleCase returns s with its first rune upper-cased. strings.Title
// is deprecated and golang.org/x/text/cases is a heavy import for a
// one-character capitalisation; bespoke helper keeps dependencies
// flat.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// --- render ---------------------------------------------------------

const outputTemplate = `// Code generated by cmd/pdb-fixture-port — DO NOT EDIT.
//
// Upstream:     peeringdb/peeringdb@{{.UpstreamCommit}}
// UpstreamHash: sha256:{{.UpstreamHash}}
// Source:       {{.UpstreamPath}}
// Category:     {{.Category}}
// Ported:       {{.Ported}}
//
// Regenerate via ` + "`go generate ./internal/testutil/parity`" + `.
// See Phase 72 CONTEXT.md D-02 / D-03 for porting rationale and
// drift-detection policy.

package parity

// Fixture is a single ported row from upstream pdb_api_test.py.
//
// Entity is the PeeringDB type namespace ("net", "org", "ix", ...).
// ID is a deterministic synthesised identifier — upstream Django
// fixtures auto-allocate pks, so the tool derives a stable value from
// the upstream source location. Cross-run stability is the contract,
// not collision with upstream database ids.
// Fields holds the upstream keyword arguments verbatim (string form),
// so consumers that need typed values can re-parse per-column.
// Upstream carries the "pdb_api_test.py:<line>" citation.
type Fixture struct {
	Entity   string
	ID       int
	Fields   map[string]string
	Upstream string
}
{{range .Sections}}
{{if .Preamble}}{{.Preamble}}
{{end}}// {{.VarName}} is the ported set for the {{.Title}} category.
var {{.VarName}} = []Fixture{
{{- range .Fixtures}}
	{
		Entity: {{printf "%q" .Entity}},
		ID:     {{.ID}},
		Fields: map[string]string{
{{- $f := .Fields}}
{{- range .FieldKeys}}
			{{printf "%q" .}}: {{printf "%q" (index $f .)}},
{{- end}}
		},
		Upstream: {{printf "%q" .Upstream}},
	},
{{- end}}
}
{{end}}`

// renderTemplate executes outputTemplate against d and returns raw
// (pre-gofmt) Go source bytes.
func renderTemplate(d renderData) ([]byte, error) {
	parsed, err := template.New("fixtures").Parse(outputTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, d); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// appendCategory implements --append: read the current --out file,
// locate the var block matching the requested category (e.g.
// "OrderingFixtures"), and replace just that block with the
// corresponding block from rendered (a freshly-rendered single-
// section file). Other vars and the file header are preserved
// byte-identically.
//
// If --out doesn't exist yet, the rendered bytes are returned
// verbatim (first-write semantics). If --out exists but the target
// var block doesn't, the new block is appended just before the file
// trailer.
//
// The merge is gofmt-stable: both halves come from format.Source(),
// and the splice happens on whole-line boundaries.
func appendCategory(outPath string, rendered []byte, category string) ([]byte, error) {
	existing, err := os.ReadFile(outPath) // #nosec G304 — operator-supplied flag.
	if err != nil {
		if os.IsNotExist(err) {
			return rendered, nil
		}
		return nil, fmt.Errorf("read existing --out: %w", err)
	}
	varName := titleCase(category) + "Fixtures"
	newBlock := extractVarBlockBytes(rendered, varName)
	if len(newBlock) == 0 {
		return nil, fmt.Errorf("rendered output missing var %s block", varName)
	}
	oldBlock := extractVarBlockBytes(existing, varName)
	var merged []byte
	if len(oldBlock) > 0 {
		merged = bytes.Replace(existing, oldBlock, newBlock, 1)
	} else {
		// Var doesn't exist in existing file: append to end with a
		// blank line separator so gofmt keeps the trailing newline.
		merged = append(append(existing, '\n'), newBlock...)
		merged = append(merged, '\n')
	}
	// Re-format to absorb any whitespace mismatches at the splice
	// boundary (extractVarBlockBytes returns the raw `var ... }`
	// substring; the file may have a comment line preceding the
	// `var` we just removed that gofmt re-attaches).
	formatted, err := format.Source(merged)
	if err != nil {
		return nil, fmt.Errorf("re-format spliced output: %w", err)
	}
	return formatted, nil
}

// extractVarBlockBytes returns the substring of src starting at the
// leading `// ...` comment block above `var <name> = []Fixture{` (when
// present) and ending at the matching closing `}`. Returns nil if the
// var declaration is not found. Tolerates nested `{}` (Fields:
// map[string]string{...}) via depth tracking.
//
// The leading-comment walk-back exists because the render template
// emits an optional `section.Preamble` line (plus the always-present
// `// <VarName> is the ported set for the <Title> category.` line)
// immediately above the var keyword. `--append` must splice the new
// preamble atomically with the new var body — otherwise the OLD
// preamble sticks above the NEW var contents, which is a silent
// provenance lie for the limit/unicode/in/traversal categories that
// carry load-bearing citation comments per Plans 72-02 D-02 / 72-03
// D-04. See WR-01 in Phase 72 REVIEW.md.
func extractVarBlockBytes(src []byte, varName string) []byte {
	marker := []byte("var " + varName + " = []Fixture{")
	varStart := bytes.Index(src, marker)
	if varStart < 0 {
		return nil
	}
	// Walk backward over contiguous `// ...` comment lines that sit
	// immediately above the var declaration. Stops at the first
	// blank line or non-comment line — matches the template's
	// "comments directly above var" invariant without over-grabbing
	// an unrelated adjacent comment block.
	start := varStart
	for start > 0 {
		// Find the start of the line that ends at start-1 (the
		// newline preceding the var keyword or the previous comment).
		prevNL := bytes.LastIndexByte(src[:start-1], '\n')
		lineStart := prevNL + 1 // 0 when no earlier newline exists.
		line := bytes.TrimLeft(src[lineStart:start-1], " \t")
		if !bytes.HasPrefix(line, []byte("//")) {
			break
		}
		start = lineStart
	}
	depth := 0
	for i := varStart + len(marker) - 1; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1]
			}
		}
	}
	return nil
}
