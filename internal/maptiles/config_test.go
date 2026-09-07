package maptiles

import (
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		url         string
		attribution string
		want        Config
		wantErr     string
	}{
		{name: "defaults", want: Default()},
		{
			name:        "custom HTTPS",
			url:         "https://tiles.example.com/{z}/{x}/{y}.png?key=public-key",
			attribution: "Example Maps",
			want: Config{
				URL:         "https://tiles.example.com/{z}/{x}/{y}.png?key=public-key",
				Attribution: "Example Maps",
			},
		},
		{
			name:        "custom HTTP",
			url:         "http://tiles.internal/{z}/{x}/{y}.png",
			attribution: "Internal map",
			want: Config{
				URL:         "http://tiles.internal/{z}/{x}/{y}.png",
				Attribution: "Internal map",
			},
		},
		{
			name:        "root relative",
			url:         "/tiles/{z}/{x}/{y}.png",
			attribution: "Company map",
			want: Config{
				URL:         "/tiles/{z}/{x}/{y}.png",
				Attribution: "Company map",
			},
		},
		{name: "custom URL needs attribution", url: "https://tiles.example.com/{z}/{x}/{y}.png", wantErr: "PDBPLUS_MAP_TILE_ATTRIBUTION"},
		{name: "missing z", url: "https://tiles.example.com/0/{x}/{y}.png", attribution: "Example", wantErr: "{z}"},
		{name: "missing x", url: "https://tiles.example.com/{z}/0/{y}.png", attribution: "Example", wantErr: "{x}"},
		{name: "missing y", url: "https://tiles.example.com/{z}/{x}/0.png", attribution: "Example", wantErr: "{y}"},
		{name: "protocol relative", url: "//tiles.example.com/{z}/{x}/{y}.png", attribution: "Example", wantErr: "root-relative"},
		{name: "unsafe scheme", url: "javascript:alert(1)?z={z}&x={x}&y={y}", attribution: "Example", wantErr: "http://"},
		{name: "missing host", url: "https:///{z}/{x}/{y}.png", attribution: "Example", wantErr: "host"},
		{name: "user information", url: "https://user:pass@tiles.example.com/{z}/{x}/{y}.png", attribution: "Example", wantErr: "user information"},
		{name: "fragment", url: "https://tiles.example.com/{z}/{x}/{y}.png#map", attribution: "Example", wantErr: "fragment"},
		{name: "root-relative fragment", url: "/tiles/{z}/{x}/{y}.png#map", attribution: "Example", wantErr: "fragment"},
		{name: "placeholders only in fragment", url: "https://tiles.example.com/map.png#{z}/{x}/{y}", attribution: "Example", wantErr: "fragment"},
		{name: "root-relative control character", url: "/tiles/{z}/{x}/\n{y}.png", attribution: "Example", wantErr: "valid URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := New(tt.url, tt.attribution)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("New() succeeded, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("New() error = %q, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("New() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestConfigCSPSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "HTTPS", url: "https://tiles.example.com:8443/{z}/{x}/{y}.png", want: "https://tiles.example.com:8443"},
		{name: "HTTP", url: "http://tiles.internal/{z}/{x}/{y}.png", want: "http://tiles.internal"},
		{name: "same origin", url: "/tiles/{z}/{x}/{y}.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := (Config{URL: tt.url}).CSPSource()
			if got != tt.want {
				t.Errorf("CSPSource() = %q, want %q", got, tt.want)
			}
		})
	}
}
