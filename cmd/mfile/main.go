// Command mfile is a static, cgo-free `file` replacement that identifies
// content types with Google's Magika model and prints basic file metadata.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"

	"github.com/DawnMagnet/mfile/internal/cli"
	"github.com/DawnMagnet/mfile/internal/cpuguard"
	ver "github.com/DawnMagnet/mfile/internal/version"
	"github.com/spf13/pflag"
)

// version is set with -ldflags "-X main.version=...".
// An empty value falls back to build info, then "dev".
var version string

func main() {
	// The guard runs at the start of main, after package init of the gomlx
	// dependencies. It assumes those init functions do not execute SIMD
	// kernels. The qemu real-inference gate (Sandy Bridge, and Haswell
	// without FMA) is what checks that assumption.
	cpuguard.Apply()
	os.Exit(run(os.Args[1:], os.Args[0], os.Stdout, os.Stderr))
}

// run is the command after the CPU guard. Usage and argument errors return 2.
// A filesystem error returns 1 only when -E is set; otherwise it returns 0.
func run(args []string, argv0 string, stdout, stderr io.Writer) int {
	cli.CapGOMAXPROCS()
	opts, err := cli.Parse(args, stderr, argv0)
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return 0
		}
		return 2
	}
	if opts.Version {
		if _, err := fmt.Fprint(stdout, ver.Format(version)); err != nil {
			fmt.Fprintln(stderr, "mfile:", err)
			return 1
		}
		return 0
	}
	if len(opts.Paths) == 0 {
		if err := cli.Usage(stderr, argv0); err != nil {
			fmt.Fprintln(stderr, "mfile:", err)
		}
		return 2
	}
	stopCPU, err := startCPUProfile(opts.CPUProfile)
	if err != nil {
		fmt.Fprintln(stderr, "mfile:", err)
		return 1
	}
	failed, runErr := cli.Run(opts, stdout, stderr)
	if stopCPU != nil {
		if err := stopCPU(); err != nil {
			fmt.Fprintln(stderr, "mfile:", err)
		}
	}
	if opts.MemProfile != "" {
		if err := writeMemProfile(opts.MemProfile); err != nil {
			fmt.Fprintln(stderr, "mfile:", err)
		}
	}
	if opts.Stats {
		if err := cli.PrintProcessStats(stderr); err != nil {
			fmt.Fprintln(stderr, "mfile:", err)
		}
	}
	if runErr != nil {
		fmt.Fprintln(stderr, "mfile:", runErr)
		return 1
	}
	if failed {
		return 1
	}
	return 0
}

func startCPUProfile(path string) (func() error, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		cerr := f.Close()
		return nil, errors.Join(err, cerr)
	}
	return func() error {
		pprof.StopCPUProfile()
		return f.Close()
	}, nil
}

func writeMemProfile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	runtime.GC()
	werr := pprof.WriteHeapProfile(f)
	cerr := f.Close()
	return errors.Join(werr, cerr)
}
