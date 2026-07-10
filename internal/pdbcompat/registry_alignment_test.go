package pdbcompat

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// registryEntStructs pairs each Registry type with its generated ent
// struct. Reflection over the struct's json tags recovers the wire-level
// column names (the generated `json:"<column>"` tags match upstream's
// field names exactly; `_fold` shadows carry `json:"-"` and self-exclude).
var registryEntStructs = map[string]reflect.Type{
	peeringdb.TypeOrg:        reflect.TypeFor[ent.Organization](),
	peeringdb.TypeNet:        reflect.TypeFor[ent.Network](),
	peeringdb.TypeFac:        reflect.TypeFor[ent.Facility](),
	peeringdb.TypeIX:         reflect.TypeFor[ent.InternetExchange](),
	peeringdb.TypePoc:        reflect.TypeFor[ent.Poc](),
	peeringdb.TypeIXLan:      reflect.TypeFor[ent.IxLan](),
	peeringdb.TypeIXPfx:      reflect.TypeFor[ent.IxPrefix](),
	peeringdb.TypeNetIXLan:   reflect.TypeFor[ent.NetworkIxLan](),
	peeringdb.TypeNetFac:     reflect.TypeFor[ent.NetworkFacility](),
	peeringdb.TypeIXFac:      reflect.TypeFor[ent.IxFacility](),
	peeringdb.TypeCarrier:    reflect.TypeFor[ent.Carrier](),
	peeringdb.TypeCarrierFac: reflect.TypeFor[ent.CarrierFacility](),
	peeringdb.TypeCampus:     reflect.TypeFor[ent.Campus](),
}

// registryFieldExclusions lists scalar ent columns deliberately absent
// from Registry[type].Fields, keyed "<type>.<column>". Every entry needs
// a justification; the test fails on stale entries (column no longer
// exists) so the list cannot rot. "status" is excluded globally below —
// it is owned by applyStatusMatrix on every type, never a client filter.
var registryFieldExclusions = map[string]string{
	// Auth-gated field: exposing it as a filter key would let anonymous
	// callers probe private values by equality/substring match even
	// though the serializer redacts the value itself (privfield.Redact).
	"ixlan.ixf_ixp_member_list_url": "auth-gated; filterable would leak via probing",
}

// scalarFieldType maps a struct field's Go type to the FieldType the
// Registry must declare for it. Pointers (Nillable columns) dereference.
// Non-scalar columns (e.g. social_media's []SocialMedia) return ok=false
// and are not filterable by design.
func scalarFieldType(t reflect.Type) (FieldType, bool) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeFor[time.Time]() {
		return FieldTime, true
	}
	switch t.Kind() { //nolint:exhaustive // default arm catches every non-scalar kind by design
	case reflect.String:
		return FieldString, true
	case reflect.Bool:
		return FieldBool, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return FieldInt, true
	case reflect.Float32, reflect.Float64:
		return FieldFloat, true
	default:
		return 0, false
	}
}

// TestRegistryFields_AlignWithEntColumns locks Registry[type].Fields to
// the generated ent schema: every scalar column (except _fold shadows,
// status, and the justified exclusion list) must be declared as a filter
// field with the matching FieldType. Without this gate, a column added
// upstream (and to schema/peeringdb.json) ships syncable and serialized
// but silently unfilterable — ParseFilters ignores unknown keys with an
// unfiltered HTTP 200, so nothing else surfaces the drift.
func TestRegistryFields_AlignWithEntColumns(t *testing.T) {
	t.Parallel()

	seenExclusions := make(map[string]bool, len(registryFieldExclusions))

	for typeName, structType := range registryEntStructs {
		tc, ok := Registry[typeName]
		if !ok {
			t.Errorf("Registry missing type %q", typeName)
			continue
		}
		for f := range structType.Fields() {
			if f.PkgPath != "" || f.Name == "Edges" {
				continue // unexported (config, selectValues) or edge container
			}
			column, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if column == "" || column == "-" {
				continue // _fold shadows and untagged internals
			}
			if column == "status" {
				continue // owned by applyStatusMatrix on every type
			}
			if reason, excluded := registryFieldExclusions[typeName+"."+column]; excluded {
				seenExclusions[typeName+"."+column] = true
				t.Logf("%s.%s excluded: %s", typeName, column, reason)
				continue
			}
			want, scalar := scalarFieldType(f.Type)
			if !scalar {
				continue // non-scalar column (JSON blob etc.), not filterable
			}
			got, declared := tc.Fields[column]
			if !declared {
				t.Errorf("%s.%s (%s) missing from Registry[%q].Fields — new columns must be filterable or added to registryFieldExclusions with a justification",
					typeName, column, want, typeName)
				continue
			}
			if got != want {
				t.Errorf("%s.%s FieldType = %s, want %s (ent struct field %s is %s)",
					typeName, column, got, want, f.Name, f.Type)
			}
		}
	}

	// The exclusion list must not outlive the columns it excuses.
	for key := range registryFieldExclusions {
		if !seenExclusions[key] {
			t.Errorf("registryFieldExclusions entry %q matches no ent column — remove the stale entry", key)
		}
	}
}
