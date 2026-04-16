package visbaseline

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Placeholder strings substituted for redacted values in committed auth
// fixtures. The "<auth-only:TYPE>" format is intentionally not valid JSON
// in any identifier position, making it impossible to confuse with real data
// and trivial to grep for in CI audits.
const (
	PlaceholderString = "<auth-only:string>"
	PlaceholderNumber = "<auth-only:number>"
	PlaceholderBool   = "<auth-only:bool>"
)

// envelope models the PeeringDB response wrapper. Both anon and auth
// responses share this shape: a freeform meta object (usually empty) and
// a data array of heterogeneous rows.
type envelope struct {
	Meta json.RawMessage  `json:"meta"`
	Data []map[string]any `json:"data"`
}

// Redact applies the phase 57 redaction policy to an authenticated PeeringDB
// response and returns bytes safe to commit. The policy:
//
//  1. Preserve envelope shape, row count, field names, integer row ids, and
//     the `visible` enum value (controlled vocabulary we need for the diff).
//  2. Replace any value whose field name appears in PIIFields with a typed
//     placeholder, regardless of whether the field appears in anon. This is
//     defence-in-depth against upstream anon bugs.
//  3. For non-PII fields, replace values with a typed placeholder when:
//     - the auth row has no matching anon row (match by `id`), OR
//     - the anon row lacks the field name.
//     Otherwise keep the auth value verbatim (anon already publishes it).
//  4. Re-marshal with json.MarshalIndent. Go's encoding/json sorts map keys
//     lexicographically in MarshalIndent output since Go 1.12, so the result
//     is deterministic — same input bytes always produce same output bytes.
//
// Redact is a pure function: no filesystem, no network, no global state, no
// goroutines. Errors from json.Unmarshal / json.MarshalIndent are wrapped with
// %w. Input bytes are never echoed in error messages.
func Redact(anonBytes, authBytes []byte) ([]byte, error) {
	var anon envelope
	if err := json.Unmarshal(anonBytes, &anon); err != nil {
		return nil, fmt.Errorf("unmarshal anon: %w", err)
	}
	var auth envelope
	if err := json.Unmarshal(authBytes, &auth); err != nil {
		return nil, fmt.Errorf("unmarshal auth: %w", err)
	}

	// Index anon rows by id for O(1) lookup during the auth walk. JSON numbers
	// decode to float64 by default; we key on that.
	anonByID := make(map[float64]map[string]any, len(anon.Data))
	for _, row := range anon.Data {
		if id, ok := row["id"].(float64); ok {
			anonByID[id] = row
		}
	}

	for _, authRow := range auth.Data {
		var anonRow map[string]any
		if id, ok := authRow["id"].(float64); ok {
			anonRow = anonByID[id] // nil if auth row has no anon counterpart
		}
		for k, v := range authRow {
			authRow[k] = redactValue(k, v, anonRow)
		}
	}

	// Deterministic output: json.Encoder with SetEscapeHTML(false) preserves
	// the literal "<" and ">" in placeholder strings (default MarshalIndent
	// would escape them to \u003c / \u003e, which breaks the greppability
	// contract expected by the plan 03 PII guard test). Indent("", "  ") and
	// Go's lexicographic map-key ordering make the output deterministic —
	// same input bytes always produce same output bytes.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&auth); err != nil {
		return nil, fmt.Errorf("marshal redacted auth: %w", err)
	}
	// json.Encoder.Encode appends a trailing newline; strip it so callers
	// get bytes equivalent to MarshalIndent output plus no trailing newline.
	out := bytes.TrimRight(buf.Bytes(), "\n")
	return out, nil
}

// redactValue applies the redaction rules to a single field within a row.
// anonRow is nil when the auth row has no matching anon counterpart.
func redactValue(fieldName string, val any, anonRow map[string]any) any {
	// 1. Identity: id flows through as the original number — fixtures must
	//    preserve row identity for the diff.
	if fieldName == "id" {
		return val
	}
	// 2. Controlled enum: `visible` must survive even on auth-only rows so the
	//    diff can report "visible:Users rows first appeared at auth mode".
	if fieldName == "visible" {
		return val
	}
	// 3. PII allow-list: always replaced, ignoring what anon says.
	if IsPIIField(fieldName) {
		return placeholderFor(val)
	}
	// 4. Auth-only row: every non-id, non-visible, non-PII field is placeholdered.
	if anonRow == nil {
		return placeholderFor(val)
	}
	// 5. Auth-only field (anon row exists but lacks this key).
	if _, hasField := anonRow[fieldName]; !hasField {
		return placeholderFor(val)
	}
	// 6. Field is in both anon and auth — keep the auth value verbatim.
	//    Anon already publishes this data; keeping it lets reviewers confirm
	//    the shape matches.
	return val
}

// placeholderFor maps a Go-decoded JSON value to the typed placeholder string.
// Null stays null (no disclosure). Nested objects and arrays collapse to the
// string placeholder — the baseline phase only tracks field presence + type,
// not inner shape (Pitfall 4: structural reveal is also a leak surface).
func placeholderFor(v any) any {
	switch v.(type) {
	case string:
		return PlaceholderString
	case float64, json.Number:
		return PlaceholderNumber
	case bool:
		return PlaceholderBool
	case nil:
		return nil
	case []any, map[string]any:
		return PlaceholderString
	default:
		return PlaceholderString
	}
}
