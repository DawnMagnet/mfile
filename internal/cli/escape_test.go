package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DawnMagnet/mfile/internal/termesc"
)

func TestRunEscapesControlNames(t *testing.T) {
	dir := t.TempDir()
	base := "\x1b]0;x\x07\n"
	exist := filepath.Join(dir, "exists", base)
	missing := filepath.Join(dir, "missing", base)
	if err := os.MkdirAll(filepath.Dir(exist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(missing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exist, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	escExist := termesc.Escape(exist)
	escMissing := termesc.Escape(missing)
	wantMissing := escMissing + ": cannot open `" + escMissing + "' (No such file or directory)\n"

	t.Run("plain existing", func(t *testing.T) {
		stdout, stderr, failed := runOut(t, Options{Paths: []string{exist}})
		if failed || len(stderr) != 0 {
			t.Fatalf("failed=%v stderr=%q", failed, stderr)
		}
		assertEscapedPlain(t, stdout, escExist)
	})
	t.Run("plain missing", func(t *testing.T) {
		stdout, stderr, failed := runOut(t, Options{Paths: []string{missing}})
		if failed || len(stderr) != 0 {
			t.Fatalf("failed=%v stderr=%q", failed, stderr)
		}
		if string(stdout) != wantMissing {
			t.Fatalf("stdout:\n%q\nwant:\n%q", stdout, wantMissing)
		}
		assertNoRawControls(t, stdout)
	})
	t.Run("-E existing", func(t *testing.T) {
		stdout, stderr, failed := runOut(t, Options{ExitOnFSError: true, Paths: []string{exist}})
		if failed || len(stderr) != 0 {
			t.Fatalf("failed=%v stderr=%q", failed, stderr)
		}
		assertEscapedPlain(t, stdout, escExist)
	})
	t.Run("-E missing", func(t *testing.T) {
		stdout, stderr, failed := runOut(t, Options{ExitOnFSError: true, Paths: []string{missing}})
		if !failed {
			t.Fatal("expected -E to fail")
		}
		if len(stdout) != 0 {
			t.Fatalf("stdout %q", stdout)
		}
		if string(stderr) != wantMissing {
			t.Fatalf("stderr:\n%q\nwant:\n%q", stderr, wantMissing)
		}
		assertNoRawControls(t, stderr)
	})
}

func TestRunJSONDoesNotOctalEscape(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing", "\x1b]0;x\x07\n")
	stdout, stderr, failed := runOut(t, Options{JSON: true, Paths: []string{missing}})
	if failed || len(stderr) != 0 {
		t.Fatalf("failed=%v stderr=%q", failed, stderr)
	}
	if bytes.Contains(stdout, []byte{'\\', '0', '3', '3'}) {
		t.Fatalf("octal escape leaked into JSON:\n%s", stdout)
	}
	if bytes.Contains(stdout, []byte{0x1b, 0x07}) {
		t.Fatalf("raw ESC/BEL in JSON:\n%x", stdout)
	}
	var got struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(stdout, &got); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	if got.Path != missing {
		t.Fatalf("path = %q, want %q", got.Path, missing)
	}
	if !strings.Contains(got.Error, "\x1b") || strings.Contains(got.Error, `\033`) {
		t.Fatalf("error field was octal-escaped: %q", got.Error)
	}
}

func TestRunPlainKeepsChineseAndEscapesBidi(t *testing.T) {
	dir := t.TempDir()
	chinese := filepath.Join(dir, "测试.txt")
	if err := os.WriteFile(chinese, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, failed := runOut(t, Options{Paths: []string{chinese}})
	if failed || len(stderr) != 0 {
		t.Fatalf("failed=%v stderr=%q", failed, stderr)
	}
	if !bytes.Contains(stdout, []byte("测试.txt: ")) {
		t.Fatalf("chinese name changed:\n%s", stdout)
	}
	if bytes.Contains(stdout, []byte(`\346`)) {
		t.Fatalf("chinese was octal-escaped:\n%s", stdout)
	}

	bidi := filepath.Join(dir, "a\u202eb")
	stdout, stderr, failed = runOut(t, Options{Paths: []string{bidi}})
	if failed || len(stderr) != 0 {
		t.Fatalf("bidi failed=%v stderr=%q", failed, stderr)
	}
	esc := termesc.Escape(bidi)
	want := esc + ": cannot open `" + esc + "' (No such file or directory)\n"
	if string(stdout) != want {
		t.Fatalf("stdout:\n%q\nwant:\n%q", stdout, want)
	}
	if !strings.Contains(string(stdout), `\342\200\256`) {
		t.Fatalf("U+202E not escaped:\n%s", stdout)
	}
	if bytes.Contains(stdout, []byte("\u202e")) {
		t.Fatalf("raw U+202E in output:\n%x", stdout)
	}
}

func runOut(t *testing.T, opts Options) (stdout, stderr []byte, failed bool) {
	t.Helper()
	var out, err bytes.Buffer
	var runErr error
	failed, runErr = Run(opts, &out, &err)
	if runErr != nil {
		t.Fatal(runErr)
	}
	return out.Bytes(), err.Bytes(), failed
}

func assertEscapedPlain(t *testing.T, stdout []byte, escapedPath string) {
	t.Helper()
	text := string(stdout)
	if !strings.HasPrefix(text, escapedPath+": ") {
		t.Fatalf("prefix:\n%q", text)
	}
	if !strings.Contains(text, "Empty file") {
		t.Fatalf("classification dropped:\n%s", text)
	}
	assertNoRawControls(t, stdout)
	if strings.Count(text, "\n") != 1 || !strings.HasSuffix(text, "\n") {
		t.Fatalf("record separator:\n%q", text)
	}
}

func assertNoRawControls(t *testing.T, b []byte) {
	t.Helper()
	for i, c := range b {
		if (c < 0x20 && (c != '\n' || i != len(b)-1)) || c == 0x7f {
			t.Fatalf("raw control %#x at %d in %x", c, i, b)
		}
	}
}
