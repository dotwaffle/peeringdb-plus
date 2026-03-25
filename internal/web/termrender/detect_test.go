package termrender

import (
	"net/url"
	"testing"
)

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input DetectInput
		want  RenderMode
	}{
		{
			name:  "curl user-agent returns ModeRich",
			input: DetectInput{UserAgent: "curl/8.5.0"},
			want:  ModeRich,
		},
		{
			name:  "Wget user-agent returns ModeRich",
			input: DetectInput{UserAgent: "Wget/1.21"},
			want:  ModeRich,
		},
		{
			name:  "HTTPie user-agent returns ModeRich",
			input: DetectInput{UserAgent: "HTTPie/3.2.4"},
			want:  ModeRich,
		},
		{
			name:  "xh user-agent returns ModeRich",
			input: DetectInput{UserAgent: "xh/0.22.2"},
			want:  ModeRich,
		},
		{
			name:  "PowerShell user-agent returns ModeRich",
			input: DetectInput{UserAgent: "PowerShell/7.4"},
			want:  ModeRich,
		},
		{
			name:  "fetch user-agent returns ModeRich",
			input: DetectInput{UserAgent: "fetch"},
			want:  ModeRich,
		},
		{
			name:  "browser user-agent returns ModeHTML",
			input: DetectInput{UserAgent: "Mozilla/5.0"},
			want:  ModeHTML,
		},
		{
			name:  "empty user-agent returns ModeHTML",
			input: DetectInput{UserAgent: ""},
			want:  ModeHTML,
		},
		{
			name: "query param T forces ModePlain over curl UA",
			input: DetectInput{
				Query:     url.Values{"T": {""}},
				UserAgent: "curl/8.5.0",
			},
			want: ModePlain,
		},
		{
			name: "query param format=plain forces ModePlain over browser UA",
			input: DetectInput{
				Query:     url.Values{"format": {"plain"}},
				UserAgent: "Mozilla/5.0",
			},
			want: ModePlain,
		},
		{
			name: "query param format=json forces ModeJSON over browser UA",
			input: DetectInput{
				Query:     url.Values{"format": {"json"}},
				UserAgent: "Mozilla/5.0",
			},
			want: ModeJSON,
		},
		{
			name: "Accept text/plain returns ModeRich over browser UA",
			input: DetectInput{
				Accept:    "text/plain",
				UserAgent: "Mozilla/5.0",
			},
			want: ModeRich,
		},
		{
			name: "Accept application/json returns ModeJSON over browser UA",
			input: DetectInput{
				Accept:    "application/json",
				UserAgent: "Mozilla/5.0",
			},
			want: ModeJSON,
		},
		{
			name: "query param beats Accept header",
			input: DetectInput{
				Query:     url.Values{"format": {"plain"}},
				Accept:    "application/json",
				UserAgent: "curl/8.5.0",
			},
			want: ModePlain,
		},
		{
			name: "Accept header beats User-Agent",
			input: DetectInput{
				Accept:    "application/json",
				UserAgent: "curl/8.5.0",
			},
			want: ModeJSON,
		},
		{
			name: "HX-Request with browser UA returns ModeHTMX",
			input: DetectInput{
				HXRequest: true,
				UserAgent: "Mozilla/5.0",
			},
			want: ModeHTMX,
		},
		{
			name: "terminal UA beats HX-Request",
			input: DetectInput{
				HXRequest: true,
				UserAgent: "curl/8.5.0",
			},
			want: ModeRich,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Detect(tt.input)
			if got != tt.want {
				t.Errorf("Detect(%+v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHasNoColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input DetectInput
		want  bool
	}{
		{
			name:  "no params returns false",
			input: DetectInput{},
			want:  false,
		},
		{
			name: "nocolor param returns true",
			input: DetectInput{
				Query: url.Values{"nocolor": {""}},
			},
			want: true,
		},
		{
			name: "nocolor with other params returns true",
			input: DetectInput{
				Query: url.Values{"nocolor": {""}, "format": {"plain"}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := HasNoColor(tt.input)
			if got != tt.want {
				t.Errorf("HasNoColor(%+v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsTerminalUA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ua   string
		want bool
	}{
		{name: "curl", ua: "curl/8.5.0", want: true},
		{name: "Wget", ua: "Wget/1.21", want: true},
		{name: "HTTPie", ua: "HTTPie/3.2.4", want: true},
		{name: "xh", ua: "xh/0.22.2", want: true},
		{name: "PowerShell", ua: "PowerShell/7.4", want: true},
		{name: "fetch no version", ua: "fetch", want: true},
		{name: "fetch with path", ua: "fetch/1.0", want: true},
		{name: "uppercase CURL", ua: "CURL/8.0", want: true},
		{name: "lowercase wget", ua: "wget/1.21", want: true},
		{name: "mixed case Httpie", ua: "httpie/3.2.4", want: true},
		{name: "browser UA", ua: "Mozilla/5.0 (X11; Linux x86_64)", want: false},
		{name: "empty string", ua: "", want: false},
		{name: "random string", ua: "my-custom-client/1.0", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isTerminalUA(tt.ua)
			if got != tt.want {
				t.Errorf("isTerminalUA(%q) = %v, want %v", tt.ua, got, tt.want)
			}
		})
	}
}
