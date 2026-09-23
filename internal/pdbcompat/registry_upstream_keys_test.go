package pdbcompat

import (
	"slices"
	"strings"
	"testing"
)

// upstreamNonModelFields lists the Registry Fields keys that are not
// fields of the upstream model: serializer fields and model properties.
// The upstream filter loop filters only model fields and
// queryable_relations (2.83.0 rest.py:525-528, :633, :670), so each of
// these keys is either an upstream query key (upstreamQueryKeys) or
// ignored upstream (TypeConfig.UpstreamIgnored). Keyed "<type>.<field>".
var upstreamNonModelFields = map[string]string{
	"fac.org_name":     "serializer field (serializers.py:1947)",
	"net.info_type":    "property (models.py:5812-5816)",
	"netixlan.name":    "property (models.py:6113-6115)",
	"netixlan.ix_id":   "property (models.py:6131-6133)",
	"netfac.name":      "serializer field (serializers.py:3372-3380)",
	"netfac.city":      "serializer field (serializers.py:3372-3380)",
	"netfac.country":   "serializer field (serializers.py:3372-3380)",
	"netfac.local_asn": "property (models.py:6046-6051)",
	"ixfac.name":       "serializer field (serializers.py:2792-2800)",
	"ixfac.city":       "serializer field (serializers.py:2792-2800)",
	"ixfac.country":    "serializer field (serializers.py:2792-2800)",
	"carrier.org_name": "serializer field (serializers.py:2667)",
	"carrierfac.name":  "serializer field (serializers.py:2601)",
	"campus.org_name":  "serializer field (serializers.py:4792)",
	"campus.city":      "property (models.py:2113-2120)",
	"campus.country":   "property (models.py:2122-2129)",
	"campus.state":     "property (models.py:2131-2138)",
	"campus.zipcode":   "property (models.py:2140-2147)",
}

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
//     that is not an upstream model field must be an upstream query key
//     or be in TypeConfig.UpstreamIgnored.
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
			_, nonModel := upstreamNonModelFields[key]
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
		for field := range tc.UpstreamIgnored {
			if _, ok := tc.Fields[field]; !ok {
				t.Errorf("%s.UpstreamIgnored[%q] is not in Fields", typ, field)
			}
		}
	}
	for _, list := range []map[string]string{upstreamNonModelFields, upstreamQueryKeys} {
		for key := range list {
			typ, field, _ := strings.Cut(key, ".")
			if _, ok := Registry[typ].Fields[field]; !ok {
				t.Errorf("stale entry %q: not a Registry field", key)
			}
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
