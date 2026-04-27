package visbaseline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// loadFixture reads a golden JSON fixture from testdata/diff_golden/.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "diff_golden", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func TestDiffNoDeltas(t *testing.T) {
	anon := loadFixture(t, "anon_identical.json")
	auth := loadFixture(t, "auth_identical.json")
	rep, err := Diff("org", anon, auth)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if len(rep.Fields) != 0 {
		t.Errorf("expected no field deltas, got %d: %+v", len(rep.Fields), rep.Fields)
	}
	if rep.AuthOnlyRowCount != 0 {
		t.Errorf("expected AuthOnlyRowCount=0, got %d", rep.AuthOnlyRowCount)
	}
	if rep.AnonRowCount != 2 || rep.AuthRowCount != 2 {
		t.Errorf("row counts: anon=%d auth=%d (want 2/2)", rep.AnonRowCount, rep.AuthRowCount)
	}
}

func TestDiffAuthOnlyField(t *testing.T) {
	anon := []byte(`{"meta":{},"data":[{"id":1,"a":"x"}]}`)
	auth := []byte(`{"meta":{},"data":[{"id":1,"a":"x","email":"<auth-only:string>"}]}`)
	rep, err := Diff("poc", anon, auth)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if len(rep.Fields) != 1 {
		t.Fatalf("expected 1 FieldDelta, got %d: %+v", len(rep.Fields), rep.Fields)
	}
	fd := rep.Fields[0]
	if fd.Name != "email" {
		t.Errorf("Name: got %q want %q", fd.Name, "email")
	}
	if !fd.AuthOnly {
		t.Errorf("AuthOnly: got %v want true", fd.AuthOnly)
	}
	if fd.Placeholder != PlaceholderString {
		t.Errorf("Placeholder: got %q want %q", fd.Placeholder, PlaceholderString)
	}
	if fd.RowsAdded != 1 {
		t.Errorf("RowsAdded: got %d want 1", fd.RowsAdded)
	}
	if !fd.IsPII {
		t.Errorf("IsPII: got %v want true for email", fd.IsPII)
	}
}

func TestDiffAuthOnlyRow(t *testing.T) {
	anon := []byte(`{"meta":{},"data":[{"id":1,"a":"x"}]}`)
	auth := []byte(`{"meta":{},"data":[{"id":1,"a":"x"},{"id":99,"a":"y","email":"<auth-only:string>"}]}`)
	rep, err := Diff("poc", anon, auth)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if rep.AuthOnlyRowCount != 1 {
		t.Errorf("AuthOnlyRowCount: got %d want 1", rep.AuthOnlyRowCount)
	}
	// The auth-only row contributes fields "id", "a", "email" as RowsAdded=1 each.
	fieldsByName := make(map[string]FieldDelta)
	for _, fd := range rep.Fields {
		fieldsByName[fd.Name] = fd
	}
	for _, want := range []string{"id", "a", "email"} {
		fd, ok := fieldsByName[want]
		if !ok {
			t.Errorf("expected field %q in Fields", want)
			continue
		}
		if fd.RowsAdded != 1 {
			t.Errorf("field %q RowsAdded: got %d want 1", want, fd.RowsAdded)
		}
	}
}

func TestDiffRowCountDrift(t *testing.T) {
	anon := loadFixture(t, "anon_rowdrift.json")
	auth := loadFixture(t, "auth_rowdrift.json")
	rep, err := Diff("poc", anon, auth)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if rep.AnonRowCount != 3 {
		t.Errorf("AnonRowCount: got %d want 3", rep.AnonRowCount)
	}
	if rep.AuthRowCount != 5 {
		t.Errorf("AuthRowCount: got %d want 5", rep.AuthRowCount)
	}
	if rep.AuthOnlyRowCount != 2 {
		t.Errorf("AuthOnlyRowCount: got %d want 2", rep.AuthOnlyRowCount)
	}
}

func TestDiffVisibleValueDrift(t *testing.T) {
	anon := loadFixture(t, "anon_rowdrift.json")
	auth := loadFixture(t, "auth_rowdrift.json")
	rep, err := Diff("poc", anon, auth)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if !reflect.DeepEqual(rep.VisibleValuesAnon, []string{"Public"}) {
		t.Errorf("VisibleValuesAnon: got %v want [Public]", rep.VisibleValuesAnon)
	}
	if !reflect.DeepEqual(rep.VisibleValuesAuth, []string{"Public", "Users"}) {
		t.Errorf("VisibleValuesAuth: got %v want [Public Users]", rep.VisibleValuesAuth)
	}
	// Find "visible" in Fields and assert ValueSetDrift is true.
	var found bool
	for _, fd := range rep.Fields {
		if fd.Name == "visible" {
			found = true
			if !fd.ValueSetDrift {
				t.Errorf("visible FieldDelta: ValueSetDrift=false, want true")
			}
		}
	}
	if !found {
		t.Error("visible not present in Fields despite value-set drift")
	}
}

func TestDiffNeverEmitsValues(t *testing.T) {
	anon := []byte(`{"meta":{},"data":[{"id":1}]}`)
	auth := []byte(`{"meta":{},"data":[{"id":1,"email":"secret@canary.example"}]}`)
	rep, err := Diff("poc", anon, auth)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	js, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(js, []byte("secret@canary.example")) {
		t.Fatalf("Report JSON contains leaked value: %s", js)
	}
}

func TestDiffNeverEmitsLengths(t *testing.T) {
	// Forbid fields that would carry signal-rich / fingerprintable data.
	// "Value" and "Values" are allowed only in composite names that carry a
	// non-value meaning (e.g. ValueSetDrift is a boolean drift flag over the
	// controlled `visible` enum, not a leaked value). Use exact field names
	// plus fuzzy-match for Length/Size/Hash/Digest.
	typ := reflect.TypeFor[FieldDelta]()
	forbiddenSubstring := []string{"Length", "Size", "Hash", "Digest"}
	forbiddenExact := map[string]struct{}{
		"Value":  {},
		"Values": {},
	}
	for field := range typ.Fields() {
		name := field.Name
		for _, bad := range forbiddenSubstring {
			if strings.Contains(name, bad) {
				t.Errorf("FieldDelta has forbidden field %q (signal-rich, PII-leaking)", name)
			}
		}
		if _, bad := forbiddenExact[name]; bad {
			t.Errorf("FieldDelta has forbidden field %q (signal-rich, PII-leaking)", name)
		}
	}
}

func TestDiffDeterministic(t *testing.T) {
	anon := loadFixture(t, "anon_simple.json")
	auth := loadFixture(t, "auth_simple.json")
	rep1, err := Diff("poc", anon, auth)
	if err != nil {
		t.Fatalf("Diff #1 error: %v", err)
	}
	rep2, err := Diff("poc", anon, auth)
	if err != nil {
		t.Fatalf("Diff #2 error: %v", err)
	}
	if !reflect.DeepEqual(rep1, rep2) {
		t.Errorf("Diff is not deterministic:\nrep1=%+v\nrep2=%+v", rep1, rep2)
	}
	// Serialise both to JSON and compare bytes for full determinism.
	j1, _ := json.Marshal(rep1)
	j2, _ := json.Marshal(rep2)
	if !bytes.Equal(j1, j2) {
		t.Errorf("Diff JSON output differs across runs:\n%s\n%s", j1, j2)
	}
}

func TestDiffFieldSorting(t *testing.T) {
	anon := []byte(`{"meta":{},"data":[{"id":1}]}`)
	auth := []byte(`{"meta":{},"data":[{"id":1,"z_field":"<auth-only:string>","a_field":"<auth-only:string>","m_field":"<auth-only:string>"}]}`)
	rep, err := Diff("poc", anon, auth)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	for i := 1; i < len(rep.Fields); i++ {
		if rep.Fields[i-1].Name > rep.Fields[i].Name {
			t.Errorf("Fields not sorted: %s > %s at index %d",
				rep.Fields[i-1].Name, rep.Fields[i].Name, i)
		}
	}
}

func TestDiffInvalidJSONReturnsError(t *testing.T) {
	garbage := []byte("not-json{")
	_, err := Diff("poc", garbage, []byte(`{"meta":{},"data":[]}`))
	if err == nil {
		t.Fatal("expected error for invalid anon JSON, got nil")
	}
	// Also ensure it doesn't panic on a nil-slice input.
	_, err = Diff("poc", []byte(`{"meta":{},"data":[]}`), garbage)
	if err == nil {
		t.Fatal("expected error for invalid auth JSON, got nil")
	}
}

func TestDiffMissingDataKeyReturnsError(t *testing.T) {
	anon := []byte(`{"meta":{}}`) // missing data
	auth := []byte(`{"meta":{},"data":[]}`)
	_, err := Diff("poc", anon, auth)
	if err == nil {
		t.Fatal("expected error for anon envelope missing data key")
	}
	if !strings.Contains(err.Error(), "anon") {
		t.Errorf("error should mention 'anon', got: %v", err)
	}
}

func TestDiffMissingDataKeyReturnsErrorAuth(t *testing.T) {
	anon := []byte(`{"meta":{},"data":[]}`)
	auth := []byte(`{"meta":{}}`) // missing data
	_, err := Diff("poc", anon, auth)
	if err == nil {
		t.Fatal("expected error for auth envelope missing data key")
	}
	if !strings.Contains(err.Error(), "auth") {
		t.Errorf("error should mention 'auth', got: %v", err)
	}
}

func TestDiffPIIFieldMetadata(t *testing.T) {
	anon := []byte(`{"meta":{},"data":[{"id":1}]}`)
	auth := []byte(`{"meta":{},"data":[{"id":1,"email":"<auth-only:string>","status":"ok"}]}`)
	rep, err := Diff("poc", anon, auth)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	var emailFD, statusFD *FieldDelta
	for i := range rep.Fields {
		switch rep.Fields[i].Name {
		case "email":
			emailFD = &rep.Fields[i]
		case "status":
			statusFD = &rep.Fields[i]
		}
	}
	if emailFD == nil {
		t.Fatal("email field not present in Fields")
	}
	if !emailFD.IsPII {
		t.Errorf("email IsPII: got false want true")
	}
	if statusFD == nil {
		t.Fatal("status field not present in Fields")
	}
	if statusFD.IsPII {
		t.Errorf("status IsPII: got true want false (status is not PII)")
	}
}

func TestDiffReportSchemaVersion(t *testing.T) {
	// ReportSchemaVersion is a public constant equal to 1.
	if ReportSchemaVersion != 1 {
		t.Errorf("ReportSchemaVersion: got %d want 1", ReportSchemaVersion)
	}
}
