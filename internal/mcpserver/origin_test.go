package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOriginGuard(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := originGuard("https://example.com,https://*.example.net", next)

	tests := []struct {
		name   string
		origin string
		want   int
	}{
		{name: "non-browser", want: http.StatusNoContent},
		{name: "exact", origin: "https://example.com", want: http.StatusNoContent},
		{name: "wildcard", origin: "https://agent.example.net", want: http.StatusNoContent},
		{name: "rejected", origin: "https://example.org", want: http.StatusForbidden},
		{name: "malformed", origin: "not a url", want: http.StatusForbidden},
		{name: "path rejected", origin: "https://example.com/path", want: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			request.Header.Set("Origin", test.origin)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			assert.Equal(t, test.want, recorder.Code)
		})
	}
}

func TestOriginGuardWildcardAll(t *testing.T) {
	t.Parallel()

	handler := originGuard("*", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Origin", "https://client.example")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}
