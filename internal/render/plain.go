package render

import (
	"fmt"
	"io"
	"time"

	"github.com/DawnMagnet/mfile/internal/detect"
	"github.com/DawnMagnet/mfile/internal/termesc"
)

// Plain writes one historical text line per result.
type Plain struct {
	w io.Writer
}

// NewPlain returns a renderer that streams plain text to w.
func NewPlain(w io.Writer) *Plain {
	return &Plain{w: w}
}

// Begin implements Renderer.
func (p *Plain) Begin() error { return nil }

// Write implements Renderer. Path, Target, and Error are octal-escaped
// here. The caller passes the raw detect.Result.
func (p *Plain) Write(r detect.Result) error {
	path := termesc.Escape(r.Path)
	target := termesc.Escape(r.Target)
	errText := termesc.Escape(r.Error)
	var err error
	switch {
	case r.Error != "":
		_, err = fmt.Fprintf(p.w, "%s: error: %s\n", path, errText)
	case r.Classified:
		_, err = fmt.Fprintf(p.w, "%s: %s [%s, %s, %s] score=%.3f size=%s mode=%s mtime=%s\n",
			path, r.Description, r.Label, r.MIME, r.Group, r.Score,
			humanSize(r.Size), r.Mode, r.ModTime.Format(time.DateTime))
	case r.Target != "":
		_, err = fmt.Fprintf(p.w, "%s: symbolic link to %s\n", path, target)
	default:
		_, err = fmt.Fprintf(p.w, "%s: %s mode=%s mtime=%s\n",
			path, r.Kind, r.Mode, r.ModTime.Format(time.DateTime))
	}
	return err
}

// End implements Renderer.
func (p *Plain) End() error { return nil }

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
