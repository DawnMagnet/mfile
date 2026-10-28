package cli

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DawnMagnet/mfile/internal/detect"
	"github.com/DawnMagnet/mfile/internal/render"
)

// Run scans opts.Paths and writes one result at a time to stdout.
// failed is true when a path has an error other than a filesystem error
// (cannot open or stat), or when opts.ExitOnFSError is set and a path
// has a filesystem error. Filesystem errors are ordinary stdout lines,
// matching file(1), unless ExitOnFSError is set: then the same line goes
// to stderr. Other paths are still written. Phase timings go to stderr
// when opts.Stats is set and at least one regular file was scanned.
func Run(opts Options, stdout, stderr io.Writer) (failed bool, err error) {
	results, stats, err := detect.Scan(opts.Paths, detect.Options{
		Follow:  opts.Follow,
		Workers: opts.Workers,
		Batch:   opts.Batch,
	})
	if err != nil {
		return false, err
	}
	if opts.Stats && stats.Active {
		if _, err := fmt.Fprintf(stderr, "stat+open %v, NewScanner %v, ScanAll %v (%d files)\n",
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
		if isFSError(res) {
			line := formatOpenError(res.Path, res.Error)
			if opts.ExitOnFSError {
				if _, err := io.WriteString(stderr, line); err != nil {
					return false, err
				}
				failed = true
			} else if !opts.JSON {
				if _, err := io.WriteString(stdout, line); err != nil {
					return false, err
				}
			}
			// JSON keeps the historical object, including its error field.
			// The file-style line is the plain-text form, so it is not
			// mixed into the JSON stream.
			if opts.JSON {
				if err := rnd.Write(res); err != nil {
					return false, err
				}
			}
			continue
		}
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

// isFSError reports a stat or open failure recorded by detect.
// The Go error text is "op path: errno". Content-scan errors are not
// filesystem errors and still fail the process.
func isFSError(r detect.Result) bool {
	if r.Error == "" || r.Classified {
		return false
	}
	_, ok := fsErrno(r.Path, r.Error)
	return ok
}

// formatOpenError is file(1)'s line for a path it could not stat:
// name: cannot open `name' (strerror).
func formatOpenError(path, goErr string) string {
	detail, ok := fsErrno(path, goErr)
	if !ok {
		detail = goErr
	}
	return fmt.Sprintf("%s: cannot open `%s' (%s)\n", path, path, capitalizeErrno(detail))
}

func fsErrno(path, goErr string) (string, bool) {
	for _, op := range []string{"lstat ", "stat ", "open "} {
		prefix := op + path + ": "
		if strings.HasPrefix(goErr, prefix) {
			return goErr[len(prefix):], true
		}
	}
	return "", false
}

func capitalizeErrno(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size <= 1 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
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
