package grpcserver

import (
	"context"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// TestIxLanToProto_FieldPrivacy is the unit-level ConnectRPC VIS-09
// contract. ixLanToProto MUST nil-out the IxfIxpMemberListUrl wrapper
// when the caller's ctx tier doesn't admit the gated URL, and MUST leave
// the _visible companion populated (D-05). Cover both tier branches plus
// the fail-closed un-stamped ctx path (D-03).
//
// The full 5-surface E2E contract lives in
// cmd/peeringdb-plus/field_privacy_e2e_test.go.
func TestIxLanToProto_FieldPrivacy(t *testing.T) {
	t.Parallel()

	const url = "https://example.test/ix/100/members.json"
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	usersGated := &ent.IxLan{
		ID:                         100,
		IxfIxpMemberListURLVisible: "Users",
		IxfIxpMemberListURL:        url,
		Created:                    now,
		Updated:                    now,
	}
	publicRow := &ent.IxLan{
		ID:                         101,
		IxfIxpMemberListURLVisible: "Public",
		IxfIxpMemberListURL:        url,
		Created:                    now,
		Updated:                    now,
	}

	anon := privctx.WithTier(context.Background(), privctx.TierPublic)
	users := privctx.WithTier(context.Background(), privctx.TierUsers)
	unstamped := context.Background()

	// TierPublic + Users-gated → URL redacted (nil wrapper).
	if got := ixLanToProto(anon, usersGated); got.IxfIxpMemberListUrl != nil {
		t.Errorf("anon + _visible=Users: IxfIxpMemberListUrl = %v, want nil", got.IxfIxpMemberListUrl)
	}
	// _visible companion STILL populated (D-05).
	if got := ixLanToProto(anon, usersGated); got.IxfIxpMemberListUrlVisible.GetValue() != "Users" {
		t.Errorf("anon + _visible=Users: IxfIxpMemberListUrlVisible = %q, want %q",
			got.IxfIxpMemberListUrlVisible.GetValue(), "Users")
	}

	// TierUsers + Users-gated → URL admitted.
	if got := ixLanToProto(users, usersGated); got.IxfIxpMemberListUrl.GetValue() != url {
		t.Errorf("users + _visible=Users: URL = %q, want %q", got.IxfIxpMemberListUrl.GetValue(), url)
	}

	// Public row at either tier → always admitted.
	for name, ctx := range map[string]context.Context{"anon": anon, "users": users} {
		got := ixLanToProto(ctx, publicRow)
		if got.IxfIxpMemberListUrl.GetValue() != url {
			t.Errorf("%s tier + _visible=Public: URL = %q, want %q",
				name, got.IxfIxpMemberListUrl.GetValue(), url)
		}
	}

	// Fail-closed: un-stamped ctx defaults to TierPublic → URL redacted
	// on the Users-gated row (Phase 64 D-03).
	if got := ixLanToProto(unstamped, usersGated); got.IxfIxpMemberListUrl != nil {
		t.Errorf("unstamped ctx + _visible=Users: IxfIxpMemberListUrl = %v, want nil (fail-closed)",
			got.IxfIxpMemberListUrl)
	}
}
