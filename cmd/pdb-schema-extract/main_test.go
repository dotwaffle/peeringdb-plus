package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// setupTestRepo creates a temporary directory structure that mimics the
// PeeringDB repo layout expected by extractSchema. It copies the testdata
// fixtures into the right locations.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	base := t.TempDir()

	// Create directory structure:
	//   base/peeringdb/peeringdb_server/serializers.py
	//   base/django-peeringdb/src/django_peeringdb/models/abstract.py
	//   base/django-peeringdb/src/django_peeringdb/models/concrete.py
	//   base/django-peeringdb/src/django_peeringdb/const.py
	repoDir := filepath.Join(base, "peeringdb")
	dirs := []string{
		filepath.Join(repoDir, "peeringdb_server"),
		filepath.Join(base, "django-peeringdb", "src", "django_peeringdb", "models"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	// Copy test fixtures.
	fixtures := map[string]string{
		"testdata/serializers.py": filepath.Join(repoDir, "peeringdb_server", "serializers.py"),
		"testdata/abstract.py":   filepath.Join(base, "django-peeringdb", "src", "django_peeringdb", "models", "abstract.py"),
		"testdata/concrete.py":   filepath.Join(base, "django-peeringdb", "src", "django_peeringdb", "models", "concrete.py"),
		"testdata/const.py":      filepath.Join(base, "django-peeringdb", "src", "django_peeringdb", "const.py"),
	}
	for src, dst := range fixtures {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read fixture %s: %v", src, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", dst, err)
		}
	}

	return repoDir
}

func TestExtractSchema(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	schema, err := extractSchema(repoDir)
	if err != nil {
		t.Fatalf("extractSchema: %v", err)
	}

	if schema.Version != "1.0" {
		t.Errorf("version = %q, want %q", schema.Version, "1.0")
	}

	if schema.SourcePath != repoDir {
		t.Errorf("source_path = %q, want %q", schema.SourcePath, repoDir)
	}

	// We should find at least org, net, and fac.
	for _, apiPath := range []string{"org", "net"} {
		if _, ok := schema.ObjectTypes[apiPath]; !ok {
			t.Errorf("missing object type %q", apiPath)
		}
	}
}

func TestExtractSchemaOutputJSON(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	schema, err := extractSchema(repoDir)
	if err != nil {
		t.Fatalf("extractSchema: %v", err)
	}

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Validate we can round-trip the JSON.
	var roundTrip Schema
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}

	if roundTrip.Version != schema.Version {
		t.Errorf("round-trip version mismatch: %q != %q", roundTrip.Version, schema.Version)
	}
}

func TestParseSerializers(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("testdata/serializers.py")
	if err != nil {
		t.Fatalf("read testdata/serializers.py: %v", err)
	}

	serializers := parseSerializers(string(src))

	tests := []struct {
		name      string
		wantModel string
		wantRO    []string
	}{
		{
			name:      "OrganizationSerializer",
			wantModel: "Organization",
			wantRO:    []string{"id", "created", "updated"},
		},
		{
			name:      "NetworkSerializer",
			wantModel: "Network",
			wantRO:    []string{"id", "created", "updated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var found *SerializerInfo
			for i := range serializers {
				if serializers[i].Name == tt.name {
					found = &serializers[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("serializer %q not found", tt.name)
			}
			if found.ModelName != tt.wantModel {
				t.Errorf("model = %q, want %q", found.ModelName, tt.wantModel)
			}
			for _, ro := range tt.wantRO {
				if !slices.Contains(found.ReadOnlyFields, ro) {
					t.Errorf("expected read_only_field %q, not found in %v", ro, found.ReadOnlyFields)
				}
			}
		})
	}
}

func TestParseModelFields(t *testing.T) {
	t.Parallel()
	abstractSrc, err := os.ReadFile("testdata/abstract.py")
	if err != nil {
		t.Fatalf("read abstract.py: %v", err)
	}
	concreteSrc, err := os.ReadFile("testdata/concrete.py")
	if err != nil {
		t.Fatalf("read concrete.py: %v", err)
	}

	modelFields := parseModelFields(string(abstractSrc), string(concreteSrc))

	tests := []struct {
		model     string
		field     string
		wantType  string
		wantMaxL  int
		wantUniq  bool
		wantRef   string
		wantHelp  string
	}{
		{
			model:    "Organization",
			field:    "name",
			wantType: "string",
			wantMaxL: 255,
			wantUniq: true,
			wantHelp: "Organization name",
		},
		{
			model:    "Network",
			field:    "asn",
			wantType: "integer",
			wantUniq: true,
			wantHelp: "Autonomous System Number",
		},
		{
			model:    "Network",
			field:    "org",
			wantType: "integer",
			wantRef:  "org",
		},
		{
			model:    "AddressModel",
			field:    "latitude",
			wantType: "float",
		},
		{
			model:    "HandleRefModel",
			field:    "status",
			wantType: "string",
			wantMaxL: 255,
		},
	}

	for _, tt := range tests {
		t.Run(tt.model+"/"+tt.field, func(t *testing.T) {
			t.Parallel()
			fields, ok := modelFields[tt.model]
			if !ok {
				t.Fatalf("model %q not found", tt.model)
			}
			fd, ok := fields[tt.field]
			if !ok {
				t.Fatalf("field %q not found in model %q", tt.field, tt.model)
			}
			if fd.Type != tt.wantType {
				t.Errorf("type = %q, want %q", fd.Type, tt.wantType)
			}
			if tt.wantMaxL > 0 && fd.MaxLength != tt.wantMaxL {
				t.Errorf("max_length = %d, want %d", fd.MaxLength, tt.wantMaxL)
			}
			if fd.Unique != tt.wantUniq {
				t.Errorf("unique = %v, want %v", fd.Unique, tt.wantUniq)
			}
			if tt.wantRef != "" && fd.References != tt.wantRef {
				t.Errorf("references = %q, want %q", fd.References, tt.wantRef)
			}
			if tt.wantHelp != "" && fd.HelpText != tt.wantHelp {
				t.Errorf("help_text = %q, want %q", fd.HelpText, tt.wantHelp)
			}
		})
	}
}

func TestParseChoiceConstants(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("testdata/const.py")
	if err != nil {
		t.Fatalf("read const.py: %v", err)
	}

	consts := parseChoiceConstants(string(src))

	tests := []struct {
		name       string
		wantCount  int
		wantFirst  string
	}{
		{name: "POC_ROLES", wantCount: 7, wantFirst: "Abuse"},
		{name: "VISIBILITY", wantCount: 3, wantFirst: "Private"},
		{name: "MEDIA", wantCount: 3, wantFirst: "Ethernet"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vals, ok := consts[tt.name]
			if !ok {
				t.Fatalf("constant %q not found", tt.name)
			}
			if len(vals) != tt.wantCount {
				t.Errorf("count = %d, want %d, got %v", len(vals), tt.wantCount, vals)
			}
			if len(vals) > 0 && vals[0] != tt.wantFirst {
				t.Errorf("first = %q, want %q", vals[0], tt.wantFirst)
			}
		})
	}
}

func TestDjangoFieldToJSONType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		djangoType string
		wantJSON   string
	}{
		{"CharField", "string"},
		{"TextField", "string"},
		{"URLField", "string"},
		{"EmailField", "string"},
		{"IntegerField", "integer"},
		{"PositiveIntegerField", "integer"},
		{"ForeignKey", "integer"},
		{"FloatField", "float"},
		{"DecimalField", "float"},
		{"BooleanField", "boolean"},
		{"DateTimeField", "datetime"},
		{"JSONField", "json_array"},
		{"UnknownField", "string"},
	}

	for _, tt := range tests {
		t.Run(tt.djangoType, func(t *testing.T) {
			t.Parallel()
			got := djangoFieldToJSONType(tt.djangoType)
			if got != tt.wantJSON {
				t.Errorf("djangoFieldToJSONType(%q) = %q, want %q", tt.djangoType, got, tt.wantJSON)
			}
		})
	}
}

func TestFKRelationshipDetection(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)

	schema, err := extractSchema(repoDir)
	if err != nil {
		t.Fatalf("extractSchema: %v", err)
	}

	// Check that Network has org FK.
	netType, ok := schema.ObjectTypes["net"]
	if !ok {
		t.Fatal("missing net object type")
	}

	// The Network model has org FK to Organization.
	found := false
	for _, fd := range netType.Fields {
		if fd.References == "org" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected net to have a field referencing org")
	}
}

func TestModelNameToAPIPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		want  string
	}{
		{"Organization", "org"},
		{"Network", "net"},
		{"Facility", "fac"},
		{"InternetExchange", "ix"},
		{"InternetExchangeLan", "ixlan"},
		{"NetworkIXLan", "netixlan"},
		{"Campus", "campus"},
		{"Carrier", "carrier"},
		{"UnknownModel", "unknownmodel"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()
			got := modelNameToAPIPath(tt.model)
			if got != tt.want {
				t.Errorf("modelNameToAPIPath(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestParsePythonDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  any
	}{
		{`True`, true},
		{`False`, false},
		{`None`, nil},
		{`""`, ""},
		{`"hello"`, "hello"},
		{`0`, float64(0)},
		{`42`, float64(42)},
		{`[]`, []any{}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := parsePythonDefault(tt.input)

			// Compare via JSON encoding for slices.
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("parsePythonDefault(%q) = %v (%s), want %v (%s)", tt.input, got, gotJSON, tt.want, wantJSON)
			}
		})
	}
}
