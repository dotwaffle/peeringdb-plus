package pdbcompat

import (
	"reflect"
	"strings"
	"sync"

	"entgo.io/ent/dialect/sql"
)

// buildSearchPredicate creates a sql.Selector predicate that ORs together
// case-insensitive contains matches across the given fields for the search
// term. Returns nil if search is empty, which signals the caller to skip
// applying a search filter.
func buildSearchPredicate(search string, searchFields []string) func(*sql.Selector) {
	search = strings.TrimSpace(search)
	if search == "" {
		return nil
	}
	return func(s *sql.Selector) {
		var ors []*sql.Predicate
		for _, f := range searchFields {
			ors = append(ors, sql.ContainsFold(f, search))
		}
		if len(ors) > 0 {
			s.Where(sql.Or(ors...))
		}
	}
}

// applyFieldProjection filters each object in data to include only the
// requested fields. The "id" field is always included even if not explicitly
// listed. Fields that do not exist on an object are silently ignored.
// If fields is empty, data is returned unchanged.
//
// For depth > 0 responses, _set fields and expanded FK edge objects are
// preserved regardless of the field list.
func applyFieldProjection(data []any, fields []string) []any {
	if len(fields) == 0 {
		return data
	}

	// Build a lookup set with the requested fields plus "id".
	want := make(map[string]bool, len(fields)+1)
	want["id"] = true
	for _, f := range fields {
		want[strings.TrimSpace(f)] = true
	}

	out := make([]any, len(data))
	for i, item := range data {
		m, ok := itemToMap(item)
		if !ok {
			out[i] = item
			continue
		}
		projected := make(map[string]any, len(want))
		for k, v := range m {
			// Always keep _set fields and expanded FK objects (depth > 0 responses).
			if want[k] || strings.HasSuffix(k, "_set") {
				projected[k] = v
				continue
			}
			// Check if this is an expanded FK object (value is a map with an
			// "id" key). These are produced by depth=2 expansion for things
			// like "org", "net", "fac", etc.
			if isExpandedObject(v) {
				projected[k] = v
			}
		}
		out[i] = projected
	}
	return out
}

// fieldAccessor holds the struct field index for a JSON-tagged field.
type fieldAccessor struct {
	index int
}

// fieldMaps caches per-type field accessor maps to avoid rebuilding on every call.
var fieldMaps sync.Map // map key: reflect.Type, value: map[string]fieldAccessor

// getFieldMap lazily builds and caches a map from JSON tag names to struct field
// indices for the given type. Unexported fields, fields tagged "-", and fields
// without json tags are excluded.
func getFieldMap(t reflect.Type) map[string]fieldAccessor {
	if v, ok := fieldMaps.Load(t); ok {
		return v.(map[string]fieldAccessor)
	}
	m := make(map[string]fieldAccessor, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		m[name] = fieldAccessor{index: i}
	}
	fieldMaps.Store(t, m)
	return m
}

// itemToMap converts an item (struct or map) to a map[string]any using reflect.
// If the item is already a map[string]any it is returned directly. Struct fields
// are accessed via cached field index maps derived from json tags, avoiding
// json.Marshal/Unmarshal overhead.
func itemToMap(item any) (map[string]any, bool) {
	if m, ok := item.(map[string]any); ok {
		return m, true
	}
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, false
	}
	fm := getFieldMap(v.Type())
	m := make(map[string]any, len(fm))
	for name, acc := range fm {
		m[name] = v.Field(acc.index).Interface()
	}
	return m, true
}

// isExpandedObject checks if a value is an expanded FK object (a map with
// an "id" key), as produced by depth=2 expansion.
func isExpandedObject(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	_, hasID := m["id"]
	return hasID
}
