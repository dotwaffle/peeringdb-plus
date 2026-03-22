package schema

import (
	"context"
	"fmt"

	"entgo.io/ent"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// otelMutationHook returns an ent.Hook that creates an OTel span around mutations.
// The span name follows the pattern "ent.{Type}.{Op}" (e.g., "ent.Organization.Create").
func otelMutationHook(typeName string) ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			spanName := fmt.Sprintf("ent.%s.%s", typeName, m.Op().String())
			ctx, span := otel.Tracer("ent").Start(ctx, spanName)
			defer span.End()

			span.SetAttributes(
				attribute.String("ent.type", typeName),
				attribute.String("ent.op", m.Op().String()),
			)

			v, err := next.Mutate(ctx, m)
			if err != nil {
				span.RecordError(err)
			}
			return v, err
		})
	}
}
