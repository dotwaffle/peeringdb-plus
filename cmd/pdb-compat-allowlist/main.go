// Command pdb-compat-allowlist reads the ent schema graph and emits
// internal/pdbcompat/allowlist_gen.go with per-entity Path A allowlists
// (upstream PeeringDB prepare_query lists) and FILTER_EXCLUDE data.
//
// Invoked from ent/generate.go after ent codegen so the gen.Graph
// reflects the latest schema annotations. See Phase 70 D-01.
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
	"fmt"
	"go/format"
	"log"
	"os"
	"sort"
	"strings"
	"text/template"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

// Annotation name constants — must match internal/pdbcompat/annotations.go.
// Redeclared here as local strings to avoid importing internal/pdbcompat
// from a cmd/ tool that runs during `go generate` (keeps the codegen tool
// independent of the runtime package layering).
const (
	prepareQueryAllowName = "PrepareQueryAllow"
	filterExcludeName     = "FilterExcludeFromTraversal"
)

// AllowlistData is the template input assembled by main() before render.
type AllowlistData struct {
	Entries        []NodeEntry    // sorted by PDBType
	FilterExcludes []ExcludeEntry // sorted by entity+edge
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

func main() {
	graph, err := entc.LoadGraph("./ent/schema", &gen.Config{})
	if err != nil {
		log.Fatalf("load ent schema graph: %v", err)
	}

	data := AllowlistData{}
	for _, node := range graph.Nodes {
		entry := extractAllowlist(node)
		if entry != nil {
			data.Entries = append(data.Entries, *entry)
		}
		data.FilterExcludes = append(data.FilterExcludes, extractExcludes(node)...)
	}

	// Deterministic ordering for byte-stable output across runs.
	sort.Slice(data.Entries, func(i, j int) bool {
		return data.Entries[i].PDBType < data.Entries[j].PDBType
	})
	sort.Slice(data.FilterExcludes, func(i, j int) bool {
		if data.FilterExcludes[i].Entity != data.FilterExcludes[j].Entity {
			return data.FilterExcludes[i].Entity < data.FilterExcludes[j].Entity
		}
		return data.FilterExcludes[i].Edge < data.FilterExcludes[j].Edge
	})

	src, err := render(data)
	if err != nil {
		log.Fatalf("render allowlist_gen.go: %v", err)
	}
	formatted, err := format.Source(src)
	if err != nil {
		// Persist raw output so the developer can diagnose template bugs.
		_ = os.WriteFile("internal/pdbcompat/allowlist_gen.go.broken", src, 0o644)
		log.Fatalf("gofmt allowlist_gen.go: %v (raw output at internal/pdbcompat/allowlist_gen.go.broken)", err)
	}
	if err := os.WriteFile("internal/pdbcompat/allowlist_gen.go", formatted, 0o644); err != nil {
		log.Fatalf("write allowlist_gen.go: %v", err)
	}
}

// extractAllowlist pulls PrepareQueryAllow.Fields from a node's
// Annotations map. ent's LoadGraph serializes annotations via JSON, so
// the concrete type in graph.Nodes[i].Annotations[<name>] is
// map[string]any (the JSON-decoded form of the struct). We tolerate
// both that and the concrete struct in case a future ent upgrade
// changes behaviour.
func extractAllowlist(node *gen.Type) *NodeEntry {
	raw, ok := node.Annotations[prepareQueryAllowName]
	if !ok {
		return nil
	}
	fields := decodeFields(raw)
	if len(fields) == 0 {
		return nil
	}
	pdbType := pdbTypeFor(node.Name)
	if pdbType == "" {
		return nil
	}
	entry := &NodeEntry{
		GoName:  node.Name,
		PDBType: pdbType,
	}
	viaMap := make(map[string][]string)
	for _, f := range fields {
		// Count "__" separators to decide direct vs 2-hop vs drop.
		parts := strings.Split(f, "__")
		switch len(parts) {
		case 2:
			entry.Direct = append(entry.Direct, f)
		case 3:
			// "ixlan__ix__fac_count" → Via["ixlan"] = ["ix__fac_count"]
			viaMap[parts[0]] = append(viaMap[parts[0]], strings.Join(parts[1:], "__"))
		case 0, 1:
			log.Printf("pdb-compat-allowlist: %s skipping malformed field %q (needs at least one __)", node.Name, f)
		default:
			// 4+ segments — violates D-04 2-hop cap. Drop with warn.
			log.Printf("pdb-compat-allowlist: %s dropping >2-hop field %q (D-04 cap)", node.Name, f)
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

// decodeFields accepts EITHER the JSON-roundtripped map form
// (map[string]any with a "Fields" key → []any of strings) OR a concrete
// struct implementing a GetFields() []string accessor. The map form is
// what ent's load.Config.Load() produces today; the interface form is a
// belt-and-suspenders fallback for future ent releases that might stop
// JSON-serializing annotation payloads.
func decodeFields(raw any) []string {
	if raw == nil {
		return nil
	}
	if m, ok := raw.(map[string]any); ok {
		arr, _ := m["Fields"].([]any)
		out := make([]string, 0, len(arr))
		for _, x := range arr {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	// Best-effort reflection-free struct match via a minimal interface.
	type fieldser interface{ GetFields() []string }
	if f, ok := raw.(fieldser); ok {
		return f.GetFields()
	}
	return nil
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

// pdbTypeFor maps ent Go type names to PeeringDB API type strings (the
// "net" / "fac" / "ix" namespace used by pdbcompat Registry keys and
// URLs). Mirrors the map in internal/peeringdb/types.go and
// modelNameOverrides in cmd/pdb-schema-generate/main.go. Unknown names
// return "" and are skipped by the caller.
func pdbTypeFor(goName string) string {
	m := map[string]string{
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
	if v, ok := m[goName]; ok {
		return v
	}
	log.Printf("pdb-compat-allowlist: no PeeringDB type mapping for %q — skipping", goName)
	return ""
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
