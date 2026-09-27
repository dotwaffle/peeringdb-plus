package pdbcompat

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// pyCorpus is testdata/pyjson_corpus.json: the results of CPython 3.13.5
// for JSON documents (the DRF JSONParser path: codecs StreamReader, then
// json.loads with strict_constant), base64.b64decode inputs and float
// reprs. The generator mutates seed documents at random.
type pyCorpus struct {
	JSON []struct {
		Doc     string `json:"doc"`
		Charset string `json:"charset"`
		Error   string `json:"error"`
		ASCII   string `json:"ascii"`
	} `json:"json"`
	Base64 []struct {
		In    string `json:"in"`
		Hex   string `json:"hex"`
		Error bool   `json:"error"`
	} `json:"base64"`
	Float []struct {
		In   string `json:"in"`
		Repr string `json:"repr"`
	} `json:"float"`
}

func loadPyCorpus(t *testing.T) pyCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/pyjson_corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var c pyCorpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// pyASCIIRepr is Python ascii() of a value whose repr is repr.
func pyASCIIRepr(repr string) string {
	var b strings.Builder
	for _, r := range repr {
		switch {
		case r < 0x80:
			b.WriteRune(r)
		case r < 0x100:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r < 0x10000:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	return b.String()
}

func TestPyJSONLoads_Corpus(t *testing.T) {
	t.Parallel()
	for _, tc := range loadPyCorpus(t).JSON {
		doc, err := base64.StdEncoding.DecodeString(tc.Doc)
		if err != nil {
			t.Fatal(err)
		}
		var got string
		text, err := pyDecode(doc, pyCodecOf(tc.Charset))
		if err == nil {
			var v pyValue
			v, err = pyJSONLoads(text)
			if err == nil {
				got = pyASCIIRepr(v.repr())
			}
		}
		want := tc.ASCII
		if tc.Error != "" {
			want = "error: " + tc.Error
		}
		switch {
		case errors.Is(err, errPyJSONDepth):
			got = "error: recursion"
		case err != nil:
			got = "error: " + err.Error()
		}
		if got != want {
			t.Errorf("%s %q:\n got %s\nwant %s", tc.Charset, doc, got, want)
		}
	}
}

func TestPyJSONLoads_Depth(t *testing.T) {
	t.Parallel()
	deep := strings.Repeat("[", pyJSONMaxDepth+1) + strings.Repeat("]", pyJSONMaxDepth+1)
	if _, err := pyJSONLoads([]rune(deep)); !errors.Is(err, errPyJSONDepth) {
		t.Errorf("depth %d: err = %v, want errPyJSONDepth", pyJSONMaxDepth+1, err)
	}
	ok := strings.Repeat("[", pyJSONMaxDepth) + strings.Repeat("]", pyJSONMaxDepth)
	if _, err := pyJSONLoads([]rune(ok)); err != nil {
		t.Errorf("depth %d: err = %v, want nil", pyJSONMaxDepth, err)
	}
}

func TestPyB64Decode_Corpus(t *testing.T) {
	t.Parallel()
	for _, tc := range loadPyCorpus(t).Base64 {
		got, ok := pyB64Decode([]rune(tc.In))
		switch {
		case tc.Error && ok:
			t.Errorf("pyB64Decode(%q) = %x, want an error", tc.In, got)
		case !tc.Error && (!ok || hex.EncodeToString(got) != tc.Hex):
			t.Errorf("pyB64Decode(%q) = %x, %v; want %s", tc.In, got, ok, tc.Hex)
		}
	}
}

func TestPyFloatRepr_Corpus(t *testing.T) {
	t.Parallel()
	for _, tc := range loadPyCorpus(t).Float {
		f, err := strconv.ParseFloat(tc.In, 64)
		if err != nil {
			t.Fatalf("%s: %v", tc.In, err)
		}
		if got := pyFloatRepr(f); got != tc.Repr {
			t.Errorf("pyFloatRepr(%s) = %s, want %s", tc.In, got, tc.Repr)
		}
	}
}
