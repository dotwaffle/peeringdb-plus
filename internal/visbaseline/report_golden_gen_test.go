package visbaseline

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateGoldens = flag.Bool("update-goldens", false, "regenerate golden files under testdata/diff_golden/")

// TestUpdateGoldens regenerates the golden Markdown and JSON fixtures under
// testdata/diff_golden/ when the -update-goldens flag is passed. Otherwise
// it is a no-op. Run as:
//
//	go test -run TestUpdateGoldens -update-goldens ./internal/visbaseline/
//
// This is the canonical path to refresh goldens after an emitter change.
func TestUpdateGoldens(t *testing.T) {
	if !*updateGoldens {
		t.Skip("pass -update-goldens to regenerate fixtures")
	}

	writeBoth := func(name string, rep Report) {
		var mdBuf, jsonBuf bytes.Buffer
		if err := WriteMarkdown(&mdBuf, rep); err != nil {
			t.Fatalf("WriteMarkdown %s: %v", name, err)
		}
		if err := WriteJSON(&jsonBuf, rep); err != nil {
			t.Fatalf("WriteJSON %s: %v", name, err)
		}
		mdPath := filepath.Join("testdata", "diff_golden", "expected_"+name+".md")
		jsonPath := filepath.Join("testdata", "diff_golden", "expected_"+name+".json")
		if err := os.WriteFile(mdPath, mdBuf.Bytes(), 0o644); err != nil {
			t.Fatalf("write %s: %v", mdPath, err)
		}
		if err := os.WriteFile(jsonPath, jsonBuf.Bytes(), 0o644); err != nil {
			t.Fatalf("write %s: %v", jsonPath, err)
		}
	}

	writeBoth("empty", buildEmptyReport())
	writeBoth("simple", buildSimpleReport(t))
}
