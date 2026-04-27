package otel

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Phase 77 OBS-07: per-route sampler that dispatches sampling decisions
// based on URL path prefix read from SamplingParameters.Attributes
// (url.path / http.target). See AUDIT.md § Tempo Trace Audit (OBS-07)
// for the route → ratio matrix this sampler enforces.

// allOnesTraceID is the maximum-valued TraceID. TraceIDRatioBased(0.0)
// drops it deterministically; TraceIDRatioBased(1.0) admits it. We use
// it to make sub-ratio decisions deterministic for assertion.
var allOnesTraceID = trace.TraceID{
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
}

func sampleParams(tid trace.TraceID, attrs ...attribute.KeyValue) sdktrace.SamplingParameters {
	return sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       tid,
		Name:          "peeringdb-plus",
		Kind:          trace.SpanKindServer,
		Attributes:    attrs,
	}
}

func TestPerRouteSampler_HealthzDispatchedToLowRatio(t *testing.T) {
	t.Parallel()

	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 1.0,
		Routes: map[string]float64{
			"/healthz": 0.0,
			"/api/":    1.0,
		},
	})

	res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/healthz")))
	if res.Decision != sdktrace.Drop {
		t.Errorf("/healthz at ratio 0.0 with all-ones TraceID: got %v, want Drop", res.Decision)
	}
}

func TestPerRouteSampler_APIDispatchedToFullRatio(t *testing.T) {
	t.Parallel()

	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 0.0,
		Routes: map[string]float64{
			"/api/": 1.0,
		},
	})

	res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/api/net")))
	if res.Decision != sdktrace.RecordAndSample {
		t.Errorf("/api/ at ratio 1.0: got %v, want RecordAndSample", res.Decision)
	}
}

func TestPerRouteSampler_RestV1PrefixMatch(t *testing.T) {
	t.Parallel()

	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 0.0,
		Routes: map[string]float64{
			"/rest/v1/": 1.0,
		},
	})

	res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/rest/v1/networks")))
	if res.Decision != sdktrace.RecordAndSample {
		t.Errorf("/rest/v1/networks: got %v, want RecordAndSample (prefix match)", res.Decision)
	}
}

func TestPerRouteSampler_ConnectRPCPrefixMatch(t *testing.T) {
	t.Parallel()

	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 0.0,
		Routes: map[string]float64{
			"/peeringdb.v1.": 1.0,
		},
	})

	res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/peeringdb.v1.NetworkService/Get")))
	if res.Decision != sdktrace.RecordAndSample {
		t.Errorf("/peeringdb.v1.NetworkService/Get: got %v, want RecordAndSample", res.Decision)
	}
}

func TestPerRouteSampler_LegacyHTTPTargetAttribute(t *testing.T) {
	t.Parallel()

	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 1.0,
		Routes: map[string]float64{
			"/healthz": 0.0,
		},
	})

	// Caller emits http.target instead of url.path (older semconv version).
	res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("http.target", "/healthz")))
	if res.Decision != sdktrace.Drop {
		t.Errorf("/healthz via legacy http.target: got %v, want Drop (sampler must read both keys)", res.Decision)
	}
}

func TestPerRouteSampler_UnmatchedPathFallsBackToDefault(t *testing.T) {
	t.Parallel()

	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 1.0,
		Routes: map[string]float64{
			"/healthz": 0.0,
		},
	})

	res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/wibble")))
	if res.Decision != sdktrace.RecordAndSample {
		t.Errorf("unmatched /wibble at default ratio 1.0: got %v, want RecordAndSample", res.Decision)
	}
}

func TestPerRouteSampler_NoPathAttributeFallsBackToDefault(t *testing.T) {
	t.Parallel()

	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 1.0,
		Routes: map[string]float64{
			"/healthz": 0.0,
		},
	})

	// Sync-worker spans created without HTTP attributes hit this branch.
	res := s.ShouldSample(sampleParams(allOnesTraceID))
	if res.Decision != sdktrace.RecordAndSample {
		t.Errorf("no path attribute at default ratio 1.0: got %v, want RecordAndSample (sync-worker fallback)", res.Decision)
	}
}

func TestPerRouteSampler_DescriptionFormat(t *testing.T) {
	t.Parallel()

	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 1.0,
		Routes: map[string]float64{
			"/healthz": 0.01,
			"/api/":    1.0,
		},
	})

	desc := s.Description()
	if !strings.Contains(desc, "PerRouteSampler") {
		t.Errorf("Description() missing PerRouteSampler marker: %q", desc)
	}
	if !strings.Contains(desc, "2") {
		t.Errorf("Description() should include route count (2): %q", desc)
	}
}

func TestPerRouteSampler_RoutesNormalised(t *testing.T) {
	t.Parallel()

	// Constructor must accept "/api" and "/api/" equivalently — operators
	// shouldn't have to remember whether a slash matters.
	sNoSlash := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 0.0,
		Routes:       map[string]float64{"/api": 1.0},
	})
	sWithSlash := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 0.0,
		Routes:       map[string]float64{"/api/": 1.0},
	})

	for name, s := range map[string]sdktrace.Sampler{"no-slash": sNoSlash, "with-slash": sWithSlash} {
		res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/api/net")))
		if res.Decision != sdktrace.RecordAndSample {
			t.Errorf("%s constructor: /api/net should sample-in at ratio 1.0; got %v", name, res.Decision)
		}
	}
}

func TestPerRouteSampler_LongestPrefixWins(t *testing.T) {
	t.Parallel()

	// Defensive: if a future API path adds a more-specific prefix, the
	// longer prefix must win. /api/auth/foo with /api/=1.0 + /api/auth/=0.0
	// must drop, not sample-in.
	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 0.0,
		Routes: map[string]float64{
			"/api/":      1.0,
			"/api/auth/": 0.0,
		},
	})

	res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/api/auth/login")))
	if res.Decision != sdktrace.Drop {
		t.Errorf("/api/auth/login (longer prefix at 0.0): got %v, want Drop", res.Decision)
	}

	res = s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/api/networks")))
	if res.Decision != sdktrace.RecordAndSample {
		t.Errorf("/api/networks (only /api/ matches at 1.0): got %v, want RecordAndSample", res.Decision)
	}
}

func TestParentBased_InheritsDecisionForSampledIn(t *testing.T) {
	t.Parallel()

	// Locks the cross-service trace continuity invariant from CONTEXT.md
	// D-02: a child span with a sampled parent inherits RecordAndSample
	// even when the child route would otherwise be dropped.

	inner := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 1.0,
		Routes:       map[string]float64{"/healthz": 0.0},
	})
	parent := sdktrace.ParentBased(inner)

	// Construct a parent SpanContext marked as sampled.
	parentCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    allOnesTraceID,
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})

	// Child params claim /healthz route, which would normally drop.
	params := sdktrace.SamplingParameters{
		ParentContext: trace.ContextWithSpanContext(context.Background(), parentCtx),
		TraceID:       allOnesTraceID,
		Name:          "child",
		Kind:          trace.SpanKindServer,
		Attributes:    []attribute.KeyValue{attribute.String("url.path", "/healthz")},
	}

	res := parent.ShouldSample(params)
	if res.Decision != sdktrace.RecordAndSample {
		t.Errorf("ParentBased with sampled-in parent: got %v, want RecordAndSample (parent decision must win regardless of /healthz route)", res.Decision)
	}
}
