//go:build ignore

package main

import (
	"encoding/json/jsontext"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"text/template"
	_ "unsafe" // Required for go:linkname.

	"entgo.io/contrib/entgql"
	"entgo.io/contrib/entproto"
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/go-openapi/inflect"
	"github.com/lrstanley/entrest"
	"github.com/ogen-go/ogen"
)

// entGenRules provides access to ent's unexported inflect ruleset so we can
// fix singularization for words like "campus" that the default rules mangle.
// The rules var is initialised at package init via inflect.NewDefaultRuleset()
// and used directly in Go code (Edge.MutationAdd, graph column names, etc.),
// so replacing template funcs alone is insufficient.
//
//go:linkname entGenRules entgo.io/ent/entc/gen.rules
var entGenRules *inflect.Ruleset

// fixCampusInflection patches an inflect.Ruleset so that "campus" is treated
// correctly. go-openapi/inflect's default rules match the trailing "s" and
// produce "campu" (singular) / "campuse" (singularising "campuses").
//
// AddIrregular alone is not enough: it only adds Singular(plural, singular)
// but omits Singular(singular, singular), so the bare word "campus" still
// falls through to the default "s" → "" rule. We add explicit rules for both
// lowercase and PascalCase forms since inflect matches case-sensitively.
func fixCampusInflection(rs *inflect.Ruleset) {
	rs.AddIrregular("campus", "campuses")
	// Prevent "campus" → "campu" (AddIrregular doesn't cover this case).
	rs.AddSingular("campus", "campus")
	// PascalCase exact matches for entrest which passes type names directly.
	rs.AddSingularExact("Campus", "Campus", true)
	rs.AddSingularExact("Campuses", "Campus", true)
	rs.AddPluralExact("Campus", "Campuses", true)
}

func main() {
	// Fix incorrect singularization of "campus" by go-openapi/inflect.
	// Two rulesets need patching:
	//   1. Global inflect default — used by entrest.Pluralize (URL paths).
	//   2. Ent's internal gen.rules — used by Edge.MutationAdd/Remove,
	//      graph column naming, and template funcs (Go code + templates).
	//      entgql and entrest capture gen.Funcs["singular"] at init as a
	//      method value bound to gen.rules, so patching gen.rules via
	//      go:linkname fixes them automatically.
	inflect.AddIrregular("campus", "campuses")
	inflect.AddSingular("campus", "campus")
	fixCampusInflection(entGenRules)

	gqlExt, err := entgql.NewExtension(
		entgql.WithSchemaGenerator(),
		entgql.WithSchemaPath("../graph/schema.graphqls"),
		entgql.WithWhereInputs(true),
		entgql.WithConfigPath("../graph/gqlgen.yml"),
		entgql.WithRelaySpec(true),
	)
	if err != nil {
		log.Fatalf("creating entgql extension: %v", err)
	}

	restExt, err := entrest.NewExtension(&entrest.Config{
		// Without a Spec, entrest titles the document "EntGo Rest API",
		// version 1.0.0. /rest/v1/docs and generated clients show this info.
		Spec: ogen.NewSpec().SetInfo(ogen.NewInfo().
			SetTitle("PeeringDB Plus REST API").
			SetDescription("Read-only REST API for the PeeringDB Plus mirror of PeeringDB.").
			SetVersion("1")),
		Handler: entrest.HandlerStdlib,
		DefaultOperations: []entrest.Operation{
			entrest.OperationRead,
			entrest.OperationList,
		},
		PostGenerateHook: dropPolicyEdgeSorts,
	})
	if err != nil {
		log.Fatalf("creating entrest extension: %v", err)
	}

	protoExt, err := entproto.NewExtension(
		entproto.SkipGenFile(),
		entproto.WithProtoDir("../proto"),
	)
	if err != nil {
		log.Fatalf("creating entproto extension: %v", err)
	}

	opts := []entc.Option{
		entc.Extensions(gqlExt, restExt, protoExt),
		// sql/upsert: used by internal/sync/upsert.go for bulk UpsertColumns.
		// sql/execquery: exposes tx.ExecContext on the generated ent.Tx so
		// the sync worker can run per-tx pragmas (`PRAGMA cache_spill = OFF`)
		// on the SAME connection as the writes (the previous connection-level
		// `PRAGMA foreign_keys = OFF` was silently non-functional because
		// the ent tx pulled a fresh pool connection).
		//
		// "privacy" enables ent's privacy package (see entgo.io/docs/privacy):
		// per-schema Policy() methods + DecisionContext bypass. Required for
		// the per-schema row-level visibility rules. Do NOT add "entql" — the typed
		// PocQueryRuleFunc adapter is sufficient; EntQL dynamic filters are
		// not required for our row-level visibility rule.
		entc.FeatureNames("sql/upsert", "sql/execquery", "privacy"),
		// Project-local override of entrest's `rest/sorting`
		// template. Injects a compound (_field, FieldCreated, FieldID)
		// tie-break into applySorting<Type> when _field matches the entity's
		// declared DefaultField, so REST default ORDER BY matches pdbcompat
		// and grpcserver. Path is relative to entc.Generate's working dir,
		// which is `ent/` (this file's dir) when invoked via `go generate ./ent`.
		//
		// We cannot use entc.TemplateDir here because it constructs the template
		// with only ent's default funcmap (gen.Funcs) — the upstream entrest
		// sorting template depends on entrest-provided funcs such as
		// `getAnnotation` and `getSortableFields` (see entrest/templates.go
		// funcMap). entrest.FuncMaps() exports those. We build a *gen.Template
		// with both ent's defaults (via gen.NewTemplate) and entrest's funcmap,
		// parse our override from disk, then append to cfg.Templates via the
		// same mechanism entc.TemplateDir uses internally (templateOption →
		// cfg.Templates). gen.Graph.templates() at codegen time merges the
		// funcmap into the root template tree, so the override's `rest/sorting`
		// definition wins over entrest's baseTemplates entry.
		entrestSortingOverride("./templates/entrest-sorting"),
	}

	if err := entc.Generate("./schema", &gen.Config{}, opts...); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
	if err := formatOpenAPISpec("./rest/openapi.json"); err != nil {
		log.Fatalf("formatting OpenAPI spec: %v", err)
	}
}

// formatOpenAPISpec sorts all object keys, including nested schema properties
// whose custom JSON marshaler bypasses entrest's deterministic map ordering.
func formatOpenAPISpec(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	value := jsontext.Value(data)
	if err := value.Format(jsontext.ReorderRawObjects(true), jsontext.Multiline(true)); err != nil {
		return err
	}
	return os.WriteFile(path, append(value, '\n'), 0o640)
}

// restSortableFields returns the REST sort fields of t: entrest's
// sortable fields without the sorts over an edge to a type that has a
// privacy policy. Such a sort (<edge>.count, <edge>.<field>.sum, or
// <edge>.<field> on a unique edge) orders by a subquery over the edge's
// rows, and the target's ent privacy policy does not filter that
// subquery. For example, ?sort=pocs.count would order networks by a
// count that includes the pocs that the caller's tier cannot read. The
// sorting template and dropPolicyEdgeSorts both use this list, so the
// served sort validation and the OpenAPI enum agree.
func restSortableFields(t *gen.Type) []string {
	sortable := entrest.GetSortableFields(t, nil)
	for _, e := range t.Edges {
		if e.Type.NumPolicy() == 0 {
			continue
		}
		prefix := e.Name + "."
		sortable = slices.DeleteFunc(sortable, func(f string) bool {
			return strings.HasPrefix(f, prefix)
		})
	}
	return sortable
}

// dropPolicyEdgeSorts is the entrest PostGenerateHook that removes the
// sorts which restSortableFields leaves out from the <Type>SortableFields
// enums of the OpenAPI spec.
func dropPolicyEdgeSorts(g *gen.Graph, spec *ogen.Spec) error {
	for _, t := range g.Nodes {
		name := entrest.Singularize(t.Name) + "SortableFields"
		schema, ok := spec.Components.Schemas[name]
		if !ok {
			continue
		}
		keep := restSortableFields(t)
		schema.Enum = slices.DeleteFunc(schema.Enum, func(v jsontext.Value) bool {
			f, err := jsontext.AppendUnquote(nil, v)
			return err == nil && !slices.Contains(keep, string(f))
		})
	}
	return nil
}

// entrestSortingOverride is a minimal replica of entc.TemplateDir that registers
// entrest's funcmap on the template before parsing. This is required because
// our project-local override of entrest's sorting.tmpl uses entrest-provided
// template funcs (getAnnotation) which aren't in ent's default funcmap, and
// the project func restSortableFields.
func entrestSortingOverride(path string) entc.Option {
	return func(cfg *gen.Config) error {
		t := gen.NewTemplate("entrest-override").
			Funcs(entrest.FuncMaps()).
			Funcs(template.FuncMap{"restSortableFields": restSortableFields})
		if _, err := t.ParseDir(path); err != nil {
			return fmt.Errorf("parsing entrest sorting override from %q: %w", path, err)
		}
		cfg.Templates = append(cfg.Templates, t)
		return nil
	}
}
