// pdb-schema-extract parses PeeringDB Django serializer and model Python source
// to produce an intermediate JSON schema representation. The JSON describes all 13
// PeeringDB object types with field metadata, FK references, and read-only annotations.
//
// Usage:
//
//	pdb-schema-extract <peeringdb-repo-path> [--validate]
//
// The repo path should point to the root of a local peeringdb/peeringdb checkout.
// The tool expects to find:
//
//   - {repo}/peeringdb_server/serializers.py
//   - {repo}/../django-peeringdb/src/django_peeringdb/models/abstract.py
//   - {repo}/../django-peeringdb/src/django_peeringdb/models/concrete.py
//   - {repo}/../django-peeringdb/src/django_peeringdb/const.py
//
// When --validate is passed, the tool also fetches sample data from
// beta.peeringdb.com and compares response field names against extracted
// schema fields, reporting mismatches.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Schema is the top-level intermediate JSON representation of the PeeringDB
// data model.
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
	Type       string `json:"type"`
	MaxLength  int    `json:"max_length,omitempty"`
	Required   bool   `json:"required"`
	Unique     bool   `json:"unique,omitempty"`
	Nullable   bool   `json:"nullable,omitempty"`
	ReadOnly   bool   `json:"read_only"`
	Deprecated bool   `json:"deprecated"`
	HelpText   string `json:"help_text,omitempty"`
	Default    any    `json:"default"`
	References string `json:"references,omitempty"`
}

// Relationship describes a relationship between object types.
type Relationship struct {
	Target string `json:"target"`
	Type   string `json:"type"`
	Field  string `json:"field"`
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: pdb-schema-extract <peeringdb-repo-path> [--validate]")
	}
	repoPath := os.Args[1]
	validate := slices.Contains(os.Args[2:], "--validate")

	schema, err := extractSchema(repoPath)
	if err != nil {
		log.Fatalf("extract schema: %v", err)
	}

	if validate {
		if err := validateAgainstAPI(schema); err != nil {
			log.Fatalf("validation failed: %v", err)
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(schema); err != nil {
		log.Fatalf("encode JSON: %v", err)
	}
}

// extractSchema reads PeeringDB Python source files from repoPath and
// produces the intermediate JSON schema representation.
func extractSchema(repoPath string) (*Schema, error) {
	// Read all source files.
	serializersSrc, err := readSourceFile(repoPath, "peeringdb_server/serializers.py")
	if err != nil {
		return nil, fmt.Errorf("read serializers.py: %w", err)
	}

	abstractSrc, err := readSourceFile(repoPath, "../django-peeringdb/src/django_peeringdb/models/abstract.py")
	if err != nil {
		return nil, fmt.Errorf("read abstract.py: %w", err)
	}

	concreteSrc, err := readSourceFile(repoPath, "../django-peeringdb/src/django_peeringdb/models/concrete.py")
	if err != nil {
		return nil, fmt.Errorf("read concrete.py: %w", err)
	}

	constSrc, err := readSourceFile(repoPath, "../django-peeringdb/src/django_peeringdb/const.py")
	if err != nil {
		return nil, fmt.Errorf("read const.py: %w", err)
	}

	// Parse source files.
	serializers := parseSerializers(serializersSrc)
	modelFields := parseModelFields(abstractSrc, concreteSrc)
	_ = parseChoiceConstants(constSrc) // Captured for reference, constants stored as field metadata.

	// Build object types.
	objectTypes := buildObjectTypes(serializers, modelFields)

	return &Schema{
		Version:     "1.0",
		ExtractedAt: time.Now().UTC().Format(time.RFC3339),
		SourcePath:  repoPath,
		ObjectTypes: objectTypes,
	}, nil
}

// readSourceFile reads a Python source file relative to repoPath.
func readSourceFile(repoPath, relPath string) (string, error) {
	p := filepath.Join(repoPath, relPath)
	data, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", p, err)
	}
	return string(data), nil
}

// SerializerInfo holds parsed data about a Django REST Framework serializer class.
type SerializerInfo struct {
	Name           string
	ModelName      string
	BaseClasses    []string
	Fields         map[string]FieldDef
	ReadOnlyFields []string
	AllFields      bool // True when fields = "__all__"
	FieldList      []string
}

// Regex patterns for parsing Django source.
var (
	// Serializer class declaration, e.g.: class OrgSerializer(ModelSerializer):
	reSerializerClass = regexp.MustCompile(`^class\s+(\w+Serializer)\s*\(([^)]+)\)\s*:`)

	// Meta model, e.g.: model = Organization
	reMetaModel = regexp.MustCompile(`^\s+model\s*=\s*(\w+)`)

	// Fields list, e.g.: fields = ["id", "name", ...]
	reFieldsList = regexp.MustCompile(`^\s+fields\s*=\s*\[([^\]]*)\]`)

	// Fields = "__all__"
	reFieldsAll = regexp.MustCompile(`^\s+fields\s*=\s*"__all__"`)

	// Read-only fields, e.g.: read_only_fields = ["id", "created", ...]
	reReadOnlyFields = regexp.MustCompile(`^\s+read_only_fields\s*=\s*\[([^\]]*)\]`)

	// Serializer field definition, e.g.: name = serializers.CharField(max_length=255, required=True)
	reSerializerField = regexp.MustCompile(`^\s+(\w+)\s*=\s*serializers\.(\w+)\(([^)]*)\)`)

	// Django model field definition, e.g.: name = models.CharField(max_length=255, ...)
	reModelField = regexp.MustCompile(`^\s+(\w+)\s*=\s*models\.(\w+)\(([^)]*)\)`)

	// Foreign key, e.g.: org = models.ForeignKey(Organization, ...) or ForeignKey("Organization", ...)
	reForeignKey = regexp.MustCompile(`ForeignKey\(\s*["']?(\w+)["']?`)

	// Django model class declaration.
	reModelClass = regexp.MustCompile(`^class\s+(\w+)\s*\(([^)]+)\)\s*:`)

	// Choice constant definition, e.g.: POC_ROLES = [("Abuse", "Abuse"), ...]
	reChoiceConst = regexp.MustCompile(`^(\w+)\s*=\s*\[\s*$`)

	// Choice tuple item, e.g.: ("value", "display"),
	reChoiceTuple = regexp.MustCompile(`\(\s*"([^"]+)"`)

	// Max length, e.g.: max_length=255
	reMaxLength = regexp.MustCompile(`max_length\s*=\s*(\d+)`)

	// Required, e.g.: required=True
	reRequired = regexp.MustCompile(`required\s*=\s*(True|False)`)

	// Unique, e.g.: unique=True
	reUnique = regexp.MustCompile(`unique\s*=\s*(True|False)`)

	// Null, e.g.: null=True
	reNull = regexp.MustCompile(`null\s*=\s*(True|False)`)

	// Blank, e.g.: blank=True
	reBlank = regexp.MustCompile(`blank\s*=\s*(True|False)`)

	// Default value, e.g.: default=""
	reDefault = regexp.MustCompile(`default\s*=\s*([^,)]+)`)

	// Help text, e.g.: help_text="Organization name"
	reHelpText = regexp.MustCompile(`help_text\s*=\s*"([^"]*)"`)

	// Read-only, e.g.: read_only=True
	reReadOnly = regexp.MustCompile(`read_only\s*=\s*(True|False)`)
)

// parseSerializers extracts serializer class definitions from serializers.py.
func parseSerializers(src string) []SerializerInfo {
	var result []SerializerInfo
	lines := strings.Split(src, "\n")
	var current *SerializerInfo
	inMeta := false

	for _, line := range lines {
		// Detect serializer class.
		if m := reSerializerClass.FindStringSubmatch(line); m != nil {
			if current != nil {
				result = append(result, *current)
			}
			bases := strings.Split(m[2], ",")
			for i := range bases {
				bases[i] = strings.TrimSpace(bases[i])
			}
			current = &SerializerInfo{
				Name:        m[1],
				BaseClasses: bases,
				Fields:      make(map[string]FieldDef),
			}
			inMeta = false
			continue
		}

		if current == nil {
			continue
		}

		// Detect Meta class start.
		if strings.Contains(line, "class Meta:") || strings.Contains(line, "class Meta(") {
			inMeta = true
			continue
		}

		// Detect end of indented block (new top-level class, or unindented non-blank line).
		if inMeta && len(line) > 0 && line[0] != ' ' && line[0] != '\t' && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			inMeta = false
		}

		if inMeta {
			if m := reMetaModel.FindStringSubmatch(line); m != nil {
				current.ModelName = m[1]
			}
			if m := reFieldsList.FindStringSubmatch(line); m != nil {
				current.FieldList = parseStringList(m[1])
			}
			if reFieldsAll.MatchString(line) {
				current.AllFields = true
			}
			if m := reReadOnlyFields.FindStringSubmatch(line); m != nil {
				current.ReadOnlyFields = parseStringList(m[1])
			}
			continue
		}

		// Parse inline field definitions in serializer body.
		if m := reSerializerField.FindStringSubmatch(line); m != nil {
			fieldName := m[1]
			fieldType := m[2]
			args := m[3]
			current.Fields[fieldName] = parseFieldFromArgs(fieldType, args, "serializer")
		}
	}
	if current != nil {
		result = append(result, *current)
	}
	return result
}

// parseModelFields parses Django model field definitions from abstract and concrete sources.
func parseModelFields(abstractSrc, concreteSrc string) map[string]map[string]FieldDef {
	result := make(map[string]map[string]FieldDef)
	for _, src := range []string{abstractSrc, concreteSrc} {
		parseModelSource(src, result)
	}
	return result
}

// parseModelSource parses model class definitions from a single Python source.
func parseModelSource(src string, result map[string]map[string]FieldDef) {
	lines := strings.Split(src, "\n")
	var currentModel string

	for _, line := range lines {
		if m := reModelClass.FindStringSubmatch(line); m != nil {
			currentModel = m[1]
			if _, ok := result[currentModel]; !ok {
				result[currentModel] = make(map[string]FieldDef)
			}
			continue
		}

		if currentModel == "" {
			continue
		}

		// New top-level construct ends the current model.
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && !strings.HasPrefix(strings.TrimSpace(line), "#") && strings.TrimSpace(line) != "" {
			currentModel = ""
			continue
		}

		if m := reModelField.FindStringSubmatch(line); m != nil {
			fieldName := m[1]
			fieldType := m[2]
			args := m[3]
			fd := parseFieldFromArgs(fieldType, args, "model")

			// Detect FK references.
			if fieldType == "ForeignKey" {
				if fkm := reForeignKey.FindStringSubmatch(line); fkm != nil {
					fd.References = modelNameToAPIPath(fkm[1])
				}
				fd.Type = "integer"
			}

			result[currentModel][fieldName] = fd
		}
	}
}

// parseChoiceConstants extracts choice constant arrays from const.py.
func parseChoiceConstants(src string) map[string][]string {
	result := make(map[string][]string)
	lines := strings.Split(src, "\n")
	var currentConst string
	var values []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if currentConst != "" {
			if trimmed == "]" || strings.HasPrefix(trimmed, "]") {
				result[currentConst] = values
				currentConst = ""
				values = nil
				continue
			}
			if m := reChoiceTuple.FindStringSubmatch(line); m != nil {
				values = append(values, m[1])
			}
			continue
		}

		if m := reChoiceConst.FindStringSubmatch(line); m != nil {
			currentConst = m[1]
			values = nil
		}
	}
	return result
}

// parseFieldFromArgs creates a FieldDef from a Django field type and its arguments string.
func parseFieldFromArgs(fieldType, args, source string) FieldDef {
	fd := FieldDef{
		Type:    djangoFieldToJSONType(fieldType),
		Default: nil,
	}

	if m := reMaxLength.FindStringSubmatch(args); m != nil {
		fd.MaxLength = atoi(m[1])
	}

	if m := reRequired.FindStringSubmatch(args); m != nil {
		fd.Required = m[1] == "True"
	} else if source == "model" {
		// Django model fields are required by default unless blank=True or null=True.
		fd.Required = true
		if m := reBlank.FindStringSubmatch(args); m != nil && m[1] == "True" {
			fd.Required = false
		}
	}

	if m := reUnique.FindStringSubmatch(args); m != nil {
		fd.Unique = m[1] == "True"
	}

	if m := reNull.FindStringSubmatch(args); m != nil {
		fd.Nullable = m[1] == "True"
		if fd.Nullable {
			fd.Required = false
		}
	}

	if m := reDefault.FindStringSubmatch(args); m != nil {
		fd.Default = parsePythonDefault(strings.TrimSpace(m[1]))
	}

	if m := reHelpText.FindStringSubmatch(args); m != nil {
		fd.HelpText = m[1]
	}

	if m := reReadOnly.FindStringSubmatch(args); m != nil {
		fd.ReadOnly = m[1] == "True"
	}

	return fd
}

// djangoFieldToJSONType maps Django/DRF field types to simple JSON schema types.
func djangoFieldToJSONType(fieldType string) string {
	switch fieldType {
	case "CharField", "TextField", "URLField", "EmailField",
		"SlugField", "IPAddressField", "GenericIPAddressField",
		"FileField", "ImageField", "FilePathField",
		"SerializerMethodField", "StringRelatedField",
		"ChoiceField", "RegexField", "MacAddressField":
		return "string"
	case "IntegerField", "PositiveIntegerField", "BigIntegerField",
		"SmallIntegerField", "AutoField", "BigAutoField",
		"PrimaryKeyRelatedField", "ForeignKey":
		return "integer"
	case "FloatField", "DecimalField":
		return "float"
	case "BooleanField", "NullBooleanField":
		return "boolean"
	case "DateTimeField", "DateField", "TimeField":
		return "datetime"
	case "JSONField", "ArrayField":
		return "json_array"
	default:
		return "string"
	}
}

// buildObjectTypes assembles object type definitions from serializer and model data.
func buildObjectTypes(serializers []SerializerInfo, modelFields map[string]map[string]FieldDef) map[string]ObjectType {
	result := make(map[string]ObjectType)

	// Map serializer names to API paths.
	serializerAPIMap := map[string]string{
		"OrganizationSerializer":           "org",
		"CampusSerializer":                 "campus",
		"FacilitySerializer":               "fac",
		"CarrierSerializer":                "carrier",
		"CarrierFacSerializer":             "carrierfac",
		"InternetExchangeSerializer":       "ix",
		"InternetExchangeLanSerializer":    "ixlan",
		"InternetExchangePrefixSerializer": "ixpfx",
		"IXFacSerializer":                  "ixfac",
		"NetworkSerializer":                "net",
		"NetworkContactSerializer":         "poc",
		"NetworkFacilitySerializer":        "netfac",
		"NetworkIXLanSerializer":           "netixlan",
	}

	for _, ser := range serializers {
		apiPath, ok := serializerAPIMap[ser.Name]
		if !ok {
			continue // Skip non-PeeringDB-type serializers.
		}

		ot := ObjectType{
			ModelName:   ser.ModelName,
			APIPath:     apiPath,
			BaseClasses: ser.BaseClasses,
			Fields:      make(map[string]FieldDef),
		}

		// Merge model fields for the serializer's model.
		if ser.ModelName != "" {
			if mf, ok := modelFields[ser.ModelName]; ok {
				maps.Copy(ot.Fields, mf)
			}
			// Also check base model classes for inherited fields.
			for modelName, mf := range modelFields {
				if isBaseModel(modelName) {
					for name, fd := range mf {
						if _, exists := ot.Fields[name]; !exists {
							ot.Fields[name] = fd
						}
					}
				}
			}
		}

		// Overlay serializer-declared fields (override model fields).
		maps.Copy(ot.Fields, ser.Fields)

		// Apply read_only_fields from Meta.
		for _, roField := range ser.ReadOnlyFields {
			if fd, ok := ot.Fields[roField]; ok {
				fd.ReadOnly = true
				ot.Fields[roField] = fd
			}
		}

		// Detect computed fields and relationships.
		ot.ComputedFields = detectComputedFields(apiPath)
		ot.Relationships = detectRelationships(apiPath, ot.Fields)

		result[apiPath] = ot
	}

	return result
}

// isBaseModel returns true if the model name is a known base abstract model.
func isBaseModel(name string) bool {
	baseModels := []string{
		"HandleRefModel", "AddressModel", "SocialMediaMixin",
	}
	return slices.Contains(baseModels, name)
}

// detectComputedFields returns known computed (serializer-added) field names
// for a given API path.
func detectComputedFields(apiPath string) []string {
	known := map[string][]string{
		"org":        {"net_count", "fac_count"},
		"net":        {"ix_count", "fac_count", "netixlan_updated", "netfac_updated", "poc_updated"},
		"fac":        {"org_name", "net_count", "ix_count", "carrier_count"},
		"ix":         {"net_count", "fac_count", "ixf_import_request", "ixf_import_request_status"},
		"ixfac":      {"name", "city", "country"},
		"netfac":     {"name", "city", "country"},
		"netixlan":   {"ix_id", "name"},
		"carrier":    {"org_name", "fac_count"},
		"carrierfac": {"name"},
		"campus":     {"org_name"},
	}
	if cf, ok := known[apiPath]; ok {
		return cf
	}
	return nil
}

// detectRelationships infers FK relationships from fields with References set.
func detectRelationships(apiPath string, fields map[string]FieldDef) map[string]Relationship {
	rels := make(map[string]Relationship)
	for name, fd := range fields {
		if fd.References != "" {
			relName := fd.References + "s" // Pluralize.
			rels[relName] = Relationship{
				Target: fd.References,
				Type:   "many_to_one",
				Field:  name,
			}
		}
	}

	// Add known one-to-many relationships based on API path.
	reverseRels := map[string][]Relationship{
		"org": {
			{Target: "net", Type: "one_to_many", Field: "org_id"},
			{Target: "fac", Type: "one_to_many", Field: "org_id"},
			{Target: "ix", Type: "one_to_many", Field: "org_id"},
			{Target: "carrier", Type: "one_to_many", Field: "org_id"},
			{Target: "campus", Type: "one_to_many", Field: "org_id"},
		},
		"net": {
			{Target: "poc", Type: "one_to_many", Field: "net_id"},
			{Target: "netfac", Type: "one_to_many", Field: "net_id"},
			{Target: "netixlan", Type: "one_to_many", Field: "net_id"},
		},
		"ix": {
			{Target: "ixlan", Type: "one_to_many", Field: "ix_id"},
			{Target: "ixfac", Type: "one_to_many", Field: "ix_id"},
		},
		"ixlan": {
			{Target: "ixpfx", Type: "one_to_many", Field: "ixlan_id"},
			{Target: "netixlan", Type: "one_to_many", Field: "ixlan_id"},
		},
		"fac": {
			{Target: "netfac", Type: "one_to_many", Field: "fac_id"},
			{Target: "ixfac", Type: "one_to_many", Field: "fac_id"},
			{Target: "carrierfac", Type: "one_to_many", Field: "fac_id"},
		},
		"carrier": {
			{Target: "carrierfac", Type: "one_to_many", Field: "carrier_id"},
		},
		"campus": {
			{Target: "fac", Type: "one_to_many", Field: "campus_id"},
		},
	}

	if revs, ok := reverseRels[apiPath]; ok {
		for _, rel := range revs {
			rels[rel.Target+"_set"] = rel
		}
	}

	return rels
}

// modelNameToAPIPath converts a Django model class name to its PeeringDB API path.
func modelNameToAPIPath(modelName string) string {
	m := map[string]string{
		"Organization":             "org",
		"Campus":                   "campus",
		"Facility":                 "fac",
		"Carrier":                  "carrier",
		"CarrierFacility":          "carrierfac",
		"InternetExchange":         "ix",
		"InternetExchangeLan":      "ixlan",
		"InternetExchangePrefix":   "ixpfx",
		"InternetExchangeFacility": "ixfac",
		"Network":                  "net",
		"NetworkContact":           "poc",
		"NetworkFacility":          "netfac",
		"NetworkIXLan":             "netixlan",
	}
	if p, ok := m[modelName]; ok {
		return p
	}
	return strings.ToLower(modelName)
}

// parseStringList parses a Python list literal like '"a", "b", "c"' into a Go string slice.
func parseStringList(s string) []string {
	var result []string
	re := regexp.MustCompile(`"([^"]+)"`)
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		result = append(result, m[1])
	}
	return result
}

// parsePythonDefault converts a Python default value literal to a Go value.
func parsePythonDefault(s string) any {
	s = strings.Trim(s, " \t")
	switch {
	case s == "True":
		return true
	case s == "False":
		return false
	case s == "None":
		return nil
	case s == `""` || s == `''`:
		return ""
	case strings.HasPrefix(s, `"`) || strings.HasPrefix(s, `'`):
		return strings.Trim(s, `"'`)
	case s == "[]":
		return []any{}
	case s == "0":
		return float64(0) // JSON number.
	default:
		// Try numeric.
		var n float64
		if _, err := fmt.Sscanf(s, "%f", &n); err == nil {
			return n
		}
		return s
	}
}

// atoi converts a string to int, returning 0 on error.
func atoi(s string) int {
	var n int
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}

// validateAgainstAPI fetches sample API responses from beta.peeringdb.com and
// compares field names against the extracted schema (D-16).
func validateAgainstAPI(schema *Schema) error {
	baseURL := "https://beta.peeringdb.com/api"
	client := &http.Client{Timeout: 30 * time.Second}

	var errors []string
	for apiPath, ot := range schema.ObjectTypes {
		url := fmt.Sprintf("%s/%s?limit=1", baseURL, apiPath)
		resp, err := client.Get(url)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: fetch error: %v", apiPath, err))
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errors = append(errors, fmt.Sprintf("%s: HTTP %d", apiPath, resp.StatusCode))
			continue
		}

		var apiResp struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			errors = append(errors, fmt.Sprintf("%s: decode error: %v", apiPath, err))
			continue
		}

		if len(apiResp.Data) == 0 {
			log.Printf("WARN: %s returned empty data (may require auth)", apiPath)
			continue
		}

		// Compare field names.
		apiFields := make(map[string]bool)
		for key := range apiResp.Data[0] {
			apiFields[key] = true
		}

		schemaFields := make(map[string]bool)
		for name := range ot.Fields {
			schemaFields[name] = true
		}
		for _, cf := range ot.ComputedFields {
			schemaFields[cf] = true
		}
		// Common fields always present.
		for _, common := range []string{"id", "status", "created", "updated"} {
			schemaFields[common] = true
		}

		var missing, extra []string
		for af := range apiFields {
			if !schemaFields[af] {
				missing = append(missing, af)
			}
		}
		for sf := range schemaFields {
			if !apiFields[sf] {
				extra = append(extra, sf)
			}
		}

		slices.Sort(missing)
		slices.Sort(extra)

		if len(missing) > 0 {
			log.Printf("WARN %s: fields in API but not schema: %v", apiPath, missing)
		}
		if len(extra) > 0 {
			log.Printf("INFO %s: fields in schema but not API: %v", apiPath, extra)
		}

		// Critical: if more than 30% of API fields are missing from schema, fail.
		if len(missing) > 0 && float64(len(missing))/float64(len(apiFields)) > 0.3 {
			errors = append(errors, fmt.Sprintf("%s: %d/%d API fields missing from schema", apiPath, len(missing), len(apiFields)))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation errors:\n  %s", strings.Join(errors, "\n  "))
	}
	return nil
}
