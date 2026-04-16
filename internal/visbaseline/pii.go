// Package visbaseline captures and diffs unauthenticated vs authenticated
// PeeringDB API responses for all 13 object types, with a strict no-PII-in-repo
// guarantee enforced by the Redact function in this package.
//
// The central hard constraint (phase 57 D-02) is: raw PeeringDB data must NOT
// be committed in raw form. Authenticated responses carry email, phone, and
// legal-name data that upstream withholds from anonymous callers; this package
// inspects those responses in memory, emits a placeholder-only shape, and
// produces a structural diff report for downstream test fixtures.
package visbaseline

// PIIFields is the authoritative allow-list of field names that carry
// personal data in PeeringDB responses. Any field whose JSON name appears
// in this list is ALWAYS replaced with a placeholder in redacted auth
// fixtures, regardless of whether the same field is present in the
// corresponding anonymous response.
//
// Derived from internal/peeringdb/types.go: Organization, Facility,
// InternetExchange, Poc, Campus, and association tables (netfac, ixfac).
// The list is deliberately conservative — a field is PII if it identifies
// or locates an individual or organisation's physical presence.
//
// Deliberately excluded (not PII by this policy):
//   - notes: business-owned free text, publishable.
//   - country: geographic region, not a locating identifier on its own.
//   - website, url, looking_glass, route_server: publishable URLs.
//   - name_long, aka: organisation/network display names, not personal names.
//
// The list is sorted for stable review diffs when it is updated.
var PIIFields = []string{
	"address1",
	"address2",
	"city",
	"email",
	"latitude",
	"longitude",
	"name",
	"phone",
	"policy_email",
	"policy_phone",
	"sales_email",
	"sales_phone",
	"state",
	"tech_email",
	"tech_phone",
	"zipcode",
}

// piiSet is an internal O(1) lookup table built from PIIFields at package init.
var piiSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(PIIFields))
	for _, f := range PIIFields {
		m[f] = struct{}{}
	}
	return m
}()

// IsPIIField reports whether the given JSON field name is in the PII allow-list.
// Comparison is exact (case-sensitive) to match PeeringDB's lowercase-underscore
// field naming convention.
func IsPIIField(name string) bool {
	_, ok := piiSet[name]
	return ok
}
