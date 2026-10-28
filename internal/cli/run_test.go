package cli

import (
	"bytes"
	"encoding/json"
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

	var plain, stderr bytes.Buffer
	failed, err := Run(Options{Paths: []string{empty, link, missing}}, &plain, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if failed {
		t.Fatal("a missing path is not a failure without -E")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr:\n%s", stderr.String())
	}
	text := plain.String()
	if !strings.Contains(text, "Empty file [empty, inode/x-empty, inode] score=1.000 size=0B") {
		t.Fatalf("plain:\n%s", text)
	}
	if !strings.Contains(text, "symbolic link to nowhere") {
		t.Fatalf("plain link:\n%s", text)
	}
	wantMissing := missing + ": cannot open `" + missing + "' (No such file or directory)\n"
	if !strings.Contains(text, wantMissing) {
		t.Fatalf("plain missing:\n%s\nwant %q", text, wantMissing)
	}
	if strings.Contains(text, "error:") {
		t.Fatalf("renderer error line leaked:\n%s", text)
	}

	var js bytes.Buffer
	failed, err = Run(Options{JSON: true, Paths: []string{empty, link}}, &js, &stderr)
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

func TestRunOneMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no such")
	var stdout, stderr bytes.Buffer
	failed, err := Run(Options{Paths: []string{missing}}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if failed {
		t.Fatal("missing path exited as a failure")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr: %q", stderr.String())
	}
	want := missing + ": cannot open `" + missing + "' (No such file or directory)\n"
	if stdout.String() != want {
		t.Fatalf("stdout:\n%q\nwant:\n%q", stdout.String(), want)
	}
}

func TestRunExitOnFSError(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")
	var stdout, stderr bytes.Buffer
	failed, err := Run(Options{ExitOnFSError: true, Paths: []string{empty, missing}}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !failed {
		t.Fatal("expected -E to fail")
	}
	if strings.Contains(stdout.String(), "cannot open") || strings.Contains(stdout.String(), missing) {
		t.Fatalf("missing path leaked to stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Empty file") {
		t.Fatalf("existing path was dropped:\n%s", stdout.String())
	}
	want := missing + ": cannot open `" + missing + "' (No such file or directory)\n"
	if stderr.String() != want {
		t.Fatalf("stderr:\n%q\nwant:\n%q", stderr.String(), want)
	}
}

func TestRunJSONMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	var stdout, stderr bytes.Buffer
	failed, err := Run(Options{JSON: true, Paths: []string{missing}}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if failed {
		t.Fatal("json missing path is not a failure without -E")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr: %q", stderr.String())
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &obj); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	// The historical object already has an error field. No field was added.
	allowed := []string{"path", "kind", "size", "mode", "mtime", "error"}
	if len(obj) != len(allowed) {
		t.Fatalf("fields: %s", stdout.String())
	}
	for _, key := range allowed {
		if _, ok := obj[key]; !ok {
			t.Fatalf("missing %q in %s", key, stdout.String())
		}
	}
	var got struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != missing {
		t.Fatalf("path: %q", got.Path)
	}
	if !strings.Contains(got.Error, missing) || !strings.Contains(got.Error, "no such file or directory") {
		t.Fatalf("error field: %q", got.Error)
	}

	stdout.Reset()
	stderr.Reset()
	failed, err = Run(Options{JSON: true, ExitOnFSError: true, Paths: []string{missing}}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !failed {
		t.Fatal("expected -E json to fail")
	}
	if _, err := json.Marshal(json.RawMessage(bytes.TrimSpace(stdout.Bytes()))); err != nil {
		t.Fatalf("stdout is not json: %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stderr.String(), "cannot open `"+missing+"'") || !strings.Contains(stderr.String(), "(No such file or directory)") {
		t.Fatalf("stderr: %q", stderr.String())
	}
}

func TestRunBrokenSymlinkFollow(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "broken")
	if err := os.Symlink("missing-target", link); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	failed, err := Run(Options{Follow: true, Paths: []string{link}}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if failed || stderr.Len() != 0 {
		t.Fatalf("failed=%v stderr=%q stdout=%q", failed, stderr.String(), stdout.String())
	}
	want := link + ": cannot open `" + link + "' (No such file or directory)\n"
	if stdout.String() != want {
		t.Fatalf("stdout:\n%q\nwant:\n%q", stdout.String(), want)
	}
}
