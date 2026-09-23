package pdbcompat

import "strings"

// This file ports the key translation of the upstream filter loop
// (PeeringDB 2.83.0 rest.py:608-631 and serializers.py:403-441). Upstream
// keys use the Django field names of a model (org, network, facility),
// while the mirror's Fields use the local column names (org_id, net_id,
// fac_id). The helpers map an upstream spelling onto the local column or
// traversal key that means the same thing.

// stripIDSuffix removes one "_id" suffix when key matches the upstream
// pattern ^.+[^_]_id$ (rest.py:608-610, serializers.py:411-414). The
// character before "_id" must not be an underscore, so "net__id" keeps
// its suffix.
func stripIDSuffix(key string) string {
	if len(key) >= 5 && strings.HasSuffix(key, "_id") && key[len(key)-4] != '_' {
		return key[:len(key)-3]
	}
	return key
}

// renameNetFac applies the renames of upstream queryable_field_xl
// (serializers.py:416-438): "fac" and "net" become "facility" and
// "network", and a leading "net_" or "fac_" becomes "network_" or
// "facility_". The prefix rules need at least one character after the
// underscore.
func renameNetFac(key string) string {
	switch {
	case key == "fac":
		return "facility"
	case key == "net":
		return "network"
	case len(key) > 4 && strings.HasPrefix(key, "net_"):
		return "network_" + key[4:]
	case len(key) > 4 && strings.HasPrefix(key, "fac_"):
		return "facility_" + key[4:]
	}
	return key
}

// queryableFieldXL is a port of upstream queryable_field_xl
// (serializers.py:403-441): strip one "_id" suffix, then apply the
// net/fac renames.
func queryableFieldXL(key string) string {
	return renameNetFac(stripIDSuffix(key))
}

// upstreamFieldName returns the name that the upstream filter loop looks
// up in field_names for a key without relation segments. The loop strips
// "_id" from a key that is not a model field (rest.py:608-610) and then
// runs queryable_field_xl (rest.py:623-631), which can strip a second
// "_id". So "org", "org_id" and "org_id_id" all become "org", and "fac"
// and "fac_id" become "facility". The operator path strips in the other
// order (xl first, then rest.py:625-627), which gives the same name for
// every key that names an FK.
func upstreamFieldName(field string) string {
	return queryableFieldXL(stripIDSuffix(field))
}

// resolveLocalField returns the column and type that a key without
// relation segments filters on tc, or ok=false when the key is unknown.
// A local column name matches first. Otherwise a key that names a
// forward FK in upstream spelling (org, network, facility_id, ...)
// filters the local FK column, as upstream filters <fk>_id
// (rest.py:670-677). The operator stays unchanged, so <fk>__in and the
// comparisons compare the FK id, and <fk>__contains is a 400 on the int
// column, as upstream raises FieldError (rest.py:702-703).
//
// A key in tc.UpstreamIgnored is unknown, as upstream ignores it.
func resolveLocalField(tc TypeConfig, field string) (string, FieldType, bool) {
	if tc.UpstreamIgnored[field] {
		return "", 0, false
	}
	if ft, ok := tc.Fields[field]; ok {
		return field, ft, true
	}
	col, ok := tc.ForeignKeys[upstreamFieldName(field)]
	if !ok {
		return "", 0, false
	}
	ft, ok := tc.Fields[col]
	return col, ft, ok
}

// traversalKeyFor maps the first relation segment of a key onto a
// traversal key of tc. A traversal key of the mirror passes unchanged.
// Otherwise the segment gets the upstream net/fac renames, and when the
// result names a forward FK of the type, the key of the edge that owns
// the FK column is returned: ?network__asn= and ?facility__name= walk the
// net and fac edges, as upstream resolves them through
// queryable_relations (serializers.py:970-996). The segment gets no
// "_id" strip: upstream runs xl on the whole key, so a relation segment
// such as net_id in net_id__name becomes network_id and matches nothing.
func traversalKeyFor(tc TypeConfig, seg string) string {
	if _, ok := LookupEdge(tc.Name, seg); ok {
		return seg
	}
	col, ok := tc.ForeignKeys[renameNetFac(seg)]
	if !ok {
		return seg
	}
	for _, e := range Edges[tc.Name] {
		if !e.Excluded && e.OwnFK && e.ParentFKColumn == col {
			return e.TraversalKey
		}
	}
	return seg
}

// namesFKColumn reports whether the field of a relation key ends in
// "_id" the way upstream's strip pattern ^.+[^_]_id$ requires
// (rest.py:608-610). The relation segments before the field supply the
// ".+" part, so the field needs only a non-underscore before "_id".
// Upstream strips the suffix from the whole key, so net__org_id becomes
// net__org, and then network__org after queryable_field_xl. That names
// a FK of the related type, and queryable_relations leaves FK fields out
// (serializers.py:991-995), so upstream ignores the key. Every Registry
// field that ends in "_id" is a FK column.
func namesFKColumn(field string) bool {
	return len(field) >= 4 && strings.HasSuffix(field, "_id") && field[len(field)-4] != '_'
}
