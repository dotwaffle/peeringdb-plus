// Package termrender provides terminal client detection and ANSI text rendering
// for serving styled text responses to CLI clients like curl, wget, and HTTPie.
package termrender

import (
	"net/url"
	"strings"
)

// RenderMode describes the output format for a response.
type RenderMode int

const (
	// ModeHTML renders the standard web UI page.
	ModeHTML RenderMode = iota
	// ModeHTMX renders an htmx fragment (no layout shell).
	ModeHTMX
	// ModeRich renders ANSI-colored terminal output with Unicode box drawing.
	ModeRich
	// ModePlain renders plain ASCII text with no ANSI codes.
	ModePlain
	// ModeJSON renders the data as JSON.
	ModeJSON
	// ModeWHOIS renders RPSL-style key-value output.
	ModeWHOIS
)

// String returns a human-readable name for the render mode.
func (m RenderMode) String() string {
	switch m {
	case ModeHTML:
		return "HTML"
	case ModeHTMX:
		return "HTMX"
	case ModeRich:
		return "Rich"
	case ModePlain:
		return "Plain"
	case ModeJSON:
		return "JSON"
	case ModeWHOIS:
		return "WHOIS"
	default:
		return "Unknown"
	}
}

// DetectInput holds parameters for detecting the render mode.
// Defined per CS-5 to bundle >2 function arguments.
type DetectInput struct {
	Query     url.Values
	Accept    string
	UserAgent string
	HXRequest bool
}

// terminalPrefixes are User-Agent prefixes that identify terminal/CLI clients.
// Sourced from wttr.in's PLAIN_TEXT_AGENTS and user decisions (D-01).
// Note: "fetch" has no trailing slash because some implementations omit version.
var terminalPrefixes = []string{
	"curl/",
	"Wget/",
	"HTTPie/",
	"xh/",
	"PowerShell/",
	"fetch",
}

// Detect returns the appropriate render mode based on the priority chain:
// query params > Accept header > User-Agent > HX-Request > default (HTML).
func Detect(input DetectInput) RenderMode {
	// 1. Query param overrides (highest priority per D-02).
	if input.Query != nil {
		if _, ok := input.Query["T"]; ok {
			return ModePlain
		}
		switch input.Query.Get("format") {
		case "plain":
			return ModePlain
		case "json":
			return ModeJSON
		case "whois":
			return ModeWHOIS
		}
	}

	// 2. Accept header (secondary per D-03).
	if strings.Contains(input.Accept, "text/plain") {
		return ModeRich
	}
	if strings.Contains(input.Accept, "application/json") {
		return ModeJSON
	}

	// 3. User-Agent prefix match (tertiary per D-01).
	if isTerminalUA(input.UserAgent) {
		return ModeRich
	}

	// 4. HX-Request header (htmx fragment).
	if input.HXRequest {
		return ModeHTMX
	}

	return ModeHTML
}

// HasNoColor reports whether the request includes a ?nocolor query parameter,
// which suppresses all ANSI escape codes regardless of render mode (D-04, RND-18).
func HasNoColor(input DetectInput) bool {
	if input.Query == nil {
		return false
	}
	return input.Query.Has("nocolor")
}

// isTerminalUA checks whether the User-Agent identifies a terminal client
// by matching against known CLI tool prefixes case-insensitively.
func isTerminalUA(ua string) bool {
	lower := strings.ToLower(ua)
	for _, prefix := range terminalPrefixes {
		if strings.HasPrefix(lower, strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}
