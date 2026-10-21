package pdbcompat

import (
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// filterExclusions follows PeeringDB 2.84.0 serializers.py:140-196.
// Bare names exclude a field or relation wherever a relation key reaches it.
// Compound names exclude adjacent relation and field segments.
var filterExclusions = map[string]bool{
	"org__latitude":                      true,
	"org__longitude":                     true,
	"ixlan_set__descr":                   true,
	"ixlan__descr":                       true,
	"ixlan_set__ixf_ixp_member_list_url": true,
	"ixf_ixp_member_list_url":            true,
	"ixlan__ixf_ixp_member_list_url":     true,
	"network__notes_private":             true,
	"net_set__notes_private":             true,
	"notes_private":                      true,
	"api_keys":                           true,
	"oauth_applications":                 true,
	"network_email_set":                  true,
	"ix_email_set":                       true,
	"ixf_member_data_set":                true,
	"ixf_set":                            true,
	"affiliation_requests":               true,
	"email_domains":                      true,
	"restrict_user_emails":               true,
	"periodic_reauth":                    true,
	"periodic_reauth_period":             true,
	"org_flags":                          true,
	"flagged":                            true,
	"flagged_date":                       true,
	"ixf_ixp_import_error":               true,
	"ixf_ixp_import_error_notified":      true,
	"ixf_ixp_import_protocol_conflict":   true,
	"irr_as_set_status":                  true,
	"irr_as_set_verified":                true,
	"irr_as_set_missing_since":           true,
	"irr_as_set_auto_prefix_candidate":   true,
	"irr_as_set_auto_prefix_checked":     true,
	"irr_as_set_notified":                true,
	"irr_as_set_verify_notified":         true,
	"irr_as_set_cap_notified":            true,
	"rir_status_notified":                true,
	"ixf_import_request_user":            true,
	"ixf_import_log_set__id":             true,
	"ixf_import_log_set__created":        true,
	"ixf_import_log_set__updated":        true,
	"ixf_import_log_entries__id":         true,
	"ixf_import_log_entries__action":     true,
	"ixf_import_log_entries__reason":     true,
	"sponsorshiporg_set__id":             true,
	"sponsorshiporg_set__url":            true,
	"partnerships__id":                   true,
	"partnerships__url":                  true,
	"merged_to__id":                      true,
	"merged_to__created":                 true,
	"merged_from__id":                    true,
	"merged_from__created":               true,
}

// filterNameForms follows serializers.py:670-684. Removing every trailing
// _id takes linear time, including long suffix runs with a nonmatching end.
func filterNameForms(name string) [4]string {
	base := name
	for strings.HasSuffix(base, "_id") {
		base = base[:len(base)-3]
	}
	if base == "" {
		base = name
	}
	return [4]string{name, base, queryableFieldXL(name), queryableFieldXL(base)}
}

// filterExcluded checks a relation key before translation and truncation
// (2.84.0 serializers.py:687-705). The cap counts segments without a lookup.
func filterExcluded(path string) bool {
	if strings.Count(path, "__") >= 4 {
		return true
	}
	var previous [4]string
	for i, part := range strings.Split(path, "__") {
		forms := filterNameForms(part)
		for _, form := range forms {
			if filterExclusions[form] {
				return true
			}
			if i > 0 {
				for _, left := range previous {
					if filterExclusions[left+"__"+form] {
						return true
					}
				}
			}
		}
		previous = forms
	}
	return false
}

// relationFilterExcluded checks both input and emitted keys, as upstream
// get_relation_filters does (2.84.0 serializers.py:714,756).
func relationFilterExcluded(key string) bool {
	path := key
	if i := strings.LastIndex(path, "__"); i >= 0 && metaOperators[path[i+2:]] {
		path = path[:i]
	}
	if filterExcluded(path) {
		return true
	}
	parts := strings.Split(key, "__")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	for i := range parts {
		parts[i] = queryableFieldXL(parts[i])
	}
	return filterExcluded(strings.Join(parts, "__"))
}

// modelFilterExcluded keeps contact details out of the local and traversal
// filter vocabulary (2.84.0 serializers.py:206-216,1070-1107).
func modelFilterExcluded(typ, field string) bool {
	if typ != peeringdb.TypePoc {
		return false
	}
	switch field {
	case "name", "phone", "email", "url":
		return true
	}
	return false
}
