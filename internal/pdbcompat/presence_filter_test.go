package pdbcompat

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestParseASNOverlap checks the order of the asn_overlap checks
// (2.83.0 models.py:2455-2461): the item count first, then the integer
// conversion of every item, then the raw-item duplicate rule.
func TestParseASNOverlap(t *testing.T) {
	t.Parallel()
	asns := func(n int) string {
		items := make([]string, n)
		for i := range items {
			items[i] = strconv.Itoa(64500 + i)
		}
		return strings.Join(items, ",")
	}
	tests := []struct {
		name     string
		value    string
		wantIDs  []int
		wantNone bool
		wantErr  string
	}{
		{name: "two", value: "1,2", wantIDs: []int{1, 2}},
		{name: "repeated_item", value: "1,1", wantNone: true},
		{name: "repeated_item_of_three", value: "1,2,1", wantNone: true},
		{name: "same_asn_other_text", value: "1, 1", wantIDs: []int{1, 1}},
		// U+0662 is ARABIC-INDIC DIGIT TWO.
		{name: "python_int_forms", value: "64_500,+1,\u0662", wantIDs: []int{64500, 1, 2}},
		{name: "saturates", value: "1,99999999999999999999", wantIDs: []int{1, math.MaxInt}},
		{name: "one", value: "1", wantErr: "at least two asns"},
		{name: "empty", value: "", wantErr: "at least two asns"},
		{name: "one_not_an_integer", value: "abc", wantErr: "at least two asns"},
		{name: "twenty_five", value: asns(25), wantIDs: nil},
		{name: "twenty_six", value: asns(26), wantErr: "maximum of 25 asns"},
		{name: "twenty_six_bad_item", value: asns(25) + ",x", wantErr: "maximum of 25 asns"},
		{name: "not_an_integer", value: "1,x", wantErr: `"x" is not an integer`},
		{name: "empty_item", value: "1,", wantErr: `"" is not an integer`},
		{name: "parse_before_duplicate", value: "1,1,x", wantErr: `"x" is not an integer`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ids, none, err := parseASNOverlap(strings.Split(tt.value, ","))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if none != tt.wantNone {
				t.Fatalf("none = %v, want %v", none, tt.wantNone)
			}
			if tt.wantIDs != nil && !slices.Equal(ids, tt.wantIDs) {
				t.Errorf("ids = %v, want %v", ids, tt.wantIDs)
			}
			if tt.name == "twenty_five" && len(ids) != 25 {
				t.Errorf("len(ids) = %d, want 25", len(ids))
			}
		})
	}
}
