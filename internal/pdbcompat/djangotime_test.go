package pdbcompat

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestDjangoDateTime compares djangoDateTime with the golden file
// testdata/django_datetime.json. The file holds the result of Django
// 5.2.17 DateTimeField().to_python, made aware in UTC, on CPython 3.14
// (the C datetime module): the UTC time with microseconds, or ERR and
// the Python exception name.
func TestDjangoDateTime(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/django_datetime.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ In, Out string }
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		got, err := djangoDateTime(tc.In)
		if strings.HasPrefix(tc.Out, "ERR") {
			if err == nil && tc.Out != "ERR OverflowError" {
				t.Errorf("djangoDateTime(%q) = %s, want an error", tc.In, got.Format("2006-01-02T15:04:05.000000Z"))
			}
			continue
		}
		if err != nil {
			t.Errorf("djangoDateTime(%q): %v, want %s", tc.In, err, tc.Out)
			continue
		}
		if s := got.Format("2006-01-02T15:04:05.000000Z"); s != tc.Out {
			t.Errorf("djangoDateTime(%q) = %s, want %s", tc.In, s, tc.Out)
		}
	}
}
