package otel

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Per-route sampler that dispatches sampling decisions based on URL path
// prefix read from SamplingParameters.Attributes (url.path / http.target).
// Tests the route → ratio matrix this sampler enforces.

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

// TestPerRouteSampler_SyncTraceGating locks the sync-trace gates. A
// scheduled cycle (origin=sync, no force_sample) uses SyncRatio, not the
// route or the default. force_sample=true always samples (POST /sync).
// force_sample=false never samples (POST /sync?trace=0), also at SyncRatio
// 1.0. Spans without the markers keep the per-route logic. The all-ones
// TraceID makes each ratio decision deterministic: ratio 1.0 admits it, and
// a ratio below 1.0 drops it.
func TestPerRouteSampler_SyncTraceGating(t *testing.T) {
	t.Parallel()

	origin := attribute.String(AttrSyncOrigin, SyncOriginValue)
	force := attribute.Bool(AttrForceSample, true)
	optOut := attribute.Bool(AttrForceSample, false)

	tests := []struct {
		name         string
		defaultRatio float64
		syncRatio    float64
		attrs        []attribute.KeyValue
		want         sdktrace.SamplingDecision
	}{
		{
			// DefaultRatio 0 drops the span if the gate falls through.
			name:         "scheduled cycle at sync ratio 1.0 samples",
			defaultRatio: 0.0,
			syncRatio:    1.0,
			attrs:        []attribute.KeyValue{origin},
			want:         sdktrace.RecordAndSample,
		},
		{
			// The production default ratio. A sync root span has no URL
			// path, so without the gate it gets this 1% ratio.
			name:         "scheduled cycle does not use the 1% default",
			defaultRatio: 0.01,
			syncRatio:    1.0,
			attrs:        []attribute.KeyValue{origin},
			want:         sdktrace.RecordAndSample,
		},
		{
			// DefaultRatio 1.0 samples the span if the gate falls through.
			name:         "scheduled cycle at sync ratio 0 drops",
			defaultRatio: 1.0,
			syncRatio:    0.0,
			attrs:        []attribute.KeyValue{origin},
			want:         sdktrace.Drop,
		},
		{
			name:         "force at sync ratio 0 samples",
			defaultRatio: 0.0,
			syncRatio:    0.0,
			attrs:        []attribute.KeyValue{origin, force},
			want:         sdktrace.RecordAndSample,
		},
		{
			name:         "force without origin samples",
			defaultRatio: 0.0,
			syncRatio:    0.0,
			attrs:        []attribute.KeyValue{force},
			want:         sdktrace.RecordAndSample,
		},
		{
			name:         "opt-out at sync ratio 1.0 drops",
			defaultRatio: 1.0,
			syncRatio:    1.0,
			attrs:        []attribute.KeyValue{origin, optOut},
			want:         sdktrace.Drop,
		},
		{
			name:         "opt-out before origin drops",
			defaultRatio: 1.0,
			syncRatio:    1.0,
			attrs:        []attribute.KeyValue{optOut, origin},
			want:         sdktrace.Drop,
		},
		{
			name:         "non-sync span uses the route",
			defaultRatio: 0.0,
			syncRatio:    0.0,
			attrs:        []attribute.KeyValue{attribute.String("url.path", "/api/net")},
			want:         sdktrace.RecordAndSample,
		},
		{
			name:         "non-sync span on a zero route drops",
			defaultRatio: 1.0,
			syncRatio:    1.0,
			attrs:        []attribute.KeyValue{attribute.String("url.path", "/healthz")},
			want:         sdktrace.Drop,
		},
		{
			name:         "non-sync span without a path uses the default",
			defaultRatio: 0.0,
			syncRatio:    1.0,
			attrs:        nil,
			want:         sdktrace.Drop,
		},
		{
			name:         "other origin value uses the default",
			defaultRatio: 0.0,
			syncRatio:    1.0,
			attrs:        []attribute.KeyValue{attribute.String(AttrSyncOrigin, "startup")},
			want:         sdktrace.Drop,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := NewPerRouteSampler(PerRouteSamplerInput{
				DefaultRatio: tt.defaultRatio,
				SyncRatio:    tt.syncRatio,
				Routes:       map[string]float64{"/api/": 1.0, "/healthz": 0.0},
			})
			res := s.ShouldSample(sampleParams(allOnesTraceID, tt.attrs...))
			if res.Decision != tt.want {
				t.Errorf("default=%v sync=%v attrs=%v: got %v, want %v",
					tt.defaultRatio, tt.syncRatio, tt.attrs, res.Decision, tt.want)
			}
		})
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

	// Internal spans created without HTTP attributes hit this branch.
	res := s.ShouldSample(sampleParams(allOnesTraceID))
	if res.Decision != sdktrace.RecordAndSample {
		t.Errorf("no path attribute at default ratio 1.0: got %v, want RecordAndSample (internal-span fallback)", res.Decision)
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

// TestPerRouteSampler_DotDeniedAtLowRatio asserts the "/." prefix entry
// catches dotfile scanner bait at 0.001 — the matchesPrefix non-alnum
// boundary branch lets it match /.env, /.git/config, /.aws/credentials,
// etc., without tokenising the URL further.
func TestPerRouteSampler_DotDeniedAtLowRatio(t *testing.T) {
	t.Parallel()

	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 1.0,
		Routes: map[string]float64{
			"/.": 0.001,
		},
	})

	dotfilePaths := []string{
		"/.env",
		"/.git/config",
		"/.git/HEAD",
		"/.aws/credentials",
		"/.docker/config.json",
		"/.kube/config",
		"/.htpasswd",
		"/.npmrc",
	}
	for _, path := range dotfilePaths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", path)))
			if res.Decision != sdktrace.Drop {
				t.Errorf("%s at /. ratio 0.001 with all-ones TraceID: got %v, want Drop", path, res.Decision)
			}
		})
	}
}

// TestPerRouteSampler_WpDeniedAtLowRatio asserts the "/wp-" prefix entry
// catches WordPress scanner bait (wp-admin, wp-login.php, wp-content/...)
// at 0.001 via the non-alnum trailing-byte branch of matchesPrefix.
func TestPerRouteSampler_WpDeniedAtLowRatio(t *testing.T) {
	t.Parallel()

	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 1.0,
		Routes: map[string]float64{
			"/wp-": 0.001,
		},
	})

	wpPaths := []string{
		"/wp-admin",
		"/wp-login.php",
		"/wp-content/themes/foo/setup-config.php",
	}
	for _, path := range wpPaths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", path)))
			if res.Decision != sdktrace.Drop {
				t.Errorf("%s at /wp- ratio 0.001 with all-ones TraceID: got %v, want Drop", path, res.Decision)
			}
		})
	}
}

// TestPerRouteSampler_UnknownPathDropsAtOnePercent asserts the inverted
// default: unmatched paths drop under DefaultRatio=0.01 (all-ones TraceID
// falls outside the sampled-in segment). Sanity-checks that an explicit
// allow-list entry still samples in.
func TestPerRouteSampler_UnknownPathDropsAtOnePercent(t *testing.T) {
	t.Parallel()

	s := NewPerRouteSampler(PerRouteSamplerInput{
		DefaultRatio: 0.01,
		Routes: map[string]float64{
			"/api/": 1.0,
		},
	})

	res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/phpinfo.php")))
	if res.Decision != sdktrace.Drop {
		t.Errorf("/phpinfo.php at default ratio 0.01 with all-ones TraceID: got %v, want Drop", res.Decision)
	}

	res = s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/api/networks")))
	if res.Decision != sdktrace.RecordAndSample {
		t.Errorf("/api/networks at allow-list ratio 1.0: got %v, want RecordAndSample", res.Decision)
	}
}

// TestPerRouteSampler_DotDoesNotMatchPlainSlash defends against a future
// regression where "/." accidentally matches "/" alone. Asserts the root
// path falls through to DefaultRatio (proven by parameterising default
// over both 1.0 → sample-in and 0.0 → drop, so the assertion proves
// "fell through to default" rather than "matched and dropped").
func TestPerRouteSampler_DotDoesNotMatchPlainSlash(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		defaultRatio float64
		want         sdktrace.SamplingDecision
	}{
		{"default=1.0_samples_in", 1.0, sdktrace.RecordAndSample},
		{"default=0.0_drops", 0.0, sdktrace.Drop},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := NewPerRouteSampler(PerRouteSamplerInput{
				DefaultRatio: tc.defaultRatio,
				Routes: map[string]float64{
					"/.": 0.001,
				},
			})
			res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/")))
			if res.Decision != tc.want {
				t.Errorf("/ with default=%v: got %v, want %v (must fall through to DefaultRatio, not match /.)",
					tc.defaultRatio, res.Decision, tc.want)
			}
		})
	}
}

func TestParentBased_InheritsDecisionForSampledIn(t *testing.T) {
	t.Parallel()

	// Locks the cross-service trace continuity invariant: a child span with
	// a sampled parent inherits RecordAndSample even when the child route
	// would otherwise be dropped.

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
