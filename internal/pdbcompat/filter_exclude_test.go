package pdbcompat

import (
	"net/url"
	"slices"
	"strings"
	"testing"
)

// Source: PeeringDB 2.84.0 tests/test_api_credential_filters.py,
// test_relation_route_drops_every_form and test_relation_route_drops_compound_entries.
func TestFilterExclusions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		key      string
		excluded bool
	}{
		{"ixlan__descr__iexact", true},
		{"ixlan_id_id__descr_id_id__regex", true},
		{"ix__api_keys_id_id__name", true},
		{"ixlan__name__notes_private", true},
		{"net__net_email_set_id_id", true},
		{"net__notes_private_id_id__in", true},
		{"fac__ixf_member_data_set__error", true},
		{"ix__merged_to_id__created", true},
		{"ixlan__ixf_ixp_member_list_url_visible", false},
		{"ix__name__startswith", false},
		{"net__name", false},
		{"net__poc_set__email", false},
		{"ix__" + strings.Repeat("_id", 3000), false},
		{strings.Repeat("fac_id__", 50) + "name", true},
		{"ix__a" + strings.Repeat("_id", 2600) + "x", false},
	} {
		t.Run(tc.key, func(t *testing.T) {
			if got := relationFilterExcluded(tc.key); got != tc.excluded {
				t.Errorf("excluded = %v, want %v", got, tc.excluded)
			}
		})
	}
}

func TestExcludedFilterCacheGate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		typ, key, value string
		filtered        bool
	}{
		{"poc", "name", "missing", false},
		{"poc", "email__in", "", false},
		{"net", "poc_set__phone", "missing", false},
		{"net", "poc__url__in", "", false},
		{"net", "ixlan__descr__iexact", "missing", false},
		{"fac", "net__notes_private_id_id", "missing", false},
		{"ix", "ixlan__ixf_ixp_import_error_id_id", "missing", false},
		{"ix", "ixlan__ixf_ixp_member_list_url", "missing", false},
		{"net", "name", "missing", true},
		{"poc", "role", "NOC", true},
		{"net", "poc__role", "NOC", true},
		{"ix", "ixlan__ixf_ixp_member_list_url_visible", "Public", true},
		{"ix", "ixlan__ixlan_id", "200", true},
	} {
		t.Run(tc.typ+"/"+tc.key, func(t *testing.T) {
			ctx := WithUnknownFields(t.Context())
			lf, err := parseListFilters(ctx, url.Values{tc.key: {tc.value}}, Registry[tc.typ], nil)
			if err != nil {
				t.Fatal(err)
			}
			if lf.upstreamFilter != tc.filtered || lf.emptyResult {
				t.Errorf("cache gate = %v, empty = %v, want filtered=%v, empty=false", lf.upstreamFilter, lf.emptyResult, tc.filtered)
			}
			if !tc.filtered && !slices.Contains(UnknownFieldsFromCtx(ctx), tc.key) {
				t.Errorf("excluded key missing from diagnostics: %v", UnknownFieldsFromCtx(ctx))
			}
		})
	}
}
