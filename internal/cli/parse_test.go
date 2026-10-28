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
