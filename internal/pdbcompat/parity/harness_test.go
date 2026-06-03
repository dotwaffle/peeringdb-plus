package parity

import (
	"net/http"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestHarness_NewTestServer_RoundTrip asserts the test server wires
// the canonical pdbcompat handler correctly: GET /api/ returns the
// index document with HTTP 200 + a non-empty JSON body.
//
// synthesised: phase72-04-harness
func TestHarness_NewTestServer_RoundTrip(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	srv := newTestServer(t, c)

	status, body := httpGet(t, srv, "/api/")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, string(body))
	}
	if len(body) == 0 {
		t.Errorf("body empty; want non-empty index JSON")
	}
}

// TestHarness_DecodeDataArray asserts the envelope decoder handles a
// well-formed list response. The index endpoint isn't a list shape, so
// it seeds one network inline and lists /api/net.
//
// synthesised: phase72-04-harness
func TestHarness_DecodeDataArray(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if _, err := c.Network.Create().
		SetID(1000).SetName("DecoderProbe").SetAsn(65000).SetStatus("ok").
		SetCreated(t0).SetUpdated(t0).
		Save(t.Context()); err != nil {
		t.Fatalf("seed net id=1000: %v", err)
	}

	srv := newTestServer(t, c)
	status, body := httpGet(t, srv, "/api/net")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", status, string(body))
	}
	rows := decodeDataArray(t, body)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if name, _ := rows[0]["name"].(string); name != "DecoderProbe" {
		t.Errorf("name = %q, want DecoderProbe", name)
	}
}
