package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanTranscripts(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "agent-transcripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"summary":"job finished","text":"ERROR: boom failed to connect"}`
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("warning: retrying"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignore.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	found := ScanTranscripts([]string{root})
	if len(found) != 2 {
		t.Fatalf("found %d: %+v", len(found), found)
	}
	joined := found[0].Text + found[1].Text
	if !strings.Contains(joined, "boom") || !strings.Contains(joined, "retrying") {
		t.Fatalf("text = %q", joined)
	}
}
