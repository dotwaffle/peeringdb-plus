package pdbtypes

import (
	"slices"
	"testing"
)

// TestAll_GoldenSet locks the canonical set of 13 type names. A rename
// or a 14th entity must update this golden deliberately.
func TestAll_GoldenSet(t *testing.T) {
	t.Parallel()
	want := []string{
		"campus", "carrier", "carrierfac", "fac", "ix", "ixfac",
		"ixlan", "ixpfx", "net", "netfac", "netixlan", "org", "poc",
	}
	if got := SortedNames(); !slices.Equal(got, want) {
		t.Errorf("SortedNames() = %v, want %v", got, want)
	}
}

// TestAll_UniqueAcrossDomains asserts no duplicate names in any of the
// three naming domains — a duplicate would make the reverse lookups
// ambiguous.
func TestAll_UniqueAcrossDomains(t *testing.T) {
	t.Parallel()
	names := map[string]bool{}
	goNames := map[string]bool{}
	djangoModels := map[string]bool{}
	for _, ty := range All {
		if names[ty.Name] {
			t.Errorf("duplicate Name %q", ty.Name)
		}
		if goNames[ty.GoName] {
			t.Errorf("duplicate GoName %q", ty.GoName)
		}
		if djangoModels[ty.DjangoModel] {
			t.Errorf("duplicate DjangoModel %q", ty.DjangoModel)
		}
		names[ty.Name] = true
		goNames[ty.GoName] = true
		djangoModels[ty.DjangoModel] = true
	}
}

// TestNames_CanonicalOrder asserts the parent-before-child invariants
// FK backfill and sync rely on: every parent type stages before its
// children.
func TestNames_CanonicalOrder(t *testing.T) {
	t.Parallel()
	pos := map[string]int{}
	for i, n := range Names() {
		pos[n] = i
	}
	deps := map[string][]string{
		"campus":     {"org"},
		"fac":        {"org"},
		"carrier":    {"org"},
		"carrierfac": {"carrier", "fac"},
		"ix":         {"org"},
		"ixlan":      {"ix"},
		"ixpfx":      {"ixlan"},
		"ixfac":      {"ix", "fac"},
		"net":        {"org"},
		"poc":        {"net"},
		"netfac":     {"net", "fac"},
		"netixlan":   {"net", "ixlan"},
	}
	for child, parents := range deps {
		for _, parent := range parents {
			if pos[parent] >= pos[child] {
				t.Errorf("parent %q (index %d) must precede child %q (index %d)",
					parent, pos[parent], child, pos[child])
			}
		}
	}
}

// TestLookups_RoundTrip exercises the three lookup helpers over every
// entry plus the unknown-input contract.
func TestLookups_RoundTrip(t *testing.T) {
	t.Parallel()
	for _, ty := range All {
		if got, ok := FromGoName(ty.GoName); !ok || got != ty.Name {
			t.Errorf("FromGoName(%q) = %q, %v; want %q, true", ty.GoName, got, ok, ty.Name)
		}
		if got, ok := GoNameOf(ty.Name); !ok || got != ty.GoName {
			t.Errorf("GoNameOf(%q) = %q, %v; want %q, true", ty.Name, got, ok, ty.GoName)
		}
		if got, ok := FromDjangoModel(ty.DjangoModel); !ok || got != ty.Name {
			t.Errorf("FromDjangoModel(%q) = %q, %v; want %q, true", ty.DjangoModel, got, ok, ty.Name)
		}
		if !Valid(ty.Name) {
			t.Errorf("Valid(%q) = false, want true", ty.Name)
		}
	}
	if _, ok := FromGoName("Nonesuch"); ok {
		t.Error("FromGoName(Nonesuch) ok = true, want false")
	}
	if _, ok := GoNameOf("nonesuch"); ok {
		t.Error("GoNameOf(nonesuch) ok = true, want false")
	}
	if _, ok := FromDjangoModel("Nonesuch"); ok {
		t.Error("FromDjangoModel(Nonesuch) ok = true, want false")
	}
	if Valid("nonesuch") {
		t.Error("Valid(nonesuch) = true, want false")
	}
}

// TestNames_FreshCopies asserts callers can't corrupt the canonical
// list through a returned slice.
func TestNames_FreshCopies(t *testing.T) {
	t.Parallel()
	n := Names()
	n[0] = "mutated"
	if got := Names()[0]; got == "mutated" {
		t.Error("Names() aliases internal state; want a fresh copy per call")
	}
}
