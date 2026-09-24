package otel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/dotwaffle/peeringdb-plus/internal/litefs"
)

// TestInitLiteFSGauges_RecordsValues checks that one collection scrapes
// once and observes every instrument from that scrape.
func TestInitLiteFSGauges_RecordsValues(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	scrapes := 0
	err := InitLiteFSGauges(func(context.Context) (litefs.Metrics, error) {
		scrapes++
		return litefs.Metrics{
			TXID:          58649,
			Commits:       12,
			LTXBytes:      203607,
			LTXFiles:      6,
			LTXLagSeconds: 0.5,
			LagSeconds:    1.25,
			Subscribers:   7,
		}, nil
	})
	if err != nil {
		t.Fatalf("InitLiteFSGauges: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if scrapes != 1 {
		t.Errorf("scrapes per collection = %d, want 1", scrapes)
	}

	for name, want := range map[string]int64{
		"pdbplus.litefs.txid":        58649,
		"pdbplus.litefs.ltx.size":    203607,
		"pdbplus.litefs.ltx.files":   6,
		"pdbplus.litefs.subscribers": 7,
	} {
		m := findMetric(rm, name)
		if m == nil {
			t.Errorf("%s not found", name)
			continue
		}
		g, ok := m.Data.(metricdata.Gauge[int64])
		if !ok || len(g.DataPoints) != 1 || g.DataPoints[0].Value != want {
			t.Errorf("%s = %+v, want one Gauge[int64] point of %d", name, m.Data, want)
		}
	}
	for name, want := range map[string]float64{
		"pdbplus.litefs.ltx.lag": 0.5,
		"pdbplus.litefs.lag":     1.25,
	} {
		m := findMetric(rm, name)
		if m == nil {
			t.Errorf("%s not found", name)
			continue
		}
		g, ok := m.Data.(metricdata.Gauge[float64])
		if !ok || len(g.DataPoints) != 1 || g.DataPoints[0].Value != want {
			t.Errorf("%s = %+v, want one Gauge[float64] point of %v", name, m.Data, want)
		}
	}
	m := findMetric(rm, "pdbplus.litefs.commits")
	if m == nil {
		t.Fatal("pdbplus.litefs.commits not found")
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok || !sum.IsMonotonic || len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 12 {
		t.Errorf("pdbplus.litefs.commits = %+v, want one monotonic Sum[int64] point of 12", m.Data)
	}
}

// TestInitLiteFSGauges_ScrapeFailureObservesNothing checks that a failed
// scrape leaves a gap instead of zero values.
func TestInitLiteFSGauges_ScrapeFailureObservesNothing(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })

	err := InitLiteFSGauges(func(context.Context) (litefs.Metrics, error) {
		return litefs.Metrics{}, errors.New("connection refused")
	})
	if err != nil {
		t.Fatalf("InitLiteFSGauges: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if strings.HasPrefix(m.Name, "pdbplus.litefs.") {
				t.Errorf("failed scrape exported %s = %+v", m.Name, m.Data)
			}
		}
	}
}
