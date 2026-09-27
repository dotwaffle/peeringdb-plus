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
		{name: "absolute tile service", tileSource: "https://tiles.example.com:8443", want: "img-src 'self' data: https://tiles.example.com:8443;"},
		{name: "same-origin tile service", want: "img-src 'self' data:;", notWant: "tiles.example.com"},
		{name: "no CDN", want: "style-src 'self' 'unsafe-inline';", notWant: "jsdelivr"},
		{name: "no plugins", want: "object-src 'none'"},
		{name: "no base element from elsewhere", want: "base-uri 'self'"},
		{name: "never framed", want: "frame-ancestors 'none'"},
		{name: "forms submit to the same origin", want: "form-action 'self'"},
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
