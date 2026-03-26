// Package httperr provides RFC 9457 Problem Details for HTTP API error responses.
//
// RFC 9457 defines a standard JSON format for conveying machine-readable error
// details in HTTP responses. This package implements a minimal helper for
// producing "application/problem+json" responses with the required fields.
package httperr

import (
	"encoding/json"
	"net/http"
)

// ProblemDetail represents an RFC 9457 Problem Details response.
// The Type field defaults to "about:blank" when no specific problem type URI
// applies. Title and Status are always present; Detail and Instance are
// optional and omitted from the JSON when empty.
type ProblemDetail struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// WriteProblemInput holds parameters for writing a problem detail response.
// Status is required. Title defaults to the standard HTTP status text when
// empty. Detail and Instance are optional.
type WriteProblemInput struct {
	Status   int
	Title    string
	Detail   string
	Instance string
}

// WriteProblem writes an RFC 9457 problem detail JSON response to w.
// It sets the Content-Type to "application/problem+json", writes the HTTP
// status code, and encodes the problem detail as JSON.
func WriteProblem(w http.ResponseWriter, input WriteProblemInput) {
	p := NewProblemDetail(input)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(input.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// NewProblemDetail constructs a ProblemDetail struct from the given input
// without writing an HTTP response. This is useful for embedding problem
// details inside other JSON response structures (e.g., terminal JSON mode).
func NewProblemDetail(input WriteProblemInput) ProblemDetail {
	title := input.Title
	if title == "" {
		title = http.StatusText(input.Status)
	}
	return ProblemDetail{
		Type:     "about:blank",
		Title:    title,
		Status:   input.Status,
		Detail:   input.Detail,
		Instance: input.Instance,
	}
}
