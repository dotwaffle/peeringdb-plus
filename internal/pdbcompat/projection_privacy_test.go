package pdbcompat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestFieldProjection_RedactedGatedFieldStaysAbsent locks the omitempty
// contract at the wire level: privfield redaction zeroes the gated
// ixf_ixp_member_list_url for anonymous callers and relies on the
// `,omitempty` json tag to drop the KEY. The ?fields= projection path
// converts the serializer struct to a map before json.Marshal ever sees
// the tag, so the converter must honour omitempty itself — the former
// projection converter did not, and a projected anonymous response
// carried `"ixf_ixp_member_list_url": ""` where the unprojected response
// (and upstream) omit the key entirely.
func TestFieldProjection_RedactedGatedFieldStaysAbsent(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	// id=1: gated (Users tier) — anonymous callers get the value redacted
	// and the key omitted. id=2: Public — key + value surface.
	if _, err := client.IxLan.Create().
		SetID(1).SetName("GatedLan").
		SetIxfIxpMemberListURL("https://example.test/gated/members.json").
		SetIxfIxpMemberListURLVisible("Users").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		Save(ctx); err != nil {
		t.Fatalf("seed gated ixlan: %v", err)
	}
	if _, err := client.IxLan.Create().
		SetID(2).SetName("PublicLan").
		SetIxfIxpMemberListURL("https://example.test/public/members.json").
		SetIxfIxpMemberListURLVisible("Public").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		Save(ctx); err != nil {
		t.Fatalf("seed public ixlan: %v", err)
	}

	h := NewHandler(client, 0)
	mux := http.NewServeMux()
	h.Register(mux)

	// No privacy-tier stamp on the request context — privctx fails closed
	// to the anonymous tier, exactly like an unauthenticated caller.
	req := httptest.NewRequest(http.MethodGet, "/api/ixlan?fields=ixf_ixp_member_list_url", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("got %d rows, want 2", len(body.Data))
	}

	for _, row := range body.Data {
		id, _ := row["id"].(float64)
		url, present := row["ixf_ixp_member_list_url"]
		switch int(id) {
		case 1:
			if present {
				t.Errorf("gated row: ixf_ixp_member_list_url present under ?fields= projection (value=%v), want key absent", url)
			}
		case 2:
			if !present {
				t.Errorf("public row: ixf_ixp_member_list_url missing, want present")
			} else if url != "https://example.test/public/members.json" {
				t.Errorf("public row: ixf_ixp_member_list_url = %v, want seeded URL", url)
			}
		default:
			t.Errorf("unexpected row id %v", row["id"])
		}
	}
}
