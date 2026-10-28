package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPlainAndJSON(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink("nowhere", link); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")

	var plain bytes.Buffer
	failed, err := Run(Options{Paths: []string{empty, link, missing}}, &plain)
	if err != nil {
		t.Fatal(err)
	}
	if !failed {
		t.Fatal("expected a failed path")
	}
	text := plain.String()
	if !strings.Contains(text, "Empty file [empty, inode/x-empty, inode] score=1.000 size=0B") {
		t.Fatalf("plain:\n%s", text)
	}
	if !strings.Contains(text, "symbolic link to nowhere") {
		t.Fatalf("plain link:\n%s", text)
	}
	if !strings.Contains(text, "error:") {
		t.Fatalf("plain error:\n%s", text)
	}

	var js bytes.Buffer
	failed, err = Run(Options{JSON: true, Paths: []string{empty, link}}, &js)
	if err != nil || failed {
		t.Fatalf("json run: failed=%v err=%v", failed, err)
	}
	out := js.String()
	if !strings.Contains(out, `"label":"empty"`) || !strings.Contains(out, `"score":1`) {
		t.Fatalf("json:\n%s", out)
	}
	if strings.Contains(out, `"source"`) {
		t.Fatalf("source in json:\n%s", out)
	}
	if !strings.Contains(out, `"symlink_target":"nowhere"`) {
		t.Fatalf("json link:\n%s", out)
	}
}
