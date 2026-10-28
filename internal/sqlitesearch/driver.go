// Package sqlitesearch provides SQLite text functions for API search.
package sqlitesearch

import (
	"database/sql/driver"
	"strings"
	"unicode"

	"modernc.org/sqlite"
)

// NewDriver returns a SQLite driver with the text functions used by
// pdbcompat search. Functions belong to this driver, not a global registry.
func NewDriver() *sqlite.Driver {
	d := &sqlite.Driver{}
	for name, fn := range map[string]func(string) string{
		"pdbplus_search_lower":  strings.ToLower,
		"pdbplus_search_tokens": Tokens,
	} {
		d.MustRegisterDeterministicScalarFunction(name, 1,
			func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
				value, _ := args[0].(string)
				return fn(value), nil
			})
	}
	return d
}

// Tokens covers common standard-analyzer word boundaries.
// It retains Unicode letters, numbers and combining marks. Lucene's
// full Unicode word-break rules remain a registered divergence.
func Tokens(value string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r)
	}), " ")
}
