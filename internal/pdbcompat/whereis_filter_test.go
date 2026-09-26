package pdbcompat

import (
	"encoding/json"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
)

// TestWhereisCandidates checks the list of prefixes that contain an
// address. Each entry must be the canonical text that upstream stores
// (str(ip_network)), so that the IN list matches the stored prefix.
func TestWhereisCandidates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		addr     string
		wantLen  int
		wantHas  []string
		wantNone func(string) bool
	}{
		{
			addr:    "10.0.0.5",
			wantLen: 33,
			wantHas: []string{"0.0.0.0/0", "10.0.0.0/8", "10.0.0.0/24", "10.0.0.4/31", "10.0.0.5/32"},
		},
		{
			addr:    "2001:db8:0:0:1::1",
			wantLen: 129,
			wantHas: []string{"::/0", "2001:db8::/32", "2001:db8::/64", "2001:db8:0:0:1::/80", "2001:db8::1:0:0:1/128"},
		},
		{
			// An IPv4-mapped address stays IPv6, as in Python.
			addr:    "::ffff:10.0.0.5",
			wantLen: 129,
			wantHas: []string{"::ffff:10.0.0.0/120", "::ffff:10.0.0.5/128"},
			wantNone: func(c string) bool {
				return !strings.Contains(c, ":")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()
			got := whereisCandidates(netip.MustParseAddr(tt.addr))
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d: %v", len(got), tt.wantLen, got)
			}
			for _, want := range tt.wantHas {
				if !slices.Contains(got, want) {
					t.Errorf("candidates do not hold %q: %v", want, got)
				}
			}
			for i, c := range got {
				p := netip.MustParsePrefix(c)
				if p.String() != c || p.Masked() != p {
					t.Errorf("candidate %q is not canonical (parsed %q, masked %q)", c, p, p.Masked())
				}
				if p.Bits() != i {
					t.Errorf("candidate %d = %q, want length %d", i, c, i)
				}
				if tt.wantNone != nil && tt.wantNone(c) {
					t.Errorf("candidate %q is an IPv4 prefix", c)
				}
			}
		})
	}
	t.Run("IPv4 ends", func(t *testing.T) {
		t.Parallel()
		got := whereisCandidates(netip.MustParseAddr("10.0.0.5"))
		if got[0] != "0.0.0.0/0" || got[24] != "10.0.0.0/24" || got[32] != "10.0.0.5/32" {
			t.Errorf("got [0]=%q [24]=%q [32]=%q", got[0], got[24], got[32])
		}
	})
	t.Run("zone has no effect", func(t *testing.T) {
		t.Parallel()
		zoned := whereisCandidates(netip.MustParseAddr("2001:db8::1%eth0"))
		plain := whereisCandidates(netip.MustParseAddr("2001:db8::1"))
		if !slices.Equal(zoned, plain) {
			t.Errorf("zoned = %v, want %v", zoned, plain)
		}
	})
}

// TestLookupWhereisKey checks the key forms of upstream
// get_relation_filters with the seed list ["ix_id", "ix", "whereis"]
// (2.83.0 serializers.py:614-656).
func TestLookupWhereisKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ, key   string
		wantInList bool
		wantOK     bool
	}{
		{"ixpfx", "whereis", false, true},
		{"ixpfx", "whereis__lt", false, true},
		{"ixpfx", "whereis__lte", false, true},
		{"ixpfx", "whereis__gt", false, true},
		{"ixpfx", "whereis__gte", false, true},
		{"ixpfx", "whereis__contains", false, true},
		{"ixpfx", "whereis__startswith", false, true},
		{"ixpfx", "whereis__in", true, true},
		// Upstream ignores these forms.
		{"ixpfx", "whereis__iexact", false, false},
		{"ixpfx", "whereis__icontains", false, false},
		{"ixpfx", "whereis__istartswith", false, false},
		{"ixpfx", "whereis__a__b", false, false},
		{"ixpfx", "whereis__in__x", false, false},
		{"ixpfx", "whereis_id", false, false},
		{"ixpfx", "whereisx", false, false},
		{"ixpfx", "xwhereis", false, false},
		{"ixpfx", "prefix", false, false},
		// Only the ixpfx serializer has the key.
		{"ix", "whereis", false, false},
		{"ixlan", "whereis", false, false},
		{"net", "whereis__in", false, false},
	}
	for _, tt := range tests {
		inList, ok := lookupWhereisKey(tt.typ, tt.key)
		if inList != tt.wantInList || ok != tt.wantOK {
			t.Errorf("lookupWhereisKey(%q, %q) = (%v, %v), want (%v, %v)", tt.typ, tt.key, inList, ok, tt.wantInList, tt.wantOK)
		}
	}
}

// TestBuildWhereisPredicate_Errors checks the values that upstream
// ipaddress.ip_address rejects with ValueError, which rest.py:493-500
// returns as 400, and the __in form, which gives ip_address a list.
func TestBuildWhereisPredicate_Errors(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"",
		"abc",
		"10.0.0.0/24",
		"10.0.0.5/32",
		" 10.0.0.5",
		"10.0.0.5 ",
		"010.0.0.5",
		"1.2.3",
		"167772165",
		"2001:db8::1%",
		"2001:db8::1%a%b",
		"2001:db8::1%a/b",
		"10.0.0.5%eth0",
	} {
		if _, err := buildWhereisPredicate(value, false); err == nil {
			t.Errorf("buildWhereisPredicate(%q, false): err = nil, want an error", value)
		} else if !strings.Contains(err.Error(), "does not appear to be an IPv4 or IPv6 address") {
			t.Errorf("buildWhereisPredicate(%q, false): err = %v", value, err)
		}
	}
	for _, value := range []string{"10.0.0.5", ""} {
		if _, err := buildWhereisPredicate(value, true); err == nil {
			t.Errorf("buildWhereisPredicate(%q, true): err = nil, want an error", value)
		}
	}
}

// TestBuildWhereisPredicate_OneJSONArray checks that the prefixes bind
// as one JSON array parameter, not as one SQL term per prefix length.
func TestBuildWhereisPredicate_OneJSONArray(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		value   string
		wantLen int
	}{
		{"10.0.0.5", 33},
		{"2001:DB8:100:0::1", 129},
		{"2001:db8:100::1%eth0", 129},
	} {
		preds, empty, err := ParseFiltersCtx(t.Context(), url.Values{"whereis__gte": {tt.value}}, Registry["ixpfx"])
		if err != nil || empty || len(preds) != 1 {
			t.Fatalf("ParseFiltersCtx(%q): preds=%d empty=%v err=%v, want one predicate", tt.value, len(preds), empty, err)
		}
		s := sql.Dialect(dialect.SQLite).Select("*").From(sql.Table("t"))
		preds[0](s)
		q, args := s.Query()
		if want := "`t`.`prefix` IN (SELECT value FROM json_each(?))"; !strings.Contains(q, want) {
			t.Errorf("%s: SQL = %q, want %q", tt.value, q, want)
		}
		if len(args) != 1 {
			t.Fatalf("%s: args = %v, want one JSON array", tt.value, args)
		}
		var list []string
		if err := json.Unmarshal([]byte(args[0].(string)), &list); err != nil {
			t.Fatalf("%s: arg %v is not a JSON array: %v", tt.value, args[0], err)
		}
		if len(list) != tt.wantLen {
			t.Errorf("%s: %d prefixes, want %d", tt.value, len(list), tt.wantLen)
		}
	}
}
