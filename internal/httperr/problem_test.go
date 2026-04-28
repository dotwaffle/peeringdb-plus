package httperr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteProblem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		input           WriteProblemInput
		wantStatus      int
		wantType        string
		wantTitle       string
		wantDetail      string
		wantInstance    string
		detailPresent   bool
		instancePresent bool
	}{
		{
			name: "not found with detail and instance",
			input: WriteProblemInput{
				Status:   http.StatusNotFound,
				Title:    "Not Found",
				Detail:   "network with id 99999 not found",
				Instance: "/api/net/99999",
			},
			wantStatus:      http.StatusNotFound,
			wantType:        "about:blank",
			wantTitle:       "Not Found",
			wantDetail:      "network with id 99999 not found",
			wantInstance:    "/api/net/99999",
			detailPresent:   true,
			instancePresent: true,
		},
		{
			name: "bad request without instance",
			input: WriteProblemInput{
				Status: http.StatusBadRequest,
				Title:  "Bad Request",
				Detail: "invalid id: not an integer",
			},
			wantStatus:      http.StatusBadRequest,
			wantType:        "about:blank",
			wantTitle:       "Bad Request",
			wantDetail:      "invalid id: not an integer",
			detailPresent:   true,
			instancePresent: false,
		},
		{
			name: "internal server error without detail or instance",
			input: WriteProblemInput{
				Status: http.StatusInternalServerError,
				Title:  "Internal Server Error",
			},
			wantStatus:      http.StatusInternalServerError,
			wantType:        "about:blank",
			wantTitle:       "Internal Server Error",
			detailPresent:   false,
			instancePresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			WriteProblem(rec, tt.input)

			result := rec.Result()
			defer result.Body.Close()

			// Check status code.
			if result.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", result.StatusCode, tt.wantStatus)
			}

			// Check Content-Type.
			ct := result.Header.Get("Content-Type")
			if ct != "application/problem+json" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/problem+json")
			}

			// Decode body.
			var body map[string]any
			if err := json.NewDecoder(result.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			// Check type field.
			if got := body["type"]; got != tt.wantType {
				t.Errorf("type = %v, want %q", got, tt.wantType)
			}

			// Check title field.
			if got := body["title"]; got != tt.wantTitle {
				t.Errorf("title = %v, want %q", got, tt.wantTitle)
			}

			// Check status field (JSON numbers decode as float64).
			if got, ok := body["status"].(float64); !ok || int(got) != tt.wantStatus {
				t.Errorf("status in body = %v, want %d", body["status"], tt.wantStatus)
			}

			// Check detail presence and value.
			detail, detailOK := body["detail"]
			if tt.detailPresent {
				if !detailOK {
					t.Error("detail field missing, want present")
				} else if detail != tt.wantDetail {
					t.Errorf("detail = %v, want %q", detail, tt.wantDetail)
				}
			} else {
				if detailOK {
					t.Errorf("detail field present = %v, want absent", detail)
				}
			}

			// Check instance presence and value.
			instance, instanceOK := body["instance"]
			if tt.instancePresent {
				if !instanceOK {
					t.Error("instance field missing, want present")
				} else if instance != tt.wantInstance {
					t.Errorf("instance = %v, want %q", instance, tt.wantInstance)
				}
			} else {
				if instanceOK {
					t.Errorf("instance field present = %v, want absent", instance)
				}
			}
		})
	}
}

func TestWriteProblem_DefaultTitle(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	WriteProblem(rec, WriteProblemInput{
		Status: http.StatusNotFound,
		// Title intentionally empty -- should default to "Not Found".
	})

	result := rec.Result()
	defer result.Body.Close()

	var body ProblemDetail
	if err := json.NewDecoder(result.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Title != "Not Found" {
		t.Errorf("title = %q, want %q", body.Title, "Not Found")
	}
	if body.Status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", body.Status, http.StatusNotFound)
	}
}

func TestNewProblemDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input WriteProblemInput
		want  ProblemDetail
	}{
		{
			name: "full fields",
			input: WriteProblemInput{
				Status:   404,
				Title:    "Not Found",
				Detail:   "resource missing",
				Instance: "/api/net/1",
			},
			want: ProblemDetail{
				Type:     "about:blank",
				Title:    "Not Found",
				Status:   404,
				Detail:   "resource missing",
				Instance: "/api/net/1",
			},
		},
		{
			name: "default title from status",
			input: WriteProblemInput{
				Status: 500,
			},
			want: ProblemDetail{
				Type:   "about:blank",
				Title:  "Internal Server Error",
				Status: 500,
			},
		},
		{
			name: "custom title overrides default",
			input: WriteProblemInput{
				Status: 400,
				Title:  "Validation Error",
				Detail: "field 'name' is required",
			},
			want: ProblemDetail{
				Type:   "about:blank",
				Title:  "Validation Error",
				Status: 400,
				Detail: "field 'name' is required",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NewProblemDetail(tt.input)
			if got != tt.want {
				t.Errorf("NewProblemDetail() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
