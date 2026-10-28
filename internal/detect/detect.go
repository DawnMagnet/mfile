package detect

import (
	"io/fs"
	"os"
	"time"

	"github.com/DawnMagnet/mfile/assets"
	"github.com/DawnMagnet/mfile/internal/magika"
)

// Options controls scanning. Zero values keep the scanner defaults.
type Options struct {
	Follow  bool
	Workers int
	Batch   int
}

// Stats holds phase timings for one Scan call that ran the model.
type Stats struct {
	// Active is false when no regular file was opened, so no phases ran.
	Active   bool
	StatOpen time.Duration
	Scanner  time.Duration
	ScanAll  time.Duration
	Files    int
}

// Scan stats each path and classifies regular files. Per-path failures are
// recorded on Result.Error. A non-nil error means the scanner could not be
// created; partial per-path results are not returned in that case.
func Scan(paths []string, opts Options) ([]Result, Stats, error) {
	t0 := time.Now()
	results := make([]Result, len(paths))
	var inputs []magika.Input
	var idx []int
	var files []*os.File
	defer func() {
		for _, f := range files {
			// Read-only input; a close error does not change the classification.
			_ = f.Close()
		}
	}()
	for i, p := range paths {
		r := &results[i]
		r.Index = i
		r.Path = p
		fi, err := stat(p, opts.Follow)
		if err != nil {
			r.Error = err.Error()
			continue
		}
		r.Size = fi.Size()
		r.Mode = fi.Mode().String()
		r.ModTime = fi.ModTime()
		r.Kind = kind(fi.Mode())
		if fi.Mode()&fs.ModeSymlink != 0 {
			target, rerr := os.Readlink(p)
			if rerr != nil {
				// Keep the symlink entry; an unreadable target stays empty.
				target = ""
			}
			r.Target = target
		}
		if !fi.Mode().IsRegular() {
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			r.Error = err.Error()
			continue
		}
		files = append(files, f)
		inputs = append(inputs, magika.Input{R: f, Size: int(fi.Size())})
		idx = append(idx, i)
	}

	var stats Stats
	if len(inputs) == 0 {
		return results, stats, nil
	}
	t1 := time.Now()
	s, err := magika.NewScanner(assets.Model, assets.Config, assets.ContentTypesKB)
	if err != nil {
		return nil, Stats{}, err
	}
	defer s.Close()
	if opts.Workers > 0 {
		s.SetWorkers(opts.Workers)
	}
	if opts.Batch > 0 {
		s.SetMaxBatch(opts.Batch)
	}
	t2 := time.Now()
	scanned, errs := s.ScanAll(inputs)
	stats = Stats{
		Active:   true,
		StatOpen: t1.Sub(t0),
		Scanner:  t2.Sub(t1),
		ScanAll:  time.Since(t2),
		Files:    len(inputs),
	}
	for j, i := range idx {
		if errs[j] != nil {
			results[i].Error = errs[j].Error()
			continue
		}
		results[i].setContent(scanned[j])
	}
	return results, stats, nil
}

func (r *Result) setContent(m magika.Result) {
	r.Label = m.Label
	r.MIME = m.MimeType
	r.Group = m.Group
	r.Description = m.Description
	r.Score = m.Score
	r.Extensions = m.Extensions
	r.IsText = m.IsText
	r.DLLabel = m.DLLabel
	r.Source = m.Source
	r.Classified = true
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
