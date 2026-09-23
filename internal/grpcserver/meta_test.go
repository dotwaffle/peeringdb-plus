package grpcserver

import (
	"bytes"
	"log"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// metaDoc is a netixlan meta document with the PeeringDB 2.83.0 launch
// keys. Numbers are float64, as they are after a JSON round trip.
func metaDoc() map[string]any {
	return map[string]any{
		"planned_status_change": map[string]any{"status": "deleted", "date": "2026-12-01"},
		"rfc8950":               true,
		"weight":                float64(3),
	}
}

func TestMetaStruct(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		doc     map[string]any
		wantNil bool
		want    map[string]any
	}{
		// Upstream serializes an unset meta as {}, so nil and empty both
		// become a present, empty Struct.
		{name: "nil", doc: nil, want: map[string]any{}},
		{name: "empty", doc: map[string]any{}, want: map[string]any{}},
		{name: "document", doc: metaDoc(), want: metaDoc()},
		// structpb rejects invalid UTF-8: the field is omitted.
		{name: "unconvertible", doc: map[string]any{"bad": "\xff"}, wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := metaStruct(t.Context(), "network", 1, tt.doc)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("metaStruct = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("metaStruct = nil, want a Struct")
			}
			if m := got.AsMap(); !reflect.DeepEqual(m, tt.want) {
				t.Errorf("metaStruct.AsMap() = %v, want %v", m, tt.want)
			}
		})
	}
}

// TestMetaStruct_LogsUnconvertible is not parallel: it swaps the default
// slog logger, and a sequential top-level test never runs alongside the
// package's parallel tests. slog.SetDefault also sends the log package
// output to the new handler, and restoring the old default does not undo
// that, so the cleanup restores the log output and flags as well (see
// captureDefaultLogs).
func TestMetaStruct_LogsUnconvertible(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	prevOut, prevFlags := log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(prev)
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})

	if got := metaStruct(t.Context(), "networkixlan", 7, map[string]any{"bad": "\xff"}); got != nil {
		t.Fatalf("metaStruct = %v, want nil", got)
	}
	out := buf.String()
	for _, want := range []string{"level=WARN", "meta document", "entity=networkixlan", "id=7"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output %q does not contain %q", out, want)
		}
	}
}

// TestMeta_RPCs checks that Get, List and Stream converters carry meta on
// Network and NetworkIxLan: the stored document when set, an empty Struct
// when not.
func TestMeta_RPCs(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

	client.Network.Create().
		SetID(1).SetName("With Meta").SetAsn(65001).
		SetMeta(map[string]any{"preferred_ip_mtu": float64(9000)}).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Network.Create().
		SetID(2).SetName("Without Meta").SetAsn(65002).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkIxLan.Create().
		SetID(1).SetAsn(65001).SetSpeed(10000).SetNetworkID(1).
		SetMeta(metaDoc()).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	netSvc := &NetworkService{Client: client}
	nixlSvc := &NetworkIxLanService{Client: client}

	t.Run("GetNetwork document", func(t *testing.T) {
		t.Parallel()
		resp, err := netSvc.GetNetwork(ctx, &pb.GetNetworkRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetNetwork(1): %v", err)
		}
		want := map[string]any{"preferred_ip_mtu": float64(9000)}
		if got := resp.GetNetwork().GetMeta().AsMap(); !reflect.DeepEqual(got, want) {
			t.Errorf("meta = %v, want %v", got, want)
		}
	})

	t.Run("ListNetworks empty document", func(t *testing.T) {
		t.Parallel()
		resp, err := netSvc.ListNetworks(ctx, &pb.ListNetworksRequest{Id: new(int64(2))})
		if err != nil {
			t.Fatalf("ListNetworks: %v", err)
		}
		if len(resp.GetNetworks()) != 1 {
			t.Fatalf("got %d networks, want 1", len(resp.GetNetworks()))
		}
		meta := resp.GetNetworks()[0].GetMeta()
		if meta == nil || len(meta.GetFields()) != 0 {
			t.Errorf("meta = %v, want a present, empty Struct", meta)
		}
	})

	t.Run("GetNetworkIxLan document", func(t *testing.T) {
		t.Parallel()
		resp, err := nixlSvc.GetNetworkIxLan(ctx, &pb.GetNetworkIxLanRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetNetworkIxLan(1): %v", err)
		}
		if got := resp.GetNetworkIxLan().GetMeta().AsMap(); !reflect.DeepEqual(got, metaDoc()) {
			t.Errorf("meta = %v, want %v", got, metaDoc())
		}
	})

	// Over the wire, so this also checks that an empty Struct stays
	// present after binary encoding.
	t.Run("StreamNetworks", func(t *testing.T) {
		t.Parallel()
		rpcClient := setupStreamTestServer(t, client)
		stream, err := rpcClient.StreamNetworks(ctx, &pb.StreamNetworksRequest{})
		if err != nil {
			t.Fatalf("StreamNetworks: %v", err)
		}
		got := map[int64]*pb.Network{}
		for stream.Receive() {
			got[stream.Msg().GetId()] = stream.Msg()
		}
		if err := stream.Err(); err != nil {
			t.Fatalf("stream error: %v", err)
		}
		want := map[string]any{"preferred_ip_mtu": float64(9000)}
		if m := got[1].GetMeta().AsMap(); !reflect.DeepEqual(m, want) {
			t.Errorf("network 1 meta = %v, want %v", m, want)
		}
		if meta := got[2].GetMeta(); meta == nil || len(meta.GetFields()) != 0 {
			t.Errorf("network 2 meta = %v, want a present, empty Struct", meta)
		}
	})

	t.Run("ListNetworkIxLans document", func(t *testing.T) {
		t.Parallel()
		resp, err := nixlSvc.ListNetworkIxLans(ctx, &pb.ListNetworkIxLansRequest{})
		if err != nil {
			t.Fatalf("ListNetworkIxLans: %v", err)
		}
		if len(resp.GetNetworkIxLans()) != 1 {
			t.Fatalf("got %d netixlans, want 1", len(resp.GetNetworkIxLans()))
		}
		if got := resp.GetNetworkIxLans()[0].GetMeta().AsMap(); !reflect.DeepEqual(got, metaDoc()) {
			t.Errorf("meta = %v, want %v", got, metaDoc())
		}
	})
}
