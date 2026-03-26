package pdbcompat

import (
	"fmt"
	"testing"
)

// benchEntity is a struct with JSON tags used for field projection benchmarks.
// It mirrors the shape of PeeringDB response objects with a mix of field types.
type benchEntity struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	Website  string   `json:"website"`
	City     string   `json:"city"`
	Country  string   `json:"country"`
	State    string   `json:"state"`
	Zipcode  string   `json:"zipcode"`
	Notes    string   `json:"notes"`
	Status   string   `json:"status"`
	OrgSet   []int    `json:"org_set"`
	AltNames []string `json:"alt_names"`
}

// makeBenchData creates n benchEntity items cast to []any for projection.
func makeBenchData(n int) []any {
	data := make([]any, n)
	for i := range n {
		data[i] = benchEntity{
			ID:       i + 1,
			Name:     fmt.Sprintf("Entity %d", i),
			Value:    fmt.Sprintf("val-%d", i),
			Website:  fmt.Sprintf("https://example%d.com", i),
			City:     "Frankfurt",
			Country:  "DE",
			State:    "HE",
			Zipcode:  "60313",
			Notes:    "Some notes here",
			Status:   "ok",
			OrgSet:   []int{100, 200, 300},
			AltNames: []string{"alias-a", "alias-b"},
		}
	}
	return data
}

// BenchmarkApplyFieldProjection measures field projection performance across
// different field counts and item volumes.
func BenchmarkApplyFieldProjection(b *testing.B) {
	data := makeBenchData(100)

	b.Run("3_fields_100_items", func(b *testing.B) {
		fields := []string{"name", "value", "org_set"}
		b.ResetTimer()
		for b.Loop() {
			_ = applyFieldProjection(data, fields)
		}
	})

	b.Run("10_fields_100_items", func(b *testing.B) {
		fields := []string{"name", "value", "website", "city", "country", "state", "zipcode", "notes", "status", "org_set"}
		b.ResetTimer()
		for b.Loop() {
			_ = applyFieldProjection(data, fields)
		}
	})

	b.Run("no_projection", func(b *testing.B) {
		var fields []string
		b.ResetTimer()
		for b.Loop() {
			_ = applyFieldProjection(data, fields)
		}
	})
}
