package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestParseLegacyAndLongFlags(t *testing.T) {
	opts, err := Parse([]string{
		"-json", "-L", "-workers", "3", "-batch=4", "-stats",
		"-cpuprofile", "cpu.out", "-memprofile=mem.out",
		"a", "b",
	}, ioDiscard(t), "mfile")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.JSON || !opts.Follow || opts.Workers != 3 || opts.Batch != 4 || !opts.Stats {
		t.Fatalf("%+v", opts)
	}
	if opts.CPUProfile != "cpu.out" || opts.MemProfile != "mem.out" {
		t.Fatalf("profiles: %+v", opts)
	}
	if strings.Join(opts.Paths, ",") != "a,b" {
		t.Fatalf("paths: %v", opts.Paths)
	}

	opts, err = Parse([]string{"--json", "--dereference", "--workers=2", "--L", "a"}, ioDiscard(t), "mfile")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.JSON || !opts.Follow || opts.Workers != 2 || len(opts.Paths) != 1 {
		t.Fatalf("long flags: %+v", opts)
	}
}

func TestParseStopsAtOperand(t *testing.T) {
	opts, err := Parse([]string{"file", "-json"}, ioDiscard(t), "mfile")
	if err != nil {
		t.Fatal(err)
	}
	if opts.JSON {
		t.Fatal("flag after operand was parsed")
	}
	if strings.Join(opts.Paths, ",") != "file,-json" {
		t.Fatalf("paths: %v", opts.Paths)
	}
}

func TestParseVersion(t *testing.T) {
	opts, err := Parse([]string{"--version"}, ioDiscard(t), "mfile")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Version {
		t.Fatal("long version flag not set")
	}
	opts, err = Parse([]string{"-version", "file"}, ioDiscard(t), "mfile")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Version || len(opts.Paths) != 1 || opts.Paths[0] != "file" {
		t.Fatalf("%+v", opts)
	}
}

func TestParseExitOnFSError(t *testing.T) {
	opts, err := Parse([]string{"-E", "missing"}, ioDiscard(t), "mfile")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.ExitOnFSError || len(opts.Paths) != 1 || opts.Paths[0] != "missing" {
		t.Fatalf("%+v", opts)
	}
	opts, err = Parse([]string{"--exit-on-fs-error", "-L", "a"}, ioDiscard(t), "mfile")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.ExitOnFSError || !opts.Follow {
		t.Fatalf("%+v", opts)
	}
	opts, err = Parse([]string{"-exit-on-fs-error", "a"}, ioDiscard(t), "mfile")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.ExitOnFSError || len(opts.Paths) != 1 {
		t.Fatalf("single-dash long flag: %+v", opts)
	}

	var buf bytes.Buffer
	if _, err := Parse([]string{"-h"}, &buf, "mfile"); err == nil || !errors.Is(err, pflag.ErrHelp) {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "-E") || !strings.Contains(buf.String(), "filesystem errors") || !strings.Contains(buf.String(), "POSIX") {
		t.Fatalf("help missing -E:\n%s", buf.String())
	}
}

func TestParseHelpAndUnknown(t *testing.T) {
	var buf bytes.Buffer
	_, err := Parse([]string{"-h"}, &buf, "mfile")
	if !errors.Is(err, pflag.ErrHelp) {
		t.Fatalf("help err: %v", err)
	}
	if !strings.Contains(buf.String(), "usage: mfile [flags] FILE...") {
		t.Fatalf("help:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "--dereference") || !strings.Contains(buf.String(), "-L") {
		t.Fatalf("help flags:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "MFILE_CPU_GUARD") || !strings.Contains(buf.String(), "debugging only") || !strings.Contains(buf.String(), "SIGILL") {
		t.Fatalf("help missing guard note:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "AVX without AVX2") || !strings.Contains(buf.String(), "AVX2 without FMA") {
		t.Fatalf("help missing affected CPUs:\n%s", buf.String())
	}

	buf.Reset()
	_, err = Parse([]string{"--nope"}, &buf, "mfile")
	if err == nil || errors.Is(err, pflag.ErrHelp) {
		t.Fatalf("unknown: %v", err)
	}
	if !strings.Contains(buf.String(), "unknown flag") {
		t.Fatalf("unknown output:\n%s", buf.String())
	}
}

func ioDiscard(t *testing.T) *bytes.Buffer {
	t.Helper()
	return &bytes.Buffer{}
}
