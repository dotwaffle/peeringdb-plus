// pdb-schema-extract parses PeeringDB's Django serializer and model Python source
// into an intermediate JSON schema describing all 13 PeeringDB object types: the
// scalar wire fields (derived from DRF serializer introspection), their FK
// references, and computed-field names.
//
// Role: this is a DRIFT DETECTOR, not the schema source of truth. The committed
// schema/peeringdb.json is hand-curated (help_text becomes ent .Comment(), the
// name uniqueness is deliberately relaxed, info_types is curated to a list,
// etc.), and pdb-schema-generate consumes that curation. Use this tool to diff a
// fresh upstream checkout against the committed schema and surface genuine
// field-level drift (adds/removes/type/ref/required/nullable changes); apply any
// real drift to the curated schema by hand. Do NOT overwrite schema/peeringdb.json
// with this tool's raw output — it would regress the curation.
//
// Known limitations (acceptable for drift detection): custom DRF field classes
// that are not serializers.<X> constructors (e.g. LegacyInfoTypeField) are not
// recognised, and some abstract-base inheritance for org/fac is incompletely
// resolved, so those types under-report fields. Cross-check unexpected removals
// against upstream source before treating them as real.
//
// Usage:
//
//	pdb-schema-extract <peeringdb-repo-path> [--validate]
//
// The repo path should point at the src/ root of a local peeringdb/peeringdb
// checkout. The tool expects to find:
//
//   - {repo}/peeringdb_server/serializers.py
//   - {repo}/peeringdb_server/models.py
//   - {repo}/../django-peeringdb/src/django_peeringdb/models/abstract.py
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

	"github.com/dotwaffle/peeringdb-plus/internal/buildinfo"
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
		if err := validateAgainstAPI(schema, validateAPIInput{
			BaseURL: "https://beta.peeringdb.com/api",
			Client:  &http.Client{Timeout: 30 * time.Second},
			Pause:   3 * time.Second,
		}); err != nil {
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

	// The scalar fields live on django-peeringdb's abstract *Base classes.
	abstractSrc, err := readSourceFile(repoPath, "../django-peeringdb/src/django_peeringdb/models/abstract.py")
	if err != nil {
		return nil, fmt.Errorf("read abstract.py: %w", err)
	}

	// The concrete models the API actually serves live in peeringdb-server:
	// each subclasses the matching django-peeringdb *Base and adds the foreign
	// keys, server-specific fields (e.g. allow_ixp_update, ixp_update_exclude)
	// and field-bearing mixins (LogoMixin, SocialMediaMixin). django-peeringdb's
	// own concrete.py is not used by the server and is intentionally skipped.
	serverModelsSrc, err := readSourceFile(repoPath, "peeringdb_server/models.py")
	if err != nil {
		return nil, fmt.Errorf("read models.py: %w", err)
	}

	constSrc, err := readSourceFile(repoPath, "../django-peeringdb/src/django_peeringdb/const.py")
	if err != nil {
		return nil, fmt.Errorf("read const.py: %w", err)
	}

	// Parse source files.
	serializers := parseSerializers(serializersSrc)
	modelFields := parseModelFields(abstractSrc, serverModelsSrc)
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
	SerFields      map[string]serField // explicitly-declared serializer fields
	MethodReturns  map[string]string   // get_<name> -> Python return annotation
	ReadOnlyFields []string
	RelatedFields  []string // Meta.related_fields (nested objects + reverse sets)
	AllFields      bool     // True when fields = "__all__"
	FieldList      []string // Meta.fields, the authoritative output field order
}

// serField is a field declared explicitly in a serializer body (as opposed to
// one DRF derives from the model). It records just enough to resolve the field
// onto the wire schema: its DRF type, any source= remap, the related model of a
// PrimaryKeyRelatedField, and the read_only/write_only/allow_null flags.
type serField struct {
	drfType   string
	source    string
	refModel  string
	readOnly  bool
	writeOnly bool
	allowNull bool
	maxLength int
	hasDef    bool
	def       any
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

	// Related fields (nested objects + reverse sets), e.g.:
	//   related_fields = ["org", "netfac_set", ...]
	reRelatedFields = regexp.MustCompile(`^\s+related_fields\s*=\s*\[([^\]]*)\]`)

	// SerializerMethodField getter with a return annotation, e.g.:
	//   def get_proto_ipv6(self, inst) -> bool:
	reMethodDef = regexp.MustCompile(`^\s+def\s+get_(\w+)\s*\(.*\)\s*->\s*([\w.]+)`)

	// source= remap on a serializer field, e.g.: source="network"
	reSource = regexp.MustCompile(`source\s*=\s*["']([\w.]+)["']`)

	// queryset target of a PrimaryKeyRelatedField, e.g.: queryset=Network.objects.all()
	reQuerySet = regexp.MustCompile(`queryset\s*=\s*(\w+)\.objects`)

	// write_only / allow_null flags.
	reWriteOnly = regexp.MustCompile(`write_only\s*=\s*True`)
	reAllowNull = regexp.MustCompile(`allow_null\s*=\s*True`)

	// Serializer field definition. Tolerates an optional PEP 526 type
	// annotation and logical-line-joined (multi-line) arguments, e.g.:
	//   name = serializers.CharField(max_length=255, required=True)
	//   asn: serializers.IntegerField = serializers.IntegerField(read_only=True)
	reSerializerField = regexp.MustCompile(`^\s+(\w+)\s*(?::\s*[^=]+?)?\s*=\s*serializers\.(\w+)\((.*)\)\s*$`)

	// Django model field definition. Tolerates an optional PEP 526 type
	// annotation, a bare custom field-type constructor (e.g. ASNField,
	// URLField, CountryField) as well as a dotted models.X constructor, and
	// logical-line-joined arguments, e.g.:
	//   name: models.CharField = models.CharField(_("Name"), max_length=255)
	//   asn: ASNField = ASNField(verbose_name="ASN", unique=True)
	reModelField = regexp.MustCompile(`^\s+(\w+)\s*(?::\s*[^=]+?)?\s*=\s*([A-Za-z_][\w.]*)\((.*)\)\s*$`)

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

// logicalLines joins physically-wrapped Python statements into single logical
// lines by tracking code-context bracket depth, so the downstream single-line
// regexes match multi-line class declarations, field definitions and list
// literals alike. The leading indentation of the first physical line is
// preserved (the field regexes anchor on it); continuation lines are trimmed
// and appended with a single separating space. Whole-line comments outside any
// open bracket are dropped.
//
// Bracket counting ignores brackets inside string literals (including
// multi-line triple-quoted docstrings) and inline comments, so an unbalanced
// bracket in prose does not run statements together.
func logicalLines(src string) []string {
	var out []string
	var buf strings.Builder
	var sc pyScanner
	depth := 0
	for raw := range strings.SplitSeq(src, "\n") {
		if depth == 0 && !sc.inString() {
			if strings.HasPrefix(strings.TrimSpace(raw), "#") {
				continue
			}
			buf.Reset()
			buf.WriteString(strings.TrimRight(raw, " \t"))
		} else {
			buf.WriteString(" ")
			buf.WriteString(strings.TrimSpace(raw))
		}
		depth += sc.delta(raw)
		if depth <= 0 && !sc.inString() {
			depth = 0
			out = append(out, buf.String())
		}
	}
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out
}

// pyScanner tracks Python lexical state across physical lines so that bracket
// counting can ignore brackets appearing inside string literals or comments.
// It understands single- and double-quoted strings (with backslash escapes)
// and triple-quoted strings that span multiple lines.
type pyScanner struct {
	triple string // "\"\"\"" or "'''" while inside a triple-quoted string, else ""
}

// inString reports whether the scanner is currently inside a multi-line
// triple-quoted string.
func (s *pyScanner) inString() bool { return s.triple != "" }

// delta scans one physical line and returns the net change in code-context
// bracket depth (opening minus closing round/square/curly brackets), updating
// the scanner's multi-line string state.
func (s *pyScanner) delta(line string) int {
	d := 0
	for i := 0; i < len(line); {
		if s.triple != "" {
			if strings.HasPrefix(line[i:], s.triple) {
				s.triple = ""
				i += 3
				continue
			}
			i++
			continue
		}
		switch {
		case line[i] == '#':
			return d // remainder of the line is a comment
		case strings.HasPrefix(line[i:], `"""`):
			s.triple = `"""`
			i += 3
		case strings.HasPrefix(line[i:], `'''`):
			s.triple = `'''`
			i += 3
		case line[i] == '"' || line[i] == '\'':
			q := line[i]
			i++
			for i < len(line) {
				if line[i] == '\\' {
					i += 2
					continue
				}
				if line[i] == q {
					i++
					break
				}
				i++
			}
		case line[i] == '(' || line[i] == '[' || line[i] == '{':
			d++
			i++
		case line[i] == ')' || line[i] == ']' || line[i] == '}':
			d--
			i++
		default:
			i++
		}
	}
	return d
}

// parseSerializers extracts serializer class definitions from serializers.py.
func parseSerializers(src string) []SerializerInfo {
	var result []SerializerInfo
	lines := logicalLines(src)
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
				Name:          m[1],
				BaseClasses:   bases,
				SerFields:     make(map[string]serField),
				MethodReturns: make(map[string]string),
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
			if m := reRelatedFields.FindStringSubmatch(line); m != nil {
				current.RelatedFields = parseStringList(m[1])
			}
			continue
		}

		// A SerializerMethodField getter records its declared return type so the
		// derived field can be typed (e.g. get_proto_ipv6(...) -> bool).
		if m := reMethodDef.FindStringSubmatch(line); m != nil {
			current.MethodReturns[m[1]] = lastSegment(m[2])
			continue
		}

		// Parse an explicitly-declared serializer field, e.g.:
		//   net_id = serializers.PrimaryKeyRelatedField(queryset=Network.objects..., source="network")
		if m := reSerializerField.FindStringSubmatch(line); m != nil {
			current.SerFields[m[1]] = parseSerField(m[2], m[3])
		}
	}
	if current != nil {
		result = append(result, *current)
	}
	return result
}

// parseSerField parses the constructor arguments of an explicitly-declared
// serializer field into a serField.
func parseSerField(drfType, args string) serField {
	sf := serField{drfType: drfType}
	if m := reSource.FindStringSubmatch(args); m != nil {
		sf.source = m[1]
	}
	if m := reQuerySet.FindStringSubmatch(args); m != nil {
		sf.refModel = m[1]
	}
	if m := reMaxLength.FindStringSubmatch(args); m != nil {
		sf.maxLength = atoi(m[1])
	}
	sf.writeOnly = reWriteOnly.MatchString(args)
	sf.allowNull = reAllowNull.MatchString(args)
	if m := reReadOnly.FindStringSubmatch(args); m != nil {
		sf.readOnly = m[1] == "True"
	}
	if m := reDefault.FindStringSubmatch(args); m != nil {
		sf.hasDef = true
		sf.def = parsePythonDefault(strings.TrimSpace(m[1]))
	}
	return sf
}

// modelDefs holds parsed Django model data: the field set of each model class
// keyed by class name, plus each model's declared base classes. The bases let
// buildObjectTypes resolve django-peeringdb's concrete -> abstract-base ->
// shared-base inheritance chain (e.g. Network -> NetworkBase -> HandleRefModel)
// when assembling a serializer's effective field set.
type modelDefs struct {
	fields map[string]map[string]FieldDef
	bases  map[string][]string
}

// parseModelFields parses Django model field definitions from the abstract base
// classes and the concrete server models, merging both into one set keyed by
// class name. Abstract bases are parsed first so a concrete server model of the
// same name (django-peeringdb and peeringdb-server both define e.g. "Network")
// contributes its foreign keys and server-specific fields on top.
func parseModelFields(abstractSrc, serverSrc string) modelDefs {
	defs := modelDefs{
		fields: make(map[string]map[string]FieldDef),
		bases:  make(map[string][]string),
	}
	for _, src := range []string{abstractSrc, serverSrc} {
		parseModelSource(src, &defs)
	}
	return defs
}

// parseModelSource parses model class definitions from a single Python source.
func parseModelSource(src string, defs *modelDefs) {
	var currentModel string

	for _, line := range logicalLines(src) {
		if m := reModelClass.FindStringSubmatch(line); m != nil {
			currentModel = m[1]
			if _, ok := defs.fields[currentModel]; !ok {
				defs.fields[currentModel] = make(map[string]FieldDef)
			}
			bases := strings.Split(m[2], ",")
			for i := range bases {
				bases[i] = lastSegment(strings.TrimSpace(bases[i]))
			}
			defs.bases[currentModel] = bases
			continue
		}

		if currentModel == "" {
			continue
		}

		// A new unindented construct ends the current model body.
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && strings.TrimSpace(line) != "" {
			currentModel = ""
			continue
		}

		m := reModelField.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		fieldName := m[1]
		ctor := lastSegment(m[2])
		if !isFieldConstructor(ctor) {
			continue
		}
		fd := parseFieldFromArgs(ctor, m[3], "model")

		// Foreign keys surface on the API as "<name>_id" integers that carry a
		// reference to the target type's API path (the model field `org`
		// becomes the schema field `org_id` referencing `org`).
		if ctor == "ForeignKey" || ctor == "OneToOneField" {
			if fkm := reForeignKey.FindStringSubmatch(line); fkm != nil {
				fd.References = modelNameToAPIPath(fkm[1])
			}
			fd.Type = "integer"
			fieldName += "_id"
		}

		defs.fields[currentModel][fieldName] = fd
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

// djangoFieldToJSONType maps a Django/DRF field type (the bare constructor name,
// e.g. "CharField" from "models.CharField", or a custom field class such as
// "ASNField") to a simple JSON schema type. Custom field classes used by
// django-peeringdb resolve through their documented base type: ASNField is an
// integer, the django_inet address/prefix/mac fields and the URL/country
// wrappers are strings.
func djangoFieldToJSONType(fieldType string) string {
	switch fieldType {
	case "CharField", "TextField", "URLField", "LG_URLField", "EmailField",
		"SlugField", "IPAddressField", "IPPrefixField", "GenericIPAddressField",
		"FileField", "ImageField", "FilePathField",
		"SerializerMethodField", "StringRelatedField",
		"ChoiceField", "MultipleChoiceField", "RegexField",
		"MacAddressField", "CountryField":
		return "string"
	case "IntegerField", "PositiveIntegerField", "BigIntegerField",
		"SmallIntegerField", "AutoField", "BigAutoField",
		"PrimaryKeyRelatedField", "ForeignKey", "ASNField":
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

// isFieldConstructor reports whether a bare constructor name denotes a Django
// model field (as opposed to a manager, manual property, or other class-body
// assignment). Only recognised field constructors are admitted so the model
// parser does not mistake e.g. `objects = Manager()` for a schema field.
func isFieldConstructor(name string) bool {
	switch name {
	case "CharField", "TextField", "URLField", "LG_URLField", "EmailField",
		"SlugField", "IPAddressField", "IPPrefixField", "GenericIPAddressField",
		"FileField", "ImageField", "FilePathField", "ChoiceField",
		"MultipleChoiceField", "RegexField", "MacAddressField", "CountryField",
		"IntegerField", "PositiveIntegerField", "BigIntegerField",
		"SmallIntegerField", "AutoField", "BigAutoField", "ASNField",
		"FloatField", "DecimalField", "BooleanField", "NullBooleanField",
		"DateTimeField", "DateField", "TimeField", "JSONField", "ArrayField",
		"ForeignKey", "OneToOneField":
		return true
	default:
		return false
	}
}

// lastSegment returns the final dotted component of a callable reference, so
// "models.CharField" becomes "CharField" and a bare "ASNField" is unchanged.
func lastSegment(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

// serializerAPIMap maps a DRF serializer class name to its PeeringDB API path.
// django-peeringdb 3.x renamed several serializers (e.g. the IX-LAN/prefix and
// the *Fac serializers); the keys track the current upstream class names.
var serializerAPIMap = map[string]string{
	"OrganizationSerializer":             "org",
	"CampusSerializer":                   "campus",
	"FacilitySerializer":                 "fac",
	"CarrierSerializer":                  "carrier",
	"CarrierFacilitySerializer":          "carrierfac",
	"InternetExchangeSerializer":         "ix",
	"IXLanSerializer":                    "ixlan",
	"IXLanPrefixSerializer":              "ixpfx",
	"InternetExchangeFacilitySerializer": "ixfac",
	"NetworkSerializer":                  "net",
	"NetworkContactSerializer":           "poc",
	"NetworkFacilitySerializer":          "netfac",
	"NetworkIXLanSerializer":             "netixlan",
}

// buildObjectTypes assembles object type definitions from serializer and model
// data using DRF serializer introspection. The authoritative output field set
// is Meta.fields, from which the scalar wire fields are derived by removing:
//   - ent-codegen-injected columns (id/status/created/updated),
//   - reverse-relation sets (<x>_set) and Meta.related_fields nested objects,
//   - SerializerMethodField getters (computed, not DB columns),
//   - write-only serializer fields.
//
// Each surviving field is resolved to a FieldDef: an explicitly-declared
// serializer field (PrimaryKeyRelatedField FK, typed method/char field) takes
// its shape from the serializer declaration plus the underlying model field it
// sources from; otherwise the model field of the same name supplies it.
func buildObjectTypes(serializers []SerializerInfo, models modelDefs) map[string]ObjectType {
	result := make(map[string]ObjectType)

	for _, ser := range serializers {
		apiPath, ok := serializerAPIMap[ser.Name]
		if !ok {
			continue // Skip non-PeeringDB-type serializers.
		}

		modelFields := resolveModelFields(models, ser.ModelName, make(map[string]bool))
		related := sliceToSet(ser.RelatedFields)
		readOnly := sliceToSet(ser.ReadOnlyFields)

		ot := ObjectType{
			ModelName:   ser.ModelName,
			APIPath:     apiPath,
			BaseClasses: primaryBaseClasses(models, ser.ModelName),
			Fields:      make(map[string]FieldDef),
		}

		for _, name := range ser.FieldList {
			switch {
			case name == "id" || name == "status" || name == "created" || name == "updated":
				continue // Injected by ent codegen, not schema fields.
			case related[name]:
				// Reverse <x>_set relations and nested related objects are all
				// listed explicitly in Meta.related_fields. A bare _set-suffix
				// test would also drop the genuine scalar field irr_as_set.
				continue
			}

			// A SerializerMethodField getter (get_<name>) yields a computed
			// field, not a DB column.
			if _, isMethod := ser.MethodReturns[name]; isMethod {
				ot.ComputedFields = append(ot.ComputedFields, name)
				continue
			}

			if sf, ok := ser.SerFields[name]; ok {
				switch {
				case sf.writeOnly:
					continue // Write-only fields are not serialized.
				case sf.drfType == "SerializerMethodField":
					// A method field whose getter lacks a return annotation (so
					// it never reached MethodReturns) is still computed, not a
					// DB column.
					ot.ComputedFields = append(ot.ComputedFields, name)
					continue
				}
				ot.Fields[name] = serFieldToDef(name, sf, modelFields)
				continue
			}

			if fd, ok := modelFields[name]; ok {
				if readOnly[name] {
					fd.ReadOnly = true
				}
				ot.Fields[name] = fd
				continue
			}
			// Field listed in Meta.fields but neither declared on the serializer
			// nor found on the model: skip silently — it is typically a method
			// field whose getter lacks a return annotation, or an inherited
			// helper not relevant to the wire schema.
		}

		ot.Relationships = detectRelationships(apiPath, ot.Fields)
		result[apiPath] = ot
	}

	return result
}

// serFieldToDef resolves an explicitly-declared serializer field to a FieldDef.
// PrimaryKeyRelatedField becomes an integer FK carrying the reference and the
// required/nullable shape of the underlying model foreign key (looked up via the
// source= remap, falling back to the field name minus the _id suffix).
// SerializerMethodField is typed from its getter's return annotation. Any other
// declared field maps its DRF type and overlays the model field it sources from.
func serFieldToDef(name string, sf serField, modelFields map[string]FieldDef) FieldDef {
	switch sf.drfType {
	case "PrimaryKeyRelatedField":
		fd := FieldDef{Type: "integer"}
		src := sf.source
		if src == "" {
			src = strings.TrimSuffix(name, "_id")
		}
		if mf, ok := modelFields[src+"_id"]; ok {
			fd.References, fd.Required, fd.Nullable = mf.References, mf.Required, mf.Nullable
		} else if mf, ok := modelFields[src]; ok {
			fd.References, fd.Required, fd.Nullable = mf.References, mf.Required, mf.Nullable
		}
		if sf.refModel != "" {
			fd.References = modelNameToAPIPath(sf.refModel)
		}
		if sf.allowNull {
			fd.Nullable, fd.Required = true, false
		}
		return fd
	default:
		fd := FieldDef{Type: djangoFieldToJSONType(sf.drfType)}
		fd.MaxLength = sf.maxLength
		fd.ReadOnly = sf.readOnly
		if sf.hasDef {
			fd.Default = sf.def
		}
		// Overlay the underlying model field for required/nullable/unique and to
		// recover attributes the serializer declaration omits.
		src := sf.source
		if src == "" {
			src = name
		}
		if mf, ok := modelFields[src]; ok {
			fd.Required, fd.Nullable, fd.Unique = mf.Required, mf.Nullable, mf.Unique
			if fd.MaxLength == 0 {
				fd.MaxLength = mf.MaxLength
			}
			if !sf.hasDef {
				fd.Default = mf.Default
			}
			if sf.drfType == "" || fd.Type == "string" {
				fd.Type = mf.Type
			}
		}
		if sf.allowNull {
			fd.Nullable, fd.Required = true, false
		}
		return fd
	}
}

// sliceToSet builds a set from a string slice for O(1) membership tests.
func sliceToSet(xs []string) map[string]bool {
	if len(xs) == 0 {
		return nil
	}
	s := make(map[string]bool, len(xs))
	for _, x := range xs {
		s[x] = true
	}
	return s
}

// resolveModelFields returns the merged field set for a model class, walking its
// base classes depth-first so abstract bases (which hold the scalar fields) and
// shared mixins such as AddressModel contribute. A field declared on a more-
// derived class overrides an inherited definition of the same name. External
// bases not present in the parsed sources (e.g. HandleRefModel, models.Model)
// contribute nothing and terminate the walk for that branch.
func resolveModelFields(models modelDefs, model string, seen map[string]bool) map[string]FieldDef {
	if model == "" || seen[model] {
		return nil
	}
	seen[model] = true

	merged := make(map[string]FieldDef)
	for _, base := range models.bases[model] {
		maps.Copy(merged, resolveModelFields(models, base, seen))
	}
	maps.Copy(merged, models.fields[model])
	return merged
}

// primaryBaseClasses returns the base classes that define a concrete model's
// shape for the schema's base_classes metadata: the bases of the model's first
// parsed abstract base (e.g. NetworkBase for Network), falling back to the
// model's own declared bases when no parsed abstract base is found.
func primaryBaseClasses(models modelDefs, model string) []string {
	for _, base := range models.bases[model] {
		if len(models.fields[base]) > 0 {
			return models.bases[base]
		}
	}
	return models.bases[model]
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
		"IXLan":                    "ixlan",
		"IXLanPrefix":              "ixpfx",
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
	case s == "[]" || s == "list":
		// "list" is the callable form `default=list` (an empty list factory).
		return []any{}
	case s == "{}" || s == "dict":
		// "dict" is the callable form `default=dict` (an empty dict factory).
		return map[string]any{}
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

// validateAPIInput parameterises validateAgainstAPI so tests can point
// it at an httptest server with zero pacing instead of live
// beta.peeringdb.com with the 3s courtesy interval.
type validateAPIInput struct {
	BaseURL string        // e.g. "https://beta.peeringdb.com/api"
	Client  *http.Client  // owns the timeout
	Pause   time.Duration // sleep between object types (0 in tests)
}

// validateAgainstAPI fetches one sample row per object type from the
// configured PeeringDB API and compares field names against the
// extracted schema. Types are visited in sorted order with in.Pause
// between requests — this is a manually-run drift detector against a
// rate-limited third-party service, so it identifies itself with a
// buildinfo User-Agent and paces itself instead of bursting 13 requests.
func validateAgainstAPI(schema *Schema, in validateAPIInput) error {
	apiPaths := slices.Sorted(maps.Keys(schema.ObjectTypes))

	var errors []string
	for i, apiPath := range apiPaths {
		ot := schema.ObjectTypes[apiPath]
		if i > 0 && in.Pause > 0 {
			time.Sleep(in.Pause)
		}
		apiResp, fetchErr := fetchSampleRow(in.Client, fmt.Sprintf("%s/%s?limit=1", in.BaseURL, apiPath))
		if fetchErr != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", apiPath, fetchErr))
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

// sampleResponse is the slice of a PeeringDB list envelope the
// validator needs.
type sampleResponse struct {
	Data []map[string]any `json:"data"`
}

// fetchSampleRow GETs one list URL and decodes the envelope, closing
// the response body before returning (the previous in-loop `defer`
// held all 13 bodies open until the whole validation pass finished).
func fetchSampleRow(client *http.Client, url string) (*sampleResponse, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "pdb-schema-extract/"+buildinfo.Version()+" (+https://github.com/dotwaffle/peeringdb-plus)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var apiResp sampleResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}
	return &apiResp, nil
}
