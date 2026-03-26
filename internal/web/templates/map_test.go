package templates

import (
	"strings"
	"testing"
)

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
