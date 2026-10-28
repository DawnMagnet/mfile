package detect

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKind(t *testing.T) {
	cases := []struct {
		mode fs.FileMode
		want string
	}{
		{0, "file"},
		{fs.ModeDir, "directory"},
		{fs.ModeSymlink, "symlink"},
		{fs.ModeNamedPipe, "fifo"},
		{fs.ModeSocket, "socket"},
		{fs.ModeCharDevice, "char device"},
		{fs.ModeDevice, "block device"},
		{fs.ModeIrregular, "other"},
	}
	for _, tc := range cases {
		if got := kind(tc.mode); got != tc.want {
			t.Errorf("kind(%v) = %s, want %s", tc.mode, got, tc.want)
		}
	}
}

func TestScanMetadataAndRuleSource(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink("empty", link); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")

	got, stats, err := Scan([]string{empty, link, sub, missing}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Active || stats.Files != 1 {
		t.Fatalf("stats: %+v", stats)
	}
	if !got[0].Classified || got[0].Label != "empty" || got[0].Source != SourceRule || got[0].Score != 1 {
		t.Fatalf("empty: %+v", got[0])
	}
	if got[0].MIME != "inode/x-empty" || got[0].Description != "Empty file" {
		t.Fatalf("empty metadata: %+v", got[0])
	}
	if got[1].Classified || got[1].Target != "empty" || got[1].Kind != "symlink" {
		t.Fatalf("link: %+v", got[1])
	}
	if got[2].Kind != "directory" || got[2].Classified {
		t.Fatalf("dir: %+v", got[2])
	}
	if got[3].Error == "" || !strings.Contains(got[3].Error, "missing") {
		t.Fatalf("missing: %+v", got[3])
	}
}
