package parity

import (
	"net/url"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent/schematypes"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

func TestStoredJSONFilters(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	mustOrg(ctx, t, c, 1, "Social", now)
	mustOrg(ctx, t, c, 2, "Empty", now)
	mustNet(ctx, t, c, 100, "Social", 64500, 1, now)
	mustNet(ctx, t, c, 101, "Empty", 64501, 2, now)
	mustFac(ctx, t, c, 200, "Social", 1, now)
	c.NetworkFacility.Create().SetID(600).SetNetID(100).SetFacID(200).SetLocalAsn(64500).SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	mustIX(ctx, t, c, 300, "Social", 1, now)
	mustCampus(ctx, t, c, 400, "Social", 1, now)
	c.Carrier.Create().SetID(500).SetName("Social").SetOrgID(1).SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	media := []schematypes.SocialMedia{{Service: "twitter", Identifier: "example_100%"}}
	c.Organization.UpdateOneID(1).SetSocialMedia(media).ExecX(ctx)
	c.Network.UpdateOneID(100).SetSocialMedia(media).SetMeta(map[string]any{"b": true, "a": 1}).SetIxpUpdateExclude([]string{"speed"}).ExecX(ctx)
	c.Network.UpdateOneID(101).SetSocialMedia([]schematypes.SocialMedia{}).ExecX(ctx)
	c.Facility.UpdateOneID(200).SetSocialMedia(media).ExecX(ctx)
	c.InternetExchange.UpdateOneID(300).SetSocialMedia(media).ExecX(ctx)
	c.Campus.UpdateOneID(400).SetSocialMedia(media).ExecX(ctx)
	c.Carrier.UpdateOneID(500).SetSocialMedia(media).ExecX(ctx)
	srv := newTestServer(t, c)
	for _, tc := range []silentIgnoreCase{
		{path: "/api/net?social_media=x", want: []int{}},
		{path: "/api/net?social_media__contains=TWITTER", want: []int{100}},
		{path: "/api/net?social_media__contains=100%25", want: []int{100}},
		{path: "/api/net?social_media__contains=100_", want: []int{}},
		{path: "/api/net?social_media=[]", want: []int{101}},
		{path: "/api/net?social_media__in=[],null", want: []int{101}},
		{path: "/api/org?social_media__contains=", want: []int{1}},
		{path: "/api/org?social_media__startswith=[", want: []int{1}},
		{path: "/api/fac?social_media__contains=twitter", want: []int{200}},
		{path: "/api/ix?social_media__contains=twitter", want: []int{300}},
		{path: "/api/campus?social_media__contains=twitter", want: []int{400}},
		{path: "/api/carrier?social_media__contains=twitter", want: []int{500}},
		{path: "/api/net?org__social_media__contains=twitter", want: []int{100}},
		{path: "/api/fac?net__social_media__contains=twitter", want: []int{200}},
		{path: "/api/fac?net__social_media__contains=nomatch", want: []int{}},
		{path: "/api/fac?net__meta=" + url.QueryEscape(`{"a":1,"b":true}`), want: []int{200}},
		{path: "/api/fac?net__ixp_update_exclude__contains=speed", want: []int{200}},
		{path: "/api/net?social_media__contains=" + url.QueryEscape(`' OR 1=1 --`), want: []int{}},
		{path: "/api/net?meta=" + url.QueryEscape(`{ "b": true, "a": 1 }`), want: []int{100}},
		{path: "/api/net?ixp_update_exclude__contains=speed", want: []int{100}},
		{path: "/api/net?ixp_update_exclude__contains=operational", want: []int{}},
		{path: "/api/net/100?social_media__contains=twitter", want: []int{100}},
	} {
		t.Run(tc.path, func(t *testing.T) { assertKeysResolve(t, srv, []silentIgnoreCase{tc}) })
	}
}
