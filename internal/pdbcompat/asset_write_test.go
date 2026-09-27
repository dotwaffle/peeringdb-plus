package pdbcompat

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// newAssetTestServer returns a server for the asset tests with the rows
// of the DRF simulation: org 1 (ok, logo), org 2 (deleted), org 3 (ok,
// no logo) and net 5 (ok).
func newAssetTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := t.Context()
	c := testutil.SetupClient(t)
	t0 := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	for id, status := range map[int]string{1: "ok", 2: "deleted", 3: "ok"} {
		q := c.Organization.Create().SetID(id).SetName("Org").SetNameFold(unifold.Fold("Org")).
			SetStatus(status).SetCreated(t0).SetUpdated(t0)
		if id == 1 {
			q.SetLogo("https://media.invalid/media/logos_user_supplied/org-1-abcd1234.png")
		}
		q.SaveX(ctx)
	}
	c.Network.Create().SetID(5).SetName("Net").SetNameFold(unifold.Fold("Net")).SetAsn(64505).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	mux := http.NewServeMux()
	NewHandler(c, 0).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// jsonTokens returns the tokens of a JSON document in order, so that
// two documents compare with their key order.
func jsonTokens(t *testing.T, b []byte) []any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	var toks []any
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return toks
		}
		if err != nil {
			t.Fatalf("jsonTokens: %v in %s", err, b)
		}
		toks = append(toks, tok)
	}
}

// TestAssetWrite_Oracle sends the POST and PUT requests of
// testdata/asset_write_oracle.json and compares the status and body with
// the responses of the upstream AssetViewSet under DRF 3.18.1 and
// Django 5.2.17 (CPython 3.13.5), for a caller without an API key. The
// cases cover the parsers (JSON, form, multipart, charsets, the
// Content-Type match), the field checks and the file checks. Upstream
// sends a 500 for JSON that is not an object, and the mirror a 400
// (DIVERGENCE_asset_non_object_json_400), so those cases want a 400
// with the non_field_errors key.
func TestAssetWrite_Oracle(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/asset_write_oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Method string          `json:"method"`
		Path   string          `json:"path"`
		CT     *string         `json:"ct"`
		Req    string          `json:"req"`
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	srv := newAssetTestServer(t)
	for _, tc := range cases {
		body, err := base64.StdEncoding.DecodeString(tc.Req)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), tc.Method, srv.URL+tc.Path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if tc.CT != nil {
			req.Header.Set("Content-Type", *tc.CT)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		what := tc.Method + " " + tc.Path
		if tc.CT != nil {
			what += " (" + *tc.CT + ")"
		}
		if tc.Status == http.StatusInternalServerError {
			if resp.StatusCode != http.StatusBadRequest || !bytes.HasPrefix(got, []byte(`{"non_field_errors":["Invalid data. Expected a dictionary, but got `)) {
				t.Errorf("%s %q: status = %d, body=%s; want 400 non_field_errors", what, body, resp.StatusCode, got)
			}
			continue
		}
		if resp.StatusCode != tc.Status {
			t.Errorf("%s %q: status = %d, want %d; body=%s", what, body, resp.StatusCode, tc.Status, got)
			continue
		}
		if tc.Body == nil {
			continue
		}
		if !reflect.DeepEqual(jsonTokens(t, got), jsonTokens(t, tc.Body)) {
			t.Errorf("%s %q:\n got %s\nwant %s", what, body, got, tc.Body)
		}
	}
}

// TestAssetWrite_FileChecks covers the file checks that the oracle
// cases do not: the size limit (a file too large for the testdata) and
// the Pillow pixel limit.
func TestAssetWrite_FileChecks(t *testing.T) {
	t.Parallel()
	noise := image.NewRGBA(image.Rect(0, 0, 300, 75))
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range noise.Pix {
		noise.Pix[i] = byte(rng.Uint32())
	}
	var big bytes.Buffer
	if err := png.Encode(&big, noise); err != nil {
		t.Fatal(err)
	}
	if big.Len() <= assetMaxLogoBytes {
		t.Fatalf("noise PNG has %d bytes, want more than %d", big.Len(), assetMaxLogoBytes)
	}
	var small bytes.Buffer
	if err := png.Encode(&small, image.NewRGBA(image.Rect(0, 0, 10, 75))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		fileType string
		data     []byte
		want     fieldError
		ok       bool
	}{
		{"too large", "image/png", big.Bytes(), fieldError{Field: "file_data", Message: "File size too big, max. 50 kb"}, false},
		{"type mismatch before size", "image/jpeg", big.Bytes(), fieldError{Field: "file_type", Message: "Declared file_type does not match actual file type. Expected: image/png"}, false},
		{"height 75", "image/png", small.Bytes(), fieldError{}, true},
	} {
		fe, ok := assetFileError(tc.fileType, []rune(base64.StdEncoding.EncodeToString(tc.data)))
		if ok != tc.ok || fe != tc.want {
			t.Errorf("%s: assetFileError = %+v, %v; want %+v, %v", tc.name, fe, ok, tc.want, tc.ok)
		}
	}
}
