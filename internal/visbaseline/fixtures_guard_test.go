package visbaseline

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// errorReporter is the minimal surface walkPII needs to report findings.
// Decoupling from testing.TB lets us drive walkPII from self-tests with a
// simple recorder. testing.T and testing.B both satisfy this interface via
// their Errorf method.
type errorReporter interface {
	Errorf(format string, args ...any)
	Helper()
}

// TestCommittedFixturesHaveNoPII is the repo-wide canary for plan 04's
// committed auth fixtures. It walks testdata/visibility-baseline/ (if
// present) and for every JSON file under a path containing "/auth/" it
// asserts every PII field's string value is one of the sentinel placeholders
// (PlaceholderString, PlaceholderNumber, PlaceholderBool) or null.
//
// The test is SKIPPED when testdata/visibility-baseline/ does not exist —
// this keeps the test runnable in CI before plan 04 has committed fixtures.
// Once fixtures land, the test enforces the no-PII invariant on every
// `go test` run. Any regression (e.g. someone edits a fixture and forgets
// to redact) fails CI with a precise file+JSON-path location.
//
// This test is the last line of defence for threat T-57-02 (PII leaked via
// unredacted auth fixture).
func TestCommittedFixturesHaveNoPII(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}
	baseDir := filepath.Join(repoRoot, "testdata", "visibility-baseline")

	info, err := os.Stat(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("testdata/visibility-baseline/ not present; fixtures not yet committed")
		}
		t.Fatalf("stat %s: %v", baseDir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", baseDir)
	}

	placeholders := map[string]struct{}{
		PlaceholderString: {},
		PlaceholderNumber: {},
		PlaceholderBool:   {},
	}

	walkErr := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		// Only scan files under an "auth" component of the path.
		if !strings.Contains(filepath.ToSlash(path), "/auth/") {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var root any
		if err := json.Unmarshal(data, &root); err != nil {
			t.Errorf("%s: invalid JSON: %v", path, err)
			return nil
		}
		walkPII(t, path, "", root, placeholders)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", baseDir, walkErr)
	}
}

// walkPII recursively inspects a decoded JSON value. Whenever it encounters
// an object field whose key is in PIIFields, it asserts the value is a
// placeholder sentinel or JSON null. Errors are reported via the errorReporter
// interface; the walk continues so a single fixture can surface multiple
// violations.
func walkPII(r errorReporter, file, jsonPath string, v any, placeholders map[string]struct{}) {
	r.Helper()
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			childPath := jsonPath + "." + k
			if IsPIIField(k) {
				switch cv := child.(type) {
				case nil:
					// null is acceptable
				case string:
					if _, ok := placeholders[cv]; !ok {
						r.Errorf("%s: PII field %s has non-placeholder string value (redaction regression: T-57-02)",
							file, childPath)
					}
				case float64, bool:
					// Non-string values on PII fields are ALSO a red flag — the
					// redactor emits placeholder strings. Anything else means
					// the fixture bypassed the redactor.
					r.Errorf("%s: PII field %s has non-placeholder non-null value of type %T",
						file, childPath, cv)
				default:
					r.Errorf("%s: PII field %s has unexpected value type %T",
						file, childPath, cv)
				}
			}
			walkPII(r, file, childPath, child, placeholders)
		}
	case []any:
		for i, child := range val {
			walkPII(r, file, jsonPath+"["+strconv.Itoa(i)+"]", child, placeholders)
		}
	}
}

// findRepoRoot walks up from the CWD looking for a go.mod file.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// recorderTB captures Errorf calls from walkPII under test so we can assert
// the guard's own behaviour without failing the outer test. Only the subset
// of the errorReporter interface that walkPII uses is implemented.
type recorderTB struct {
	failed bool
	errors []string
}

func (r *recorderTB) Helper() {}
func (r *recorderTB) Errorf(format string, args ...any) {
	_ = args // self-test only cares that Errorf was invoked
	r.failed = true
	r.errors = append(r.errors, format)
}

// TestPIIGuardDetectsUnredactedString is a self-test for the guard detector:
// build a synthetic bad fixture and confirm walkPII flags it.
func TestPIIGuardDetectsUnredactedString(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "bad.json")
	bad := []byte(`{"data":[{"id":1,"email":"leaked@example.invalid","phone":"<auth-only:string>"}]}`)
	if err := os.WriteFile(tempFile, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	placeholders := map[string]struct{}{
		PlaceholderString: {}, PlaceholderNumber: {}, PlaceholderBool: {},
	}
	var root any
	if err := json.Unmarshal(bad, &root); err != nil {
		t.Fatal(err)
	}

	rec := &recorderTB{}
	walkPII(rec, tempFile, "", root, placeholders)
	if !rec.failed {
		t.Fatal("walkPII did not detect unredacted PII; guard is broken")
	}
}

// TestPIIGuardAcceptsRedactedFixture confirms the detector accepts a fixture
// in which every PII field is either a placeholder sentinel or null.
func TestPIIGuardAcceptsRedactedFixture(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "good.json")
	good := []byte(`{"data":[{"id":1,"email":"<auth-only:string>","phone":"<auth-only:string>","name":null}]}`)
	if err := os.WriteFile(tempFile, good, 0o600); err != nil {
		t.Fatal(err)
	}
	placeholders := map[string]struct{}{
		PlaceholderString: {}, PlaceholderNumber: {}, PlaceholderBool: {},
	}
	var root any
	if err := json.Unmarshal(good, &root); err != nil {
		t.Fatal(err)
	}

	rec := &recorderTB{}
	walkPII(rec, tempFile, "", root, placeholders)
	if rec.failed {
		t.Fatalf("walkPII flagged correctly-redacted fixture as bad; guard is over-strict; messages: %v", rec.errors)
	}
}

// TestPIIGuardDetectsNonStringPIIValue confirms the detector flags PII field
// values that are numbers or booleans (anything other than a placeholder
// string or null). This catches fixtures that bypass the redactor entirely.
func TestPIIGuardDetectsNonStringPIIValue(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "bad_num.json")
	bad := []byte(`{"data":[{"id":1,"latitude":37.7749}]}`)
	if err := os.WriteFile(tempFile, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	placeholders := map[string]struct{}{
		PlaceholderString: {}, PlaceholderNumber: {}, PlaceholderBool: {},
	}
	var root any
	if err := json.Unmarshal(bad, &root); err != nil {
		t.Fatal(err)
	}
	rec := &recorderTB{}
	walkPII(rec, tempFile, "", root, placeholders)
	if !rec.failed {
		t.Fatal("walkPII did not detect numeric latitude value on PII field; guard is broken")
	}
}
