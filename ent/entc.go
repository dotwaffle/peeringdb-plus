//go:build ignore

package main

import (
	"fmt"
	"log"
	_ "unsafe" // Required for go:linkname.

	"entgo.io/contrib/entgql"
	"entgo.io/contrib/entproto"
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/go-openapi/inflect"
	"github.com/lrstanley/entrest"
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
		Handler: entrest.HandlerStdlib,
		DefaultOperations: []entrest.Operation{
			entrest.OperationRead,
			entrest.OperationList,
		},
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
		// the sync worker can run `PRAGMA defer_foreign_keys = ON` on the
		// SAME connection as the writes (the previous connection-level
		// `PRAGMA foreign_keys = OFF` was silently non-functional because
		// the ent tx pulled a fresh pool connection). See Phase 54-01 Commit B.
		//
		// "privacy" enables ent's privacy package (see entgo.io/docs/privacy):
		// per-schema Policy() methods + DecisionContext bypass. Required for
		// Phase 59 VIS-04/VIS-05. Do NOT add "entql" — the typed
		// PocQueryRuleFunc adapter is sufficient; EntQL dynamic filters are
		// not required for our row-level visibility rule.
		entc.FeatureNames("sql/upsert", "sql/execquery", "privacy"),
		// Phase 67 D-07: project-local override of entrest's `rest/sorting`
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
}

// entrestSortingOverride is a minimal replica of entc.TemplateDir that registers
// entrest's funcmap on the template before parsing. This is required because
// our project-local override of entrest's sorting.tmpl uses entrest-provided
// template funcs (getAnnotation, getSortableFields) which aren't in ent's
// default funcmap. See Phase 67 Plan 02.
func entrestSortingOverride(path string) entc.Option {
	return func(cfg *gen.Config) error {
		t := gen.NewTemplate("entrest-override").Funcs(entrest.FuncMaps())
		if _, err := t.ParseDir(path); err != nil {
			return fmt.Errorf("parsing entrest sorting override from %q: %w", path, err)
		}
		cfg.Templates = append(cfg.Templates, t)
		return nil
	}
}
