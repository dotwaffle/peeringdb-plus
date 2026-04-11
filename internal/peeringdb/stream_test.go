package peeringdb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// fixtureDir is the location of shared PeeringDB fixture JSON files.
const fixtureDir = "../../testdata/fixtures"

// fixtureTypes is the table of all 13 fixture types for round-trip decode tests.
// Each entry maps a PeeringDB type constant to its fixture filename.
var fixtureTypes = []struct {
	objectType string
	file       string
}{
	{TypeOrg, "org.json"},
	{TypeCampus, "campus.json"},
	{TypeFac, "fac.json"},
	{TypeCarrier, "carrier.json"},
	{TypeCarrierFac, "carrierfac.json"},
	{TypeIX, "ix.json"},
	{TypeIXLan, "ixlan.json"},
	{TypeIXPfx, "ixpfx.json"},
	{TypeIXFac, "ixfac.json"},
	{TypeNet, "net.json"},
	{TypePoc, "poc.json"},
	{TypeNetFac, "netfac.json"},
	{TypeNetIXLan, "netixlan.json"},
}

// newStreamingTestClient returns a Client pointed at the given server URL with
// the rate limiter disabled so tests run fast.
func newStreamingTestClient(serverURL string) *Client {
	c := NewClient(serverURL, slog.Default())
	c.limiter.SetLimit(1000)
	c.limiter.SetBurst(1000)
	c.retryBaseDelay = 0
	return c
}

// newFixtureServer stands up an httptest server that serves a fixture blob at
// /api/{objectType} for the given object type and a fixed blob for all other
// paths. If blob is nil the server returns 404.
func newFixtureServer(objectType string, blob []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/")
		path = strings.Split(path, "?")[0]
		if path != objectType || blob == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(blob)
	}))
}

// TestStreamingDecoder_FixtureRoundTrip asserts that for each of the 13 fixture
// files, the streaming decoder emits the same element list (byte-for-byte)
// as the legacy io.ReadAll+json.Unmarshal path. This is the parity lock
// between the new StreamAll and the old FetchAll behavior.
func TestStreamingDecoder_FixtureRoundTrip(t *testing.T) {
	t.Parallel()

	for _, ft := range fixtureTypes {
		t.Run(ft.objectType, func(t *testing.T) {
			t.Parallel()

			blob, err := os.ReadFile(fixtureDir + "/" + ft.file)
			if err != nil {
				t.Fatalf("read fixture %s: %v", ft.file, err)
			}

			// Legacy path: io.ReadAll + json.Unmarshal into Response[RawMessage].
			var legacy Response[json.RawMessage]
			if err := json.Unmarshal(blob, &legacy); err != nil {
				t.Fatalf("legacy unmarshal %s: %v", ft.file, err)
			}

			server := newFixtureServer(ft.objectType, blob)
			defer server.Close()

			client := newStreamingTestClient(server.URL)

			var streamed []json.RawMessage
			_, err = client.StreamAll(t.Context(), ft.objectType, func(raw json.RawMessage) error {
				clone := make(json.RawMessage, len(raw))
				copy(clone, raw)
				streamed = append(streamed, clone)
				return nil
			})
			if err != nil {
				t.Fatalf("StreamAll(%s): %v", ft.objectType, err)
			}

			if len(streamed) != len(legacy.Data) {
				t.Fatalf("%s: streamed %d items, legacy %d items",
					ft.objectType, len(streamed), len(legacy.Data))
			}

			// Normalise both via json.Compact for whitespace insensitivity.
			// The streaming decoder emits compact form; the legacy path
			// preserves whatever whitespace was in the source.
			for i := range streamed {
				var sBuf, lBuf bytes.Buffer
				if err := json.Compact(&sBuf, streamed[i]); err != nil {
					t.Fatalf("%s[%d] compact streamed: %v", ft.objectType, i, err)
				}
				if err := json.Compact(&lBuf, legacy.Data[i]); err != nil {
					t.Fatalf("%s[%d] compact legacy: %v", ft.objectType, i, err)
				}
				if !bytes.Equal(sBuf.Bytes(), lBuf.Bytes()) {
					t.Fatalf("%s[%d] mismatch:\n streamed: %s\n  legacy: %s",
						ft.objectType, i, sBuf.String(), lBuf.String())
				}
			}
		})
	}
}

// TestStreamingDecoder_MetaBeforeAndAfterData asserts that the two-pass token
// walk in streamDecodeResponse handles both {"meta":{...},"data":[...]} and
// {"data":[...],"meta":{...}} key orderings identically — same item list and
// same FetchMeta.Generated.
func TestStreamingDecoder_MetaBeforeAndAfterData(t *testing.T) {
	t.Parallel()

	const generatedEpoch = 1234567890

	bodyMetaFirst := []byte(`{"meta":{"generated":1234567890},"data":[{"id":1},{"id":2}]}`)
	bodyMetaLast := []byte(`{"data":[{"id":1},{"id":2}],"meta":{"generated":1234567890}}`)

	decode := func(t *testing.T, blob []byte) (FetchMeta, []json.RawMessage) {
		t.Helper()
		server := newFixtureServer("testtype", blob)
		defer server.Close()
		client := newStreamingTestClient(server.URL)

		var items []json.RawMessage
		meta, err := client.StreamAll(t.Context(), "testtype", func(raw json.RawMessage) error {
			clone := make(json.RawMessage, len(raw))
			copy(clone, raw)
			items = append(items, clone)
			return nil
		})
		if err != nil {
			t.Fatalf("StreamAll: %v", err)
		}
		return meta, items
	}

	metaFirst, itemsFirst := decode(t, bodyMetaFirst)
	metaLast, itemsLast := decode(t, bodyMetaLast)

	if metaFirst.Generated.Unix() != generatedEpoch {
		t.Errorf("meta-first generated: got %d, want %d", metaFirst.Generated.Unix(), generatedEpoch)
	}
	if metaLast.Generated.Unix() != generatedEpoch {
		t.Errorf("meta-last generated: got %d, want %d", metaLast.Generated.Unix(), generatedEpoch)
	}
	if !metaFirst.Generated.Equal(metaLast.Generated) {
		t.Errorf("generated timestamps differ between orderings: %v != %v",
			metaFirst.Generated, metaLast.Generated)
	}

	if len(itemsFirst) != 2 || len(itemsLast) != 2 {
		t.Fatalf("item counts: got %d and %d, want 2 and 2", len(itemsFirst), len(itemsLast))
	}
	for i := range itemsFirst {
		var a, b bytes.Buffer
		_ = json.Compact(&a, itemsFirst[i])
		_ = json.Compact(&b, itemsLast[i])
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Errorf("item[%d] differs: %s vs %s", i, a.String(), b.String())
		}
	}
}

// TestStreamingDecoder_TruncatedInput asserts that truncating a fixture at
// multiple offsets yields a wrapped decoder error. The error must carry the
// objectType for operator diagnosis and unwrap to a json/io error via
// errors.As or errors.Is.
func TestStreamingDecoder_TruncatedInput(t *testing.T) {
	t.Parallel()

	blob, err := os.ReadFile(fixtureDir + "/net.json")
	if err != nil {
		t.Fatalf("read net.json: %v", err)
	}

	offsets := []int{
		len(blob) * 25 / 100,
		len(blob) * 50 / 100,
		len(blob) * 75 / 100,
		len(blob) * 90 / 100,
	}

	for _, off := range offsets {
		t.Run(fmt.Sprintf("truncate_%d", off), func(t *testing.T) {
			t.Parallel()

			truncated := blob[:off]
			server := newFixtureServer(TypeNet, truncated)
			defer server.Close()

			client := newStreamingTestClient(server.URL)

			_, err := client.StreamAll(t.Context(), TypeNet, func(_ json.RawMessage) error {
				return nil
			})
			if err == nil {
				t.Fatalf("expected error for truncated input at offset %d, got nil", off)
			}
			if !strings.Contains(err.Error(), TypeNet) {
				t.Errorf("error message does not contain objectType %q: %v", TypeNet, err)
			}

			// Must unwrap to either io.ErrUnexpectedEOF or a *json.SyntaxError.
			if _, ok := errors.AsType[*json.SyntaxError](err); !errors.Is(err, io.ErrUnexpectedEOF) && !ok {
				t.Errorf("error does not unwrap to io.ErrUnexpectedEOF or *json.SyntaxError: %v", err)
			}
		})
	}
}

// TestFetchAll_WrapperParity asserts that the new FetchAll (wrapping StreamAll)
// returns byte-identical []json.RawMessage compared to the legacy io.ReadAll
// path for each fixture. Also asserts the clone is real: the first two
// elements do not share backing memory.
func TestFetchAll_WrapperParity(t *testing.T) {
	t.Parallel()

	for _, ft := range fixtureTypes {
		t.Run(ft.objectType, func(t *testing.T) {
			t.Parallel()

			blob, err := os.ReadFile(fixtureDir + "/" + ft.file)
			if err != nil {
				t.Fatalf("read fixture %s: %v", ft.file, err)
			}

			var legacy Response[json.RawMessage]
			if err := json.Unmarshal(blob, &legacy); err != nil {
				t.Fatalf("legacy unmarshal: %v", err)
			}

			server := newFixtureServer(ft.objectType, blob)
			defer server.Close()

			client := newStreamingTestClient(server.URL)

			result, err := client.FetchAll(t.Context(), ft.objectType)
			if err != nil {
				t.Fatalf("FetchAll: %v", err)
			}

			if len(result.Data) != len(legacy.Data) {
				t.Fatalf("FetchAll %s: got %d items, want %d",
					ft.objectType, len(result.Data), len(legacy.Data))
			}

			for i := range result.Data {
				var a, b bytes.Buffer
				_ = json.Compact(&a, result.Data[i])
				_ = json.Compact(&b, legacy.Data[i])
				if !bytes.Equal(a.Bytes(), b.Bytes()) {
					t.Fatalf("%s[%d] mismatch:\n wrapper: %s\n  legacy: %s",
						ft.objectType, i, a.String(), b.String())
				}
			}

			// Anti-aliasing check: if there are >= 2 elements, assert the
			// underlying byte slices do not alias. The clone inside the
			// FetchAll wrapper handler exists specifically to break aliasing.
			if len(result.Data) >= 2 && len(result.Data[0]) > 0 && len(result.Data[1]) > 0 {
				if &result.Data[0][0] == &result.Data[1][0] {
					t.Errorf("FetchAll %s: data[0] and data[1] share backing memory (clone missing)",
						ft.objectType)
				}
			}
		})
	}
}

// errSentinelHandler is the sentinel error returned by the handler in
// TestStreamingDecoder_HandlerError to verify error propagation.
var errSentinelHandler = errors.New("sentinel handler error")

// TestStreamingDecoder_HandlerError asserts that if the handler returns an
// error mid-stream, StreamAll aborts, wraps the error with the objectType,
// and unwraps back to the sentinel via errors.Is.
func TestStreamingDecoder_HandlerError(t *testing.T) {
	t.Parallel()

	blob, err := os.ReadFile(fixtureDir + "/net.json")
	if err != nil {
		t.Fatalf("read net.json: %v", err)
	}

	server := newFixtureServer(TypeNet, blob)
	defer server.Close()

	client := newStreamingTestClient(server.URL)

	// net.json has 2 items in the fixture; return the sentinel on the
	// first element so the test doesn't depend on a specific item count.
	var called atomic.Int32
	_, err = client.StreamAll(t.Context(), TypeNet, func(_ json.RawMessage) error {
		if called.Add(1) == 1 {
			return errSentinelHandler
		}
		return nil
	})
	if err == nil {
		t.Fatalf("expected handler error, got nil")
	}
	if !errors.Is(err, errSentinelHandler) {
		t.Errorf("error does not unwrap to errSentinelHandler: %v", err)
	}
	if !strings.Contains(err.Error(), "handler") {
		t.Errorf("error message does not contain %q: %v", "handler", err)
	}
	if !strings.Contains(err.Error(), TypeNet) {
		t.Errorf("error message does not contain objectType %q: %v", TypeNet, err)
	}
}
