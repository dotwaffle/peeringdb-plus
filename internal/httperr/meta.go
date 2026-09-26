package httperr

import (
	"encoding/json"
	"maps"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// problemJSONMediaType is the RFC 9457 media type.
const problemJSONMediaType = "application/problem+json"

// MetaErrorInput holds the parameters of a PeeringDB /api/ error body.
// Upstream 2.83.0 renderers.py:134-148 writes a 4xx body as
// {"meta": {"error": "<detail>"}} with no "data" key.
type MetaErrorInput struct {
	// Status is the HTTP status code.
	Status int
	// Error is the meta.error text.
	Error string
	// Meta holds more meta keys. An "error" key here is ignored.
	Meta map[string]any
	// EmptyData adds "data": [] (the upstream unique-query 404,
	// rest.py:809-815).
	EmptyData bool
}

// WriteMetaError writes a PeeringDB /api/ error body as
// application/json: {"meta": {"error": "<text>", ...}}, plus
// "data": [] when in.EmptyData is set.
func WriteMetaError(w http.ResponseWriter, in MetaErrorInput) {
	meta := make(map[string]any, len(in.Meta)+1)
	maps.Copy(meta, in.Meta)
	meta["error"] = in.Error

	body := map[string]any{"meta": meta}
	if in.EmptyData {
		body["data"] = []any{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(in.Status)
	_ = json.NewEncoder(w).Encode(body)
}

// WantsProblemJSON reports whether the Accept header of a request names
// application/problem+json with a q value above 0 and not above 1.
// The wildcards */* and application/* do not count, and the q values
// of the ranges are not compared: a client that names the type can
// parse it. A range that does not parse is skipped.
func WantsProblemJSON(h http.Header) bool {
	for _, value := range h.Values("Accept") {
		for part := range strings.SplitSeq(value, ",") {
			mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(part))
			if err != nil || mediaType != problemJSONMediaType {
				continue
			}
			if v, ok := params["q"]; ok {
				q, err := strconv.ParseFloat(v, 64)
				// The range check also rejects NaN and Inf, which
				// ParseFloat accepts.
				if err != nil || !(q > 0 && q <= 1) {
					continue
				}
			}
			return true
		}
	}
	return false
}
