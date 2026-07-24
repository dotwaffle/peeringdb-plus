// Package agentdocs serves the installable PeeringDB Plus agent skill.
package agentdocs

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	// SkillPath is the raw skill document endpoint.
	SkillPath = "/skills/peeringdb-plus/SKILL.md"
	// ArchivePath is the installable skill archive endpoint.
	ArchivePath = "/skills/peeringdb-plus.zip"

	skillArchivePath  = "peeringdb-plus/SKILL.md"
	openAIArchivePath = "peeringdb-plus/agents/openai.yaml"
	mcpPath           = "/mcp"
)

var zipEpoch = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

//go:embed peeringdb-plus/SKILL.md
var skillDocument []byte

// Options configures a Handler.
type Options struct {
	// PublicURL overrides request-derived origins in generated metadata. It
	// must be an absolute HTTP(S) origin with no credentials, path, query, or
	// fragment.
	PublicURL string
	// SourceTime sets archive entry modification times. Zero values and dates
	// before the ZIP epoch use the ZIP epoch.
	SourceTime time.Time
}

// Handler serves the raw skill document and its installable archive.
type Handler struct {
	publicURL  *url.URL
	sourceTime time.Time
}

// NewHandler validates options and returns a skill document handler.
func NewHandler(options Options) (*Handler, error) {
	var publicURL *url.URL
	if options.PublicURL != "" {
		parsed, err := parsePublicURL(options.PublicURL)
		if err != nil {
			return nil, fmt.Errorf("validate public URL: %w", err)
		}
		publicURL = parsed
	}

	return &Handler{
		publicURL:  publicURL,
		sourceTime: normalizeSourceTime(options.SourceTime),
	}, nil
}

// Register mounts the skill document routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+SkillPath, h.serveSkill)
	mux.HandleFunc("HEAD "+SkillPath, h.serveSkill)
	mux.HandleFunc("GET "+ArchivePath, h.serveArchive)
	mux.HandleFunc("HEAD "+ArchivePath, h.serveArchive)
}

func (h *Handler) serveSkill(w http.ResponseWriter, r *http.Request) {
	serveDocument(w, r, serveDocumentInput{
		name:        "SKILL.md",
		content:     skillDocument,
		contentType: "text/markdown; charset=utf-8",
		disposition: `inline; filename="SKILL.md"`,
		modified:    h.sourceTime,
	})
}

func (h *Handler) serveArchive(w http.ResponseWriter, r *http.Request) {
	origin, err := h.origin(r)
	if err != nil {
		http.Error(w, "invalid request host", http.StatusBadRequest)
		return
	}

	content, err := h.archive(origin)
	if err != nil {
		http.Error(w, "build skill archive", http.StatusInternalServerError)
		return
	}

	serveDocument(w, r, serveDocumentInput{
		name:        "peeringdb-plus.zip",
		content:     content,
		contentType: "application/zip",
		disposition: `attachment; filename="peeringdb-plus.zip"`,
		modified:    h.sourceTime,
	})
}

type serveDocumentInput struct {
	name        string
	content     []byte
	contentType string
	disposition string
	modified    time.Time
}

func serveDocument(w http.ResponseWriter, r *http.Request, input serveDocumentInput) {
	sum := sha256.Sum256(input.content)
	header := w.Header()
	header.Set("Cache-Control", "public, max-age=600")
	header.Set("Content-Disposition", input.disposition)
	header.Set("Content-Type", input.contentType)
	header.Set("ETag", `"`+fmt.Sprintf("%x", sum)+`"`)
	header.Set("Vary", "X-Forwarded-Proto")

	http.ServeContent(
		w,
		r,
		input.name,
		input.modified,
		bytes.NewReader(input.content),
	)
}

func (h *Handler) archive(origin string) ([]byte, error) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entries := []struct {
		name    string
		content []byte
	}{
		{name: skillArchivePath, content: skillDocument},
		{name: openAIArchivePath, content: openAIYAML(origin)},
	}

	for _, entry := range entries {
		header := &zip.FileHeader{
			Name:     entry.name,
			Method:   zip.Store,
			Modified: h.sourceTime,
		}
		header.SetMode(0o644)

		file, err := writer.CreateHeader(header)
		if err != nil {
			return nil, fmt.Errorf("create %s: %w", entry.name, err)
		}
		if _, err := file.Write(entry.content); err != nil {
			return nil, fmt.Errorf("write %s: %w", entry.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close archive: %w", err)
	}
	return buf.Bytes(), nil
}

func (h *Handler) origin(r *http.Request) (string, error) {
	if h.publicURL != nil {
		return h.publicURL.String(), nil
	}

	host, err := normalizeHost(r.Host)
	if err != nil {
		return "", err
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if forwarded := forwardedProto(r.Header.Values("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}

	return (&url.URL{Scheme: scheme, Host: host}).String(), nil
}

func openAIYAML(origin string) []byte {
	return fmt.Appendf(nil, `interface:
  display_name: "PeeringDB Plus"
  short_description: "Query the PeeringDB Plus network mirror"
  default_prompt: "Use $peeringdb-plus to research networks and interconnection data."

dependencies:
  tools:
    - type: "mcp"
      value: "peeringdb-plus"
      description: "PeeringDB Plus read-only research tools"
      transport: "streamable_http"
      url: %s
`, strconv.Quote(origin+mcpPath))
}

func parsePublicURL(raw string) (*url.URL, error) {
	if hasUnsafeText(raw) {
		return nil, fmt.Errorf("contains whitespace or control characters")
	}
	if strings.Contains(raw, "#") {
		return nil, fmt.Errorf("fragment is not allowed")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("scheme must be http or https")
	}
	if parsed.Opaque != "" {
		return nil, fmt.Errorf("opaque URL is not allowed")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("userinfo is not allowed")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, fmt.Errorf("path is not allowed")
	}
	if parsed.RawPath != "" {
		return nil, fmt.Errorf("encoded path is not allowed")
	}
	if parsed.ForceQuery || parsed.RawQuery != "" {
		return nil, fmt.Errorf("query is not allowed")
	}
	if parsed.Fragment != "" || parsed.RawFragment != "" {
		return nil, fmt.Errorf("fragment is not allowed")
	}

	host, err := normalizeHost(parsed.Host)
	if err != nil {
		return nil, fmt.Errorf("host: %w", err)
	}
	parsed.Host = host
	parsed.Path = ""
	return parsed, nil
}

func normalizeHost(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("is empty")
	}
	if hasUnsafeText(raw) {
		return "", fmt.Errorf("contains whitespace or control characters")
	}
	if strings.ContainsAny(raw, `/\?#@`) {
		return "", fmt.Errorf("contains URL delimiters")
	}

	host := raw
	port := ""
	if strings.HasPrefix(raw, "[") {
		end := strings.IndexByte(raw, ']')
		if end < 0 {
			return "", fmt.Errorf("IPv6 address is missing closing bracket")
		}
		host = raw[1:end]
		rest := raw[end+1:]
		if rest != "" {
			if !strings.HasPrefix(rest, ":") {
				return "", fmt.Errorf("unexpected text after IPv6 address")
			}
			port = rest[1:]
		}
		if strings.Contains(host, "%") || net.ParseIP(host) == nil || !strings.Contains(host, ":") {
			return "", fmt.Errorf("invalid IPv6 address")
		}
		host = "[" + strings.ToLower(host) + "]"
	} else {
		if strings.ContainsAny(raw, "[]") {
			return "", fmt.Errorf("invalid brackets")
		}
		if strings.Count(raw, ":") > 1 {
			return "", fmt.Errorf("IPv6 addresses must be bracketed")
		}
		if name, value, found := strings.Cut(raw, ":"); found {
			host = name
			port = value
		}
		if err := validateHostname(host); err != nil {
			return "", err
		}
		host = strings.ToLower(host)
	}

	if port != "" {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return "", fmt.Errorf("invalid port")
		}
		return host + ":" + strconv.FormatUint(value, 10), nil
	}
	if strings.HasSuffix(raw, ":") {
		return "", fmt.Errorf("port is empty")
	}
	return host, nil
}

func validateHostname(host string) error {
	if host == "" {
		return fmt.Errorf("hostname is empty")
	}
	if ip := net.ParseIP(host); ip != nil {
		return nil
	}
	if len(host) > 253 || strings.HasSuffix(host, ".") {
		return fmt.Errorf("invalid hostname")
	}
	for label := range strings.SplitSeq(host, ".") {
		if len(label) == 0 || len(label) > 63 ||
			label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("invalid hostname")
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') &&
				(char < 'A' || char > 'Z') &&
				(char < '0' || char > '9') &&
				char != '-' {
				return fmt.Errorf("invalid hostname")
			}
		}
	}
	return nil
}

func forwardedProto(values []string) string {
	if len(values) != 1 || hasUnsafeText(values[0]) {
		return ""
	}
	switch strings.ToLower(values[0]) {
	case "http":
		return "http"
	case "https":
		return "https"
	default:
		return ""
	}
}

func hasUnsafeText(value string) bool {
	return strings.IndexFunc(value, func(char rune) bool {
		return unicode.IsSpace(char) || unicode.IsControl(char)
	}) >= 0
}

func normalizeSourceTime(value time.Time) time.Time {
	if value.IsZero() || value.Before(zipEpoch) {
		return zipEpoch
	}
	return value.UTC().Truncate(time.Second)
}
