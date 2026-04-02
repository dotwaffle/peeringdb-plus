package middleware_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

func TestCompression(t *testing.T) {
	t.Parallel()

	// gzhttp has a minimum size threshold (~256 bytes by default).
	// Use a body large enough to trigger compression.
	largeBody := strings.Repeat("hello world ", 100) // ~1200 bytes

	tests := []struct {
		name           string
		acceptEncoding string
		contentType    string
		wantGzip       bool
	}{
		{
			name:           "gzip response when Accept-Encoding includes gzip",
			acceptEncoding: "gzip",
			contentType:    "text/html",
			wantGzip:       true,
		},
		{
			name:           "no gzip when Accept-Encoding is absent",
			acceptEncoding: "",
			contentType:    "text/html",
			wantGzip:       false,
		},
		{
			name:           "no gzip for application/grpc content type",
			acceptEncoding: "gzip",
			contentType:    "application/grpc",
			wantGzip:       false,
		},
		{
			name:           "no gzip for application/grpc+proto content type",
			acceptEncoding: "gzip",
			contentType:    "application/grpc+proto",
			wantGzip:       false,
		},
		{
			name:           "no gzip for application/connect+proto content type",
			acceptEncoding: "gzip",
			contentType:    "application/connect+proto",
			wantGzip:       false,
		},
		{
			name:           "gzip for application/json content type",
			acceptEncoding: "gzip",
			contentType:    "application/json",
			wantGzip:       true,
		},
	}

	compressionMW := middleware.Compression()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, largeBody)
			})

			handler := compressionMW(inner)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tc.acceptEncoding)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			gotEncoding := rec.Header().Get("Content-Encoding")

			if tc.wantGzip {
				if gotEncoding != "gzip" {
					t.Fatalf("Content-Encoding = %q, want %q", gotEncoding, "gzip")
				}

				// Verify body is valid gzip.
				gz, err := gzip.NewReader(rec.Body)
				if err != nil {
					t.Fatalf("failed to create gzip reader: %v", err)
				}
				defer gz.Close()

				decoded, err := io.ReadAll(gz)
				if err != nil {
					t.Fatalf("failed to read gzip body: %v", err)
				}
				if string(decoded) != largeBody {
					t.Errorf("decoded body length = %d, want %d", len(decoded), len(largeBody))
				}
			} else {
				if gotEncoding == "gzip" {
					t.Errorf("Content-Encoding = %q, want no gzip for content-type %q", gotEncoding, tc.contentType)
				}
			}
		})
	}
}
