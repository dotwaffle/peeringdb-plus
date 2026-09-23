package main

import (
	"strings"
	"testing"
)

func TestUICSPPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tileSource string
		want       string
		notWant    string
	}{
		{name: "absolute tile service", tileSource: "https://tiles.example.com:8443", want: "img-src 'self' data: https://tiles.example.com:8443 https://cdn.jsdelivr.net"},
		{name: "same-origin tile service", want: "img-src 'self' data: https://cdn.jsdelivr.net", notWant: "tiles.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := uiCSPPolicy(tt.tileSource)
			if !strings.Contains(got, tt.want) {
				t.Errorf("uiCSPPolicy() = %q, want substring %q", got, tt.want)
			}
			if tt.notWant != "" && strings.Contains(got, tt.notWant) {
				t.Errorf("uiCSPPolicy() = %q, must not contain %q", got, tt.notWant)
			}
		})
	}
}

// TestGraphQLCSPPolicy_AllowsDataFonts checks that the /graphql policy lets
// GraphiQL load the fonts that its stylesheet embeds as data: URIs.
func TestGraphQLCSPPolicy_AllowsDataFonts(t *testing.T) {
	t.Parallel()
	const want = "font-src 'self' data:"
	if !strings.Contains(graphQLCSPPolicy, want) {
		t.Errorf("graphQLCSPPolicy = %q, want substring %q", graphQLCSPPolicy, want)
	}
}
