package graphql

import (
	"fmt"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

func TestClassifyError(t *testing.T) {
	t.Parallel()
	// ValidationError has an unexported err field; wrap in fmt.Errorf to
	// produce a usable error value without accessing private fields.
	validationErr := fmt.Errorf("field: %w", &ent.ValidationError{Name: "field"})

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "INTERNAL_ERROR"},
		{"not found", &ent.NotFoundError{}, "NOT_FOUND"},
		{"validation wrapped", validationErr, "VALIDATION_ERROR"},
		{"constraint", &ent.ConstraintError{}, "CONSTRAINT_ERROR"},
		{"wrapped not found", fmt.Errorf("query: %w", &ent.NotFoundError{}), "NOT_FOUND"},
		{"wrapped constraint", fmt.Errorf("update: %w", &ent.ConstraintError{}), "CONSTRAINT_ERROR"},
		{"unknown", fmt.Errorf("random error"), "INTERNAL_ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classifyError(tt.err)
			if got != tt.want {
				t.Errorf("classifyError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
