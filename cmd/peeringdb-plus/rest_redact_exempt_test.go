package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestRESTOpenAPIRedactExemption locks the openapi.json skip in
// middleware.RESTFieldRedact: the spec is a static document that
// describes the ixf_ixp_member_list_url SCHEMA but can never carry the
// _visible companion as a data key, so it bypasses the buffer+parse+walk
// and must still contain the schema property intact. The sibling
// assertion proves the exemption is exact-path only: an ix-lans data
// response through the same fixture still redacts the gated field for
// the anonymous tier.
func TestRESTOpenAPIRedactExemption(t *testing.T) {
	t.Parallel()
	fix := buildPrivacySurfacesFixture(t)

	resp, err := fix.server.Client().Get(fix.server.URL + "/rest/v1/openapi.json")
	if err != nil {
		t.Fatalf("GET openapi.json: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("spec status = %d, want 200", resp.StatusCode)
	}
	var spec map[string]any
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("spec is not valid JSON: %v", err)
	}
	if !strings.Contains(string(body), `"ixf_ixp_member_list_url"`) {
		t.Errorf("spec no longer documents ixf_ixp_member_list_url — the redaction exemption should leave the schema untouched")
	}

	// Contrast: a data response is NOT exempt. seed.Full's ixlan 100 is
	// Users-gated, so the anonymous fixture must not serve its URL.
	resp, err = fix.server.Client().Get(fix.server.URL + "/rest/v1/ix-lans")
	if err != nil {
		t.Fatalf("GET ix-lans: %v", err)
	}
	body, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read ix-lans: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ix-lans status = %d, want 200", resp.StatusCode)
	}
	if strings.Contains(string(body), "ix/100/members.json") {
		t.Errorf("anonymous ix-lans response leaked the Users-gated member list URL — redaction must still apply to data paths")
	}
}
