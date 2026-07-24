package mcpserver

import (
	"net/http"
	"net/url"
	"strings"
)

func originGuard(allowed string, next http.Handler) http.Handler {
	patterns := splitOrigins(allowed)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.Host == "" || parsed.User != nil || parsed.Path != "" ||
			parsed.RawQuery != "" || parsed.Fragment != "" {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		for _, pattern := range patterns {
			if pattern == "*" || originMatches(pattern, origin) {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "origin not allowed", http.StatusForbidden)
	})
}

func splitOrigins(value string) []string {
	var output []string
	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			output = append(output, item)
		}
	}
	return output
}

func originMatches(pattern, origin string) bool {
	if pattern == origin {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return false
	}
	prefix, suffix, ok := strings.Cut(pattern, "*")
	return ok && strings.HasPrefix(origin, prefix) && strings.HasSuffix(origin, suffix)
}
