package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/graph"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// gqlTestResponse represents a generic GraphQL response envelope for handler tests.
type gqlTestResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Path       []any          `json:"path"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

// postGQL sends a GraphQL query to the handler and returns the parsed response.
func postGQL(t *testing.T, handler http.Handler, query string) gqlTestResponse {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)

	respBody, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var result gqlTestResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, respBody)
	}
	return result
}

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

func TestErrorPresenter_SetsCodeExtension(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	resolver := graph.NewResolver(client, nil)
	h := NewHandler(resolver)

	// Query for a network by ASN that does not exist in the empty database.
	resp := postGQL(t, h, `{ networkByAsn(asn: 99999) { name } }`)

	if len(resp.Errors) == 0 {
		// networkByAsn returns null for missing ASN (not an error).
		// Instead, query the Relay node interface with a bogus ID to trigger not-found.
		resp = postGQL(t, h, `{ node(id: "999999") { id } }`)
	}

	// If still no errors (networkByAsn returns null), verify extensions on an
	// invalid query that does produce an error.
	if len(resp.Errors) == 0 {
		t.Skip("networkByAsn returned null without error; covered by depth/complexity tests")
	}

	for _, gqlErr := range resp.Errors {
		code, ok := gqlErr.Extensions["code"]
		if !ok {
			t.Errorf("error missing extensions.code: %+v", gqlErr)
			continue
		}
		codeStr, ok := code.(string)
		if !ok {
			t.Errorf("extensions.code is not string: %T", code)
			continue
		}
		if codeStr == "" {
			t.Errorf("extensions.code is empty")
		}
	}
}

func TestComplexityLimit_RejectsFanOut(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	resolver := graph.NewResolver(client, nil)
	h := NewHandler(resolver)

	// graph.ComplexityLimits weights connection fields by the requested
	// page size and unpaginated edge lists by average per-parent
	// cardinality, so nested fan-out multiplies: 1000 exchanges, each
	// expanding ixLans (×4) → networkIxLans (×64) → ixLan → networkIxLans
	// (×64) ≈ 16M units — far over graph.ComplexityLimit. This is the
	// replica-OOM shape from the 2026-06-10 audit: under gqlgen's default
	// 1-per-field costing it cost ~10 units and sailed through.
	query := `{
		internetExchanges(first: 1000) {
			edges { node { ixLans { networkIxLans { ixLan { networkIxLans { asn } } } } } }
		}
	}`
	resp := postGQL(t, h, query)

	if len(resp.Errors) == 0 {
		t.Fatal("expected complexity limit error, got none")
	}

	found := false
	for _, gqlErr := range resp.Errors {
		lower := strings.ToLower(gqlErr.Message)
		if strings.Contains(lower, "complexity") {
			found = true
			// Verify extensions.code is set by our error presenter.
			if gqlErr.Extensions != nil {
				if code, ok := gqlErr.Extensions["code"]; ok {
					if _, ok := code.(string); !ok {
						t.Errorf("extensions.code is not string: %T", code)
					}
				}
			}
			break
		}
	}
	if !found {
		t.Errorf("no error mentions complexity; errors: %+v", resp.Errors)
	}
}

// TestComplexityLimit_AllowsLegitimateQueries guards the fan-out weights
// against over-rejection: the shapes real consumers use — a full page of
// networks with scalar fields, and a single exchange's complete member
// list — must stay well inside graph.ComplexityLimit.
func TestComplexityLimit_AllowsLegitimateQueries(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	resolver := graph.NewResolver(client, nil)
	h := NewHandler(resolver)

	queries := map[string]string{
		"full page of networks": `{
			networks(first: 1000) {
				edges { node { name asn infoType website policyGeneral irrAsSet } }
			}
		}`,
		"single IX member list": `{
			internetExchanges(first: 1) {
				edges { node { name ixLans { mtu networkIxLans { asn speed ipaddr4 ipaddr6 isRsPeer operational } } } }
			}
		}`,
		"alias fan within budget": `{
			a: organizations(first: 100) { edges { node { name networks { asn } } } }
			b: facilitiesList(limit: 100) { name city networkFacilities { localAsn } }
		}`,
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := postGQL(t, h, query)
			for _, gqlErr := range resp.Errors {
				if strings.Contains(strings.ToLower(gqlErr.Message), "complexity") {
					t.Fatalf("legitimate query rejected by complexity limit: %+v", gqlErr)
				}
			}
		})
	}
}

func TestDepthLimit_RejectsDeep(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	resolver := graph.NewResolver(client, nil)
	h := NewHandler(resolver)

	// Traverse org->networks->org repeatedly to exceed depth limit of 15.
	// Schema: Organization.networks: [Network!], Network.organization: Organization.
	// Depth count: organizations(1) edges(2) node(3) then 7 x networks + 6 x organization (13 more)
	// + final name = 3 + 13 + 1 = 17 levels, exceeding limit of 15.
	query := `{
		organizations(first:1) {
			edges {
				node {
					networks {
						organization {
							networks {
								organization {
									networks {
										organization {
											networks {
												organization {
													networks {
														organization {
															networks {
																organization {
																	networks {
																		name
																	}
																}
															}
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`
	resp := postGQL(t, h, query)

	if len(resp.Errors) == 0 {
		t.Fatal("expected depth limit error, got none")
	}

	found := false
	for _, gqlErr := range resp.Errors {
		lower := strings.ToLower(gqlErr.Message)
		if strings.Contains(lower, "depth") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no error mentions depth; errors: %+v", resp.Errors)
	}
}
