package pdbcompat

import (
	"context"
	"net/url"
	"strings"

	"entgo.io/ent/dialect/sql"
)

// ctfParam is the upstream flag that also applies a date filter to the
// rows of every _set (2.83.0 rest.py:654-655, serializers.py:998-1002).
// The API cache generator sends it (pdb_api_cache.py:195-199).
const ctfParam = "_ctf"

// ctfOps are the operators of the upstream filter loop (rest.py:620).
// With _ctf, a date key with one of them sets request._ctf (:633-655).
// An __in date key never reaches the _set filter: the loop fails on the
// datetime value first (see docs/API.md § Known Divergences).
var ctfOps = map[string]bool{
	"lt": true, "lte": true, "gt": true, "gte": true,
	"contains": true, "startswith": true,
}

// ctfFields are the date fields that every model has (django-handleref
// models.py:88-89). Upstream filters the related model of each _set
// with the key as given, so only these fields can filter every _set.
var ctfFields = map[string]bool{"created": true, "updated": true}

// noteCTF records a key that sets the upstream _ctf filter. A created or
// updated key of the listed type records its predicate, which filters
// the rows of a _set in the same way. Another date key records nil:
// upstream fails with a FieldError when it filters a _set with it (see
// docs/API.md § Known Divergences), and the mirror then filters no _set.
// A key that is not a date key records nothing.
func (st *filterState) noteCTF(key string, relSegs []string, field, op string, p func(*sql.Selector)) {
	if st.ctf == nil || !ctfOps[op] {
		return
	}
	if len(relSegs) == 0 && ctfFields[field] && key == field+"__"+op {
		st.ctf[key] = p
		return
	}
	if keyFieldType(st.tc, relSegs, field) == FieldTime {
		st.ctf[key] = nil
	}
}

// keyFieldType returns the type of the field that a local or 1-hop key
// filters, or FieldString when it cannot tell.
func keyFieldType(tc TypeConfig, relSegs []string, field string) FieldType {
	switch len(relSegs) {
	case 0:
		if _, ft, ok := resolveLocalField(tc, field); ok {
			return ft
		}
	case 1:
		if e, ok := LookupEdge(tc.Name, traversalKeyFor(tc, relSegs[0])); ok {
			if ft, ok := traversalTargetField(Registry[e.TargetType], field); ok {
				return ft
			}
		}
	}
	return FieldString
}

// pickCTF returns the _set filter of the date key that the upstream
// filter loop reads last, or nil. The loop reads query_params.items(),
// a QueryDict, which keeps the keys in the order of their first
// appearance in the query string, and each key sets request._ctf again
// (rest.py:564, :654-655). url.Values does not keep the key order, so
// the order comes from the raw query.
func pickCTF(rawQuery string, cands map[string]func(*sql.Selector)) func(*sql.Selector) {
	if len(cands) == 0 {
		return nil
	}
	var last func(*sql.Selector)
	seen := map[string]bool{}
	for pair := range strings.SplitSeq(rawQuery, "&") {
		k, _, _ := strings.Cut(pair, "=")
		k, err := url.QueryUnescape(k)
		if err != nil || seen[k] {
			continue
		}
		seen[k] = true
		if p, ok := cands[k]; ok {
			last = p
		}
	}
	return last
}

type setDateFilterKey struct{}

// withSetDateFilter returns ctx with the _set filter p. A nil p returns
// ctx unchanged.
func withSetDateFilter(ctx context.Context, p func(*sql.Selector)) context.Context {
	if p == nil {
		return ctx
	}
	return context.WithValue(ctx, setDateFilterKey{}, p)
}

// setDateFilter returns the _ctf filter of the request for the rows of a
// _set, or a filter that keeps every row. Every query of a _set rows ANDs
// it, at every depth and nesting level, as upstream applies it in
// prefetch_query to each Prefetch of prefetch_related
// (serializers.py:1137-1149). The related object of a through set (the
// facility of ix fac_set, the network of ixlan net_set) is not filtered:
// upstream filters the join rows.
func setDateFilter(ctx context.Context) func(*sql.Selector) {
	if p, ok := ctx.Value(setDateFilterKey{}).(func(*sql.Selector)); ok {
		return p
	}
	return func(*sql.Selector) {}
}
