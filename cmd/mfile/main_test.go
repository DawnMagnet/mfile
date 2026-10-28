package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")
	openLine := missing + ": cannot open `" + missing + "' (No such file or directory)\n"

	t.Run("one missing", func(t *testing.T) {
		stdout, stderr, code := call(t, missing)
		if code != 0 {
			t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
		}
		if stdout != openLine || stderr != "" {
			t.Fatalf("stdout %q stderr %q", stdout, stderr)
		}
	})

	t.Run("existing and missing", func(t *testing.T) {
		stdout, stderr, code := call(t, empty, missing)
		if code != 0 {
			t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
		}
		if stderr != "" {
			t.Fatalf("stderr %q", stderr)
		}
		if !strings.Contains(stdout, "Empty file") || !strings.Contains(stdout, openLine) {
			t.Fatalf("stdout:\n%s", stdout)
		}
	})

	t.Run("E missing", func(t *testing.T) {
		stdout, stderr, code := call(t, "-E", missing)
		if code != 1 {
			t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
		}
		if stdout != "" || stderr != openLine {
			t.Fatalf("stdout %q stderr %q", stdout, stderr)
		}
	})

	t.Run("json missing", func(t *testing.T) {
		stdout, stderr, code := call(t, "-json", missing)
		if code != 0 {
			t.Fatalf("exit %d\nstdout %s\nstderr %s", code, stdout, stderr)
		}
		if stderr != "" {
			t.Fatalf("stderr %q", stderr)
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(stdout), &obj); err != nil {
			t.Fatalf("json: %v\n%s", err, stdout)
		}
		for _, key := range []string{"path", "kind", "size", "mode", "mtime", "error"} {
			if _, ok := obj[key]; !ok {
				t.Fatalf("missing %q in %s", key, stdout)
			}
		}
		if len(obj) != 6 {
			t.Fatalf("fields changed: %s", stdout)
		}
	})

	t.Run("usage", func(t *testing.T) {
		_, _, code := call(t)
		if code != 2 {
			t.Fatalf("no args exit %d", code)
		}
	})

	t.Run("bad flag", func(t *testing.T) {
		_, stderr, code := call(t, "--nope")
		if code != 2 {
			t.Fatalf("bad flag exit %d\n%s", code, stderr)
		}
		if !strings.Contains(stderr, "unknown") {
			t.Fatalf("stderr:\n%s", stderr)
		}
	})
}

func call(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, err bytes.Buffer
	code = run(args, "mfile", &out, &err)
	return out.String(), err.String(), code
}
