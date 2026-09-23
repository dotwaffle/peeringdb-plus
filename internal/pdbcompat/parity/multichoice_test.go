package parity

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestParity_MultiChoice locks the filters of the multi-value choice
// fields, net info_types and fac available_voltage_services, against
// PeeringDB 2.83.0.
//
// Upstream stores such a field as one string: the selected choices in
// the order of the choice list, joined with commas (django-peeringdb
// 969dd11 fields.py:61-71, const.py:114-125, :203-208). The filters
// compare that string. The API returns the values in no fixed order, so
// the seeds below store them out of choice-list order.
//
// NetworkSerializer.finalize_query_params rewrites the legacy info_type
// keys, and info_types with __in or __startswith, before the filter loop
// (serializers.py:3768-3813, rest.py:559-563).
func TestParity_MultiChoice(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

	// Net IDs and their info_types. The upstream string is in comments.
	const (
		netNSP     = 1 // "NSP"
		netBoth    = 2 // "NSP,Content"
		netNone    = 3 // ""
		netRouting = 4 // "Educational/Research,Route Server"
	)
	// Fac IDs and their available_voltage_services.
	const (
		facTwo  = 10 // "No Power,48 VDC"
		facOne  = 11 // "400 VAC"
		facNone = 12 // ""
	)

	seed := func(t *testing.T) *ent.Client {
		t.Helper()
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "MultiChoice Org", t0)
		for id, types := range map[int][]string{
			netNSP:     {"NSP"},
			netBoth:    {"Content", "NSP"},
			netNone:    {},
			netRouting: {"Route Server", "Educational/Research"},
		} {
			mustNet(ctx, t, c, id, "MultiChoice Net", 64500+id, 1, t0)
			c.Network.UpdateOneID(id).SetInfoTypes(types).SaveX(ctx)
		}
		for id, volts := range map[int][]string{
			facTwo:  {"48 VDC", "No Power"},
			facOne:  {"400 VAC"},
			facNone: {},
		} {
			mustFac(ctx, t, c, id, "MultiChoice Fac", 1, t0)
			c.Facility.UpdateOneID(id).SetAvailableVoltageServices(volts).SaveX(ctx)
		}
		mustIX(ctx, t, c, 20, "MultiChoice IX", 1, t0)
		mustIxLan(ctx, t, c, 20, "", 20, t0)
		for _, n := range []struct{ id, net int }{{30, netNSP}, {31, netBoth}} {
			c.NetworkIxLan.Create().
				SetID(n.id).SetNetID(n.net).SetIxlanID(20).SetIxID(20).
				SetName("MultiChoice IX").SetAsn(64500 + n.net).SetSpeed(1000).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		mustIX(ctx, t, c, 21, "MultiChoice IX 2", 1, t0)
		for _, x := range []struct{ id, ix, fac int }{{40, 20, facTwo}, {41, 21, facOne}} {
			c.IxFacility.Create().
				SetID(x.id).SetIxID(x.ix).SetFacID(x.fac).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		return c
	}
	c := seed(t)
	srv := newTestServer(t, c)

	// check GETs each path and compares the sorted row IDs with want.
	check := func(t *testing.T, want []int, paths ...string) {
		t.Helper()
		for _, path := range paths {
			status, body := httpGet(t, srv, path)
			if status != http.StatusOK {
				t.Errorf("GET %s: status = %d; body=%s", path, status, string(body))
				continue
			}
			got := extractIDs(t, body)
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Errorf("GET %s: IDs = %v, want %v", path, got, want)
			}
		}
	}
	allNets := []int{netNSP, netBoth, netNone, netRouting}
	allFacs := []int{facTwo, facOne, facNone}

	t.Run("legacy_info_type_filters", func(t *testing.T) {
		t.Parallel()
		// upstream: src/peeringdb_server/management/commands/pdb_api_test.py:1015-1094 at 2.83.0
		check(t, []int{netNSP, netBoth}, "/api/net?info_type=NSP")
		check(t, []int{netBoth}, "/api/net?info_type=Content")
		check(t, []int{netNSP, netBoth},
			"/api/net?info_type__in=NSP",
			"/api/net?info_type__in=Content,NSP",
			"/api/net?info_type__in=Enterprise,NSP",
			"/api/net?info_type__contains=NSP",
			"/api/net?info_type__startswith=NS")
		check(t, nil, "/api/net?info_type__in=Enterprise", "/api/net?info_type__startswith=En")
		check(t, []int{netBoth}, "/api/net?info_type__contains=Content", "/api/net?info_type__startswith=Co")
	})

	t.Run("info_type_matches_whole_value_or_string_prefix", func(t *testing.T) {
		t.Parallel()
		// upstream: src/peeringdb_server/serializers.py:3776-3788 at 2.83.0
		// (istartswith on the whole string, or ",v," or ",v" inside it)
		check(t, []int{netRouting}, "/api/net?info_type=Route+Server", "/api/net?info_type=educational")
		check(t, nil, "/api/net?info_type=Research", "/api/net?info_type=Route")
		check(t, allNets, "/api/net?info_type=")
	})

	t.Run("info_types_in_and_startswith_match_items", func(t *testing.T) {
		t.Parallel()
		// upstream: src/peeringdb_server/serializers.py:3794-3811 at 2.83.0
		// (__in is an OR of icontains per stripped item, so an empty
		// item matches every row)
		check(t, []int{netNSP, netBoth, netRouting}, "/api/net?info_types__in=nsp,+server")
		check(t, allNets, "/api/net?info_types__in=", "/api/net?info_types__in=Enterprise,")
		check(t, []int{netRouting}, "/api/net?info_types__startswith=Route")
	})

	t.Run("info_types_in_long_list_filters", func(t *testing.T) {
		t.Parallel()
		// synthesised: upstream ORs one icontains term per item
		// (serializers.py:3794-3801 at 2.83.0), so a list of any length
		// filters and returns 200. 1500 items that match nothing and one
		// item that matches: one SQL term per item would pass the
		// SQLite expression depth limit of 1000.
		items := make([]string, 0, 1501)
		for i := range 1500 {
			items = append(items, fmt.Sprintf("zz%d", i))
		}
		items = append(items, "Content")
		check(t, []int{netBoth},
			"/api/net?info_types__in="+strings.Join(items, ","),
			"/api/net?info_type__in="+strings.Join(items, ","))
	})

	t.Run("info_types_exact_compares_stored_string", func(t *testing.T) {
		t.Parallel()
		// upstream: src/peeringdb_server/rest.py:682-683 at 2.83.0 (iexact
		// with the value as given) and django-peeringdb 969dd11
		// fields.py:61-71 (stored in choice-list order)
		check(t, []int{netBoth}, "/api/net?info_types=NSP,Content", "/api/net?info_types=nsp,content")
		check(t, nil, "/api/net?info_types=Content,NSP")
		check(t, []int{netNSP}, "/api/net?info_types=NSP")
		check(t, []int{netNone}, "/api/net?info_types=")
		check(t, []int{netBoth}, "/api/net?info_types__contains=SP,Con")
	})

	t.Run("info_types_comparison_uses_stored_form", func(t *testing.T) {
		t.Parallel()
		// upstream: django-peeringdb 969dd11 fields.py:61-71 at 2.83.0
		// (get_prep_value converts the value of a comparison: "Content
		// and junk" becomes "Content", and "Z" becomes "")
		check(t, []int{netNone}, "/api/net?info_types__lt=Content+and+junk")
		check(t, nil, "/api/net?info_types__lt=Z")
	})

	t.Run("info_type_other_keys_ignored", func(t *testing.T) {
		t.Parallel()
		// upstream: src/peeringdb_server/models.py:5812-5816 at 2.83.0
		// (info_type is a property, not a field, so rest.py:633 and :670
		// match no field)
		check(t, allNets, "/api/net?info_type__lt=Z", "/api/net?info_type__gt=A")
		// The relation key names a property of the related model, which
		// queryable_relations leaves out (serializers.py:970-995).
		check(t, []int{30, 31}, "/api/netixlan?net__info_type=NSP")
	})

	t.Run("relation_keys_filter_stored_string", func(t *testing.T) {
		t.Parallel()
		// upstream: src/peeringdb_server/rest.py:682-683 at 2.83.0
		// (queryable_relations field, iexact as given)
		check(t, []int{31}, "/api/netixlan?net__info_types=NSP,Content", "/api/netixlan?net__info_types__contains=content")
		// upstream: src/peeringdb_server/models.py:221-234 at 2.83.0
		// (make_relation_filter builds an exact lookup, and get_prep_value
		// converts the value to the stored form)
		check(t, []int{20}, "/api/ix?fac__available_voltage_services=48+VDC,No+Power")
		check(t, nil, "/api/ix?fac__available_voltage_services=48+VDC")
	})

	t.Run("fac_voltage_filters", func(t *testing.T) {
		t.Parallel()
		// upstream: src/peeringdb_server/rest.py:659-666, :682-683 at
		// 2.83.0 and django-peeringdb 969dd11 fields.py:61-71 (an __in
		// item converts to the stored form, so an item with no choice
		// becomes "" and matches the rows without a value)
		check(t, []int{facTwo}, "/api/fac?available_voltage_services=No+Power,48+VDC",
			"/api/fac?available_voltage_services__contains=48",
			"/api/fac?available_voltage_services__startswith=no+power")
		check(t, nil, "/api/fac?available_voltage_services=48+VDC,No+Power")
		check(t, []int{facOne}, "/api/fac?available_voltage_services__in=48+VDC,400+VAC")
		check(t, []int{facNone}, "/api/fac?available_voltage_services__in=junk")
		check(t, allFacs[:2], "/api/fac?available_voltage_services__contains=V")
	})
}
