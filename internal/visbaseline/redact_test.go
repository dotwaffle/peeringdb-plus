package visbaseline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadTestdata reads a file from testdata/ and fails the test on error.
func loadTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return b
}

// TestRedactionStripsPII is the canary test: under no circumstances must the
// redacted output contain any PII substring from the auth input. This is the
// single most important property the redactor provides.
func TestRedactionStripsPII(t *testing.T) {
	t.Parallel()

	anon := loadTestdata(t, "anon_sample.json")
	auth := loadTestdata(t, "auth_sample.json")

	redacted, err := Redact(anon, auth)
	if err != nil {
		t.Fatalf("Redact returned error: %v", err)
	}

	piiSubstrings := []string{
		"noc@alpha.example",
		"noc@beta.example",
		"secret@private.example",
		"+1-555-0001",
		"+1-555-0002",
		"+1-555-9999",
		"Alice Admin",
		"Bob Admin",
		"Hidden Person",
	}
	out := string(redacted)
	for _, s := range piiSubstrings {
		if strings.Contains(out, s) {
			t.Errorf("redacted output contains PII substring %q; redactor is LEAKING data", s)
		}
	}

	// Positive assertion: the placeholder must appear.
	if !strings.Contains(out, PlaceholderString) {
		t.Errorf("redacted output does not contain %q; expected redaction of string fields", PlaceholderString)
	}
}

// TestRedactionDeterministic asserts that Redact is a pure function: same
// inputs yield byte-identical outputs.
func TestRedactionDeterministic(t *testing.T) {
	t.Parallel()

	anon := loadTestdata(t, "anon_sample.json")
	auth := loadTestdata(t, "auth_sample.json")

	out1, err := Redact(anon, auth)
	if err != nil {
		t.Fatalf("first Redact: %v", err)
	}
	out2, err := Redact(anon, auth)
	if err != nil {
		t.Fatalf("second Redact: %v", err)
	}
	if !bytes.Equal(out1, out2) {
		t.Errorf("Redact is not deterministic: outputs differ\nfirst:\n%s\nsecond:\n%s", out1, out2)
	}
}

// TestRedactionAuthOnlyRow asserts that a row present only in auth (no
// matching anon id) has every non-id value replaced with a typed placeholder,
// while the id itself is preserved.
func TestRedactionAuthOnlyRow(t *testing.T) {
	t.Parallel()

	anonBytes := []byte(`{"meta":{},"data":[{"id":1,"name_long":"Alpha"}]}`)
	authBytes := []byte(`{"meta":{},"data":[
		{"id":1,"name_long":"Alpha"},
		{"id":99,"name_long":"Private","count":42,"enabled":true,"notes":"confidential"}
	]}`)

	redacted, err := Redact(anonBytes, authBytes)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}

	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(redacted, &env); err != nil {
		t.Fatalf("unmarshal redacted: %v", err)
	}

	if len(env.Data) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(env.Data))
	}

	authOnly := env.Data[1]
	// id must survive as original integer (float64 after JSON decode).
	if id, ok := authOnly["id"].(float64); !ok || id != 99 {
		t.Errorf("auth-only row id not preserved; got %v (%T)", authOnly["id"], authOnly["id"])
	}
	// string field → string placeholder
	if got := authOnly["name_long"]; got != PlaceholderString {
		t.Errorf("auth-only string field: got %v, want %q", got, PlaceholderString)
	}
	// numeric field (not id) → number placeholder (string-typed — intentional)
	if got := authOnly["count"]; got != PlaceholderNumber {
		t.Errorf("auth-only numeric field: got %v, want %q", got, PlaceholderNumber)
	}
	// bool field → bool placeholder
	if got := authOnly["enabled"]; got != PlaceholderBool {
		t.Errorf("auth-only bool field: got %v, want %q", got, PlaceholderBool)
	}
	// string field (notes) → string placeholder; must not leak "confidential"
	if strings.Contains(string(redacted), "confidential") {
		t.Errorf("redacted output leaked 'confidential' from auth-only row")
	}
}

// TestRedactionAuthOnlyField asserts that a field present in an auth row but
// absent from the matching anon row is replaced with a placeholder.
func TestRedactionAuthOnlyField(t *testing.T) {
	t.Parallel()

	anonBytes := []byte(`{"meta":{},"data":[{"id":1,"name_long":"Alpha"}]}`)
	authBytes := []byte(`{"meta":{},"data":[{"id":1,"name_long":"Alpha","phone":"+1-555-1234"}]}`)

	redacted, err := Redact(anonBytes, authBytes)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}

	if strings.Contains(string(redacted), "+1-555-1234") {
		t.Errorf("redacted output leaked phone number: %s", redacted)
	}

	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(redacted, &env); err != nil {
		t.Fatalf("unmarshal redacted: %v", err)
	}
	if got := env.Data[0]["phone"]; got != PlaceholderString {
		t.Errorf("auth-only field: got %v, want %q", got, PlaceholderString)
	}
	// name_long was present in both anon and auth with same value — kept.
	if got := env.Data[0]["name_long"]; got != "Alpha" {
		t.Errorf("shared field: got %v, want %q", got, "Alpha")
	}
}

// TestRedactionPreservesVisible asserts that the `visible` enum value is
// preserved verbatim — it's a controlled vocabulary we need for the diff.
func TestRedactionPreservesVisible(t *testing.T) {
	t.Parallel()

	anonBytes := []byte(`{"meta":{},"data":[{"id":1,"visible":"Public"}]}`)
	authBytes := []byte(`{"meta":{},"data":[
		{"id":1,"visible":"Public"},
		{"id":2,"visible":"Users"}
	]}`)

	redacted, err := Redact(anonBytes, authBytes)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}

	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(redacted, &env); err != nil {
		t.Fatalf("unmarshal redacted: %v", err)
	}

	if got := env.Data[0]["visible"]; got != "Public" {
		t.Errorf("row 0 visible: got %v, want %q", got, "Public")
	}
	if got := env.Data[1]["visible"]; got != "Users" {
		t.Errorf("row 1 visible (auth-only row): got %v, want %q (controlled enum must survive even for auth-only rows)", got, "Users")
	}
}

// TestRedactionPreservesIDs asserts that all numeric `id` fields survive
// untouched, including ids belonging to auth-only rows.
func TestRedactionPreservesIDs(t *testing.T) {
	t.Parallel()

	anonBytes := []byte(`{"meta":{},"data":[{"id":1},{"id":2}]}`)
	authBytes := []byte(`{"meta":{},"data":[{"id":1},{"id":2},{"id":99}]}`)

	redacted, err := Redact(anonBytes, authBytes)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}

	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(redacted, &env); err != nil {
		t.Fatalf("unmarshal redacted: %v", err)
	}

	wantIDs := []float64{1, 2, 99}
	if len(env.Data) != len(wantIDs) {
		t.Fatalf("row count: got %d, want %d", len(env.Data), len(wantIDs))
	}
	for i, want := range wantIDs {
		got, ok := env.Data[i]["id"].(float64)
		if !ok || got != want {
			t.Errorf("row %d id: got %v (%T), want %v", i, env.Data[i]["id"], env.Data[i]["id"], want)
		}
	}
}

// TestRedactionPIIFieldAlwaysRedacted asserts that a PII field is redacted
// even when present in BOTH anon and auth with the same value. This is
// defence-in-depth against upstream anon bugs that might leak PII.
func TestRedactionPIIFieldAlwaysRedacted(t *testing.T) {
	t.Parallel()

	// Simulate upstream anon bug: email visible on anon response.
	anonBytes := []byte(`{"meta":{},"data":[{"id":1,"email":"leaked@anon.example"}]}`)
	authBytes := []byte(`{"meta":{},"data":[{"id":1,"email":"leaked@anon.example"}]}`)

	redacted, err := Redact(anonBytes, authBytes)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}

	if strings.Contains(string(redacted), "leaked@anon.example") {
		t.Errorf("PII field leaked even though it matches anon: %s", redacted)
	}

	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(redacted, &env); err != nil {
		t.Fatalf("unmarshal redacted: %v", err)
	}
	if got := env.Data[0]["email"]; got != PlaceholderString {
		t.Errorf("PII email: got %v, want %q (PII fields must always be redacted)", got, PlaceholderString)
	}
}

// TestRedactionEmptyInputs asserts that invalid or empty JSON inputs return
// a wrapped error without panicking.
func TestRedactionEmptyInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		anon []byte
		auth []byte
	}{
		{name: "both empty", anon: []byte{}, auth: []byte{}},
		{name: "anon invalid", anon: []byte("not json"), auth: []byte(`{"data":[]}`)},
		{name: "auth invalid", anon: []byte(`{"data":[]}`), auth: []byte("not json")},
		{name: "both nil", anon: nil, auth: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Redact(tt.anon, tt.auth)
			if err == nil {
				t.Errorf("expected error for invalid input, got nil")
			}
		})
	}
}

// TestRedactionPreservesStructure asserts that the envelope shape
// ({"meta":..., "data":[...]}) and row count are preserved across redaction.
func TestRedactionPreservesStructure(t *testing.T) {
	t.Parallel()

	anon := loadTestdata(t, "anon_sample.json")
	auth := loadTestdata(t, "auth_sample.json")

	redacted, err := Redact(anon, auth)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}

	var env struct {
		Meta json.RawMessage  `json:"meta"`
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(redacted, &env); err != nil {
		t.Fatalf("unmarshal redacted: %v", err)
	}

	// Input auth has 3 rows; redacted must preserve count.
	if len(env.Data) != 3 {
		t.Errorf("row count: got %d, want 3", len(env.Data))
	}

	// Envelope keys must still be "meta" and "data".
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(redacted, &raw); err != nil {
		t.Fatalf("unmarshal raw envelope: %v", err)
	}
	if _, ok := raw["meta"]; !ok {
		t.Errorf("redacted envelope missing 'meta' key")
	}
	if _, ok := raw["data"]; !ok {
		t.Errorf("redacted envelope missing 'data' key")
	}

	// Every row must still have an id field.
	for i, row := range env.Data {
		if _, ok := row["id"]; !ok {
			t.Errorf("row %d missing id after redaction", i)
		}
	}
}
