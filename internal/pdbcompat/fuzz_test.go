package pdbcompat

import (
	"net/url"
	"strings"
	"testing"
)

// FuzzFilterParser exercises ParseFilters with arbitrary key/value pairs to
// ensure the filter parser never panics on untrusted input. Errors from
// invalid input are expected and acceptable -- the contract is no panics.
//
// Phase 69 Plan 05 extends the seed corpus per CONTEXT.md D-07: diacritics,
// CJK, combining marks, ZWJ sequences, RTL, RLO overrides, null bytes, and
// long strings (>64 KB). IN-01/IN-02 edges (>999 element lists, empty lists,
// all-empty parts) are also seeded so the fuzzer exercises json_each rewrite
// and the empty-__in sentinel path.
//
// CI runs the default (non-fuzzing) test execution which replays the seed
// corpus. The 500k-execution deliverable from plan D-07 is a LOCAL run
// invoked as:
//
//	go test -fuzz=FuzzFilterParser -fuzztime=60s -run '^$' ./internal/pdbcompat/
//
// per the v1.10 Phase 48 convention (fuzz exec counts recorded in plan
// SUMMARY, never gated by CI wall-clock).
func FuzzFilterParser(f *testing.F) {
	// Seed corpus covering all 5 field types and key edge cases.
	f.Add("name", "Cloudflare")         // string exact
	f.Add("asn__gt", "1000")            // int comparison
	f.Add("name__contains", "cloud")    // string contains
	f.Add("asn__in", "13335,174")       // int IN
	f.Add("info_unicast", "true")       // bool exact
	f.Add("created__gte", "1700000000") // time comparison
	f.Add("latitude", "37.7749")        // float exact
	f.Add("name__regex", ".*")          // unsupported operator
	f.Add("asn", "not-a-number")        // type conversion error
	f.Add("", "")                       // empty key
	f.Add("__", "val")                  // empty field name with operator separator

	// Phase 69 UNICODE-03 corpus extension (CONTEXT.md D-07).
	// Diacritics, CJK, combining marks, ZWJ, RTL, null bytes, RLO.
	f.Add("name__contains", "Zürich")       // diacritic
	f.Add("name__contains", "Straße")       // non-decomposable ligature (ß→ss)
	f.Add("name__contains", "日本語")          // CJK
	f.Add("name__contains", "中文")           // CJK simplified
	f.Add("name__contains", "한글")           // Hangul
	f.Add("name__contains", "עברית")        // RTL Hebrew
	f.Add("name__contains", "e\u0301")      // combining acute on e
	f.Add("name__contains", "a\u0308")      // combining diaeresis on a
	f.Add("name__contains", "\x00\xff\xfe") // null + invalid UTF-8 sequence
	f.Add("name__contains", "\u202e\u202d") // RLO + LRO overrides
	f.Add("name__contains", "\u200d")       // zero-width joiner
	f.Add("name__startswith", "Zür")        // diacritic startswith
	f.Add("name", "Zürich")                 // diacritic exact
	f.Add("name__iexact", "ZÜRICH")         // diacritic iexact (uppercase)

	// Phase 69 IN-01 / IN-02 corpus extension.
	f.Add("asn__in", "")                             // empty __in → empty-result sentinel (IN-02)
	f.Add("asn__in", "1,2,3,4,5")                    // small int IN
	f.Add("asn__in", strings.Repeat("1,", 1200)+"1") // >999 values (IN-01 json_each)
	f.Add("name__in", "a,b,c")                       // string IN
	f.Add("name__in", "Zürich,Köln,München")         // unicode IN
	f.Add("name__in", ",,,")                         // all-empty IN parts
	f.Add("name__in", strings.Repeat(",", 1000))     // 1000 empty strings

	// Long-string stress (>64 KB payload, D-07 and v1.10 Phase 48 pattern).
	f.Add("name__contains", strings.Repeat("x", 70_000))
	f.Add("name", strings.Repeat("Z\u0301", 5_000)) // zalgo at scale

	// TypeConfig with entries for all 5 FieldType values. Phase 69 Plan 04:
	// ParseFilters takes TypeConfig so it can consult FoldedFields. Mark
	// "name" as folded so the shadow-routing path (UNICODE-01) is
	// exercised in addition to the non-shadow path.
	tc := TypeConfig{
		Name: "fuzz",
		Fields: map[string]FieldType{
			"name":         FieldString,
			"asn":          FieldInt,
			"info_unicast": FieldBool,
			"created":      FieldTime,
			"latitude":     FieldFloat,
		},
		FoldedFields: map[string]bool{
			"name": true,
		},
	}

	f.Fuzz(func(_ *testing.T, key, value string) {
		params := url.Values{key: {value}}
		// Must not panic. Errors and emptyResult=true are both acceptable.
		_, _, _ = ParseFilters(params, tc)
	})
}
