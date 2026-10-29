// Package sqlitesearch provides SQLite text functions for API search.
package sqlitesearch

import (
	"database/sql/driver"
	"strings"

	"modernc.org/sqlite"
)

// NewDriver returns a SQLite driver with the text functions used by
// pdbcompat search. Functions belong to this driver, not a global registry.
func NewDriver() *sqlite.Driver {
	d := &sqlite.Driver{}
	for name, fn := range map[string]func(string) string{
		"pdbplus_search_lower":  strings.ToLower,
		"pdbplus_search_tokens": Tokens,
		"pdbplus_json_text":     JSONText,
	} {
		d.MustRegisterDeterministicScalarFunction(name, 1,
			func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
				value, _ := args[0].(string)
				return fn(value), nil
			})
	}
	return d
}
