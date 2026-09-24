package sync

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
)

// TestRootSpanAttributes locks the trace markers on the root span of a sync
// cycle and the sampler decision that they give. A scheduled cycle has
// origin=sync and no force_sample, so the sync ratio
// (PDBPLUS_OTEL_SYNC_SAMPLE_RATE) decides. WithForceTrace sets
// force_sample=true and WithNoTrace sets force_sample=false, and both
// override the sync ratio.
func TestRootSpanAttributes(t *testing.T) {
	t.Parallel()

	// The all-ones TraceID is dropped by every ratio below 1.0, so a
	// scheduled cycle that falls through to the 1% default ratio drops.
	allOnes := trace.TraceID{
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	}

	tests := []struct {
		name      string
		ctx       context.Context
		wantForce *bool // nil: no force_sample attribute
		// wantSampled maps the sync ratio to the expected decision.
		wantSampled map[float64]bool
	}{
		{
			name:        "scheduled cycle",
			ctx:         t.Context(),
			wantForce:   nil,
			wantSampled: map[float64]bool{0: false, 1: true},
		},
		{
			name:        "WithForceTrace",
			ctx:         WithForceTrace(t.Context()),
			wantForce:   new(true),
			wantSampled: map[float64]bool{0: true, 1: true},
		},
		{
			name:        "WithNoTrace",
			ctx:         WithNoTrace(t.Context()),
			wantForce:   new(false),
			wantSampled: map[float64]bool{0: false, 1: false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			attrs := RootSpanAttributes(tt.ctx)
			set := attribute.NewSet(attrs...)

			if v, ok := set.Value(pdbotel.AttrSyncOrigin); !ok || v.AsString() != pdbotel.SyncOriginValue {
				t.Errorf("%s = %v (present=%v), want %q", pdbotel.AttrSyncOrigin, v, ok, pdbotel.SyncOriginValue)
			}
			v, ok := set.Value(pdbotel.AttrForceSample)
			switch {
			case tt.wantForce == nil && ok:
				t.Errorf("%s = %v, want no attribute (scheduled cycle)", pdbotel.AttrForceSample, v)
			case tt.wantForce != nil && (!ok || v.Type() != attribute.BOOL || v.AsBool() != *tt.wantForce):
				t.Errorf("%s = %v (present=%v), want %v", pdbotel.AttrForceSample, v, ok, *tt.wantForce)
			}

			for ratio, wantSampled := range tt.wantSampled {
				s := pdbotel.NewPerRouteSampler(pdbotel.PerRouteSamplerInput{
					DefaultRatio: 0.01, // the production default
					SyncRatio:    ratio,
				})
				res := s.ShouldSample(sdktrace.SamplingParameters{
					ParentContext: tt.ctx,
					TraceID:       allOnes,
					Name:          "sync-incremental",
					Attributes:    attrs,
				})
				if got := res.Decision == sdktrace.RecordAndSample; got != wantSampled {
					t.Errorf("sync ratio %v: sampled = %v, want %v", ratio, got, wantSampled)
				}
			}
		})
	}
}
