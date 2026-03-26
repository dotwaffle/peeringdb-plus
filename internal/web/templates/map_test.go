package templates

import (
	"strings"
	"testing"
)

func TestBuildMultiPinPopupHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		marker   MapMarker
		wantBody []string
		noBody   []string
	}{
		{
			name: "full popup with extra",
			marker: MapMarker{
				Name:      "Equinix DC2",
				Location:  "Washington, US",
				Extra:     "Cloudflare + Google",
				DetailURL: "/ui/fac/100",
			},
			wantBody: []string{
				"Equinix DC2",
				"Washington, US",
				"Cloudflare + Google",
				"/ui/fac/100",
				"View facility",
			},
		},
		{
			name: "no location",
			marker: MapMarker{
				Name:      "Unknown DC",
				DetailURL: "/ui/fac/200",
			},
			wantBody: []string{"Unknown DC", "View facility"},
			noBody:   []string{`color:#666;margin-bottom:4px;"`},
		},
		{
			name: "no extra",
			marker: MapMarker{
				Name:      "Telehouse London",
				Location:  "London, GB",
				DetailURL: "/ui/fac/300",
			},
			wantBody: []string{"Telehouse London", "London, GB"},
			noBody:   []string{`margin-bottom:8px;"`},
		},
		{
			name: "XSS escaping",
			marker: MapMarker{
				Name:      "<script>alert('xss')</script>",
				Location:  "<img onerror=alert(1)>",
				Extra:     "<b>bold</b>",
				DetailURL: "/ui/fac/400",
			},
			wantBody: []string{
				"&lt;script&gt;",
				"&lt;img onerror",
				"&lt;b&gt;bold&lt;/b&gt;",
			},
			noBody: []string{"<script>", "<img onerror", "<b>bold</b>"},
		},
		{
			name: "empty detail URL",
			marker: MapMarker{
				Name: "Test DC",
			},
			wantBody: []string{"Test DC"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildMultiPinPopupHTML(tt.marker)

			for _, want := range tt.wantBody {
				if !strings.Contains(got, want) {
					t.Errorf("popup missing %q in:\n%s", want, got)
				}
			}
			for _, notWant := range tt.noBody {
				if strings.Contains(got, notWant) {
					t.Errorf("popup should not contain %q in:\n%s", notWant, got)
				}
			}
		})
	}
}

func TestFilterMappableMarkers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		markers      []MapMarker
		wantMappable int
		wantUnmapped int
	}{
		{
			name: "all mappable",
			markers: []MapMarker{
				{Lat: 51.5, Lng: -0.1, Name: "London"},
				{Lat: 48.8, Lng: 2.3, Name: "Paris"},
				{Lat: 40.7, Lng: -74.0, Name: "New York"},
			},
			wantMappable: 3,
			wantUnmapped: 0,
		},
		{
			name: "all unmappable",
			markers: []MapMarker{
				{Lat: 0, Lng: 0, Name: "Unknown 1"},
				{Lat: 0, Lng: 0, Name: "Unknown 2"},
			},
			wantMappable: 0,
			wantUnmapped: 2,
		},
		{
			name: "mixed",
			markers: []MapMarker{
				{Lat: 51.5, Lng: -0.1, Name: "London"},
				{Lat: 0, Lng: 0, Name: "Unknown"},
				{Lat: 48.8, Lng: 2.3, Name: "Paris"},
			},
			wantMappable: 2,
			wantUnmapped: 1,
		},
		{
			name:         "empty input",
			markers:      nil,
			wantMappable: 0,
			wantUnmapped: 0,
		},
		{
			name: "equator non-zero lng",
			markers: []MapMarker{
				{Lat: 0, Lng: 10.5, Name: "Equator point"},
			},
			wantMappable: 1,
			wantUnmapped: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mappable, unmapped := filterMappableMarkers(tt.markers)

			if len(mappable) != tt.wantMappable {
				t.Errorf("mappable count = %d, want %d", len(mappable), tt.wantMappable)
			}
			if unmapped != tt.wantUnmapped {
				t.Errorf("unmapped count = %d, want %d", unmapped, tt.wantUnmapped)
			}
		})
	}
}

func TestFormatMapLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		city    string
		country string
		want    string
	}{
		{"both", "London", "GB", "London, GB"},
		{"city only", "London", "", "London"},
		{"country only", "", "GB", "GB"},
		{"neither", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatMapLocation(tt.city, tt.country); got != tt.want {
				t.Errorf("formatMapLocation(%q, %q) = %q, want %q", tt.city, tt.country, got, tt.want)
			}
		})
	}
}

func TestBuildPopupHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		marker   MapMarker
		wantBody []string
		noBody   []string
	}{
		{
			name: "full popup",
			marker: MapMarker{
				Lat:       51.5074,
				Lng:       -0.1278,
				Name:      "Equinix LD8",
				Location:  "London, GB",
				NetCount:  42,
				IXCount:   5,
				DetailURL: "/ui/fac/100",
			},
			wantBody: []string{
				"Equinix LD8",
				"London, GB",
				"42 Networks",
				"5 IXPs",
				"/ui/fac/100",
				"View facility",
			},
		},
		{
			name: "no location",
			marker: MapMarker{
				Name:      "Test DC",
				NetCount:  1,
				IXCount:   1,
				DetailURL: "/ui/fac/200",
			},
			wantBody: []string{"Test DC"},
			noBody:   []string{`color:#666;margin-bottom:4px;"`},
		},
		{
			name: "zero counts",
			marker: MapMarker{
				Name:      "Empty DC",
				Location:  "Nowhere",
				NetCount:  0,
				IXCount:   0,
				DetailURL: "/ui/fac/300",
			},
			wantBody: []string{"Empty DC", "Nowhere"},
			noBody:   []string{"Networks", "IXPs"},
		},
		{
			name: "XSS escaping",
			marker: MapMarker{
				Name:      "<script>alert('xss')</script>",
				Location:  "Test",
				NetCount:  1,
				IXCount:   1,
				DetailURL: "/ui/fac/400",
			},
			wantBody: []string{"&lt;script&gt;"},
			noBody:   []string{"<script>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildPopupHTML(tt.marker)

			for _, want := range tt.wantBody {
				if !strings.Contains(got, want) {
					t.Errorf("popup missing %q in:\n%s", want, got)
				}
			}
			for _, notWant := range tt.noBody {
				if strings.Contains(got, notWant) {
					t.Errorf("popup should not contain %q in:\n%s", notWant, got)
				}
			}
		})
	}
}
