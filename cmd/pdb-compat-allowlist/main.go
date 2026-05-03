// Command pdb-compat-allowlist reads the ent schema graph and emits
// internal/pdbcompat/allowlist_gen.go with:
//
//   - Path A per-entity prepare_query allowlists (Phase 70 D-01),
//   - FilterExcludes (Phase 70 D-03 FILTER_EXCLUDE parity), and
//   - Path B edge map keyed by PeeringDB type string (Phase 70 D-02
//     amended 2026-04-19: codegen-time static emission replaces a
//     runtime client.Schema.Tables walk — deterministic, testable,
//     no init-order coupling, freshness-gated by the existing
//     go-generate drift check).
//
// Invoked from ent/generate.go after ent codegen so the gen.Graph
// reflects the latest schema annotations.
//
// Usage:
//
//	go run ./cmd/pdb-compat-allowlist
//
// Working directory is expected to be the repo root; paths are
// relative to that (./ent/schema input, ./internal/pdbcompat/
// allowlist_gen.go output).
package main

import (
	"bytes"
	"cmp"
	"fmt"
	"go/format"
	"log"
	"os"
	"slices"
	"sort"
	"strings"
	"text/template"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"

	"github.com/dotwaffle/peeringdb-plus/ent/schema"
)

// Annotation name constants — must match internal/pdbcompat/annotations.go.
// Redeclared here as local strings to avoid importing internal/pdbcompat
// from a cmd/ tool that runs during `go generate` (keeps the codegen tool
// independent of the runtime package layering).
//
// Only FilterExcludeFromTraversal is still consumed here: Path A
// PrepareQueryAllow is now read directly from schema.PrepareQueryAllows
// (ent/schema/pdb_allowlists.go) per the 260420-esb sibling-files
// refactor.
const (
	filterExcludeName = "FilterExcludeFromTraversal"
)

// AllowlistData is the template input assembled by main() before render.
type AllowlistData struct {
	Entries        []NodeEntry    // sorted by PDBType (Path A — per-entity prepare_query allowlist)
	FilterExcludes []ExcludeEntry // sorted by entity+edge (upstream FILTER_EXCLUDE parity)
	EdgeEntries    []EdgeMapEntry // sorted by PDBType (Path B — codegen-emitted edge map, Phase 70 D-02 amended)
}

// NodeEntry carries one PeeringDB type's Path A allowlist.
type NodeEntry struct {
	GoName  string     // e.g. "Network"
	PDBType string     // e.g. "net"
	Direct  []string   // sorted; single-hop keys like "org__name"
	Via     []ViaEntry // sorted by FirstHop; 2-hop keys grouped
}

// ViaEntry groups 2-hop allowlist keys by their first relationship segment.
type ViaEntry struct {
	FirstHop string   // e.g. "ixlan"
	Tails    []string // e.g. ["ix__fac_count"]; sorted
}

// ExcludeEntry captures a single (entity, edge) pair annotated with
// WithFilterExcludeFromTraversal.
type ExcludeEntry struct {
	Entity string // ent Go name, e.g. "Network"
	Edge   string // edge name, e.g. "pocs"
}

// EdgeMapEntry groups the outgoing edges of one PeeringDB type for
// the Path B (automatic introspection) lookup map.
type EdgeMapEntry struct {
	PDBType string       // e.g. "net"
	Edges   []EdgeMapRow // sorted by Name
}

// EdgeMapRow is the codegen-side mirror of internal/pdbcompat.EdgeMetadata.
// Kept as a separate type in this cmd/ tool so the tool has zero import
// dependency on the runtime package it generates.
//
// Source fields (from gen.Edge / gen.Type):
//   - Name           = edge.Name
//   - TargetType     = pdbTypeFor(edge.Type.Name)
//   - TraversalKey   = TargetType (parser lookup key is the target PeeringDB type)
//   - Excluded       = edge.Annotations[FilterExcludeFromTraversal] present
//   - ParentFKColumn = edge.Rel.Column()   (O2O/O2M/M2O; edge is skipped if Columns is empty)
//   - TargetTable    = edge.Type.Table()
//   - TargetIDColumn = edge.Type.ID.StorageKey() (typically "id")
//   - OwnFK          = (edge.Rel.Type == gen.M2O); true when ParentFKColumn
//     lives on the PARENT table, false when it lives on
//     the CHILD table. Used by the runtime subquery
//     builder to pick the correct WHERE/IN pairing.
//     See Phase 70 REVIEW CR-01.
//
// Per Phase 70 D-02 amended: this map is emitted once at `go generate`
// time and read-only at runtime. No sync.Once, no init-order coupling.
type EdgeMapRow struct {
	Name           string
	TargetType     string
	TraversalKey   string
	Excluded       bool
	ParentFKColumn string
	TargetTable    string
	TargetIDColumn string
	OwnFK          bool
}

func main() {
	graph, err := entc.LoadGraph("./ent/schema", &gen.Config{})
	if err != nil {
		log.Fatalf("load ent schema graph: %v", err)
	}

	data := AllowlistData{}

	// Path A: read directly from the hand-written schema.PrepareQueryAllows
	// map (ent/schema/pdb_allowlists.go). Moved off per-schema Annotations()
	// calls in the 260420-esb sibling-files refactor so cmd/pdb-schema-generate
	// can freely regenerate ent/schema/{type}.go without stripping hand-edits.
	//
	// Keys in the map are PeeringDB type strings ("net", "fac", etc.) — the
	// same namespace this tool emits into allowlist_gen.go.
	for pdbType, annot := range schema.PrepareQueryAllows {
		entry := buildAllowlistEntry(pdbType, annot.Fields)
		if entry != nil {
			data.Entries = append(data.Entries, *entry)
		}
	}

	// Path B + FilterExcludes still walk the ent gen.Graph — they describe
	// edge relationships that are authoritatively modelled in ent schema,
	// not hand-authored annotations.
	for _, node := range graph.Nodes {
		data.FilterExcludes = append(data.FilterExcludes, extractExcludes(node)...)
		if edgeEntry := extractEdges(node); edgeEntry != nil {
			data.EdgeEntries = append(data.EdgeEntries, *edgeEntry)
		}
	}

	// Deterministic ordering for byte-stable output across runs.
	slices.SortFunc(data.Entries, func(a, b NodeEntry) int {
		return cmp.Compare(a.PDBType, b.PDBType)
	})
	slices.SortFunc(data.FilterExcludes, func(a, b ExcludeEntry) int {
		if r := cmp.Compare(a.Entity, b.Entity); r != 0 {
			return r
		}
		return cmp.Compare(a.Edge, b.Edge)
	})
	slices.SortFunc(data.EdgeEntries, func(a, b EdgeMapEntry) int {
		return cmp.Compare(a.PDBType, b.PDBType)
	})

	src, err := render(data)
	if err != nil {
		log.Fatalf("render allowlist_gen.go: %v", err)
	}
	formatted, err := format.Source(src)
	if err != nil {
		// Persist raw output so the developer can diagnose template bugs.
		_ = os.WriteFile("internal/pdbcompat/allowlist_gen.go.broken", src, 0o600)
		log.Fatalf("gofmt allowlist_gen.go: %v (raw output at internal/pdbcompat/allowlist_gen.go.broken)", err)
	}
	if err := os.WriteFile("internal/pdbcompat/allowlist_gen.go", formatted, 0o600); err != nil {
		log.Fatalf("write allowlist_gen.go: %v", err)
	}
}

// buildAllowlistEntry converts a PDB type ("net") + a verbatim list of
// upstream-shaped filter keys ("org__name", "ixlan__ix__fac_count") into
// the NodeEntry shape consumed by the outputTemplate. Splits on "__" to
// route 2-segment keys into Direct and 3-segment keys into Via; drops
// keys with 0/1 or 4+ segments with a logged warning.
//
// This replaces the previous ent-graph-annotation walk. Source-of-truth
// for Path A is now ent/schema/pdb_allowlists.go (the 260420-esb
// sibling-files refactor); the ent graph is still consulted for Path B
// (Edges) and FilterExcludes, which encode edge-relationship metadata
// that only ent knows.
func buildAllowlistEntry(pdbType string, fields []string) *NodeEntry {
	if pdbType == "" || len(fields) == 0 {
		return nil
	}
	entry := &NodeEntry{
		GoName:  goNameFor(pdbType),
		PDBType: pdbType,
	}
	viaMap := make(map[string][]string)
	for _, f := range fields {
		parts := strings.Split(f, "__")
		switch len(parts) {
		case 2:
			entry.Direct = append(entry.Direct, f)
		case 3:
			// "ixlan__ix__fac_count" → Via["ixlan"] = ["ix__fac_count"]
			viaMap[parts[0]] = append(viaMap[parts[0]], strings.Join(parts[1:], "__"))
		case 0, 1:
			log.Printf("pdb-compat-allowlist: %s skipping malformed field %q (needs at least one __)", pdbType, f)
		default:
			// 4+ segments — violates D-04 2-hop cap. Drop with warn.
			log.Printf("pdb-compat-allowlist: %s dropping >2-hop field %q (D-04 cap)", pdbType, f)
		}
	}
	sort.Strings(entry.Direct)
	for k := range viaMap {
		sort.Strings(viaMap[k])
	}
	hops := make([]string, 0, len(viaMap))
	for k := range viaMap {
		hops = append(hops, k)
	}
	sort.Strings(hops)
	for _, h := range hops {
		entry.Via = append(entry.Via, ViaEntry{FirstHop: h, Tails: viaMap[h]})
	}
	return entry
}

// extractExcludes walks a node's edges and collects those carrying the
// WithFilterExcludeFromTraversal annotation into a flat slice. The
// downstream template renders a `FilterExcludes[Entity]{Edge: true}`
// map entry for each.
func extractExcludes(node *gen.Type) []ExcludeEntry {
	var out []ExcludeEntry
	for _, edge := range node.Edges {
		if _, ok := edge.Annotations[filterExcludeName]; ok {
			out = append(out, ExcludeEntry{Entity: node.Name, Edge: edge.Name})
		}
	}
	return out
}

// extractEdges walks a node's edges and produces the Path B EdgeMapEntry
// for internal/pdbcompat.Edges. Per Phase 70 D-02 amended, this is the
// codegen-time source of truth for the runtime lookup — no
// client.Schema.Tables walk exists at request time.
//
// SQL-join metadata (ParentFKColumn, TargetTable, TargetIDColumn) is
// sourced from gen.Edge.Rel.Column() and gen.Type.Table() /
// gen.Type.ID.StorageKey(). Edges whose Rel.Columns slice is empty are
// logged and skipped rather than emitted with a blank column — Plan
// 70-05's subquery construction must not receive empty column names.
//
// Edges whose target type has no pdbTypeFor mapping (e.g. if a future
// schema introduces a non-PeeringDB-visible table) are silently skipped.
func extractEdges(node *gen.Type) *EdgeMapEntry {
	pdbType := pdbTypeFor(node.Name)
	if pdbType == "" {
		return nil
	}
	entry := &EdgeMapEntry{PDBType: pdbType}
	for _, e := range node.Edges {
		targetPDB := pdbTypeFor(e.Type.Name)
		if targetPDB == "" {
			continue
		}
		_, excluded := e.Annotations[filterExcludeName]
		parentFK := resolveParentFKColumn(e)
		if parentFK == "" {
			log.Printf("pdb-compat-allowlist: %s.%s — unable to resolve FK column (no Rel.Columns); skipping edge", node.Name, e.Name)
			continue
		}
		targetTable := e.Type.Table()
		if targetTable == "" {
			log.Printf("pdb-compat-allowlist: %s.%s — target type %q has empty Table(); skipping edge", node.Name, e.Name, e.Type.Name)
			continue
		}
		targetID := "id"
		if e.Type.ID != nil {
			if k := e.Type.ID.StorageKey(); k != "" {
				targetID = k
			}
		}
		entry.Edges = append(entry.Edges, EdgeMapRow{
			Name:           e.Name,
			TargetType:     targetPDB,
			TraversalKey:   targetPDB,
			Excluded:       excluded,
			ParentFKColumn: parentFK,
			TargetTable:    targetTable,
			TargetIDColumn: targetID,
			// OwnFK: true for M2O edges where the FK column lives on
			// THIS (parent) table; false for O2M/O2O-from-edge where
			// the FK column lives on the target (child) table. Drives
			// the runtime subquery WHERE/IN pairing in buildSinglHop /
			// buildTwoHop (Phase 70 REVIEW CR-01). M2M edges are not
			// used in this schema, but if one existed the slice has
			// two FK columns and this flag no longer applies.
			OwnFK: e.Rel.Type == gen.M2O,
		})
	}
	if len(entry.Edges) == 0 {
		return nil
	}
	slices.SortFunc(entry.Edges, func(a, b EdgeMapRow) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return entry
}

// resolveParentFKColumn returns the FK column name for a gen.Edge. For
// O2O, O2M, and M2O edges, gen.Relation.Columns has a single entry —
// we return Columns[0]. For M2M edges the slice has two entries (join
// table owner_id, reference_id); we take the first since our schema has
// no M2M edges today (and emit empty so the caller skips with a log).
//
// Column semantics note: for M2O edges (edge.From with Ref().Field())
// the column lives on the PARENT table (e.g. networks.org_id). For O2M
// edges (edge.To) the column lives on the CHILD table (e.g. netfac.
// network_id). The EdgeMetadata consumer in Plan 70-05 uses OwnFK
// knowledge indirectly via edge direction to construct the correct
// subquery shape. For Path B today (<fk>__<field> where <fk> is the
// target PeeringDB type), both directions produce valid subqueries
// using the same {ParentFKColumn, TargetTable, TargetIDColumn} triple;
// only the WHERE/IN pairing differs and that's Plan 70-05 territory.
func resolveParentFKColumn(e *gen.Edge) string {
	if len(e.Rel.Columns) > 0 {
		return e.Rel.Columns[0]
	}
	return ""
}

// pdbTypeFor maps ent Go type names to PeeringDB API type strings (the
// "net" / "fac" / "ix" namespace used by pdbcompat Registry keys and
// URLs). Mirrors the map in internal/peeringdb/types.go and
// modelNameOverrides in cmd/pdb-schema-generate/main.go. Unknown names
// return "" and are skipped by the caller.
func pdbTypeFor(goName string) string {
	if v, ok := pdbTypeMap[goName]; ok {
		return v
	}
	log.Printf("pdb-compat-allowlist: no PeeringDB type mapping for %q — skipping", goName)
	return ""
}

// goNameFor is the reverse of pdbTypeFor — maps a PeeringDB type string
// ("net") back to its ent Go type name ("Network"). Used when building
// NodeEntry records from schema.PrepareQueryAllows (whose keys are PDB
// type strings). Unknown inputs return "" and are logged — the caller
// skips the entry.
func goNameFor(pdbType string) string {
	for goName, pt := range pdbTypeMap {
		if pt == pdbType {
			return goName
		}
	}
	log.Printf("pdb-compat-allowlist: no Go type mapping for PDB type %q — skipping", pdbType)
	return ""
}

// pdbTypeMap is the single source-of-truth for the PDB-type ↔ Go-name
// correspondence. Exposed as a package-level var (not built inline in
// pdbTypeFor) so goNameFor can do a reverse lookup without maintaining
// a second parallel declaration.
var pdbTypeMap = map[string]string{
	"Organization":     "org",
	"Network":          "net",
	"Facility":         "fac",
	"InternetExchange": "ix",
	"Poc":              "poc",
	"IxLan":            "ixlan",
	"IxPrefix":         "ixpfx",
	"NetworkIxLan":     "netixlan",
	"NetworkFacility":  "netfac",
	"IxFacility":       "ixfac",
	"Carrier":          "carrier",
	"CarrierFacility":  "carrierfac",
	"Campus":           "campus",
}

// outputTemplate is the Go source template for allowlist_gen.go. Every
// string value is emitted via `printf "%q"` so entity/field names with
// unusual characters get correctly Go-quoted (threat T-70-02-01).
const outputTemplate = `// Code generated by cmd/pdb-compat-allowlist; DO NOT EDIT.
//
// Source: ent/schema/*.go PrepareQueryAllow and FilterExcludeFromTraversal
// annotations. Regenerate via ` + "`go generate ./...`" + ` (runs
// cmd/pdb-compat-allowlist after ent codegen per ent/generate.go).
//
// See Phase 70 D-01 / D-03 for the upstream PeeringDB parity rationale.

package pdbcompat

// Allowlists maps a PeeringDB type name (e.g. "net") to its Path A
// allowlist — the set of <fk>__<field> and <fk>__<fk>__<field> keys
// that mirror upstream serializers.py get_relation_filters(...) lists.
var Allowlists = map[string]AllowlistEntry{
{{- range .Entries }}
{{- if .PDBType }}
	{{ printf "%q" .PDBType }}: {
{{- if .Direct }}
		Direct: []string{
{{- range .Direct }}
			{{ printf "%q" . }},
{{- end }}
		},
{{- end }}
{{- if .Via }}
		Via: map[string][]string{
{{- range .Via }}
			{{ printf "%q" .FirstHop }}: {
{{- range .Tails }}
				{{ printf "%q" . }},
{{- end }}
			},
{{- end }}
		},
{{- end }}
	},
{{- end }}
{{- end }}
}

// FilterExcludes mirrors upstream serializers.py:128-157 FILTER_EXCLUDE.
// Outer key: entity Go name (e.g. "Network"). Inner key: edge name
// (e.g. "pocs"). Value is always true; the map is used as a set.
var FilterExcludes = map[string]map[string]bool{
{{- range .FilterExcludes }}
	{{ printf "%q" .Entity }}: {{"{"}}{{ printf "%q" .Edge }}: true{{"}"}},
{{- end }}
}

// Edges maps a PeeringDB type name (e.g. "net") to a slice of
// EdgeMetadata describing its outgoing ent edges. Consumed at request
// time by internal/pdbcompat.LookupEdge for Path B traversal.
//
// Phase 70 D-02 (amended 2026-04-19): the map is emitted at
// ` + "`go generate`" + ` time from gen.Graph — no runtime client.Schema walk,
// no sync.Once, no init-order coupling. Freshness is enforced by the
// existing go-generate drift-check CI gate (same precedent as
// v1.15 Phase 63 hygiene drops).
//
// TraversalKey is the <fk> token in filter params (equals TargetType
// today). Excluded edges (WithFilterExcludeFromTraversal annotation)
// are emitted with Excluded=true; LookupEdge hides them from its
// callers so consumers see them as missing.
//
// ParentFKColumn, TargetTable, TargetIDColumn carry SQL-level metadata
// for Plan 70-05's subquery construction. Edges whose FK column or
// target table could not be resolved at codegen time are logged and
// skipped entirely (never emitted with blank metadata).
var Edges = map[string][]EdgeMetadata{
{{- range .EdgeEntries }}
	{{ printf "%q" .PDBType }}: {
{{- range .Edges }}
		{Name: {{ printf "%q" .Name }}, TargetType: {{ printf "%q" .TargetType }}, TraversalKey: {{ printf "%q" .TraversalKey }}, Excluded: {{ .Excluded }}, ParentFKColumn: {{ printf "%q" .ParentFKColumn }}, TargetTable: {{ printf "%q" .TargetTable }}, TargetIDColumn: {{ printf "%q" .TargetIDColumn }}, OwnFK: {{ .OwnFK }}},
{{- end }}
	},
{{- end }}
}
`

// render executes outputTemplate against data and returns the raw Go
// source bytes (pre-gofmt). The caller is responsible for passing the
// result through go/format.Source before writing to disk.
func render(data AllowlistData) ([]byte, error) {
	tmpl, err := template.New("allowlist").Parse(outputTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}
