package litefs_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/litefs"
)

func TestPosPath(t *testing.T) {
	t.Parallel()
	if got, want := litefs.PosPath("/litefs/peeringdb-plus.db"), "/litefs/peeringdb-plus.db-pos"; got != want {
		t.Errorf("PosPath = %q, want %q", got, want)
	}
}

func TestReadPos(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{
			name:    "valid position",
			content: "0000000000000abc/00000000deadbeef\n",
			want:    "0000000000000abc/00000000deadbeef",
		},
		{name: "short", content: "0000000000000abc/00000000deadbee\n", wantErr: true},
		{name: "too long", content: "0000000000000abc/00000000deadbeef\n\n", wantErr: true},
		{name: "non-hex TXID", content: "000000000000zabc/00000000deadbeef\n", wantErr: true},
		{name: "non-hex checksum", content: "0000000000000abc/00000000deadbeeg\n", wantErr: true},
		{name: "missing slash", content: "0000000000000abc:00000000deadbeef\n", wantErr: true},
		{name: "missing newline", content: "0000000000000abc/00000000deadbeef ", wantErr: true},
		{name: "empty", content: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "test.db-pos")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write pos file: %v", err)
			}
			got, err := litefs.ReadPos(path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ReadPos(%q) = %q, want error", tc.content, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadPos(%q): %v", tc.content, err)
			}
			if got != tc.want {
				t.Errorf("ReadPos = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReadPos_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := litefs.ReadPos(filepath.Join(t.TempDir(), "absent.db-pos"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadPos(missing) error = %v, want one that matches fs.ErrNotExist", err)
	}
}
