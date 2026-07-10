// Package pdbtypes is the leaf source of truth for the 13 PeeringDB
// object types. It imports nothing, so every layer of the tree —
// codegen tools, sync, observability, operator tooling — can share one
// canonical list instead of hand-copying it.
//
// Each type is named in three domains:
//
//   - Name: the PeeringDB API path segment ("net", "ixpfx") used in
//     URLs, pdbcompat Registry keys, and metric attributes
//   - GoName: the ent Go type name ("Network", "IxPrefix")
//   - DjangoModel: the upstream django-peeringdb model class
//     ("Network", "IXLanPrefix") used when parsing upstream source
//
// internal/sync deliberately keeps its own ordered step list
// (worker.go canonicalStepOrder): cmd/loadtest's ordering parity test
// cross-checks it against Names(), so the two lists guard each other
// rather than one deriving from the other.
package pdbtypes

import "slices"

// Type names one PeeringDB object type across the three naming domains
// used in this repo and upstream.
type Type struct {
	Name        string // PeeringDB API path segment, e.g. "net"
	GoName      string // ent Go type name, e.g. "Network"
	DjangoModel string // upstream Django model class, e.g. "Network"
}

// All lists the 13 PeeringDB object types in canonical
// parent-before-child order (the order sync steps run in).
var All = []Type{
	{Name: "org", GoName: "Organization", DjangoModel: "Organization"},
	{Name: "campus", GoName: "Campus", DjangoModel: "Campus"},
	{Name: "fac", GoName: "Facility", DjangoModel: "Facility"},
	{Name: "carrier", GoName: "Carrier", DjangoModel: "Carrier"},
	{Name: "carrierfac", GoName: "CarrierFacility", DjangoModel: "CarrierFacility"},
	{Name: "ix", GoName: "InternetExchange", DjangoModel: "InternetExchange"},
	{Name: "ixlan", GoName: "IxLan", DjangoModel: "IXLan"},
	{Name: "ixpfx", GoName: "IxPrefix", DjangoModel: "IXLanPrefix"},
	{Name: "ixfac", GoName: "IxFacility", DjangoModel: "InternetExchangeFacility"},
	{Name: "net", GoName: "Network", DjangoModel: "Network"},
	{Name: "poc", GoName: "Poc", DjangoModel: "NetworkContact"},
	{Name: "netfac", GoName: "NetworkFacility", DjangoModel: "NetworkFacility"},
	{Name: "netixlan", GoName: "NetworkIxLan", DjangoModel: "NetworkIXLan"},
}

// Names returns the 13 PeeringDB type names in canonical
// parent-before-child order. The slice is a fresh copy.
func Names() []string {
	out := make([]string, len(All))
	for i, t := range All {
		out[i] = t.Name
	}
	return out
}

// SortedNames returns the 13 PeeringDB type names in alphabetical
// order. The slice is a fresh copy.
func SortedNames() []string {
	out := Names()
	slices.Sort(out)
	return out
}

// FromGoName maps an ent Go type name ("Network") to its PeeringDB
// type name ("net"). ok is false for unknown names.
func FromGoName(goName string) (name string, ok bool) {
	for _, t := range All {
		if t.GoName == goName {
			return t.Name, true
		}
	}
	return "", false
}

// GoNameOf maps a PeeringDB type name ("net") to its ent Go type name
// ("Network"). ok is false for unknown names.
func GoNameOf(name string) (goName string, ok bool) {
	for _, t := range All {
		if t.Name == name {
			return t.GoName, true
		}
	}
	return "", false
}

// FromDjangoModel maps an upstream Django model class ("IXLanPrefix")
// to its PeeringDB type name ("ixpfx"). ok is false for unknown names.
func FromDjangoModel(model string) (name string, ok bool) {
	for _, t := range All {
		if t.DjangoModel == model {
			return t.Name, true
		}
	}
	return "", false
}

// Valid reports whether name is one of the 13 PeeringDB type names.
func Valid(name string) bool {
	_, ok := GoNameOf(name)
	return ok
}
