package visbaseline

import (
	"sort"
	"testing"
)

func TestIsPIIField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		field   string
		wantPII bool
	}{
		// Contact fields — PII.
		{name: "email is PII", field: "email", wantPII: true},
		{name: "phone is PII", field: "phone", wantPII: true},
		{name: "name is PII (POC name)", field: "name", wantPII: true},
		{name: "tech_email is PII", field: "tech_email", wantPII: true},
		{name: "tech_phone is PII", field: "tech_phone", wantPII: true},
		{name: "sales_email is PII", field: "sales_email", wantPII: true},
		{name: "sales_phone is PII", field: "sales_phone", wantPII: true},
		{name: "policy_email is PII", field: "policy_email", wantPII: true},
		{name: "policy_phone is PII", field: "policy_phone", wantPII: true},

		// Address fields — PII.
		{name: "address1 is PII", field: "address1", wantPII: true},
		{name: "address2 is PII", field: "address2", wantPII: true},
		{name: "city is PII", field: "city", wantPII: true},
		{name: "state is PII", field: "state", wantPII: true},
		{name: "zipcode is PII", field: "zipcode", wantPII: true},

		// Geocoded from address — PII.
		{name: "latitude is PII", field: "latitude", wantPII: true},
		{name: "longitude is PII", field: "longitude", wantPII: true},

		// Non-PII fields that exist on PeeringDB types.
		{name: "id is not PII", field: "id", wantPII: false},
		{name: "status is not PII", field: "status", wantPII: false},
		{name: "visible is not PII (controlled enum)", field: "visible", wantPII: false},
		{name: "name_long is not PII (org long name)", field: "name_long", wantPII: false},
		{name: "country is not PII (geographic region)", field: "country", wantPII: false},
		{name: "website is not PII", field: "website", wantPII: false},
		{name: "notes is not PII (business-owned)", field: "notes", wantPII: false},
		{name: "url is not PII", field: "url", wantPII: false},

		// Edge cases.
		{name: "empty field name", field: "", wantPII: false},
		{name: "case-sensitive: Email (uppercase E)", field: "Email", wantPII: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsPIIField(tt.field)
			if got != tt.wantPII {
				t.Errorf("IsPIIField(%q) = %v, want %v", tt.field, got, tt.wantPII)
			}
		})
	}
}

func TestPIIFieldsSorted(t *testing.T) {
	t.Parallel()

	if !sort.StringsAreSorted(PIIFields) {
		t.Errorf("PIIFields must be sorted alphabetically for stable review diffs; got %v", PIIFields)
	}
}

func TestPIIFieldsNonEmpty(t *testing.T) {
	t.Parallel()

	if len(PIIFields) == 0 {
		t.Fatal("PIIFields must not be empty")
	}
}
