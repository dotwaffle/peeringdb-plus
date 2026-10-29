package sqlitesearch

import (
	"bytes"
	"encoding/json"
	"strings"
)

// JSONText normalizes JSON whitespace and object-key order for text
// filters. A fragment that is not a complete document stays unchanged.
func JSONText(value string) string {
	if !json.Valid([]byte(value)) {
		return value
	}
	d := json.NewDecoder(strings.NewReader(value))
	d.UseNumber()
	var decoded any
	if err := d.Decode(&decoded); err != nil {
		return value
	}
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(decoded); err != nil {
		return value
	}
	return strings.TrimSuffix(b.String(), "\n")
}
