package pdbcompat

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
)

// statusIndexUse matches a plan step that reads a status-leading index:
// the single-column <type>_status index or the composite
// <type>_status_updated_created_id index.
var statusIndexUse = regexp.MustCompile(`INDEX \w+_status\b`)

// TestDetailPlan_KeepsFKIndex runs every detail read at depth 1 and 2,
// and the depth-2 in-flight estimate, and checks the plan of each query
// that the read sends. A depth set selects the children of one parent
// with one live status. The app never runs ANALYZE, so SQLite scores
// status = ? as selective as the parent FK equality. It can then read
// every live row of the child table through the status index. likely()
// on the status test keeps the plan on the FK index. No query of a
// detail read may use a status-leading index, and no query of the
// estimate may use one or sort.
func TestDetailPlan_KeepsFKIndex(t *testing.T) {
	t.Parallel()
	seedClient, db := testutil.SetupClientWithDB(t)
	r := seed.Full(t, seedClient)
	ids := map[string]int{
		peeringdb.TypeOrg:        r.Org.ID,
		peeringdb.TypeNet:        r.Network.ID,
		peeringdb.TypeFac:        r.Facility.ID,
		peeringdb.TypeIX:         r.IX.ID,
		peeringdb.TypePoc:        r.Poc.ID,
		peeringdb.TypeIXLan:      r.IxLan.ID,
		peeringdb.TypeIXPfx:      r.IxPrefix.ID,
		peeringdb.TypeNetIXLan:   r.NetworkIxLan.ID,
		peeringdb.TypeNetFac:     r.NetworkFacility.ID,
		peeringdb.TypeIXFac:      r.IxFacility.ID,
		peeringdb.TypeCarrier:    r.Carrier.ID,
		peeringdb.TypeCarrierFac: r.CarrierFacility.ID,
		peeringdb.TypeCampus:     r.Campus.ID,
	}
	for typ, id := range ids {
		for _, depth := range []int{1, 2} {
			t.Run(typ+"_depth"+strconv.Itoa(depth), func(t *testing.T) {
				t.Parallel()
				rec := &recordingDriver{Driver: entsql.OpenDB(dialect.SQLite, db)}
				client := ent.NewClient(ent.Driver(rec))
				if _, err := Registry[typ].Get(t.Context(), client, id, depth); err != nil {
					t.Fatalf("get %s/%d depth %d: %v", typ, id, depth, err)
				}
				for _, q := range rec.queries() {
					if plan := explainPlan(t, db, q.q, q.args); statusIndexUse.MatchString(plan) {
						t.Errorf("query reads a status index:\n  sql:  %s\n  plan: %s", q.q, plan)
					}
				}
				if depth < 2 {
					return
				}
				// The estimate counts each set with one GROUP BY over an
				// FK IN json_each list. The FK index gives the group
				// order, so no step sorts.
				est := &recordingDriver{Driver: entsql.OpenDB(dialect.SQLite, db)}
				detailInflightEstimate(t.Context(), ent.NewClient(ent.Driver(est)), typ, id, depth)
				for _, q := range est.queries() {
					plan := explainPlan(t, db, q.q, q.args)
					if statusIndexUse.MatchString(plan) || strings.Contains(plan, "TEMP B-TREE") {
						t.Errorf("estimate query reads a status index or sorts:\n  sql:  %s\n  plan: %s", q.q, plan)
					}
				}
			})
		}
	}
}

// TestDetailFilterPlan_PrimaryKeyFirst checks the plan of the Match
// query of a detail request with filter keys: the listed table is read
// by its primary key, and no step sorts.
func TestDetailFilterPlan_PrimaryKeyFirst(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		typ   string
		query url.Values
	}{
		{peeringdb.TypeNet, url.Values{"name": {"x"}}},
		{peeringdb.TypeNetIXLan, url.Values{"status": {"ok"}}},
		{peeringdb.TypeNet, url.Values{"ix": {"1"}}},
	} {
		t.Run(tc.typ+"_"+tc.query.Encode(), func(t *testing.T) {
			t.Parallel()
			_, db := testutil.SetupClientWithDB(t)
			rec := &recordingDriver{Driver: entsql.OpenDB(dialect.SQLite, db)}
			client := ent.NewClient(ent.Driver(rec))
			tcfg := Registry[tc.typ]
			preds, empty, err := ParseFiltersCtx(t.Context(), tc.query, tcfg)
			if err != nil || empty {
				t.Fatalf("ParseFiltersCtx: empty=%v err=%v", empty, err)
			}
			if _, err := tcfg.Match(t.Context(), client, 1, preds); err != nil {
				t.Fatalf("Match: %v", err)
			}
			q, args := rec.lastQuery(t)
			plan := explainPlan(t, db, q, args)
			first, _, _ := strings.Cut(plan, " | ")
			want := "SEARCH " + tableFor(t, tc.typ) + " USING INTEGER PRIMARY KEY (rowid=?)"
			if first != want {
				t.Errorf("first plan step = %q, want %q (plan %q)", first, want, plan)
			}
			if strings.Contains(plan, "TEMP B-TREE") {
				t.Errorf("plan sorts: %q", plan)
			}
		})
	}
}

// TestRelationFilterPlan_KeepsFKIndex checks the plan of the relation
// keys whose status pin applies to a row in a subquery. The pinned
// subquery must read the FK index of the related table, not every
// "ok" row through the status index.
func TestRelationFilterPlan_KeepsFKIndex(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		typ, key string
		want     string // FK index that the pinned subquery reads
	}{
		{peeringdb.TypeNet, "ixlan", "networkixlan_ixlan_id"},
		{peeringdb.TypeNet, "ix", "networkixlan_ixlan_id"},
		{peeringdb.TypeNet, "fac", "networkfacility_fac_id"},
		{peeringdb.TypeIX, "net", "networkixlan_net_id"},
		{peeringdb.TypeIX, "fac", "ixfacility_fac_id"},
		{peeringdb.TypeFac, "net", "networkfacility_net_id"},
		{peeringdb.TypeFac, "ix", "ixfacility_ix_id"},
		{peeringdb.TypeNetIXLan, "ix", "ixlan_ix_id"},
		{peeringdb.TypeOrg, "asn", "networks_asn_key"},
	} {
		t.Run(tc.typ+"_"+tc.key, func(t *testing.T) {
			t.Parallel()
			tcfg := Registry[tc.typ]
			preds, empty, err := ParseFiltersCtx(t.Context(), url.Values{tc.key: {"1"}}, tcfg)
			if err != nil || empty {
				t.Fatalf("ParseFiltersCtx(%s=1): empty=%v err=%v", tc.key, empty, err)
			}
			list, _ := listPlans(t, tc.typ, QueryOptions{Filters: preds, Limit: 250})
			if !strings.Contains(list, tc.want) {
				t.Errorf("plan = %q, want it to read %s", list, tc.want)
			}
			listed := " " + tableFor(t, tc.typ) + " "
			for step := range strings.SplitSeq(list, " | ") {
				if statusIndexUse.MatchString(step) && !strings.Contains(step, listed) {
					t.Errorf("subquery reads a status index: %q (plan %q)", step, list)
				}
			}
		})
	}
}

// TestPresencePlan_KeepsNetIndex checks the plan of the presence keys
// that count the networks of each listed row (all_net, asn_overlap).
// The subquery must read the net_id index of the link table, and
// asn_overlap must find the networks through the unique asn index. A
// link status test without likely() reads every link row with that
// status through a status index.
func TestPresencePlan_KeepsNetIndex(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		typ, key, value string
		want            []string // indexes that the plan must read
	}{
		{peeringdb.TypeFac, "asn_overlap", "64500,64501", []string{"networks_asn_key", "networkfacility_net_id"}},
		{peeringdb.TypeIX, "asn_overlap", "64500,64501", []string{"networks_asn_key", "networkixlan_net_id"}},
		{peeringdb.TypeFac, "all_net", "1,2", []string{"networkfacility_net_id"}},
		{peeringdb.TypeIX, "all_net", "1,2", []string{"networkixlan_net_id"}},
	} {
		t.Run(tc.typ+"_"+tc.key, func(t *testing.T) {
			t.Parallel()
			tcfg := Registry[tc.typ]
			preds, empty, err := ParseFiltersCtx(t.Context(), url.Values{tc.key: {tc.value}}, tcfg)
			if err != nil || empty {
				t.Fatalf("ParseFiltersCtx(%s=%s): empty=%v err=%v", tc.key, tc.value, empty, err)
			}
			list, count := listPlans(t, tc.typ, QueryOptions{Filters: preds, Limit: 250})
			listed := " " + tableFor(t, tc.typ) + " "
			for name, plan := range map[string]string{"list": list, "count": count} {
				for _, idx := range tc.want {
					if !strings.Contains(plan, idx) {
						t.Errorf("%s plan = %q, want it to read %s", name, plan, idx)
					}
				}
				for step := range strings.SplitSeq(plan, " | ") {
					if statusIndexUse.MatchString(step) && !strings.Contains(step, listed) {
						t.Errorf("%s subquery reads a status index: %q (plan %q)", name, step, plan)
					}
				}
			}
		})
	}
}

// TestListDepthPlan_KeepsFKIndex checks the plans of the queries of a
// list at depth 1 and 2, with and without ?since, for the 6 types with
// reverse sets: ListIDs, the chunk query of ListDepth, the set loaders
// and the listDepthEstimate counts.
//   - A step on the listed table may read its status index (the default
//     list reads <type>_status), but the chunk query must seek each id
//     (rowid=?), not scan the status range.
//   - A step on a set table must not read a status index. likely() on
//     the status test keeps it on the FK index.
//   - The estimate counts must not sort: the FK index gives the group
//     order. A loader may sort the rows that it fetched for the last
//     ORDER BY terms (the facility order of netfac, ixfac and
//     carrierfac), but never all the rows of a table.
func TestListDepthPlan_KeepsFKIndex(t *testing.T) {
	t.Parallel()
	seedClient, db := testutil.SetupClientWithDB(t)
	seedListDepthRows(t, seedClient)
	for _, typ := range setTypes() {
		for _, depth := range []int{1, 2} {
			for _, since := range []bool{false, true} {
				name := typ + "_depth" + strconv.Itoa(depth)
				if since {
					name += "_since"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					ctx := t.Context()
					var opts QueryOptions
					if since {
						ts := time.Unix(1, 0).UTC()
						opts.Since = &ts
					}
					listed := " " + tableFor(t, typ) + " "
					tc := Registry[typ]

					idsRec := &recordingDriver{Driver: entsql.OpenDB(dialect.SQLite, db)}
					ids, err := tc.ListIDs(ctx, ent.NewClient(ent.Driver(idsRec)), opts)
					if err != nil || len(ids) == 0 {
						t.Fatalf("ListIDs: %v ids, err %v", ids, err)
					}
					chunkRec := &recordingDriver{Driver: entsql.OpenDB(dialect.SQLite, db)}
					if _, err := tc.ListDepth(ctx, ent.NewClient(ent.Driver(chunkRec)), opts, ids, depth, childSets[typ]); err != nil {
						t.Fatalf("ListDepth: %v", err)
					}
					estRec := &recordingDriver{Driver: entsql.OpenDB(dialect.SQLite, db)}
					if _, err := listDepthEstimate(ctx, ent.NewClient(ent.Driver(estRec)), typ, ids, depth, childSets[typ], defaultListDepthChunk); err != nil {
						t.Fatalf("listDepthEstimate: %v", err)
					}

					sawChunk := false
					for _, q := range append(idsRec.queries(), chunkRec.queries()...) {
						plan := explainPlan(t, db, q.q, q.args)
						onListed := strings.Contains(q.q, "FROM `"+strings.TrimSpace(listed)+"`")
						if onListed && strings.Contains(q.q, "json_each") {
							sawChunk = true
							if !strings.Contains(plan, "rowid=?") {
								t.Errorf("chunk query does not seek by id:\n  sql:  %s\n  plan: %s", q.q, plan)
							}
						}
						for step := range strings.SplitSeq(plan, " | ") {
							if statusIndexUse.MatchString(step) && !strings.Contains(step, listed) {
								t.Errorf("set query reads a status index:\n  sql:  %s\n  plan: %s", q.q, plan)
							}
							if strings.Contains(step, "TEMP B-TREE") && !strings.Contains(step, "LAST 2 TERMS") {
								t.Errorf("query sorts every row:\n  sql:  %s\n  plan: %s", q.q, plan)
							}
						}
					}
					if !sawChunk {
						t.Error("no chunk query recorded")
					}
					for _, q := range estRec.queries() {
						plan := explainPlan(t, db, q.q, q.args)
						if statusIndexUse.MatchString(plan) || strings.Contains(plan, "TEMP B-TREE") {
							t.Errorf("estimate query reads a status index or sorts:\n  sql:  %s\n  plan: %s", q.q, plan)
						}
					}
				})
			}
		}
	}
}
