package pdbcompat

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
)

const (
	// DefaultLimit is the default page size for list endpoints.
	DefaultLimit = 250

	// MaxLimit is the maximum allowed page size per D-21.
	MaxLimit = 1000

	// poweredByHeader identifies this server in responses per D-25.
	poweredByHeader = "PeeringDB-Plus/1.1"
)

// envelope is the PeeringDB response wrapper: {"meta": {}, "data": [...]}.
type envelope struct {
	Meta any `json:"meta"`
	Data any `json:"data"`
}

// WriteResponse writes a successful PeeringDB-compatible JSON response with
// the standard envelope format per D-04. Data must be a slice.
func WriteResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Powered-By", poweredByHeader)

	resp := envelope{
		Meta: struct{}{},
		Data: data,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// WriteProblem writes an RFC 9457 problem detail error response with the
// X-Powered-By header. This replaces the former PeeringDB error envelope
// with a standards-based format per ARCH-01.
func WriteProblem(w http.ResponseWriter, input httperr.WriteProblemInput) {
	w.Header().Set("X-Powered-By", poweredByHeader)
	httperr.WriteProblem(w, input)
}

// ParsePaginationParams extracts limit and skip from query parameters with
// defaults and bounds per D-16, D-21.
func ParsePaginationParams(params url.Values) (limit, skip int) {
	limit = DefaultLimit
	if v := params.Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	if v := params.Get("skip"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			skip = parsed
		}
	}
	return limit, skip
}

// ParseSinceParam parses the ?since= query parameter as a Unix timestamp
// per D-15. Returns nil if the parameter is absent or empty.
func ParseSinceParam(params url.Values) (*time.Time, error) {
	v := params.Get("since")
	if v == "" {
		return nil, nil
	}
	t, err := parseTime(v)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
