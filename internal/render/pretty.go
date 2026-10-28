package render

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/DawnMagnet/mfile/internal/detect"
)

// Pretty prints one aligned line per detection result.
//
// Columns are padded to a fixed display width so rows line up while results
// still stream out of Write. Paths wider than the column are middle-truncated
// on rune boundaries, which keeps the file name. CJK and other wide runes
// count as two cells. Color uses the 16 ANSI colors and is omitted when
// stdout is not a terminal or NO_COLOR is set to a non-empty value. Icons
// default to unicode on a terminal and to none otherwise.
//
// Write is safe for concurrent use. Begin and End should bracket the writes:
// call Begin, then Write any number of results, then End. End flushes results
// that never became contiguous, in Seq order.
type Pretty struct {
	w   io.Writer
	opt Options

	mu    sync.Mutex
	begun bool
	ended bool
	color bool
	icons iconSet

	next int
	buf  map[int]detect.Result
}

// NewPretty returns a renderer that writes to w. A nil writer is rejected
// from Begin.
func NewPretty(w io.Writer, opt Options) *Pretty {
	return &Pretty{w: w, opt: opt}
}

// Begin resets the renderer and resolves color and icons from the terminal
// and the environment.
func (p *Pretty) Begin() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.w == nil {
		return errors.New("render: nil writer")
	}
	color, icons := resolveAppearance(p.w, p.opt)
	p.color = color
	p.icons = icons
	p.next = 0
	p.buf = make(map[int]detect.Result)
	p.begun = true
	p.ended = false
	return nil
}

// Write buffers r until every lower Seq has been seen, then writes those
// lines. A Seq that was already emitted returns an error. Writing a Seq that
// is still buffered replaces it, so a failed emit can be retried.
func (p *Pretty) Write(r detect.Result) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.begun {
		return errors.New("render: Write before Begin or after End")
	}
	if r.Seq < p.next {
		return fmt.Errorf("render: duplicate or stale seq %d", r.Seq)
	}
	// Overwrite a seq that is still buffered so a failed emit can be retried.
	p.buf[r.Seq] = r
	return p.flushReady()
}

// End writes any results still buffered, in Seq order, including gaps.
// A second End is a no-op.
func (p *Pretty) End() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.begun {
		if p.ended {
			return nil
		}
		return errors.New("render: End before Begin")
	}
	if err := p.flushReady(); err != nil {
		return err
	}
	if len(p.buf) > 0 {
		keys := make([]int, 0, len(p.buf))
		for k := range p.buf {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, k := range keys {
			if err := p.emit(p.buf[k]); err != nil {
				return err
			}
			delete(p.buf, k)
		}
	}
	p.begun = false
	p.ended = true
	return nil
}

// flushReady writes the contiguous run starting at p.next.
// The caller holds p.mu.
func (p *Pretty) flushReady() error {
	for {
		r, ok := p.buf[p.next]
		if !ok {
			return nil
		}
		if err := p.emit(r); err != nil {
			return err
		}
		delete(p.buf, p.next)
		p.next++
	}
}

func (p *Pretty) emit(r detect.Result) error {
	_, err := io.WriteString(p.w, p.format(r))
	return err
}

func (p *Pretty) format(r detect.Result) string {
	var b strings.Builder
	failed := r.Err != nil
	if p.icons != iconsNone {
		glyph, sgr := iconGlyph(p.icons, r.Label, r.Group, failed)
		b.WriteString(paint(p.color, sgr, glyph))
		b.WriteByte(' ')
	}

	path := fitMiddle(sanitize(r.Path), pathWidth)
	pathSGR := sgrBold
	if failed {
		pathSGR = sgrErr
	}
	b.WriteString(paint(p.color, pathSGR, path))
	b.WriteString("  ")

	if failed {
		msg := "error: " + sanitize(r.Err.Error())
		b.WriteString(paint(p.color, sgrErr, msg))
		b.WriteByte('\n')
		return b.String()
	}

	sgr := groupSGR[r.Group]
	desc := fitEnd(sanitize(r.Description), descWidth)
	b.WriteString(paint(p.color, sgr, desc))
	b.WriteString("  ")
	b.WriteString(paint(p.color, sgr, fitEnd(sanitize(r.Group), groupWidth)))
	b.WriteString("  ")

	low := lowConfidence(r)
	b.WriteString(paint(p.color, sgrDim, scoreField(r.Score, low)))
	b.WriteString("  ")
	b.WriteString(paint(p.color, sgrDim, sizeField(r.Size)))
	b.WriteString("  ")
	b.WriteString(paint(p.color, sgrDim, fitEnd(sanitize(string(r.Source)), sourceWidth)))
	b.WriteString("  ")
	b.WriteString(paint(p.color, sgrDim, fitEnd(sanitize(r.Label), labelWidth)))
	b.WriteString("  ")
	// MIME is the last data column: truncate it, but do not pad it, so a
	// short type does not leave trailing spaces. "low" follows when needed.
	b.WriteString(paint(p.color, sgrDim, truncEnd(sanitize(r.MimeType), mimeWidth)))
	if low {
		b.WriteString("  ")
		b.WriteString(paint(p.color, sgrLow, "low"))
	}
	b.WriteByte('\n')
	return b.String()
}

var _ Renderer = (*Pretty)(nil)
