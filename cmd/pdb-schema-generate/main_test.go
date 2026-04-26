package main

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSchema(t *testing.T) {
	t.Parallel()

	// Create a minimal test schema.
	schema := Schema{
		Version:     "1.0",
		ExtractedAt: "2026-03-22T12:00:00Z",
		SourcePath:  "/test",
		ObjectTypes: map[string]ObjectType{
			"org": {
				ModelName: "Organization",
				APIPath:   "org",
				Fields: map[string]FieldDef{
					"name": {
						Type:     "string",
						Required: true,
						Unique:   true,
						HelpText: "Organization name",
					},
				},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	loaded, err := loadSchema(path)
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}

	if loaded.Version != "1.0" {
		t.Errorf("version = %q, want %q", loaded.Version, "1.0")
	}
	if _, ok := loaded.ObjectTypes["org"]; !ok {
		t.Error("missing org object type")
	}
}

func TestResolveModelName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"Organization", "Organization"},
		{"IXLan", "IxLan"},
		{"IXPrefix", "IxPrefix"},
		{"IXFacility", "IxFacility"},
		{"NetworkIXLan", "NetworkIxLan"},
		{"NetworkContact", "Poc"},
		{"Network", "Network"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := resolveModelName(tt.input)
			if got != tt.want {
				t.Errorf("resolveModelName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSimplePlural(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"network", "networks"},
		{"facility", "facilities"},
		{"carrier_facility", "carrier_facilities"},
		{"campus", "campuses"},
		{"prefix", "prefixes"},
		{"ix_lan", "ix_lans"},
		{"poc", "pocs"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := simplePlural(tt.input)
			if got != tt.want {
				t.Errorf("simplePlural(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateEntSchemaCompiles(t *testing.T) {
	t.Parallel()

	schema := &Schema{
		ObjectTypes: map[string]ObjectType{
			"org": {
				ModelName: "Organization",
				APIPath:   "org",
				Fields: map[string]FieldDef{
					"name": {
						Type:     "string",
						Required: true,
						// Unique deliberately false: PeeringDB introduced
						// duplicate org display names 2026-04-04. Generator
						// must still emit entgql.OrderField("NAME") despite
						// the dropped uniqueness.
						Unique:   false,
						HelpText: "Organization name",
					},
					"notes": {
						Type:     "string",
						Required: false,
						Default:  "",
						HelpText: "Notes",
					},
				},
				ComputedFields: []string{"net_count"},
				Relationships: map[string]Relationship{
					"networks": {
						Target: "net",
						Type:   "one_to_many",
						Field:  "org_id",
					},
				},
			},
			"net": {
				ModelName: "Network",
				APIPath:   "net",
				Fields: map[string]FieldDef{
					"org_id": {
						Type:       "integer",
						Required:   false,
						Nullable:   true,
						References: "org",
						HelpText:   "FK to organization",
					},
					"name": {
						Type:     "string",
						Required: true,
						Unique:   true,
						HelpText: "Network name",
					},
					"asn": {
						Type:     "integer",
						Required: true,
						Unique:   true,
						HelpText: "Autonomous System Number",
					},
				},
				Relationships: map[string]Relationship{
					"organization": {
						Target: "org",
						Type:   "many_to_one",
						Field:  "org_id",
					},
				},
			},
			"poc": {
				ModelName: "Poc",
				APIPath:   "poc",
				Fields: map[string]FieldDef{
					"net_id": {
						Type:       "integer",
						Required:   false,
						Nullable:   true,
						References: "net",
						HelpText:   "FK to network",
					},
					"name": {
						Type:     "string",
						Required: false,
						Default:  "",
						HelpText: "Contact name",
					},
					"role": {
						Type:      "string",
						MaxLength: 27,
						Required:  true,
						Nullable:  false,
						HelpText:  "Contact role",
					},
				},
				Relationships: map[string]Relationship{
					"network": {
						Target: "net",
						Type:   "many_to_one",
						Field:  "net_id",
					},
				},
			},
		},
	}

	tests := []struct {
		name         string
		apiPath      string
		wantParts    []string
		notWantParts []string
	}{
		{
			name:    "Organization",
			apiPath: "org",
			wantParts: []string{
				`field.Int("id")`,
				`field.String("name")`,
				`entgql.QueryField()`,
				`entgql.RelayConnection()`,
				`entrest.WithIncludeOperations`,
				`entrest.WithEagerLoad(true)`,
				`edge.To("networks"`,
				`otelMutationHook("Organization")`,
				// GraphQL OrderField is emitted for "name" fields whether
				// or not the column is UNIQUE — see generator fieldAnnotations.
				// organizations.name is deliberately non-unique because PeeringDB
				// began serving duplicate display names 2026-04-04 onward.
				`entgql.OrderField("NAME")`,
			},
			notWantParts: []string{
				// organizations.name must NOT be UNIQUE (see schema/peeringdb.json
				// org.name.unique = false). Regression guard against accidentally
				// re-adding .Unique() to the field.
				`field.String("name").
			Unique()`,
				// SEED-001 (active 2026-04-26): organizations.name must NOT have
				// NotEmpty() — upstream tombstones arrive with PII-scrubbed
				// name="" and the validator would reject them at upsert.
				`field.String("name").
			NotEmpty()`,
			},
		},
		{
			name:    "Network",
			apiPath: "net",
			wantParts: []string{
				`field.Int("id")`,
				`field.Int("org_id")`,
				`Optional()`,
				`Nillable()`,
				`edge.From("organization"`,
				`entrest.WithFilter(entrest.FilterEQ`,
				`otelMutationHook("Network")`,
			},
		},
		{
			// Phase 73 BUG-02: poc.role must NOT carry NotEmpty(). Upstream
			// PeeringDB ?since= responses plausibly emit status='deleted' poc
			// tombstones with PII-scrubbed role="" (same GDPR pattern that
			// 260426-pms confirmed for name="" on the 6 folded entities). A
			// NotEmpty() validator on poc.role would reject those tombstones
			// at the upsert builder, aborting incremental sync. Regression
			// guard against accidentally re-introducing NotEmpty() on role.
			name:    "Poc",
			apiPath: "poc",
			wantParts: []string{
				`field.Int("id")`,
				`field.String("role")`,
				`entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)`,
				`index.Fields("role")`,
				`otelMutationHook("Poc")`,
			},
			notWantParts: []string{
				// The literal NotEmpty() emission shape — formatted exactly
				// the way generateFieldCode produces it (period+newline+
				// three tabs+NotEmpty). Built via string concatenation
				// rather than a raw-string-with-source-indentation so the
				// substring contains the EXACT three-tab depth the
				// formatter emits, not however-many-tabs the source line
				// happens to be indented at.
				"field.String(\"role\").\n\t\t\tNotEmpty()",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ot := schema.ObjectTypes[tt.apiPath]
			code, err := generateEntSchema(tt.apiPath, ot, schema)
			if err != nil {
				t.Fatalf("generateEntSchema: %v", err)
			}

			src := string(code)

			// Verify it parses as valid Go.
			fset := token.NewFileSet()
			_, parseErr := parser.ParseFile(fset, "test.go", code, parser.AllErrors)
			if parseErr != nil {
				t.Fatalf("generated code does not parse:\n%s\n\nError: %v", src, parseErr)
			}

			// Verify expected patterns.
			for _, part := range tt.wantParts {
				if !strings.Contains(src, part) {
					t.Errorf("generated code missing %q\n\nCode:\n%s", part, src)
				}
			}
			// Verify forbidden patterns are absent (regression guards).
			for _, part := range tt.notWantParts {
				if strings.Contains(src, part) {
					t.Errorf("generated code must NOT contain %q\n\nCode:\n%s", part, src)
				}
			}
		})
	}
}

func TestGenerateFieldCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		field      FieldDef
		wantSub    []string
		notWantSub []string
	}{
		{
			// SEED-001 (active 2026-04-26): the "name" field is intentionally
			// emitted WITHOUT NotEmpty() because upstream PeeringDB ?since=
			// emits status='deleted' tombstones with PII-scrubbed name="".
			// A NotEmpty() validator would reject those tombstones at upsert
			// and break incremental sync. NotEmpty() is preserved for "prefix"
			// (ixprefix — structurally meaningful row identity, not PII).
			// "role" is dropped from NotEmpty() emission as of Phase 73 BUG-02
			// (symmetric drop for the next likely PII-scrub target on poc rows).
			name: "name",
			field: FieldDef{
				Type:      "string",
				MaxLength: 255,
				Required:  true,
				Unique:    true,
				HelpText:  "Name",
			},
			wantSub: []string{
				`field.String("name")`,
				`Unique()`,
				`entgql.OrderField("NAME")`,
				`entrest.WithFilter(entrest.FilterGroupEqual`,
			},
			notWantSub: []string{
				// SEED-001 regression guard mirroring the role case below:
				// name must not regain NotEmpty() either. Match the literal
				// emission shape `.\n\t\t\tNotEmpty()` produced by
				// generateFieldCode (period+newline+three tabs+NotEmpty).
				".\n\t\t\tNotEmpty()",
			},
		},
		{
			// Phase 73 BUG-02: poc.role is intentionally emitted WITHOUT
			// NotEmpty() because upstream PeeringDB ?since= responses
			// plausibly arrive with PII-scrubbed role="" on tombstones (same
			// GDPR pattern empirically confirmed for name="" by the
			// 260426-pms SEED-001 spike). Belt-and-braces drop — the
			// validator would reject those tombstones at the sync upsert
			// builder, aborting the cycle.
			name: "role",
			field: FieldDef{
				Type:      "string",
				MaxLength: 27,
				Required:  true,
				Nullable:  false,
				HelpText:  "Contact role",
			},
			wantSub: []string{
				`field.String("role")`,
				`entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)`,
			},
			notWantSub: []string{
				// The load-bearing assertion: role must never regain a
				// NotEmpty() validator via codegen. Match the literal
				// emission shape produced by generateFieldCode (period+
				// newline+three tabs+NotEmpty).
				".\n\t\t\tNotEmpty()",
			},
		},
		{
			// Regression guard for ixprefix.prefix: it MUST retain its
			// NotEmpty() validator. IP prefixes are structural row identity,
			// not PII; upstream retains the prefix value on tombstones
			// because the prefix IS the row's natural key. Out of scope per
			// CONTEXT.md D-02 ("ixprefix.prefix retains its validator").
			name: "prefix",
			field: FieldDef{
				Type:     "string",
				Required: true,
				Nullable: false,
				HelpText: "IP prefix",
			},
			wantSub: []string{
				`field.String("prefix")`,
				// Match the literal emission shape produced by
				// generateFieldCode for the validator chain.
				".\n\t\t\tNotEmpty()",
			},
		},
		{
			name: "city",
			field: FieldDef{
				Type:     "string",
				Required: true,
				HelpText: "City",
			},
			wantSub: []string{
				`field.String("city")`,
				`Optional()`,
				`Default("")`,
				`entrest.WithFilter(entrest.FilterGroupEqual`,
			},
		},
		{
			name: "nullable_int",
			field: FieldDef{
				Type:     "integer",
				Nullable: true,
				HelpText: "Nullable integer",
			},
			wantSub: []string{
				`field.Int("nullable_int")`,
				`Optional()`,
				`Nillable()`,
			},
		},
		{
			name: "fk_field",
			field: FieldDef{
				Type:       "integer",
				Nullable:   true,
				References: "org",
				HelpText:   "FK to organization",
			},
			wantSub: []string{
				`field.Int("fk_field")`,
				`Optional()`,
				`Nillable()`,
				`entrest.WithFilter(entrest.FilterEQ`,
			},
		},
		{
			name: "asn",
			field: FieldDef{
				Type:     "integer",
				Required: true,
				Unique:   true,
				HelpText: "ASN",
			},
			wantSub: []string{
				`field.Int("asn")`,
				`Positive()`,
				`Unique()`,
				`entrest.WithFilter(entrest.FilterEQ`,
			},
		},
		{
			name: "bool_with_default",
			field: FieldDef{
				Type:    "boolean",
				Default: false,
			},
			wantSub: []string{
				`field.Bool("bool_with_default")`,
				`Default(false)`,
			},
		},
		{
			name: "datetime_nullable",
			field: FieldDef{
				Type:     "datetime",
				Nullable: true,
			},
			wantSub: []string{
				`field.Time("datetime_nullable")`,
				`Optional()`,
				`Nillable()`,
			},
		},
		{
			name: "social_media",
			field: FieldDef{
				Type: "json_array",
			},
			wantSub: []string{
				`field.JSON("social_media", []schematypes.SocialMedia{})`,
				`Optional()`,
				`socialMediaSchema()`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code := generateFieldCode(tt.name, tt.field)
			for _, sub := range tt.wantSub {
				if !strings.Contains(code, sub) {
					t.Errorf("field code missing %q\n\nCode: %s", sub, code)
				}
			}
			for _, ns := range tt.notWantSub {
				if strings.Contains(code, ns) {
					t.Errorf("field code must NOT contain %q\n\nCode: %s", ns, code)
				}
			}
		})
	}
}

func TestToSnakeCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"Organization", "organization"},
		{"NetworkIXLan", "network_ix_lan"},
		{"InternetExchange", "internet_exchange"},
		{"IXPrefix", "ix_prefix"},
		{"Poc", "poc"},
		{"IXLan", "ix_lan"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := toSnakeCase(tt.input)
			if got != tt.want {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateIndexes(t *testing.T) {
	t.Parallel()

	// Phase 74 D-01: replace the previous hand-maintained allow-list with a
	// derivation-driven assertion. The expected index set comes from the same
	// schema/peeringdb.json source the generator consumes; if a future schema
	// edit introduces a new index rule, this test passes without an allow-list
	// edit. If generateIndexes accidentally emits a typo'd or non-existent
	// field name, the structural sanity check in the per-entity loop catches
	// it because the field will not appear in ot.Fields.

	// Sub-test 1: legacy unit fixture — proves generateIndexes still produces
	// the historically-expected set for the "net" minimal fixture. Pinning this
	// guards against accidental rule changes (e.g. someone removing the asn
	// special-case for net).
	t.Run("legacy_net_fixture", func(t *testing.T) {
		t.Parallel()
		ot := ObjectType{
			Fields: map[string]FieldDef{
				"name":   {Type: "string", Required: true},
				"org_id": {Type: "integer", References: "org"},
				"asn":    {Type: "integer", Unique: true},
			},
		}
		got := generateIndexes("net", ot)
		// Independent expectation: the historically-pinned index set for the
		// "net" minimal fixture, hand-derived from the rules in generateIndexes
		// (asn special-case, status+updated always-on, FK fields, named field).
		// Comparing against ExpectedIndexesFor here would be tautological —
		// that helper is currently a thin wrapper around generateIndexes — so
		// the literal is the only assertion that can actually catch drift.
		want := []string{"asn", "name", "org_id", "status", "updated"}
		if !slicesEqual(got, want) {
			t.Errorf("generateIndexes(net, fixture) = %v, want %v", got, want)
		}
	})

	// Sub-test 2: per-entity drive from schema/peeringdb.json — derivation
	// proper. Asserts that every index emitted for every real entity refers to
	// a field that actually exists, OR to one of the always-on schema fields
	// (status, updated), OR to a documented apiPath-special case.
	t.Run("per_entity_from_schema_source", func(t *testing.T) {
		t.Parallel()
		schemaPath := filepath.Join("..", "..", "schema", "peeringdb.json")
		raw, err := os.ReadFile(schemaPath)
		if err != nil {
			t.Fatalf("reading %s: %v", schemaPath, err)
		}
		var sch Schema
		if err := json.Unmarshal(raw, &sch); err != nil {
			t.Fatalf("unmarshalling %s: %v", schemaPath, err)
		}
		if len(sch.ObjectTypes) == 0 {
			t.Fatal("schema/peeringdb.json contains no ObjectTypes — schema source missing or malformed")
		}

		// Always-on indexes that may not appear as fields in the schema JSON
		// but are appended unconditionally by generateIndexes.
		alwaysOn := map[string]bool{"status": true, "updated": true}

		// Documented apiPath-special-case indexes (mirrors the switch in
		// generateIndexes). Keep this set explicit so a future addition forces
		// the maintainer to update both sides.
		apiPathSpecials := map[string]map[string]bool{
			"net":   {"asn": true},
			"ixpfx": {"prefix": true},
			"poc":   {"role": true},
		}

		for apiPath, ot := range sch.ObjectTypes {
			t.Run(apiPath, func(t *testing.T) {
				t.Parallel()

				got := generateIndexes(apiPath, ot)

				// Note: an `ExpectedIndexesFor(apiPath, ot)` round-trip would
				// be tautological — that helper is a thin wrapper around
				// generateIndexes (see main.go doc comment on
				// ExpectedIndexesFor). The drift protection in this loop comes
				// from the structural-sanity invariants below (every emitted
				// index is a real field, an always-on synthetic, or a
				// documented apiPath special-case; the slice is strictly
				// ascending). Re-deriving the same rules in two places adds
				// no signal and dresses up dead assertion code as a check.

				// Structural sanity: every emitted index is either an actual
				// field, an always-on synthetic, or a documented special case.
				specials := apiPathSpecials[apiPath]
				for _, idx := range got {
					_, isField := ot.Fields[idx]
					if isField {
						continue
					}
					if alwaysOn[idx] {
						continue
					}
					if specials[idx] {
						continue
					}
					t.Errorf("entity %q: index %q refers to no field in schema, is not always-on (status/updated), and is not a documented apiPath special-case — likely a typo or stale rule", apiPath, idx)
				}

				// Sort + dedup invariant: the result must be sorted ascending
				// and contain no duplicates.
				for i := 1; i < len(got); i++ {
					if got[i-1] >= got[i] {
						t.Errorf("entity %q: indexes not strictly ascending: %v", apiPath, got)
						break
					}
				}
			})
		}
	})
}

// slicesEqual returns true when a and b contain the same elements in the
// same order.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGenerateTypesFile(t *testing.T) {
	t.Parallel()

	code, err := generateTypesFile()
	if err != nil {
		t.Fatalf("generateTypesFile: %v", err)
	}

	src := string(code)
	// Phase 59-04: SocialMedia moved to ent/schematypes to break an
	// import cycle. types.go now only holds socialMediaSchema() (ogen)
	// and a pointer comment; the value type is verified by
	// ent/schematypes compile-time presence, not by string-matching
	// here.
	for _, want := range []string{
		"socialMediaSchema()",
		"ogen.NewSchema()",
		"ent/schematypes",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("types.go missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"type SocialMedia",
		`json:"service"`,
	} {
		if strings.Contains(src, unwanted) {
			t.Errorf("types.go should no longer contain %q (moved to ent/schematypes)", unwanted)
		}
	}

	// Verify it parses.
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "types.go", code, parser.AllErrors); err != nil {
		t.Fatalf("types.go does not parse: %v", err)
	}
}

func TestSynthesizeReverseEdges(t *testing.T) {
	t.Parallel()

	schema := &Schema{
		ObjectTypes: map[string]ObjectType{
			"fac": {
				ModelName: "Facility",
				APIPath:   "fac",
			},
			"carrierfac": {
				ModelName: "CarrierFacility",
				APIPath:   "carrierfac",
				Relationships: map[string]Relationship{
					"facility": {
						Target: "fac",
						Type:   "many_to_one",
						Field:  "fac_id",
					},
				},
			},
		},
	}

	synthesizeReverseEdges(schema)

	facOT := schema.ObjectTypes["fac"]
	if len(facOT.Relationships) != 1 {
		t.Fatalf("expected 1 synthesized relationship on Facility, got %d", len(facOT.Relationships))
	}

	rel, ok := facOT.Relationships["carrier_facilities"]
	if !ok {
		t.Fatalf("missing synthesized carrier_facilities relationship; got keys: %v", func() []string {
			var keys []string
			for k := range facOT.Relationships {
				keys = append(keys, k)
			}
			return keys
		}())
	}

	if rel.Target != "carrierfac" {
		t.Errorf("target = %q, want %q", rel.Target, "carrierfac")
	}
	if rel.Type != "one_to_many" {
		t.Errorf("type = %q, want %q", rel.Type, "one_to_many")
	}
}

func TestFullPipelineFromJSON(t *testing.T) {
	t.Parallel()

	// Load the actual schema/peeringdb.json.
	schema, err := loadSchema("../../schema/peeringdb.json")
	if err != nil {
		t.Fatalf("loadSchema: %v", err)
	}

	if len(schema.ObjectTypes) != 13 {
		t.Errorf("expected 13 object types, got %d", len(schema.ObjectTypes))
	}

	// Resolve model names (as main() does).
	for key, ot := range schema.ObjectTypes {
		ot.ModelName = resolveModelName(ot.ModelName)
		schema.ObjectTypes[key] = ot
	}
	synthesizeReverseEdges(schema)

	// Generate all schemas to a temp dir and verify they parse.
	dir := t.TempDir()
	for apiPath, ot := range schema.ObjectTypes {
		code, err := generateEntSchema(apiPath, ot, schema)
		if err != nil {
			t.Fatalf("generateEntSchema(%s): %v", apiPath, err)
		}

		fset := token.NewFileSet()
		_, parseErr := parser.ParseFile(fset, ot.ModelName+".go", code, parser.AllErrors)
		if parseErr != nil {
			t.Errorf("generated code for %s does not parse: %v", apiPath, parseErr)
		}

		// Verify lowercase filename convention.
		filename := strings.ToLower(ot.ModelName) + ".go"
		fullPath := filepath.Join(dir, filename)
		if err := os.WriteFile(fullPath, code, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}
