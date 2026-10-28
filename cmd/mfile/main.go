// Command mfile is a static, cgo-free `file` replacement that identifies
// content types with Google's Magika model and prints basic file metadata.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/DawnMagnet/mfile/assets"
	"github.com/DawnMagnet/mfile/internal/magika"
)

type entry struct {
	Path    string    `json:"path"`
	Kind    string    `json:"kind"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"mtime"`
	Target  string    `json:"symlink_target,omitempty"`
	*magika.Result
	Error string `json:"error,omitempty"`
}

type options struct {
	jsonOut, follow, stats bool
	workers, batch         int
}

func main() {
	if os.Getenv("GOMAXPROCS") == "" && maxProcs > 0 && runtime.GOMAXPROCS(0) > maxProcs {
		runtime.GOMAXPROCS(maxProcs)
	}
	var opts options
	flag.BoolVar(&opts.jsonOut, "json", false, "print one JSON object per line")
	flag.BoolVar(&opts.follow, "L", false, "follow symlinks")
	flag.IntVar(&opts.workers, "workers", 0, "concurrent inference calls (0: GOMAXPROCS)")
	flag.IntVar(&opts.batch, "batch", 0, "max files per inference call, a power of two (0: 1)")
	flag.BoolVar(&opts.stats, "stats", false, "print timings and memory use to stderr")
	cpuProfile := flag.String("cpuprofile", "", "write a CPU profile to `file`")
	memProfile := flag.String("memprofile", "", "write a heap profile to `file` at exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [flags] FILE...\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mfile:", err)
			os.Exit(1)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintln(os.Stderr, "mfile:", err)
			os.Exit(1)
		}
	}
	failed, err := run(flag.Args(), opts)
	if *cpuProfile != "" {
		pprof.StopCPUProfile()
	}
	if *memProfile != "" {
		if f, err := os.Create(*memProfile); err == nil {
			runtime.GC()
			// Best-effort diagnostic dump: keep the process exit code unchanged.
			_ = pprof.WriteHeapProfile(f)
			_ = f.Close()
		}
	}
	if opts.stats {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		fmt.Fprintf(os.Stderr, "heap sys %.1f MiB, total alloc %.1f MiB, %d GCs, GOMAXPROCS %d\n",
			float64(ms.HeapSys)/(1<<20), float64(ms.TotalAlloc)/(1<<20), ms.NumGC, runtime.GOMAXPROCS(0))
		if rss, ok := peakRSS(); ok {
			fmt.Fprintf(os.Stderr, "peak RSS %.1f MiB\n", float64(rss)/(1<<20))
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mfile:", err)
		os.Exit(1)
	}
	if failed {
		os.Exit(1)
	}
}

func run(paths []string, opts options) (bool, error) {
	t0 := time.Now()
	entries := make([]entry, len(paths))
	var inputs []magika.Input
	var idx []int
	var files []*os.File
	defer func() {
		for _, f := range files {
			_ = f.Close()
		}
	}()
	for i, p := range paths {
		e := &entries[i]
		e.Path = p
		fi, err := stat(p, opts.follow)
		if err != nil {
			e.Error = err.Error()
			continue
		}
		e.Size, e.Mode, e.ModTime = fi.Size(), fi.Mode().String(), fi.ModTime()
		e.Kind = kind(fi.Mode())
		if fi.Mode()&fs.ModeSymlink != 0 {
			e.Target, _ = os.Readlink(p)
		}
		if !fi.Mode().IsRegular() {
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			e.Error = err.Error()
			continue
		}
		files = append(files, f)
		inputs = append(inputs, magika.Input{R: f, Size: int(fi.Size())})
		idx = append(idx, i)
	}

	var failed bool
	if len(inputs) > 0 {
		t1 := time.Now()
		s, err := magika.NewScanner(assets.Model, assets.Config, assets.ContentTypesKB)
		if err != nil {
			return false, err
		}
		defer s.Close()
		if opts.workers > 0 {
			s.SetWorkers(opts.workers)
		}
		if opts.batch > 0 {
			s.SetMaxBatch(opts.batch)
		}
		t2 := time.Now()
		results, errs := s.ScanAll(inputs)
		if opts.stats {
			fmt.Fprintf(os.Stderr, "stat+open %v, NewScanner %v, ScanAll %v (%d files)\n",
				t1.Sub(t0), t2.Sub(t1), time.Since(t2), len(inputs))
		}
		for j, i := range idx {
			if errs[j] != nil {
				entries[i].Error = errs[j].Error()
				continue
			}
			entries[i].Result = &results[j]
		}
	}

	enc := json.NewEncoder(os.Stdout)
	for _, e := range entries {
		if e.Error != "" {
			failed = true
		}
		if opts.jsonOut {
			if err := enc.Encode(e); err != nil {
				return false, err
			}
			continue
		}
		printText(e)
	}
	return failed, nil
}

func stat(p string, follow bool) (fs.FileInfo, error) {
	if follow {
		return os.Stat(p)
	}
	return os.Lstat(p)
}

func kind(m fs.FileMode) string {
	switch {
	case m.IsRegular():
		return "file"
	case m.IsDir():
		return "directory"
	case m&fs.ModeSymlink != 0:
		return "symlink"
	case m&fs.ModeNamedPipe != 0:
		return "fifo"
	case m&fs.ModeSocket != 0:
		return "socket"
	case m&fs.ModeCharDevice != 0:
		return "char device"
	case m&fs.ModeDevice != 0:
		return "block device"
	}
	return "other"
}

func printText(e entry) {
	switch {
	case e.Error != "":
		fmt.Printf("%s: error: %s\n", e.Path, e.Error)
	case e.Result != nil:
		r := e.Result
		fmt.Printf("%s: %s [%s, %s, %s] score=%.3f size=%s mode=%s mtime=%s\n",
			e.Path, r.Description, r.Label, r.MimeType, r.Group, r.Score,
			humanSize(e.Size), e.Mode, e.ModTime.Format(time.DateTime))
	case e.Target != "":
		fmt.Printf("%s: symbolic link to %s\n", e.Path, e.Target)
	default:
		fmt.Printf("%s: %s mode=%s mtime=%s\n", e.Path, e.Kind, e.Mode, e.ModTime.Format(time.DateTime))
	}
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	v, i := float64(n), 0
	for v >= unit && i < 6 {
		v /= unit
		i++
	}
	return fmt.Sprintf("%.1f%ciB", v, "KMGTPE"[i-1])
}
