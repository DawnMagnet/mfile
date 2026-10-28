package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/pflag"
)

// Options is the parsed command line.
type Options struct {
	JSON       bool
	Follow     bool
	Workers    int
	Batch      int
	Stats      bool
	CPUProfile string
	MemProfile string
	Paths      []string
}

// longFlags are the historical single-dash names (-json, -workers, ...).
// pflag treats a single dash as a cluster of shorthands, so Parse rewrites
// these to long options before parsing. -L stays a shorthand.
var longFlags = map[string]struct{}{
	"json":       {},
	"workers":    {},
	"batch":      {},
	"stats":      {},
	"cpuprofile": {},
	"memprofile": {},
	"help":       {},
}

// valueFlags consume the following argument when it is not attached with '='.
var valueFlags = map[string]struct{}{
	"workers":    {},
	"batch":      {},
	"cpuprofile": {},
	"memprofile": {},
}

// Parse parses args as the mfile command line. argv0 is the program name
// printed in the usage line. Help requests return pflag.ErrHelp after usage
// has been written to stderr. Other parse errors are also written to stderr.
func Parse(args []string, stderr io.Writer, argv0 string) (Options, error) {
	var opts Options
	var usageErr error
	fs := newFlagSet(&opts, stderr, argv0, &usageErr)
	if err := fs.Parse(normalizeArgs(args)); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			if usageErr != nil {
				return Options{}, usageErr
			}
			return Options{}, err
		}
		if _, werr := fmt.Fprintln(stderr, err); werr != nil {
			return Options{}, werr
		}
		fs.Usage()
		if usageErr != nil {
			return Options{}, usageErr
		}
		return Options{}, err
	}
	opts.Paths = fs.Args()
	return opts, nil
}

// Usage writes the command usage to stderr.
func Usage(stderr io.Writer, argv0 string) error {
	var opts Options
	var usageErr error
	fs := newFlagSet(&opts, stderr, argv0, &usageErr)
	fs.Usage()
	return usageErr
}

func newFlagSet(opts *Options, stderr io.Writer, argv0 string, usageErr *error) *pflag.FlagSet {
	fs := pflag.NewFlagSet(argv0, pflag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.SetInterspersed(false)
	fs.SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		switch name {
		case "L", "follow":
			return "dereference"
		default:
			return pflag.NormalizedName(name)
		}
	})
	fs.BoolVar(&opts.JSON, "json", false, "print one JSON object per line")
	fs.BoolVarP(&opts.Follow, "dereference", "L", false, "follow symlinks")
	fs.IntVar(&opts.Workers, "workers", 0, "concurrent inference calls (0: GOMAXPROCS)")
	fs.IntVar(&opts.Batch, "batch", 0, "max files per inference call, a power of two (0: 1)")
	fs.BoolVar(&opts.Stats, "stats", false, "print timings and memory use to stderr")
	fs.StringVar(&opts.CPUProfile, "cpuprofile", "", "write a CPU profile to `file`")
	fs.StringVar(&opts.MemProfile, "memprofile", "", "write a heap profile to `file` at exit")
	fs.Usage = func() {
		if _, err := fmt.Fprintf(stderr, "usage: %s [flags] FILE...\n", argv0); err != nil {
			*usageErr = err
			return
		}
		fs.PrintDefaults()
	}
	return fs
}

// normalizeArgs rewrites -name and -name=value to --name for known long options.
// Rewriting stops at the first positional argument or --, matching Go's flag
// package (flags do not follow operands), so a later -json stays a path.
// A separate argument that belongs to a flag (-workers 3) is not positional.
func normalizeArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" || len(a) == 0 || a[0] != '-' || len(a) == 1 {
			break
		}
		var name string
		if a[1] == '-' {
			name = a[2:]
		} else {
			name = a[1:]
		}
		name, _, hasEq := strings.Cut(name, "=")
		if a[1] != '-' {
			if _, ok := longFlags[name]; ok {
				out[i] = "-" + a
			}
		}
		if !hasEq {
			if _, ok := valueFlags[name]; ok && i+1 < len(args) {
				i++
			}
		}
	}
	return out
}
