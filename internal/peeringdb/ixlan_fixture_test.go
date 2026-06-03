package peeringdb_test

// JSON-decoder contract test for the ixf_ixp_member_list_url field.
//
// Proves that once IXFIXPMemberListURL is defined on peeringdb.IxLan with
// the json tag `ixf_ixp_member_list_url,omitempty`, the stdlib json decoder
// picks up the upstream field without any client.go change.
//
// Guards against a future regression where the tag is accidentally stripped
// or renamed. The authenticated baseline fixture is canonical.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

func TestIxLan_FixtureRoundTrip_HasURLField(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "testdata", "visibility-baseline", "beta", "auth", "api", "ixlan", "page-1.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}

	var env struct {
		Data []peeringdb.IxLan `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if len(env.Data) == 0 {
		t.Fatal("fixture contained zero ixlan rows")
	}

	var populated int
	for _, il := range env.Data {
		if il.IXFIXPMemberListURL != "" {
			populated++
		}
	}
	if populated == 0 {
		t.Fatalf("expected at least one ixlan row with IXFIXPMemberListURL populated; none found across %d rows — decoder did not pick up the json tag", len(env.Data))
	}
	t.Logf("populated URL rows: %d / %d", populated, len(env.Data))
}
