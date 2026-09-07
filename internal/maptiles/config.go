// Package maptiles validates browser basemap configuration.
package maptiles

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	// DefaultURL is the OpenStreetMap Standard raster tile endpoint.
	DefaultURL = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"
	// DefaultAttribution is the attribution required for OpenStreetMap tiles.
	DefaultAttribution = `&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors`
)

// Config contains the URL template and visible attribution for a basemap.
type Config struct {
	URL         string
	Attribution string
}

// New validates a tile URL template and its attribution. Empty values select
// the OpenStreetMap defaults. A custom URL requires custom attribution.
func New(rawURL, attribution string) (Config, error) {
	rawURL = strings.TrimSpace(rawURL)
	attribution = strings.TrimSpace(attribution)

	if rawURL == "" {
		rawURL = DefaultURL
	}
	if attribution == "" {
		if rawURL != DefaultURL {
			return Config{}, errors.New("PDBPLUS_MAP_TILE_ATTRIBUTION is required when PDBPLUS_MAP_TILE_URL is customized")
		}
		attribution = DefaultAttribution
	}

	if err := validateURL(rawURL); err != nil {
		return Config{}, err
	}

	return Config{URL: rawURL, Attribution: attribution}, nil
}

// Default returns the validated OpenStreetMap tile configuration.
func Default() Config {
	return Config{URL: DefaultURL, Attribution: DefaultAttribution}
}

// CSPSource returns the absolute tile origin for a Content Security Policy.
// A root-relative URL returns an empty source because 'self' already permits it.
func (c Config) CSPSource() string {
	if strings.HasPrefix(c.URL, "/") {
		return ""
	}
	u, err := url.Parse(c.URL)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func validateURL(rawURL string) error {
	if strings.HasPrefix(rawURL, "//") || strings.Contains(rawURL, `\`) {
		return errors.New("PDBPLUS_MAP_TILE_URL must be root-relative or use http:// or https://")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("PDBPLUS_MAP_TILE_URL is not a valid URL: %w", err)
	}
	if u.Fragment != "" {
		return errors.New("PDBPLUS_MAP_TILE_URL must not include a fragment")
	}

	if strings.HasPrefix(rawURL, "/") {
		if u.IsAbs() || u.Host != "" {
			return errors.New("PDBPLUS_MAP_TILE_URL must be root-relative or use http:// or https://")
		}
	} else {
		if u.Scheme != "http" && u.Scheme != "https" {
			return errors.New("PDBPLUS_MAP_TILE_URL must be root-relative or use http:// or https://")
		}
		if u.Hostname() == "" {
			return errors.New("PDBPLUS_MAP_TILE_URL must include a host")
		}
		if u.User != nil {
			return errors.New("PDBPLUS_MAP_TILE_URL must not include user information")
		}
		if strings.ContainsAny(u.Host, "'\"; \\	\r\n") {
			return errors.New("PDBPLUS_MAP_TILE_URL contains an invalid host")
		}
	}

	template := u.Path + "?" + u.RawQuery
	for _, placeholder := range []string{"{z}", "{x}", "{y}"} {
		if !strings.Contains(template, placeholder) {
			return fmt.Errorf("PDBPLUS_MAP_TILE_URL must contain %s", placeholder)
		}
	}
	return nil
}
