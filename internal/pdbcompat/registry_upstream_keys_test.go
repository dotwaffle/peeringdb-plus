package pdbcompat

import (
	"slices"
	"strings"
	"testing"
)

// upstreamQueryKeys lists the Registry Fields keys that upstream handles
// before its model-field filters, in a serializer's prepare_query or
// finalize_query_params. Keyed "<type>.<field>".
var upstreamQueryKeys = map[string]string{
	"fac.org_name":   "prepare_query seed (serializers.py:2100, :2115-2117)",
	"fac.net_count":  "prepare_query seed, network_count (serializers.py:2102, :2119-2124)",
	"net.fac_count":  "prepare_query seed, facility_count (serializers.py:3729, :3743-3748)",
	"ix.net_count":   "prepare_query seed, network_count (serializers.py:4516, :4531-4536)",
	"ix.fac_count":   "prepare_query seed, facility_count (serializers.py:4517, :4538-4543)",
	"net.info_type":  "finalize_query_params (serializers.py:3765-3813)",
	"netixlan.name":  "prepare_query seed (serializers.py:3161)",
	"netixlan.ix_id": "prepare_query seed (serializers.py:3161)",
	"netfac.name":    "prepare_query seed (serializers.py:3417)",
	"netfac.city":    "prepare_query seed (serializers.py:3417)",
	"netfac.country": "prepare_query seed (serializers.py:3417)",
	"ixfac.name":     "prepare_query seed (serializers.py:2840)",
	"ixfac.city":     "prepare_query seed (serializers.py:2840)",
	"ixfac.country":  "prepare_query seed (serializers.py:2840)",
}

// TestRegistryFields_UpstreamKeyClass puts every Registry Fields key in
// one class and checks that the parser treats it to match upstream:
//   - A FK column (a TypeConfig.ForeignKeys value) filters as the FK.
//   - A key that queryable_field_xl renames (serializers.py:428-438) or
//     that is not an upstream model field (TypeConfig.NonModelFields)
//     must be an upstream query key or be in TypeConfig.UpstreamIgnored.
//   - Every other key is an upstream model field under the same name.
//
// A new net_ or fac_ column fails the test until someone decides how
// upstream treats it.
func TestRegistryFields_UpstreamKeyClass(t *testing.T) {
	t.Parallel()
	for typ, tc := range Registry {
		for field := range tc.Fields {
			key := typ + "." + field
			if slices.Contains(mapValues(tc.ForeignKeys), field) {
				continue
			}
			renamed := upstreamFieldName(field) != stripIDSuffix(field)
			nonModel := tc.NonModelFields[field]
			_, queryKey := upstreamQueryKeys[key]
			ignored := tc.UpstreamIgnored[field]
			switch {
			case queryKey && ignored:
				t.Errorf("%s is an upstream query key and also in UpstreamIgnored", key)
			case (renamed || nonModel) && !queryKey && !ignored:
				t.Errorf("%s: upstream does not filter it as a model field (renamed=%v, not a model field=%v); add it to UpstreamIgnored or upstreamQueryKeys",
					key, renamed, nonModel)
			case ignored && !renamed && !nonModel:
				t.Errorf("%s is in UpstreamIgnored, but it is an upstream model field", key)
			}
		}
		for name, set := range map[string]map[string]bool{
			"UpstreamIgnored": tc.UpstreamIgnored,
			"NonModelFields":  tc.NonModelFields,
		} {
			for field := range set {
				if _, ok := tc.Fields[field]; !ok {
					t.Errorf("%s.%s[%q] is not in Fields", typ, name, field)
				}
			}
		}
	}
	for key := range upstreamQueryKeys {
		typ, field, _ := strings.Cut(key, ".")
		if _, ok := Registry[typ].Fields[field]; !ok {
			t.Errorf("stale entry %q: not a Registry field", key)
		}
	}
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
