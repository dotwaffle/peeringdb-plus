package pdbcompat

import (
	"net/url"
	"testing"
)

// FuzzFilterParser exercises ParseFilters with arbitrary key/value pairs to
// ensure the filter parser never panics on untrusted input. Errors from
// invalid input are expected and acceptable -- the contract is no panics.
func FuzzFilterParser(f *testing.F) {
	// Seed corpus covering all 5 field types and key edge cases.
	f.Add("name", "Cloudflare")             // string exact
	f.Add("asn__gt", "1000")                // int comparison
	f.Add("name__contains", "cloud")        // string contains
	f.Add("asn__in", "13335,174")           // int IN
	f.Add("info_unicast", "true")           // bool exact
	f.Add("created__gte", "1700000000")     // time comparison
	f.Add("latitude", "37.7749")            // float exact
	f.Add("name__regex", ".*")              // unsupported operator
	f.Add("asn", "not-a-number")            // type conversion error
	f.Add("", "")                           // empty key
	f.Add("__", "val")                      // empty field name with operator separator

	// fields map with entries for all 5 FieldType values.
	fields := map[string]FieldType{
		"name":         FieldString,
		"asn":          FieldInt,
		"info_unicast": FieldBool,
		"created":      FieldTime,
		"latitude":     FieldFloat,
	}

	f.Fuzz(func(t *testing.T, key, value string) {
		params := url.Values{key: {value}}
		// Must not panic. Errors are acceptable.
		_, _ = ParseFilters(params, fields)
	})
}
