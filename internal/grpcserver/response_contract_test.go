package grpcserver

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	entschema "entgo.io/ent"
	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/schema"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type responseContract struct {
	schema   entschema.Interface
	row      reflect.Type
	convert  func(context.Context, any) proto.Message
	excluded map[string]string
	legacy   map[string]string
}

func responseType[E any, P proto.Message](s entschema.Interface, convert func(*E) P) responseContract {
	return responseTypeContext(s, func(_ context.Context, row *E) P { return convert(row) })
}

func responseTypeContext[E any, P proto.Message](s entschema.Interface, convert func(context.Context, *E) P) responseContract {
	return responseContract{schema: s, row: reflect.TypeFor[E](), convert: func(ctx context.Context, row any) proto.Message { return convert(ctx, row.(*E)) }}
}

func responseContracts() map[string]responseContract {
	contracts := map[string]responseContract{
		"Campus":           responseType(schema.Campus{}, campusToProto),
		"Carrier":          responseType(schema.Carrier{}, carrierToProto),
		"CarrierFacility":  responseType(schema.CarrierFacility{}, carrierFacilityToProto),
		"Facility":         responseType(schema.Facility{}, facilityToProto),
		"InternetExchange": responseType(schema.InternetExchange{}, internetExchangeToProto),
		"IxFacility":       responseType(schema.IxFacility{}, ixFacilityToProto),
		"IxLan":            responseTypeContext(schema.IxLan{}, ixLanToProto),
		"IxPrefix":         responseType(schema.IxPrefix{}, ixPrefixToProto),
		"Network":          responseTypeContext(schema.Network{}, networkToProto),
		"NetworkFacility":  responseType(schema.NetworkFacility{}, networkFacilityToProto),
		"NetworkIxLan":     responseTypeContext(schema.NetworkIxLan{}, networkIxLanToProto),
		"Organization":     responseType(schema.Organization{}, organizationToProto),
		"Poc":              responseType(schema.Poc{}, pocToProto),
	}
	// These fields have never been part of the response contract. Keep each
	// exclusion explicit so a new storage field requires a wire API decision.
	exclusions := map[string][]string{
		"Campus":           {"social_media", "name_fold"},
		"Carrier":          {"social_media", "name_fold", "aka_fold"},
		"Facility":         {"social_media", "name_fold", "aka_fold", "city_fold"},
		"InternetExchange": {"social_media", "name_fold", "aka_fold", "name_long_fold", "city_fold"},
		"Network":          {"social_media", "ixp_update_exclude", "name_fold", "aka_fold", "name_long_fold"},
		"Organization":     {"social_media", "name_fold", "aka_fold", "city_fold"},
	}
	for name, fields := range exclusions {
		c := contracts[name]
		c.excluded = make(map[string]string)
		for _, field := range fields {
			reason := "Internal search column, never exposed on the wire."
			switch field {
			case "social_media":
				reason = "Custom JSON omitted when v1 messages were introduced; no response field has been added."
			case "ixp_update_exclude":
				reason = "Added to storage after v1; not added to the response API."
			}
			c.excluded[field] = reason
		}
		contracts[name] = c
	}
	// Removed storage fields retain their original wire declarations and numbers.
	for name, fields := range map[string][]string{"IxPrefix": {"notes"}, "Organization": {"fac_count", "net_count"}} {
		c := contracts[name]
		c.legacy = make(map[string]string)
		for _, field := range fields {
			c.legacy[field] = "Removed from storage; retained on the wire as absent for compatibility."
		}
		contracts[name] = c
	}
	return contracts
}

func TestResponseFieldContract(t *testing.T) {
	contracts := responseContracts()
	for _, entity := range pdbtypes.All {
		c, ok := contracts[entity.GoName]
		if !ok {
			t.Errorf("missing response contract for %s", entity.GoName)
			continue
		}
		delete(contracts, entity.GoName)
		t.Run(entity.GoName, func(t *testing.T) { checkResponseContract(t, c) })
	}
	for name := range contracts {
		t.Errorf("stale response contract %s", name)
	}
}

func checkResponseContract(t *testing.T, c responseContract) {
	t.Helper()
	ctx := privctx.WithTier(t.Context(), privctx.TierUsers)
	empty := c.convert(ctx, reflect.New(c.row).Interface()).ProtoReflect()
	fields := empty.Descriptor().Fields()
	storage := make(map[string]bool)
	schemaFields := c.schema.Fields()
	for _, mixin := range c.schema.Mixin() {
		schemaFields = append(schemaFields, mixin.Fields()...)
	}
	for _, field := range schemaFields {
		storage[field.Descriptor().Name] = true
	}
	for name, reason := range c.excluded {
		if !storage[name] || fields.ByName(protoreflect.Name(name)) != nil || reason == "" {
			t.Errorf("stale or invalid exclusion %s: %q", name, reason)
		}
	}
	for name, reason := range c.legacy {
		fd := fields.ByName(protoreflect.Name(name))
		if storage[name] || fd == nil || reason == "" {
			t.Errorf("stale or invalid legacy field %s: %q", name, reason)
			continue
		}
		if empty.Has(fd) {
			t.Errorf("legacy field %s must remain absent", name)
		}
	}
	for name := range storage {
		if _, excluded := c.excluded[name]; excluded {
			continue
		}
		fd := fields.ByName(protoreflect.Name(name))
		if fd == nil {
			t.Errorf("storage field %s has no response field or exclusion", name)
			continue
		}
		t.Run(name, func(t *testing.T) { checkResponseConversion(ctx, t, c, name, fd) })
	}
	for i := range fields.Len() {
		name := string(fields.Get(i).Name())
		if !storage[name] && c.legacy[name] == "" {
			t.Errorf("response field %s has no storage field or legacy declaration", name)
		}
	}
}

func checkResponseConversion(ctx context.Context, t *testing.T, c responseContract, name string, fd protoreflect.FieldDescriptor) {
	t.Helper()
	row := reflect.New(c.row)
	var value reflect.Value
	for i := range c.row.NumField() {
		if strings.Split(c.row.Field(i).Tag.Get("json"), ",")[0] == name {
			value = row.Elem().Field(i)
			break
		}
	}
	if !value.IsValid() {
		t.Fatalf("storage field %s missing from generated Ent row", name)
	}
	want := responseSentinel(t, value)
	// The URL converter needs an admitted visibility on its companion field.
	if il, ok := reflect.TypeAssert[*ent.IxLan](row); ok && name == "ixf_ixp_member_list_url" {
		il.IxfIxpMemberListURLVisible = "Users"
	}
	got := c.convert(ctx, row.Interface()).ProtoReflect().Get(fd)
	if actual := responseValue(t, fd, got); !reflect.DeepEqual(actual, want) {
		t.Errorf("converter field %s: got %#v, want %#v", name, actual, want)
	}
}

// Set one field at a time. Other fields stay zero, so copying from the wrong
// source also fails for booleans and fields with identical Go types.
func responseSentinel(t *testing.T, value reflect.Value) any {
	t.Helper()
	if value.Kind() == reflect.Pointer {
		value.Set(reflect.New(value.Type().Elem()))
		return responseSentinel(t, value.Elem())
	}
	if value.Type() == reflect.TypeFor[time.Time]() {
		stamp := time.Date(2026, 4, 5, 6, 7, 8, 123456789, time.UTC)
		value.Set(reflect.ValueOf(stamp))
		return stamp
	}
	switch {
	case value.Kind() == reflect.String:
		value.SetString("response sentinel")
		return value.String()
	case value.Kind() == reflect.Bool:
		value.SetBool(true)
		return true
	case value.Kind() == reflect.Int:
		value.SetInt(137)
		return int64(137)
	case value.Kind() == reflect.Float64:
		value.SetFloat(12.75)
		return float64(12.75)
	case value.Kind() == reflect.Slice:
		if value.Type() == reflect.TypeFor[[]string]() {
			items := []string{"sentinel one", "sentinel two"}
			value.Set(reflect.ValueOf(items))
			return items
		}
	case value.Kind() == reflect.Map:
		if value.Type() == reflect.TypeFor[map[string]any]() {
			doc := map[string]any{"sentinel": "metadata", "number": float64(137)}
			value.Set(reflect.ValueOf(doc))
			return doc
		}
	}
	t.Fatalf("unsupported response source type %s", value.Type())
	return nil
}

func responseValue(t *testing.T, fd protoreflect.FieldDescriptor, value protoreflect.Value) any {
	t.Helper()
	if fd.IsList() {
		items := make([]string, value.List().Len())
		for i := range items {
			items[i] = value.List().Get(i).String()
		}
		return items
	}
	if fd.Kind() != protoreflect.MessageKind {
		return value.Interface()
	}
	msg := value.Message()
	if !msg.IsValid() {
		return nil
	}
	switch typed := msg.Interface().(type) {
	case *timestamppb.Timestamp:
		return typed.AsTime()
	case *structpb.Struct:
		return typed.AsMap()
	}
	// Scalar wrappers carry their source value in field 1.
	wrapped := msg.Descriptor().Fields().ByName("value")
	if wrapped == nil {
		t.Fatalf("unsupported response message %s", msg.Descriptor().FullName())
	}
	return msg.Get(wrapped).Interface()
}
