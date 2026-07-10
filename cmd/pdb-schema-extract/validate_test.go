package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// validateTestSchema builds a two-type schema whose "net" fields match
// the fake API and whose "org" schema is missing most API fields (to
// trip the >30% missing-field failure).
func validateTestSchema() *Schema {
	return &Schema{ObjectTypes: map[string]ObjectType{
		"net": {
			APIPath: "net",
			Fields: map[string]FieldDef{
				"name": {Type: "CharField"},
				"asn":  {Type: "IntegerField"},
			},
		},
		"org": {
			APIPath: "org",
			Fields:  map[string]FieldDef{"name": {Type: "CharField"}},
		},
	}}
}

// TestValidateAgainstAPI exercises the httptest-parameterised validator:
// requests carry the buildinfo User-Agent, types are visited in sorted
// order, aligned types pass, and a type whose schema is missing >30% of
// the API's fields fails the run.
func TestValidateAgainstAPI(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	var uaSeen atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		uaSeen.Store(r.Header.Get("User-Agent"))
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/net"):
			fmt.Fprint(w, `{"data":[{"id":1,"name":"n","asn":64500,"status":"ok","created":"x","updated":"x"}]}`)
		case strings.HasPrefix(r.URL.Path, "/api/org"):
			// 6 fields beyond the common set; schema declares only "name"
			// → >30% of API fields missing from schema.
			fmt.Fprint(w, `{"data":[{"id":1,"name":"o","aka":"a","website":"w","city":"c","state":"s","country":"cc","zipcode":"z","status":"ok","created":"x","updated":"x"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	err := validateAgainstAPI(validateTestSchema(), validateAPIInput{
		BaseURL: srv.URL + "/api",
		Client:  srv.Client(),
		Pause:   0,
	})
	if err == nil {
		t.Fatal("want validation error for org (>30% API fields missing from schema), got nil")
	}
	if !strings.Contains(err.Error(), "org:") {
		t.Errorf("error should cite org, got: %v", err)
	}
	if strings.Contains(err.Error(), "net:") {
		t.Errorf("net is aligned and must not error, got: %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("request count = %d, want 2 (one per type)", got)
	}
	ua, _ := uaSeen.Load().(string)
	if !strings.HasPrefix(ua, "pdb-schema-extract/") || !strings.Contains(ua, "github.com/dotwaffle/peeringdb-plus") {
		t.Errorf("User-Agent = %q, want pdb-schema-extract/<version> (+repo URL)", ua)
	}
}

// TestValidateAgainstAPI_HTTPErrorSurfaces confirms a non-200 response
// becomes a per-type validation error rather than a silent skip.
func TestValidateAgainstAPI_HTTPErrorSurfaces(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	schema := &Schema{ObjectTypes: map[string]ObjectType{
		"net": {APIPath: "net", Fields: map[string]FieldDef{"name": {Type: "CharField"}}},
	}}
	err := validateAgainstAPI(schema, validateAPIInput{BaseURL: srv.URL + "/api", Client: srv.Client()})
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") {
		t.Errorf("want HTTP 429 validation error, got: %v", err)
	}
}
