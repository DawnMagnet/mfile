package cli

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/DawnMagnet/mfile/internal/detect"
	"github.com/DawnMagnet/mfile/internal/render"
)

// Run scans opts.Paths and writes one result at a time to stdout.
// failed is true when any path has an error. Phase timings go to stderr
// when opts.Stats is set and at least one regular file was scanned.
func Run(opts Options, stdout io.Writer) (failed bool, err error) {
	results, stats, err := detect.Scan(opts.Paths, detect.Options{
		Follow:  opts.Follow,
		Workers: opts.Workers,
		Batch:   opts.Batch,
	})
	if err != nil {
		return false, err
	}
	if opts.Stats && stats.Active {
		if _, err := fmt.Fprintf(os.Stderr, "stat+open %v, NewScanner %v, ScanAll %v (%d files)\n",
			stats.StatOpen, stats.Scanner, stats.ScanAll, stats.Files); err != nil {
			return false, err
		}
	}
	var rnd render.Renderer
	if opts.JSON {
		rnd = render.NewJSON(stdout)
	} else {
		rnd = render.NewPlain(stdout)
	}
	if err := rnd.Begin(); err != nil {
		return false, err
	}
	for _, res := range results {
		if res.Error != "" {
			failed = true
		}
		if err := rnd.Write(res); err != nil {
			return false, err
		}
	}
	if err := rnd.End(); err != nil {
		return false, err
	}
	return failed, nil
}

// PrintProcessStats writes heap counters and, when available, peak RSS.
func PrintProcessStats(w io.Writer) error {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	if _, err := fmt.Fprintf(w, "heap sys %.1f MiB, total alloc %.1f MiB, %d GCs, GOMAXPROCS %d\n",
		float64(ms.HeapSys)/(1<<20), float64(ms.TotalAlloc)/(1<<20), ms.NumGC, runtime.GOMAXPROCS(0)); err != nil {
		return err
	}
	if rss, ok := peakRSS(); ok {
		if _, err := fmt.Fprintf(w, "peak RSS %.1f MiB\n", float64(rss)/(1<<20)); err != nil {
			return err
		}
	}
	return nil
}
