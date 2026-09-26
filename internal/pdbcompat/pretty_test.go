package pdbcompat

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrettyWriter(t *testing.T) {
	t.Parallel()
	inputs := []string{
		`{"meta":{},"data":[]}`,
		`{"meta":{"error":"x"}}` + "\n",
		`{"data":[{"id":1,"name":"a, b: [c] {d}","aka":"q\"uote\\","n":null,"t":true,"s":[1,2],"e":{},"f":-1.5e3}],"meta":{}}`,
		`[]`,
		`[[],[{}],{"a":[]}]`,
		`"just a string"`,
		`{"u":"\u00fc é"}`,
	}
	for _, in := range inputs {
		var want bytes.Buffer
		if err := json.Indent(&want, bytes.TrimSpace([]byte(in)), "", prettyIndent); err != nil {
			t.Fatalf("indent %s: %v", in, err)
		}

		// One Write with the whole body.
		rec := httptest.NewRecorder()
		pw := &prettyWriter{ResponseWriter: rec}
		if n, err := pw.Write([]byte(in)); err != nil || n != len(in) {
			t.Fatalf("Write(%s) = %d, %v", in, n, err)
		}
		if got := rec.Body.String(); got != want.String() {
			t.Errorf("one write %s:\n%s\nwant\n%s", in, got, want.String())
		}

		// One Write per byte: the state carries over.
		rec = httptest.NewRecorder()
		pw = &prettyWriter{ResponseWriter: rec}
		for i := range len(in) {
			if _, err := pw.Write([]byte{in[i]}); err != nil {
				t.Fatalf("Write byte %d of %s: %v", i, in, err)
			}
		}
		if got := rec.Body.String(); got != want.String() {
			t.Errorf("byte writes %s:\n%s\nwant\n%s", in, got, want.String())
		}
	}
}

func TestPrettyJSON_OnlyWithPrettyKey(t *testing.T) {
	t.Parallel()
	body := `{"meta":{},"data":[1]}`
	h := prettyJSON(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Error("writer is not an http.Flusher")
		}
		w.Header().Set("X-Test", "1")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(body))
	}))
	for _, tc := range []struct {
		target string
		want   string
	}{
		{"/api/net", body},
		{"/api/net?prettyx", body},
		{"/api/net?pretty", "{\n  \"meta\": {},\n  \"data\": [\n    1\n  ]\n}"},
		{"/api/net?pretty=false", "{\n  \"meta\": {},\n  \"data\": [\n    1\n  ]\n}"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))
		if rec.Code != http.StatusTeapot || rec.Header().Get("X-Test") != "1" {
			t.Errorf("%s: status %d, X-Test %q: want the handler's status and headers", tc.target, rec.Code, rec.Header().Get("X-Test"))
		}
		if got := rec.Body.String(); got != tc.want {
			t.Errorf("%s: body\n%s\nwant\n%s", tc.target, got, tc.want)
		}
	}
}
