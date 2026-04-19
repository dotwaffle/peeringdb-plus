package pdbcompat

import (
	"reflect"
	"testing"
)

func TestWithPrepareQueryAllow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{name: "empty", input: nil, want: []string{}},
		{name: "single_hop", input: []string{"org__name"}, want: []string{"org__name"}},
		{name: "mixed_hops", input: []string{"org__name", "ixlan__ix__fac_count"}, want: []string{"org__name", "ixlan__ix__fac_count"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ann := WithPrepareQueryAllow(tc.input...)
			if got := ann.Name(); got != "PrepareQueryAllow" {
				t.Errorf("Name() = %q, want PrepareQueryAllow", got)
			}
			if !reflect.DeepEqual(ann.Fields, tc.want) {
				t.Errorf("Fields = %v, want %v", ann.Fields, tc.want)
			}
		})
	}
}

// TestWithPrepareQueryAllow_NoAliasing ensures callers can mutate their
// source slice without affecting the annotation's stored Fields.
func TestWithPrepareQueryAllow_NoAliasing(t *testing.T) {
	t.Parallel()
	src := []string{"org__name"}
	ann := WithPrepareQueryAllow(src...)
	src[0] = "mutated"
	if ann.Fields[0] != "org__name" {
		t.Errorf("aliasing detected: Fields[0] = %q, want org__name", ann.Fields[0])
	}
}

func TestWithFilterExcludeFromTraversal(t *testing.T) {
	t.Parallel()
	ann := WithFilterExcludeFromTraversal()
	if got := ann.Name(); got != "FilterExcludeFromTraversal" {
		t.Errorf("Name() = %q, want FilterExcludeFromTraversal", got)
	}
}

// TestAllowlistEntry_Shape is a structural smoke test — locks
// AllowlistEntry field shape against silent drift. Plan 70-02's codegen
// emits values of this type.
func TestAllowlistEntry_Shape(t *testing.T) {
	t.Parallel()
	entry := AllowlistEntry{
		Direct: []string{"org__name"},
		Via:    map[string][]string{"ixlan": {"ix__fac_count"}},
	}
	if len(entry.Direct) != 1 {
		t.Errorf("Direct len = %d, want 1", len(entry.Direct))
	}
	if got, ok := entry.Via["ixlan"]; !ok || len(got) != 1 {
		t.Errorf("Via[ixlan] = %v, want [ix__fac_count]", got)
	}
}
