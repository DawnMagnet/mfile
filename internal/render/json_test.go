package render

import (
	"bytes"
	"testing"
	"time"

	"github.com/DawnMagnet/mfile/internal/detect"
)

func TestJSONStreamsHistoricalObjects(t *testing.T) {
	when := time.Date(2026, 10, 28, 6, 12, 47, 834538193, time.UTC)
	var buf bytes.Buffer
	j := NewJSON(&buf)
	if err := j.Begin(); err != nil {
		t.Fatal(err)
	}
	rows := []detect.Result{
		{
			Path: "go.mod", Kind: "file", Size: 599, Mode: "-rw-r--r--", ModTime: when,
			Label: "verilog", MIME: "text/x-verilog", Group: "code", Description: "Verilog source",
			Extensions: []string{"v", "verilog", "vlg", "vh"}, IsText: true, Score: 0.74777484,
			DLLabel: "verilog", Source: detect.SourceModel, Classified: true, Index: 0,
		},
		{
			Path: "empty", Kind: "file", Size: 0, Mode: "-rw-r--r--", ModTime: when,
			Label: "empty", MIME: "inode/x-empty", Group: "inode", Description: "Empty file",
			Extensions: []string{}, IsText: false, Score: 1, Source: detect.SourceRule, Classified: true, Index: 1,
		},
		{Path: "link", Kind: "symlink", Size: 9, Mode: "Lrwxrwxrwx", ModTime: when, Target: "hello.txt", Index: 2},
		{Path: "subdir", Kind: "directory", Size: 4096, Mode: "drwxr-xr-x", ModTime: when, Index: 3},
		{Path: "missing", Error: "lstat missing: no such file or directory", Index: 4},
	}
	for _, r := range rows {
		if err := j.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.End(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	want := `{"path":"go.mod","kind":"file","size":599,"mode":"-rw-r--r--","mtime":"2026-10-28T06:12:47.834538193Z","label":"verilog","mime_type":"text/x-verilog","group":"code","description":"Verilog source","extensions":["v","verilog","vlg","vh"],"is_text":true,"score":0.74777484,"dl_label":"verilog"}
{"path":"empty","kind":"file","size":0,"mode":"-rw-r--r--","mtime":"2026-10-28T06:12:47.834538193Z","label":"empty","mime_type":"inode/x-empty","group":"inode","description":"Empty file","extensions":[],"is_text":false,"score":1}
{"path":"link","kind":"symlink","size":9,"mode":"Lrwxrwxrwx","mtime":"2026-10-28T06:12:47.834538193Z","symlink_target":"hello.txt"}
{"path":"subdir","kind":"directory","size":4096,"mode":"drwxr-xr-x","mtime":"2026-10-28T06:12:47.834538193Z"}
{"path":"missing","kind":"","size":0,"mode":"","mtime":"0001-01-01T00:00:00Z","error":"lstat missing: no such file or directory"}
`
	if got != want {
		t.Fatalf("json output:\n%s\nwant:\n%s", got, want)
	}
	if bytes.Contains(buf.Bytes(), []byte(`"source"`)) || bytes.Contains(buf.Bytes(), []byte(`"index"`)) {
		t.Fatal("source or index leaked into JSON")
	}
}
