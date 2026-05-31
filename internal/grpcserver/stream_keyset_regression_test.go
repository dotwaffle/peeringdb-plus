package grpcserver

import (
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestStreamKeysetCarriesCreated is the regression guard for the three-key
// stream cursor. The Stream* handlers order by (-updated, -created, -id), but
// the keyset cursor formerly carried only (updated, id). Within an equal-
// `updated` group ordered by `created` DESC, resuming on id alone at a batch
// boundary silently DROPS rows whose `created` is older but whose id is higher
// than the boundary row — and can duplicate others.
//
// The seed below makes `created` and `id` disagree: every row shares one
// `updated` timestamp, but `created` decreases as `id` increases (id=1 is the
// newest-created, id=N the oldest). Under (-updated, -created, -id) the only
// correct order is therefore id ASCENDING: [1, 2, 3, ... N]. The id-DESC
// tiebreaker that the two-key cursor resumed on points the opposite way, so a
// boundary-straddling group exposed the drop.
//
// streamBatchSize is a hard const (500), so the test seeds 1001 rows — enough
// that the single equal-`updated` group straddles two batch boundaries. The
// stream MUST return all 1001 rows exactly once in id-ascending order.
//
// With the two-key cursor this test fails (fewer than 1001 rows, wrong order);
// with the three-key cursor it passes.
func TestStreamKeysetCarriesCreated(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)

	const total = streamBatchSize*2 + 1 // 1001: straddles two 500-row boundaries.

	// One shared `updated` so the whole table is a single equal-updated group,
	// forcing the (-created, -id) secondary keys to drive the order. `created`
	// strictly decreases as id increases, so created-DESC == id-ASC and the
	// id-DESC tiebreaker disagrees — the exact shape that drops rows under a
	// two-key (updated, id) cursor.
	updated := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	createdBase := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	builders := make([]*ent.NetworkCreate, total)
	for i := range total {
		id := i + 1
		created := createdBase.Add(time.Duration(total-id) * time.Minute)
		builders[i] = client.Network.Create().
			SetID(id).
			SetName("net").
			SetAsn(65000 + id).
			SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
			SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
			SetAllowIxpUpdate(false).
			SetCreated(created).SetUpdated(updated).
			SetStatus("ok")
	}
	client.Network.CreateBulk(builders...).SaveX(ctx)

	rpcClient := setupStreamTestServer(t, client)

	stream, err := rpcClient.StreamNetworks(ctx, &pb.StreamNetworksRequest{})
	if err != nil {
		t.Fatalf("StreamNetworks returned error: %v", err)
	}

	seen := make(map[int64]int, total)
	var order []int64
	for stream.Receive() {
		msg := stream.Msg()
		if msg == nil {
			t.Fatal("received nil message from stream")
		}
		seen[msg.GetId()]++
		order = append(order, msg.GetId())
	}
	if streamErr := stream.Err(); streamErr != nil {
		t.Fatalf("stream error: %v", streamErr)
	}

	// No drops: every seeded id present exactly once.
	if len(order) != total {
		t.Errorf("streamed %d rows, want %d (a row-count gap means the cursor dropped rows at a batch boundary)", len(order), total)
	}
	var missing, dup []int64
	for id := int64(1); id <= total; id++ {
		switch seen[id] {
		case 1:
			// present exactly once — correct.
		case 0:
			missing = append(missing, id)
		default:
			dup = append(dup, id)
		}
	}
	if len(missing) > 0 {
		head := missing
		if len(head) > 10 {
			head = head[:10]
		}
		t.Errorf("dropped %d rows at batch boundaries; first missing ids: %v", len(missing), head)
	}
	if len(dup) > 0 {
		head := dup
		if len(head) > 10 {
			head = head[:10]
		}
		t.Errorf("duplicated %d rows across batch boundaries; first duplicated ids: %v", len(dup), head)
	}

	// Correct (-updated, -created, -id) order: with shared updated and
	// created-DESC == id-ASC, the emitted ids must be strictly ascending.
	for i := 1; i < len(order); i++ {
		if order[i] <= order[i-1] {
			lo := max(i-3, 0)
			hi := min(i+3, len(order))
			t.Fatalf("order not (-updated,-created,-id) at position %d: id %d follows id %d (window %v)",
				i, order[i], order[i-1], order[lo:hi])
		}
	}
}
