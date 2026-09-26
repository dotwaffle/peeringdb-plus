package pdbcompat

import (
	"testing"

	"entgo.io/ent/dialect/sql"
)

func TestPickCTF(t *testing.T) {
	t.Parallel()
	mark := func(name string, hit *string) func(*sql.Selector) {
		return func(*sql.Selector) { *hit = name }
	}
	for _, tc := range []struct {
		name, raw, want string
	}{
		{"last key wins", "updated__lte=1&updated__gte=2", "gte"},
		{"first appearance keeps place", "updated__lte=1&updated__gte=2&updated__lte=3", "gte"},
		{"escaped key", "updated%5F%5Flte=1", "lte"},
		{"other date key clears", "updated__lte=1&rir_status_updated__gt=2", ""},
		{"no candidate", "name=x", ""},
		{"bad escape skipped", "updated__lte=1&%zz=2", "lte"},
	} {
		var hit string
		cands := map[string]func(*sql.Selector){
			"updated__lte":           mark("lte", &hit),
			"updated__gte":           mark("gte", &hit),
			"rir_status_updated__gt": nil,
		}
		if p := pickCTF(tc.raw, cands); p != nil {
			p(nil)
		}
		if hit != tc.want {
			t.Errorf("%s: picked %q, want %q", tc.name, hit, tc.want)
		}
	}
}
