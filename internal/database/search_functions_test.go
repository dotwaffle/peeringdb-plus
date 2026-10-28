package database

import (
	"path/filepath"
	"testing"
)

func TestSearchFunctions(t *testing.T) {
	t.Parallel()
	for _, traced := range []bool{false, true} {
		client, db, err := Open(filepath.Join(t.TempDir(), "search.db"), traced)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })
		var lower, tokens string
		err = db.QueryRowContext(t.Context(), "SELECT pdbplus_search_lower(?), pdbplus_search_tokens(?)", "MÜNCHEN", "Gamma-Net").Scan(&lower, &tokens)
		if err != nil {
			t.Fatal(err)
		}
		if lower != "münchen" || tokens != "gamma net" {
			t.Errorf("lower=%q, tokens=%q", lower, tokens)
		}
	}
}
