package parity

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// Sources: PeeringDB 2.84.0 tests/test_api_contact_filters.py and
// serializers.py:206-216,1070-1107. Exclusions affect filters, not output.
func TestParity_ContactFilterExclusions(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	mustOrg(ctx, t, c, 1, "ContactOrg", now)
	mustNet(ctx, t, c, 100, "ContactNetA", 64500, 1, now)
	mustNet(ctx, t, c, 101, "ContactNetB", 64501, 1, now)
	for i, visibility := range []string{"Public", "Users", "Private", "Public"} {
		net := 100
		if i == 3 {
			net = 101
		}
		c.Poc.Create().SetID(200 + i).SetNetID(net).SetVisible(visibility).
			SetRole([]string{"Technical", "NOC", "Policy", "Sales"}[i]).
			SetName(fmt.Sprintf("Contact%d", i)).SetPhone(fmt.Sprintf("+1206555010%d", i)).
			SetEmail(fmt.Sprintf("contact%d@example.invalid", i)).SetURL(fmt.Sprintf("https://example.invalid/%d", i)).
			SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	}
	for name, tier := range map[string]privctx.Tier{"public": privctx.TierPublic, "users": privctx.TierUsers} {
		t.Run(name, func(t *testing.T) {
			srv := newTestServerWithTier(t, c, tier)
			pocs := []int{200, 203}
			if tier == privctx.TierUsers {
				pocs = []int{200, 201, 203}
			}
			for field, matching := range map[string]string{"name": "Contact0", "phone": "+12065550100", "email": "contact0@example.invalid", "url": "https://example.invalid/0"} {
				for _, key := range []string{field, field + "__contains", field + "__startswith", field + "__in", field + "__iexact", field + "__gt", field + "_id", strings.ToUpper(field)} {
					for _, value := range []string{"missing", "", matching} {
						q := url.Values{key: {value}}.Encode()
						assertKeysSilentlyIgnored(t, srv, []silentIgnoreCase{
							{path: "/api/poc?" + q, want: pocs},
							{path: "/api/poc/200?" + q, want: []int{200}},
							{path: "/api/net?" + url.Values{"poc_set__" + key: {value}}.Encode(), want: []int{100, 101}},
							{path: "/api/net/100?" + url.Values{"poc__" + key: {value}}.Encode(), want: []int{100}},
						})
					}
				}
			}
			assertKeysResolve(t, srv, []silentIgnoreCase{
				{path: "/api/poc?role=Sales", want: []int{203}},
				{path: "/api/poc?visible=Public", want: []int{200, 203}},
				{path: "/api/poc?net_id=101", want: []int{203}},
				{path: "/api/poc?id=200", want: []int{200}},
				{path: "/api/net?poc__role=Sales", want: []int{101}},
				{path: "/api/net?poc__visible=Private", want: []int{}},
				{path: "/api/net?name=ContactNetA", want: []int{100}},
			})
			status, body := httpGet(t, srv, "/api/poc/200?email=missing")
			if status != http.StatusOK || !strings.Contains(string(body), "contact0@example.invalid") {
				t.Errorf("admitted contact output changed: status %d, body %s", status, body)
			}
			status, body = httpGet(t, srv, "/api/poc/202?email=missing")
			if status != http.StatusNotFound {
				t.Errorf("private contact: status %d, want 404: %s", status, body)
			}
		})
	}
}

// Source: PeeringDB 2.84.0 tests/test_api_credential_filters.py.
// Private columns and relations are ignored before key translation or truncation.
func TestParity_RelationFilterExclusions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	c := seedRelationSeedKeys(t, now)
	mustOrg(t.Context(), t, c, 2, "OtherOrganization", now)
	mustFac(t.Context(), t, c, 401, "OtherFacility", 1, now)
	for name, tier := range map[string]privctx.Tier{"public": privctx.TierPublic, "users": privctx.TierUsers} {
		t.Run(name, func(t *testing.T) {
			srv := newTestServerWithTier(t, c, tier)
			for _, key := range []string{
				"ixlan__descr", "ixlan__descr__iexact", "ixlan__name__notes_private", "ixlan_id__descr_id_id__regex",
				"ixlan__ixf_ixp_member_list_url", "ixlan__ixf_ixp_member_list_url__in",
				"ix__api_keys_id_id__name", "ix__oauth_applications__name",
				"ix__affiliation_requests__id", "ix__ixf_import_request_user_id_id",
				"fac__ixf_member_data_set_id_id", "ixlan__ixf_ixp_import_error_id_id",
				"ixlan__ixf_set__name", "ix__merged_to_id__created", "ix__partnerships_id__url",
			} {
				for _, value := range []string{"missing", ""} {
					q := url.Values{key: {value}}.Encode()
					assertKeysSilentlyIgnored(t, srv, []silentIgnoreCase{
						{path: "/api/net?" + q, want: []int{100, 101, 102}},
						{path: "/api/net/100?" + q, want: []int{100}},
					})
				}
			}
			assertKeysSilentlyIgnored(t, srv, []silentIgnoreCase{
				{path: "/api/org?org_flags=1", want: []int{1, 2}},
				{path: "/api/org?org_flags__gt=0", want: []int{1, 2}},
				{path: "/api/net?org__org_flags=1", want: []int{100, 101, 102}},
				{path: "/api/ix?ixf_import_request_user=1", want: []int{20, 21, 22}},
				{path: "/api/fac?net__notes_private=x", want: []int{400, 401}},
				{path: "/api/ix?ixlan__ixf_ixp_member_list_url=x", want: []int{20, 21, 22}},
				{path: "/api/fac?net__notes_private_id_id=x", want: []int{400, 401}},
				{path: "/api/fac?net__net_email_set_id_id=x", want: []int{400, 401}},
				{path: "/api/fac?net__irr_as_set_status_id_id=x", want: []int{400, 401}},
			})
		})
	}
}
