package agentdocs

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var testSourceTime = time.Date(2026, time.July, 24, 12, 34, 56, 0, time.UTC)

func TestNewHandlerValidatesPublicURL(t *testing.T) {
	t.Parallel()

	valid := []struct {
		name string
		raw  string
		want string
	}{
		{name: "unset"},
		{name: "HTTP", raw: "HTTP://Example.COM:8080/", want: "http://example.com:8080"},
		{name: "HTTPS", raw: "https://example.com", want: "https://example.com"},
		{name: "IPv4", raw: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{name: "IPv6", raw: "https://[2001:DB8::1]:443", want: "https://[2001:db8::1]:443"},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, err := NewHandler(Options{PublicURL: test.raw})
			if err != nil {
				t.Fatalf("NewHandler() error = %v", err)
			}
			if test.want == "" {
				if handler.publicURL != nil {
					t.Fatalf("publicURL = %q, want nil", handler.publicURL)
				}
				return
			}
			if got := handler.publicURL.String(); got != test.want {
				t.Errorf("publicURL = %q, want %q", got, test.want)
			}
		})
	}

	invalid := []struct {
		name string
		raw  string
	}{
		{name: "scheme", raw: "ftp://example.com"},
		{name: "relative", raw: "example.com"},
		{name: "missing host", raw: "https:///"},
		{name: "userinfo", raw: "https://user@example.com"},
		{name: "path", raw: "https://example.com/base"},
		{name: "encoded path", raw: "https://example.com/%2F"},
		{name: "query", raw: "https://example.com?source=test"},
		{name: "empty query", raw: "https://example.com?"},
		{name: "fragment", raw: "https://example.com#top"},
		{name: "empty fragment", raw: "https://example.com#"},
		{name: "control", raw: "https://example.com/\n"},
		{name: "space", raw: "https://example .com"},
		{name: "bad port", raw: "https://example.com:abc"},
		{name: "empty port", raw: "https://example.com:"},
		{name: "port zero", raw: "https://example.com:0"},
		{name: "port overflow", raw: "https://example.com:65536"},
		{name: "bad hostname", raw: "https://bad_host.example"},
		{name: "trailing dot", raw: "https://example.com."},
		{name: "unbracketed IPv6", raw: "https://2001:db8::1"},
		{name: "IPv6 zone", raw: "https://[fe80::1%25eth0]"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewHandler(Options{PublicURL: test.raw}); err == nil {
				t.Fatalf("NewHandler(%q) error = nil, want validation error", test.raw)
			}
		})
	}
}

func TestHandlerServesRawSkill(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, Options{SourceTime: testSourceTime})
	mux := http.NewServeMux()
	handler.Register(mux)

	get := serveRequest(mux, http.MethodGet, "http://example.com"+SkillPath)
	if get.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", get.Code, http.StatusOK)
	}
	if !bytes.Equal(get.Body.Bytes(), skillDocument) {
		t.Error("GET body differs from embedded SKILL.md")
	}
	assertDocumentHeaders(t, get, "text/markdown; charset=utf-8", `inline; filename="SKILL.md"`)

	sum := sha256.Sum256(skillDocument)
	wantETag := fmt.Sprintf(`"%x"`, sum)
	if got := get.Header().Get("ETag"); got != wantETag {
		t.Errorf("ETag = %q, want %q", got, wantETag)
	}

	head := serveRequest(mux, http.MethodHead, "http://example.com"+SkillPath)
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want %d", head.Code, http.StatusOK)
	}
	if head.Body.Len() != 0 {
		t.Errorf("HEAD body length = %d, want 0", head.Body.Len())
	}
	if got := head.Header().Get("Content-Length"); got != fmt.Sprint(len(skillDocument)) {
		t.Errorf("HEAD Content-Length = %q, want %d", got, len(skillDocument))
	}
	if got := head.Header().Get("ETag"); got != wantETag {
		t.Errorf("HEAD ETag = %q, want %q", got, wantETag)
	}

	conditionalRequest := httptest.NewRequest(http.MethodGet, "http://example.com"+SkillPath, nil)
	conditionalRequest.Header.Set("If-None-Match", wantETag)
	conditional := httptest.NewRecorder()
	mux.ServeHTTP(conditional, conditionalRequest)
	if conditional.Code != http.StatusNotModified {
		t.Errorf("conditional status = %d, want %d", conditional.Code, http.StatusNotModified)
	}
	if conditional.Body.Len() != 0 {
		t.Errorf("conditional body length = %d, want 0", conditional.Body.Len())
	}

	modifiedRequest := httptest.NewRequest(http.MethodGet, "http://example.com"+SkillPath, nil)
	modifiedRequest.Header.Set("If-Modified-Since", get.Header().Get("Last-Modified"))
	notModified := httptest.NewRecorder()
	mux.ServeHTTP(notModified, modifiedRequest)
	if notModified.Code != http.StatusNotModified {
		t.Errorf("If-Modified-Since status = %d, want %d", notModified.Code, http.StatusNotModified)
	}
}

func TestHandlerArchiveOriginResolution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		host           string
		tls            bool
		forwardedProto []string
		forwardedHost  string
		wantOrigin     string
	}{
		{
			name:       "plain request",
			host:       "Example.COM:8080",
			wantOrigin: "http://example.com:8080",
		},
		{
			name:           "forwarded HTTPS",
			host:           "example.com",
			forwardedProto: []string{"https"},
			wantOrigin:     "https://example.com",
		},
		{
			name:           "forwarded HTTP",
			host:           "example.com",
			forwardedProto: []string{"http"},
			wantOrigin:     "http://example.com",
		},
		{
			name:           "TLS wins over forwarded proto",
			host:           "example.com",
			tls:            true,
			forwardedProto: []string{"http"},
			wantOrigin:     "https://example.com",
		},
		{
			name:           "invalid forwarded proto falls back",
			host:           "example.com",
			forwardedProto: []string{"javascript"},
			wantOrigin:     "http://example.com",
		},
		{
			name:           "multiple forwarded values fall back",
			host:           "example.com",
			forwardedProto: []string{"https", "http"},
			wantOrigin:     "http://example.com",
		},
		{
			name:          "forwarded host ignored",
			host:          "request.example",
			forwardedHost: "attacker.example",
			wantOrigin:    "http://request.example",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler := newTestHandler(t, Options{SourceTime: testSourceTime})
			mux := http.NewServeMux()
			handler.Register(mux)

			request := httptest.NewRequest(http.MethodGet, "http://placeholder"+ArchivePath, nil)
			request.Host = test.host
			if test.tls {
				request.TLS = &tls.ConnectionState{}
			}
			for _, value := range test.forwardedProto {
				request.Header.Add("X-Forwarded-Proto", value)
			}
			if test.forwardedHost != "" {
				request.Header.Set("X-Forwarded-Host", test.forwardedHost)
			}

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
			}

			files := readArchive(t, recorder.Body.Bytes())
			if got := string(files[openAIArchivePath].content); got != string(openAIYAML(test.wantOrigin)) {
				t.Errorf("openai.yaml =\n%s\nwant:\n%s", got, openAIYAML(test.wantOrigin))
			}
		})
	}
}

func TestHandlerConfiguredOriginOverridesRequest(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, Options{
		PublicURL:  "https://Public.Example/",
		SourceTime: testSourceTime,
	})
	mux := http.NewServeMux()
	handler.Register(mux)

	request := httptest.NewRequest(http.MethodGet, "http://internal"+ArchivePath, nil)
	request.Host = "bad_host"
	request.Header.Set("X-Forwarded-Proto", "javascript")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	files := readArchive(t, recorder.Body.Bytes())
	if got := string(files[openAIArchivePath].content); got != string(openAIYAML("https://public.example")) {
		t.Errorf("openai.yaml =\n%s\nwant configured public origin", got)
	}
}

func TestHandlerRejectsInvalidRequestHost(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"bad_host",
		"example.com/path",
		"example.com\n",
		"user@example.com",
		"example.com:0",
		"example.com:65536",
		"2001:db8::1",
		"[bad]",
	}

	handler := newTestHandler(t, Options{SourceTime: testSourceTime})
	mux := http.NewServeMux()
	handler.Register(mux)
	for _, host := range tests {
		t.Run(host, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(http.MethodGet, "http://placeholder"+ArchivePath, nil)
			request.Host = host
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandlerArchiveContentsAndCaching(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, Options{SourceTime: testSourceTime})
	mux := http.NewServeMux()
	handler.Register(mux)

	first := serveRequest(mux, http.MethodGet, "https://example.com"+ArchivePath)
	if first.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", first.Code, http.StatusOK)
	}
	assertDocumentHeaders(t, first, "application/zip", `attachment; filename="peeringdb-plus.zip"`)

	files := readArchive(t, first.Body.Bytes())
	wantNames := []string{skillArchivePath, openAIArchivePath}
	if got := archiveNames(t, first.Body.Bytes()); strings.Join(got, "\n") != strings.Join(wantNames, "\n") {
		t.Errorf("archive order = %q, want %q", got, wantNames)
	}
	for _, name := range wantNames {
		file := files[name]
		if got := file.mode.Perm(); got != 0o644 {
			t.Errorf("%s mode = %o, want 644", name, got)
		}
		if !file.modified.Equal(testSourceTime) {
			t.Errorf("%s modified = %v, want %v", name, file.modified, testSourceTime)
		}
	}
	if !bytes.Equal(files[skillArchivePath].content, skillDocument) {
		t.Error("archived SKILL.md differs from embedded document")
	}
	if got := string(files[openAIArchivePath].content); got != string(openAIYAML("https://example.com")) {
		t.Errorf("openai.yaml =\n%s\nwant:\n%s", got, openAIYAML("https://example.com"))
	}

	sum := sha256.Sum256(first.Body.Bytes())
	wantETag := fmt.Sprintf(`"%x"`, sum)
	if got := first.Header().Get("ETag"); got != wantETag {
		t.Errorf("ETag = %q, want %q", got, wantETag)
	}

	second := serveRequest(mux, http.MethodGet, "https://example.com"+ArchivePath)
	if !bytes.Equal(second.Body.Bytes(), first.Body.Bytes()) {
		t.Error("repeated archive response is not byte-stable")
	}
	if got := second.Header().Get("ETag"); got != wantETag {
		t.Errorf("second ETag = %q, want %q", got, wantETag)
	}

	otherOrigin := serveRequest(mux, http.MethodGet, "https://other.example"+ArchivePath)
	if bytes.Equal(otherOrigin.Body.Bytes(), first.Body.Bytes()) {
		t.Error("archives for different origins are equal")
	}
	if got := otherOrigin.Header().Get("ETag"); got == wantETag {
		t.Errorf("other-origin ETag = %q, want content-specific value", got)
	}

	head := serveRequest(mux, http.MethodHead, "https://example.com"+ArchivePath)
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want %d", head.Code, http.StatusOK)
	}
	if head.Body.Len() != 0 {
		t.Errorf("HEAD body length = %d, want 0", head.Body.Len())
	}
	if got := head.Header().Get("Content-Length"); got != fmt.Sprint(first.Body.Len()) {
		t.Errorf("HEAD Content-Length = %q, want %d", got, first.Body.Len())
	}
	if got := head.Header().Get("ETag"); got != wantETag {
		t.Errorf("HEAD ETag = %q, want %q", got, wantETag)
	}

	request := httptest.NewRequest(http.MethodGet, "https://example.com"+ArchivePath, nil)
	request.Header.Set("If-None-Match", wantETag)
	conditional := httptest.NewRecorder()
	mux.ServeHTTP(conditional, request)
	if conditional.Code != http.StatusNotModified {
		t.Errorf("conditional status = %d, want %d", conditional.Code, http.StatusNotModified)
	}
	if conditional.Body.Len() != 0 {
		t.Errorf("conditional body length = %d, want 0", conditional.Body.Len())
	}
}

func TestHandlerUsesZIPEpochFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sourceTime time.Time
	}{
		{name: "zero"},
		{name: "before epoch", sourceTime: time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler := newTestHandler(t, Options{SourceTime: test.sourceTime})
			mux := http.NewServeMux()
			handler.Register(mux)
			recorder := serveRequest(mux, http.MethodGet, "http://example.com"+ArchivePath)
			files := readArchive(t, recorder.Body.Bytes())
			for name, file := range files {
				if !file.modified.Equal(zipEpoch) {
					t.Errorf("%s modified = %v, want %v", name, file.modified, zipEpoch)
				}
			}
		})
	}
}

func TestHandlerRoutesRejectOtherMethods(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, Options{})
	mux := http.NewServeMux()
	handler.Register(mux)

	for _, path := range []string{SkillPath, ArchivePath} {
		request := httptest.NewRequest(http.MethodPost, "http://example.com"+path, nil)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s status = %d, want %d", path, recorder.Code, http.StatusMethodNotAllowed)
		}
	}
}

type archiveFile struct {
	content  []byte
	mode     fs.FileMode
	modified time.Time
}

func readArchive(t *testing.T, content []byte) map[string]archiveFile {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	files := make(map[string]archiveFile, len(reader.File))
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		body, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil {
			t.Fatalf("read %s: %v", file.Name, readErr)
		}
		if closeErr != nil {
			t.Fatalf("close %s: %v", file.Name, closeErr)
		}
		files[file.Name] = archiveFile{
			content:  body,
			mode:     file.Mode(),
			modified: file.Modified,
		}
	}
	return files
}

func archiveNames(t *testing.T, content []byte) []string {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	return names
}

func assertDocumentHeaders(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	contentType string,
	disposition string,
) {
	t.Helper()

	headers := recorder.Header()
	if got := headers.Get("Cache-Control"); got != "public, max-age=600" {
		t.Errorf("Cache-Control = %q, want public, max-age=600", got)
	}
	if got := headers.Get("Content-Type"); got != contentType {
		t.Errorf("Content-Type = %q, want %q", got, contentType)
	}
	if got := headers.Get("Content-Disposition"); got != disposition {
		t.Errorf("Content-Disposition = %q, want %q", got, disposition)
	}
	if got := headers.Get("Vary"); got != "X-Forwarded-Proto" {
		t.Errorf("Vary = %q, want X-Forwarded-Proto", got)
	}
	if got := headers.Get("Last-Modified"); got != testSourceTime.Format(http.TimeFormat) {
		t.Errorf("Last-Modified = %q, want %q", got, testSourceTime.Format(http.TimeFormat))
	}
}

func newTestHandler(t *testing.T, options Options) *Handler {
	t.Helper()

	handler, err := NewHandler(options)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

func serveRequest(handler http.Handler, method string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
