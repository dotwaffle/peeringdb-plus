//go:build ignore

package main

import (
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
		entc.FeatureNames("sql/upsert", "sql/execquery"),
	}

	if err := entc.Generate("./schema", &gen.Config{}, opts...); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
