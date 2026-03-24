// pdb-schema-generate reads an intermediate JSON schema produced by
// pdb-schema-extract and generates entgo schema Go files.
//
// Usage:
//
//	pdb-schema-generate <schema.json> [output-dir]
//
// If output-dir is not specified, it defaults to "ent/schema".
// Generated files are formatted with go/format.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

// Schema is the top-level intermediate JSON representation.
type Schema struct {
	Version     string                `json:"version"`
	ExtractedAt string                `json:"extracted_at"`
	SourcePath  string                `json:"source_path"`
	ObjectTypes map[string]ObjectType `json:"object_types"`
}

// ObjectType describes a single PeeringDB object type.
type ObjectType struct {
	ModelName      string                  `json:"model_name"`
	APIPath        string                  `json:"api_path"`
	BaseClasses    []string                `json:"base_classes,omitempty"`
	Fields         map[string]FieldDef     `json:"fields"`
	ComputedFields []string                `json:"computed_fields,omitempty"`
	Relationships  map[string]Relationship `json:"relationships,omitempty"`
}

// FieldDef describes a single field within an object type.
type FieldDef struct {
	Type       string      `json:"type"`
	MaxLength  int         `json:"max_length,omitempty"`
	Required   bool        `json:"required"`
	Unique     bool        `json:"unique,omitempty"`
	Nullable   bool        `json:"nullable,omitempty"`
	ReadOnly   bool        `json:"read_only"`
	Deprecated bool        `json:"deprecated"`
	HelpText   string      `json:"help_text,omitempty"`
	Default    interface{} `json:"default"`
	References string      `json:"references,omitempty"`
}

// Relationship describes a relationship between object types.
type Relationship struct {
	Target string `json:"target"`
	Type   string `json:"type"`
	Field  string `json:"field"`
}

// modelNameOverrides maps JSON schema model names (derived from Python) to
// the Go type names used in the committed ent schemas.
var modelNameOverrides = map[string]string{
	"IXLan":          "IxLan",
	"IXPrefix":       "IxPrefix",
	"IXFacility":     "IxFacility",
	"NetworkIXLan":   "NetworkIxLan",
	"NetworkContact": "Poc",
}

// resolveModelName converts a JSON schema model name to the Go type name.
func resolveModelName(jsonName string) string {
	if override, ok := modelNameOverrides[jsonName]; ok {
		return override
	}
	return jsonName
}

// synthesizeReverseEdges ensures every many_to_one relationship has a
// corresponding one_to_many on the target type. The JSON schema from
// pdb-schema-extract may omit reverse edges when only the FK side is
// defined in the Python model.
func synthesizeReverseEdges(schema *Schema) {
	for sourceAPI, sourceOT := range schema.ObjectTypes {
		for _, rel := range sourceOT.Relationships {
			if rel.Type != "many_to_one" {
				continue
			}
			targetOT, ok := schema.ObjectTypes[rel.Target]
			if !ok {
				continue
			}
			// Check if target already has a one_to_many back to source.
			hasReverse := false
			for _, tRel := range targetOT.Relationships {
				if tRel.Type == "one_to_many" && tRel.Target == sourceAPI {
					hasReverse = true
					break
				}
			}
			if hasReverse {
				continue
			}
			// Synthesize the reverse edge.
			edgeName := simplePlural(toSnakeCase(sourceOT.ModelName))
			if targetOT.Relationships == nil {
				targetOT.Relationships = make(map[string]Relationship)
			}
			targetOT.Relationships[edgeName] = Relationship{
				Target: sourceAPI,
				Type:   "one_to_many",
				Field:  rel.Field,
			}
			schema.ObjectTypes[rel.Target] = targetOT
		}
	}
}

// simplePlural applies basic English pluralization rules.
func simplePlural(s string) string {
	if strings.HasSuffix(s, "y") && len(s) > 1 {
		// consonant + y → ies (e.g., facility → facilities)
		prev := s[len(s)-2]
		if prev != 'a' && prev != 'e' && prev != 'i' && prev != 'o' && prev != 'u' {
			return s[:len(s)-1] + "ies"
		}
	}
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") || strings.HasSuffix(s, "z") ||
		strings.HasSuffix(s, "sh") || strings.HasSuffix(s, "ch") {
		return s + "es"
	}
	return s + "s"
}

// filterableStringFields is the set of string field names that receive
// entrest.FilterGroupEqual | entrest.FilterGroupArray annotations.
// Applies to both regular and computed fields.
var filterableStringFields = map[string]bool{
	"name": true, "aka": true, "name_long": true,
	"city": true, "state": true, "country": true,
	"role": true, "visible": true, "email": true,
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: pdb-schema-generate <schema.json> [output-dir]")
	}
	schemaPath := os.Args[1]
	outputDir := "ent/schema"
	if len(os.Args) > 2 {
		outputDir = os.Args[2]
	}

	schema, err := loadSchema(schemaPath)
	if err != nil {
		log.Fatalf("load schema: %v", err)
	}

	// Resolve model names before generation.
	for key, ot := range schema.ObjectTypes {
		ot.ModelName = resolveModelName(ot.ModelName)
		schema.ObjectTypes[key] = ot
	}

	// Synthesize missing reverse (one_to_many) relationships.
	// The JSON schema may omit reverse edges when only the FK side is defined.
	synthesizeReverseEdges(schema)

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	for typeName, typeDef := range schema.ObjectTypes {
		code, err := generateEntSchema(typeName, typeDef, schema)
		if err != nil {
			log.Fatalf("generate schema for %s: %v", typeName, err)
		}
		filename := filepath.Join(outputDir, strings.ToLower(typeDef.ModelName)+".go")
		if err := os.WriteFile(filename, code, 0o644); err != nil {
			log.Fatalf("write %s: %v", filename, err)
		}
		log.Printf("wrote %s", filename)
	}

	// Generate shared types file.
	typesCode, err := generateTypesFile()
	if err != nil {
		log.Fatalf("generate types.go: %v", err)
	}
	typesPath := filepath.Join(outputDir, "types.go")
	if err := os.WriteFile(typesPath, typesCode, 0o644); err != nil {
		log.Fatalf("write %s: %v", typesPath, err)
	}
	log.Printf("wrote %s", typesPath)
}

// loadSchema reads and parses a JSON schema file.
func loadSchema(path string) (*Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var schema Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &schema, nil
}

// entSchemaData holds template data for generating an entgo schema file.
type entSchemaData struct {
	ModelName      string
	APIPath        string
	Fields         []entFieldData
	ComputedFields []entFieldData
	Edges          []entEdgeData
	Indexes        []string
	HasEdges       bool
	HasJSON        bool
	HasSocialMedia bool
}

// entFieldData represents a single entgo field definition.
type entFieldData struct {
	Name     string
	Code     string // The Go code for this field definition.
	IsFK     bool
	FKTarget string
}

// entEdgeData represents a single entgo edge definition.
type entEdgeData struct {
	Code string
}

// generateEntSchema produces Go source for a single entgo schema.
func generateEntSchema(apiPath string, ot ObjectType, schema *Schema) ([]byte, error) {
	data := entSchemaData{
		ModelName: ot.ModelName,
		APIPath:   apiPath,
	}

	// Sort field names for deterministic output.
	fieldNames := sortedFieldNames(ot.Fields)

	// Generate field definitions.
	for _, name := range fieldNames {
		fd := ot.Fields[name]
		code := generateFieldCode(name, fd)
		isFK := fd.References != ""
		ef := entFieldData{
			Name:     name,
			Code:     code,
			IsFK:     isFK,
			FKTarget: fd.References,
		}
		data.Fields = append(data.Fields, ef)
		if fd.Type == "json_array" {
			data.HasJSON = true
			if name == "social_media" {
				data.HasSocialMedia = true
			}
		}
	}

	// Generate computed fields (stored per D-40).
	for _, cf := range ot.ComputedFields {
		code := generateComputedFieldCode(cf, apiPath)
		data.ComputedFields = append(data.ComputedFields, entFieldData{
			Name: cf,
			Code: code,
		})
	}

	// Generate edges from relationships.
	for _, rel := range sortedRelationships(ot.Relationships) {
		edgeCode := generateEdgeCode(rel.name, rel.rel, ot, schema)
		if edgeCode != "" {
			data.Edges = append(data.Edges, entEdgeData{Code: edgeCode})
			data.HasEdges = true
		}
	}

	// Generate indexes.
	data.Indexes = generateIndexes(apiPath, ot)

	// Render template.
	var buf bytes.Buffer
	funcMap := template.FuncMap{
		"toLower": strings.ToLower,
	}
	tmpl, err := template.New("schema").Funcs(funcMap).Parse(schemaTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	// Format the generated code.
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format source for %s: %w\n\nRaw source:\n%s", ot.ModelName, err, buf.String())
	}
	return formatted, nil
}

// generateFieldCode produces the Go code for a single entgo field definition.
func generateFieldCode(name string, fd FieldDef) string {
	var b strings.Builder

	switch fd.Type {
	case "string":
		fmt.Fprintf(&b, "field.String(%q)", name)
		if fd.Required && !fd.Nullable && fd.References == "" && isNameField(name) {
			b.WriteString(".\n\t\t\tNotEmpty()")
		}
		if fd.Unique {
			b.WriteString(".\n\t\t\tUnique()")
		}
		// For a read-only mirror, string fields with Required from Django
		// form validation are stored as Optional with defaults. Only name-type
		// fields enforce NotEmpty. All non-name string fields are Optional.
		if !isNameField(name) || !fd.Required || fd.Nullable {
			b.WriteString(".\n\t\t\tOptional()")
		}
		if fd.Nullable {
			b.WriteString(".\n\t\t\tNillable()")
		}
		if fd.Default != nil && !fd.Nullable {
			fmt.Fprintf(&b, ".\n\t\t\tDefault(%q)", fmt.Sprintf("%v", fd.Default))
		} else if !fd.Nullable && !isNameField(name) && fd.Required {
			// Required non-name string fields get empty default for the mirror.
			b.WriteString(".\n\t\t\tDefault(\"\")")
		}

	case "integer":
		fmt.Fprintf(&b, "field.Int(%q)", name)
		if fd.Unique {
			b.WriteString(".\n\t\t\tUnique()")
		}
		if name == "asn" {
			b.WriteString(".\n\t\t\tPositive()")
		}
		if !fd.Required || fd.Nullable || fd.References != "" {
			b.WriteString(".\n\t\t\tOptional()")
		}
		if fd.Nullable || fd.References != "" {
			b.WriteString(".\n\t\t\tNillable()")
		}
		if fd.Default != nil {
			defVal := fmt.Sprintf("%v", fd.Default)
			// JSON numbers are float64.
			if f, ok := fd.Default.(float64); ok {
				defVal = fmt.Sprintf("%d", int(f))
			}
			fmt.Fprintf(&b, ".\n\t\t\tDefault(%s)", defVal)
		}

	case "float":
		fmt.Fprintf(&b, "field.Float(%q)", name)
		if !fd.Required || fd.Nullable {
			b.WriteString(".\n\t\t\tOptional()")
		}
		if fd.Nullable {
			b.WriteString(".\n\t\t\tNillable()")
		}

	case "boolean":
		fmt.Fprintf(&b, "field.Bool(%q)", name)
		if !fd.Required || fd.Nullable {
			if fd.Nullable {
				b.WriteString(".\n\t\t\tOptional()")
				b.WriteString(".\n\t\t\tNillable()")
			}
		}
		if fd.Default != nil && !fd.Nullable {
			fmt.Fprintf(&b, ".\n\t\t\tDefault(%v)", fd.Default)
		}

	case "datetime":
		fmt.Fprintf(&b, "field.Time(%q)", name)
		if !fd.Required || fd.Nullable {
			b.WriteString(".\n\t\t\tOptional()")
		}
		if fd.Nullable {
			b.WriteString(".\n\t\t\tNillable()")
		}

	case "json_array":
		if name == "social_media" {
			fmt.Fprintf(&b, "field.JSON(%q, []SocialMedia{})", name)
		} else {
			fmt.Fprintf(&b, "field.JSON(%q, []string{})", name)
		}
		b.WriteString(".\n\t\t\tOptional()")
	}

	// Add field-level annotations.
	b.WriteString(fieldAnnotations(name, fd))

	if fd.HelpText != "" {
		fmt.Fprintf(&b, ".\n\t\t\tComment(%q)", fd.HelpText)
	}

	return b.String()
}

const (
	fkFilterAnnotation          = "Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn))"
	equalArrayFilterAnnotation  = "Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray))"
	socialMediaSchemaAnnotation = "Annotations(entrest.WithSchema(socialMediaSchema()))"
)

// filterableIntFields is the set of non-FK integer field names that receive
// the full numeric filter annotation (same as FK fields).
var filterableIntFields = map[string]bool{
	"asn": true,
}

// fieldAnnotations returns the annotation chain for a field based on its characteristics.
func fieldAnnotations(name string, fd FieldDef) string {
	if fd.References != "" || filterableIntFields[name] {
		return ".\n\t\t\t" + fkFilterAnnotation
	}
	if name == "social_media" {
		return ".\n\t\t\t" + socialMediaSchemaAnnotation
	}
	if name == "name" && fd.Required && fd.Unique {
		return ".\n\t\t\tAnnotations(\n\t\t\t\tentgql.OrderField(\"NAME\"),\n\t\t\t\tentrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray),\n\t\t\t)"
	}
	if filterableStringFields[name] {
		return ".\n\t\t\t" + equalArrayFilterAnnotation
	}
	return ""
}

// generateComputedFieldCode produces entgo field code for serializer-computed fields.
func generateComputedFieldCode(name, _ string) string {
	var annotation string
	if filterableStringFields[name] {
		annotation = fmt.Sprintf(".\n\t\t\t%s", equalArrayFilterAnnotation)
	}

	// Infer type from field name patterns.
	switch {
	case strings.HasSuffix(name, "_count"):
		return fmt.Sprintf("field.Int(%q).\n\t\t\tOptional().\n\t\t\tDefault(0).\n\t\t\tComment(%q)", name, toTitleCase(name)+" (computed)")
	case strings.HasSuffix(name, "_updated"):
		return fmt.Sprintf("field.Time(%q).\n\t\t\tOptional().\n\t\t\tNillable().\n\t\t\tComment(%q)", name, toTitleCase(name)+" (computed)")
	case name == "org_name":
		return fmt.Sprintf("field.String(%q).\n\t\t\tOptional().\n\t\t\tDefault(\"\").\n\t\t\tComment(%q)", name, toTitleCase(name)+" (computed)")
	case name == "name" || name == "city" || name == "country":
		return fmt.Sprintf("field.String(%q).\n\t\t\tOptional().\n\t\t\tDefault(\"\")%s.\n\t\t\tComment(%q)", name, annotation, toTitleCase(name)+" (computed)")
	case name == "ix_id":
		return fmt.Sprintf("field.Int(%q).\n\t\t\tOptional().\n\t\t\tComment(%q)", name, "Internet exchange ID (computed)")
	case strings.HasSuffix(name, "_request"):
		return fmt.Sprintf("field.String(%q).\n\t\t\tOptional().\n\t\t\tNillable().\n\t\t\tComment(%q)", name, toTitleCase(name)+" (computed)")
	case strings.HasSuffix(name, "_status"):
		return fmt.Sprintf("field.String(%q).\n\t\t\tOptional().\n\t\t\tDefault(\"\").\n\t\t\tComment(%q)", name, toTitleCase(name)+" (computed)")
	default:
		return fmt.Sprintf("field.String(%q).\n\t\t\tOptional().\n\t\t\tComment(%q)", name, toTitleCase(name)+" (computed)")
	}
}

type namedRelationship struct {
	name string
	rel  Relationship
}

func sortedRelationships(rels map[string]Relationship) []namedRelationship {
	var result []namedRelationship
	for name, rel := range rels {
		result = append(result, namedRelationship{name, rel})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].name < result[j].name
	})
	return result
}

// generateEdgeCode produces entgo edge definition code for a relationship.
func generateEdgeCode(name string, rel Relationship, ot ObjectType, schema *Schema) string {
	targetType := apiPathToModelName(rel.Target, schema)
	if targetType == "" {
		return ""
	}

	eagerLoad := ".\n\t\t\tAnnotations(entrest.WithEagerLoad(true))"

	switch rel.Type {
	case "many_to_one":
		// This type has a FK to the target: edge.From
		refName := inferEdgeRefName(ot.APIPath, rel.Target, schema)
		return fmt.Sprintf("edge.From(%q, %s.Type).\n\t\t\tRef(%q).\n\t\t\tField(%q).\n\t\t\tUnique()%s",
			name, targetType, refName, rel.Field, eagerLoad)

	case "one_to_many":
		// This type owns the reverse edge: edge.To
		return fmt.Sprintf("edge.To(%q, %s.Type)%s",
			name, targetType, eagerLoad)
	}
	return ""
}

// apiPathToModelName converts an API path to its entgo model type name.
func apiPathToModelName(apiPath string, schema *Schema) string {
	if ot, ok := schema.ObjectTypes[apiPath]; ok {
		return ot.ModelName
	}
	return ""
}

// inferEdgeRefName determines the edge reference name (the corresponding
// edge.To name on the target type).
func inferEdgeRefName(sourceAPI, targetAPI string, schema *Schema) string {
	targetOT, ok := schema.ObjectTypes[targetAPI]
	if !ok {
		return sourceAPI + "s"
	}

	// Find the relationship on the target that points back to the source.
	for relName, rel := range targetOT.Relationships {
		if rel.Type == "one_to_many" && rel.Target == sourceAPI {
			return relName
		}
	}

	return simplePlural(toSnakeCase(apiPathToModelName(sourceAPI, schema)))
}

// generateIndexes produces index field lists for common query patterns (D-45).
func generateIndexes(apiPath string, ot ObjectType) []string {
	var indexes []string

	// Always index name and status if they exist.
	if _, ok := ot.Fields["name"]; ok {
		indexes = append(indexes, "name")
	}

	// Index FK fields.
	for name, fd := range ot.Fields {
		if fd.References != "" {
			indexes = append(indexes, name)
		}
	}

	// Index special fields.
	switch apiPath {
	case "net":
		indexes = append(indexes, "asn")
	case "ixpfx":
		indexes = append(indexes, "prefix")
	case "poc":
		indexes = append(indexes, "role")
	}

	// Always index status (common field).
	indexes = append(indexes, "status")

	sort.Strings(indexes)
	// Deduplicate.
	result := indexes[:0]
	seen := make(map[string]bool)
	for _, idx := range indexes {
		if !seen[idx] {
			seen[idx] = true
			result = append(result, idx)
		}
	}
	return result
}

// generateTypesFile produces the shared types.go file.
func generateTypesFile() ([]byte, error) {
	src := `// Package schema defines the entgo schema types for PeeringDB objects.
package schema

import "github.com/ogen-go/ogen"

// SocialMedia represents a social media link from PeeringDB.
// Used by Organization, Network, Facility, InternetExchange, Carrier, and Campus schemas.
type SocialMedia struct {
	Service    string ` + "`json:\"service\"`" + `
	Identifier string ` + "`json:\"identifier\"`" + `
}

// socialMediaSchema returns the OpenAPI schema for the social_media JSON field.
// Required because entrest cannot auto-infer the schema for custom struct types.
func socialMediaSchema() *ogen.Schema {
	return ogen.NewSchema().SetType("array").SetItems(
		ogen.NewSchema().SetType("object").
			AddRequiredProperties(
				ogen.String().ToProperty("service"),
				ogen.String().ToProperty("identifier"),
			),
	)
}
`
	return format.Source([]byte(src))
}

// schemaTemplate is the Go text template for entgo schema files.
var schemaTemplate = `package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	{{- if .HasEdges}}
	"entgo.io/ent/schema/edge"
	{{- end}}
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/lrstanley/entrest"
)

// {{.ModelName}} holds the schema definition for the {{.ModelName}} entity.
// Maps to the PeeringDB "{{.APIPath}}" object type.
type {{.ModelName}} struct {
	ent.Schema
}

// Fields of the {{.ModelName}}.
func ({{.ModelName}}) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB {{toLower .ModelName}} ID"),
		{{- range .Fields}}
		{{.Code}},
		{{- end}}
		{{- if .ComputedFields}}

		// Computed fields (from serializer, stored per D-40)
		{{- range .ComputedFields}}
		{{.Code}},
		{{- end}}
		{{- end}}

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Record status"),
	}
}

// Edges of the {{.ModelName}}.
func ({{.ModelName}}) Edges() []ent.Edge {
	{{- if .HasEdges}}
	return []ent.Edge{
		{{- range .Edges}}
		{{.Code}},
		{{- end}}
	}
	{{- else}}
	return nil
	{{- end}}
}

// Indexes of the {{.ModelName}}.
func ({{.ModelName}}) Indexes() []ent.Index {
	return []ent.Index{
		{{- range .Indexes}}
		index.Fields("{{.}}"),
		{{- end}}
	}
}

// Annotations of the {{.ModelName}}.
func ({{.ModelName}}) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
	}
}

// Hooks returns {{.ModelName}} mutation hooks for OTel tracing per D-46.
func ({{.ModelName}}) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("{{.ModelName}}"),
	}
}
`

// toSnakeCase converts CamelCase to snake_case.
func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := rune(s[i-1])
				if unicode.IsLower(prev) || (i+1 < len(s) && unicode.IsLower(rune(s[i+1]))) {
					result = append(result, '_')
				}
			}
			result = append(result, unicode.ToLower(r))
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

// toTitleCase converts snake_case to Title Case.
func toTitleCase(s string) string {
	words := strings.Split(s, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// sortedFieldNames returns field names sorted alphabetically, but with FK fields
// (those with References) sorted first.
func sortedFieldNames(fields map[string]FieldDef) []string {
	var fks, regular []string
	for name, fd := range fields {
		if fd.References != "" {
			fks = append(fks, name)
		} else {
			regular = append(regular, name)
		}
	}
	sort.Strings(fks)
	sort.Strings(regular)
	return append(fks, regular...)
}

// isNameField returns true if the field is a "name" type field that should have NotEmpty.
func isNameField(name string) bool {
	return name == "name" || name == "prefix" || name == "role"
}
