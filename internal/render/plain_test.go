package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/DawnMagnet/mfile/internal/detect"
)

func TestPlainStreamsHistoricalLines(t *testing.T) {
	when := time.Date(2026, 10, 28, 6, 12, 47, 0, time.UTC)
	var buf bytes.Buffer
	p := NewPlain(&buf)
	if err := p.Begin(); err != nil {
		t.Fatal(err)
	}
	rows := []detect.Result{
		{
			Path: "go.mod", Description: "Verilog source", Label: "verilog",
			MIME: "text/x-verilog", Group: "code", Score: 0.748, Size: 599,
			Mode: "-rw-r--r--", ModTime: when, Kind: "file", Classified: true,
		},
		{
			Path: "empty", Description: "Empty file", Label: "empty",
			MIME: "inode/x-empty", Group: "inode", Score: 1, Size: 0,
			Mode: "-rw-r--r--", ModTime: when, Kind: "file", Classified: true,
		},
		{Path: "link", Kind: "symlink", Target: "hello.txt", Mode: "Lrwxrwxrwx", ModTime: when},
		{Path: "subdir", Kind: "directory", Mode: "drwxr-xr-x", ModTime: when},
		{Path: "missing", Error: "lstat missing: no such file or directory"},
	}
	for _, r := range rows {
		if err := p.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.End(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	want := strings.Join([]string{
		"go.mod: Verilog source [verilog, text/x-verilog, code] score=0.748 size=599B mode=-rw-r--r-- mtime=2026-10-28 06:12:47",
		"empty: Empty file [empty, inode/x-empty, inode] score=1.000 size=0B mode=-rw-r--r-- mtime=2026-10-28 06:12:47",
		"link: symbolic link to hello.txt",
		"subdir: directory mode=drwxr-xr-x mtime=2026-10-28 06:12:47",
		"missing: error: lstat missing: no such file or directory",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("plain output:\n%q\nwant:\n%q", got, want)
	}
}

func TestPlainEscapesPathTargetAndError(t *testing.T) {
	var buf bytes.Buffer
	p := NewPlain(&buf)
	if err := p.Begin(); err != nil {
		t.Fatal(err)
	}
	rows := []detect.Result{
		{Path: "a\x1b]0;x\x07\n", Kind: "symlink", Target: "b\x07"},
		{Path: "c\n", Error: "oops\x1b"},
	}
	for _, r := range rows {
		if err := p.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	want := "a\\033]0;x\\007\\012: symbolic link to b\\007\n" +
		"c\\012: error: oops\\033\n"
	if buf.String() != want {
		t.Fatalf("got %q\nwant %q", buf.String(), want)
	}
}

func TestHumanSize(t *testing.T) {
	if got := humanSize(0); got != "0B" {
		t.Fatalf("0: %s", got)
	}
	if got := humanSize(555); got != "555B" {
		t.Fatalf("555: %s", got)
	}
	if got := humanSize(11358); got != "11.1KiB" {
		t.Fatalf("11358: %s", got)
	}
}
