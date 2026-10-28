// Command mfile is a static, cgo-free `file` replacement that identifies
// content types with Google's Magika model and prints basic file metadata.
package main

import (
	"errors"
	"fmt"
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
	cli.CapGOMAXPROCS()
	opts, err := cli.Parse(os.Args[1:], os.Stderr, os.Args[0])
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(2)
	}
	if opts.Version {
		if _, err := fmt.Fprint(os.Stdout, ver.Format(version)); err != nil {
			fmt.Fprintln(os.Stderr, "mfile:", err)
			os.Exit(1)
		}
		return
	}
	if len(opts.Paths) == 0 {
		if err := cli.Usage(os.Stderr, os.Args[0]); err != nil {
			fmt.Fprintln(os.Stderr, "mfile:", err)
		}
		os.Exit(2)
	}
	stopCPU, err := startCPUProfile(opts.CPUProfile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mfile:", err)
		os.Exit(1)
	}
	failed, runErr := cli.Run(opts, os.Stdout)
	if stopCPU != nil {
		if err := stopCPU(); err != nil {
			fmt.Fprintln(os.Stderr, "mfile:", err)
		}
	}
	if opts.MemProfile != "" {
		if err := writeMemProfile(opts.MemProfile); err != nil {
			fmt.Fprintln(os.Stderr, "mfile:", err)
		}
	}
	if opts.Stats {
		if err := cli.PrintProcessStats(os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, "mfile:", err)
		}
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "mfile:", runErr)
		os.Exit(1)
	}
	if failed {
		os.Exit(1)
	}
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
