package pdbcompat

import (
	"net/url"
	"reflect"
	"strconv"
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

// parseASNQuery returns (asn, true) if q looks like an ASN literal: optional
// leading "AS"/"as" prefix followed by digits, parsed as a positive 32-bit
// value. Otherwise returns (0, false). Callers use this to OR an exact asn
// equality into an otherwise text-only search predicate.
func parseASNQuery(q string) (int64, bool) {
	s := strings.TrimSpace(q)
	if len(s) >= 2 && (s[0] == 'A' || s[0] == 'a') && (s[1] == 'S' || s[1] == 's') {
		s = s[2:]
	}
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n > 0xFFFFFFFF {
		return 0, false
	}
	return n, true
}

// buildNetworkSearchPredicate builds a search predicate for the Network type
// that ORs text-field ContainsFold matches with an exact asn equality when
// the query parses as an ASN literal. Returns nil if search is empty.
func buildNetworkSearchPredicate(search string, searchFields []string) func(*sql.Selector) {
	search = strings.TrimSpace(search)
	if search == "" {
		return nil
	}
	asnVal, hasASN := parseASNQuery(search)
	return func(s *sql.Selector) {
		var ors []*sql.Predicate
		for _, f := range searchFields {
			ors = append(ors, sql.ContainsFold(f, search))
		}
		if hasASN {
			ors = append(ors, sql.EQ("asn", asnVal))
		}
		if len(ors) > 0 {
			s.Where(sql.Or(ors...))
		}
	}
}

// applyFieldProjection keeps the keys of each row that fields names.
// Upstream removes every serializer field that fields does not name
// (2.83.0 serializers.py:942-950), id, the _set fields and the nested
// objects too, and compares the names as given. A nested object keeps
// every key: upstream builds it without the request, so the fields filter
// does not run on it (sub_serializer, serializers.py:1335-1341). Names
// that no row has are ignored. If fields is empty, data is returned
// unchanged.
//
// An ixlan row that keeps ixf_ixp_member_list_url also keeps
// ixf_ixp_member_list_url_visible: the serializer adds it back
// (serializers.py:4319-4337), and the permission check removes it again
// only when the URL and _visible are the only keys left
// (permissions.py:355-372). The URL key is absent when the caller may not
// see it, so such a row keeps _visible.
func applyFieldProjection(data []any, fields []string) []any {
	if len(fields) == 0 {
		return data
	}
	want := make(map[string]bool, len(fields))
	for _, f := range fields {
		want[f] = true
	}
	addVisible := want[ixfURLKey] && !want[ixfURLVisibleKey]
	out := make([]any, len(data))
	for i, item := range data {
		m, ok := itemToMap(item)
		if !ok {
			out[i] = item
			continue
		}
		projected := make(map[string]any, len(want)+1)
		for k, v := range m {
			if want[k] {
				projected[k] = v
			}
		}
		if v, has := m[ixfURLVisibleKey]; addVisible && has {
			projected[ixfURLVisibleKey] = v
			if _, url := projected[ixfURLKey]; url && len(projected) == 2 {
				delete(projected, ixfURLVisibleKey)
			}
		}
		out[i] = projected
	}
	return out
}

// The ixlan member list URL and its visibility companion, as JSON keys.
const (
	ixfURLKey        = "ixf_ixp_member_list_url"
	ixfURLVisibleKey = "ixf_ixp_member_list_url_visible"
)

// fieldsParam returns the names of ?fields=, or nil when the key is absent
// or its value is empty. Upstream reads the last value
// (QueryDict.get) and splits it on "," with no trimming
// (serializers.py:942-946).
func fieldsParam(params url.Values) []string {
	vals := params["fields"]
	if len(vals) == 0 || vals[len(vals)-1] == "" {
		return nil
	}
	return strings.Split(vals[len(vals)-1], ",")
}

// fieldAccessor holds the struct field index for a JSON-tagged field, plus
// whether the tag carries the `omitempty` option (honoured by structToMap
// below, which must mirror json.Marshal's key-omission semantics).
type fieldAccessor struct {
	index     int
	omitEmpty bool
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
		name, opts, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		omitEmpty := false
		for opt := range strings.SplitSeq(opts, ",") {
			if opt == "omitempty" {
				omitEmpty = true
				break
			}
		}
		m[name] = fieldAccessor{index: i, omitEmpty: omitEmpty}
	}
	fieldMaps.Store(t, m)
	return m
}

// structToMap converts a serializer struct (or pointer to one) to
// map[string]any via the cached field maps, mirroring json.Marshal's
// semantics: keys come from json tags, and `omitempty` fields with empty
// values are dropped. Returns ok=false when v is not a struct (nil
// pointer, scalar, …) so callers can decide their own fallback. This is
// the single struct→map converter for the package — depth.go's toMap and
// itemToMap below are thin adapters over it; it previously existed as two
// near-duplicate walkers that disagreed on omitempty, which let a
// privfield-redacted (zero) gated field keep its KEY under ?fields=
// projection while the unprojected response dropped it.
//
// The omitempty parity is load-bearing: ixLanResponse declares
// `ixf_ixp_member_list_url,omitempty` on a pointer, so a redacted (nil)
// value must drop the key exactly as json.Marshal would. A key with a nil
// value would show anonymous callers that the field exists.
func structToMap(v any) (map[string]any, bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, false
	}
	fm := getFieldMap(rv.Type())
	m := make(map[string]any, len(fm))
	for name, acc := range fm {
		fv := rv.Field(acc.index)
		if acc.omitEmpty && isEmptyJSONValue(fv) {
			continue
		}
		m[name] = fv.Interface()
	}
	return m, true
}

// itemToMap converts a projection item (struct or map) to map[string]any.
// A map[string]any (depth-expanded object) passes through unchanged;
// structs convert via structToMap, honouring omitempty.
func itemToMap(item any) (map[string]any, bool) {
	if m, ok := item.(map[string]any); ok {
		return m, true
	}
	return structToMap(item)
}
