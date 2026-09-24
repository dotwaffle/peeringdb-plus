package litefs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// posFileSize is the size of a LiteFS "-pos" file: two 16-digit hex
// numbers, a slash between them and a trailing newline.
const posFileSize = 34

// PosPath returns the path of the LiteFS position file for the database at
// dbPath. LiteFS exposes one "<db>-pos" file next to each database in the
// FUSE mount.
func PosPath(dbPath string) string {
	return dbPath + "-pos"
}

// ReadPos returns the replication position of a LiteFS database, read from
// its "-pos" file at path. The position changes on every committed
// transaction that LiteFS applies to the database, on the primary and on
// each replica.
//
// LiteFS v0.5 renders the file in fuse/pos_node.go as TXID and
// PostApplyChecksum separated by "/", each formatted %016x, with a
// trailing newline. ReadPos requires exactly that shape and returns it
// without the newline. Each call opens and reads the file again, because
// LiteFS generates the content on each read.
//
// A missing file returns an error that matches fs.ErrNotExist.
func ReadPos(path string) (string, error) {
	// The path derives from the operator-set PDBPLUS_DB_PATH. Cleaning it
	// also satisfies gosec G304's static analysis.
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("read litefs position %s: %w", path, err)
	}
	if len(b) != posFileSize || b[16] != '/' || b[33] != '\n' {
		return "", fmt.Errorf("parse litefs position %s: unexpected content %q", path, b)
	}
	for _, field := range [][]byte{b[:16], b[17:33]} {
		if _, err := strconv.ParseUint(string(field), 16, 64); err != nil {
			return "", fmt.Errorf("parse litefs position %s: %w", path, err)
		}
	}
	return string(b[:33]), nil
}
