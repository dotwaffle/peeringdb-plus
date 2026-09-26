package privfield_test

import (
	"context"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/privfield"
)

// TestRedact exercises the full admission matrix plus the fail-closed
// case for an unstamped context. The table mirrors the helper's truth
// table.
func TestRedact(t *testing.T) {
	t.Parallel()

	const url = "https://example.test/members.json"

	publicCtx := privctx.WithTier(context.Background(), privctx.TierPublic)
	usersCtx := privctx.WithTier(context.Background(), privctx.TierUsers)
	bareCtx := context.Background() // un-stamped: TierFrom returns TierPublic

	tests := []struct {
		name     string
		ctx      context.Context
		visible  string
		value    string
		wantOut  string
		wantOmit bool
	}{
		{"public-tier-visible-public", publicCtx, "Public", url, url, false},
		{"public-tier-visible-users", publicCtx, "Users", url, "", true},
		{"public-tier-visible-private", publicCtx, "Private", url, "", true},
		{"public-tier-visible-empty", publicCtx, "", url, "", true},
		{"public-tier-visible-garbage", publicCtx, "garbage", url, "", true},
		{"users-tier-visible-public", usersCtx, "Public", url, url, false},
		{"users-tier-visible-users", usersCtx, "Users", url, url, false},
		{"users-tier-visible-private", usersCtx, "Private", url, "", true},
		{"users-tier-visible-empty", usersCtx, "", url, "", true},
		{"unstamped-ctx-fail-closed", bareCtx, "Users", url, "", true},
		{"unstamped-ctx-public-admits", bareCtx, "Public", url, url, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotOut, gotOmit := privfield.Redact(tc.ctx, tc.visible, tc.value)
			if gotOut != tc.wantOut {
				t.Errorf("out = %q, want %q", gotOut, tc.wantOut)
			}
			if gotOmit != tc.wantOmit {
				t.Errorf("omit = %v, want %v", gotOmit, tc.wantOmit)
			}
		})
	}
}

// TestRedact_Pointer checks Redact with a *string value, the stored type
// of a nullable column. An admitted value comes back as the same pointer,
// so nil stays nil and a pointer to "" stays a pointer to "". An omitted
// value comes back as nil.
func TestRedact_Pointer(t *testing.T) {
	t.Parallel()

	url := "https://example.test/members.json"
	empty := ""

	publicCtx := privctx.WithTier(context.Background(), privctx.TierPublic)
	usersCtx := privctx.WithTier(context.Background(), privctx.TierUsers)
	bareCtx := context.Background() // un-stamped: TierFrom returns TierPublic

	tests := []struct {
		name     string
		ctx      context.Context
		visible  string
		value    *string
		wantOut  *string
		wantOmit bool
	}{
		{"admitted-nil-stays-nil", publicCtx, "Public", nil, nil, false},
		{"admitted-empty-stays-empty", usersCtx, "Users", &empty, &empty, false},
		{"admitted-url", usersCtx, "Public", &url, &url, false},
		{"redacted-url", publicCtx, "Users", &url, nil, true},
		{"redacted-nil", publicCtx, "Private", nil, nil, true},
		{"unstamped-ctx-fail-closed", bareCtx, "Users", &url, nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotOut, gotOmit := privfield.Redact(tc.ctx, tc.visible, tc.value)
			if gotOut != tc.wantOut {
				t.Errorf("out = %p, want %p", gotOut, tc.wantOut)
			}
			if gotOmit != tc.wantOmit {
				t.Errorf("omit = %v, want %v", gotOmit, tc.wantOmit)
			}
		})
	}
}
