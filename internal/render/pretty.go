package render

import (
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/DawnMagnet/mfile/internal/detect"
	"github.com/DawnMagnet/mfile/internal/termesc"
)

// Pretty prints one aligned line per detection result.
//
// Columns are padded to a display width chosen in Begin so rows line up
// while results still stream out of Write. A pipe keeps the full layout.
// A terminal narrows it to the terminal width (or Options.Columns): the path budget
// shrinks first, then the label, MIME, source, and size columns drop, in
// that order. Path, description, group, score, the low-confidence marker,
// and error text stay. Paths wider than the column are middle-truncated on
// the raw text, then octal-escaped, so a \ooo sequence is never cut. CJK
// and other wide runes count as two cells. Color uses the 16 ANSI colors and is omitted when stdout is
// not a terminal or NO_COLOR is set to a non-empty value. Icons default to
// unicode on a terminal and to none otherwise.
//
// Write emits each result immediately, in call order. Detection returns
// results in input order, so pretty does not reorder. Begin and End bracket
// the writes. Write is safe for concurrent use; each call emits one whole line.
type Pretty struct {
	w   io.Writer
	opt Options

	mu      sync.Mutex
	begun   bool
	ended   bool
	color   bool
	icons   iconSet
	lay     layout
	restore func() error
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
	if p.restore != nil {
		if err := p.restore(); err != nil {
			return err
		}
		p.restore = nil
	}
	color, icons := resolveAppearance(p.w, p.opt)
	// On a Windows console this turns on ANSI and remembers the previous
	// mode. Failure means the console cannot show color; icons stay as
	// already chosen. End puts the saved mode back.
	colorOK, restore := enableVirtualTerminal(p.w)
	if !colorOK {
		color = false
	}
	p.restore = restore
	p.color = color
	p.icons = icons
	p.lay = resolveLayout(p.w, p.opt, icons != iconsNone)
	p.begun = true
	p.ended = false
	return nil
}

// Write emits r immediately. Call order is output order.
func (p *Pretty) Write(r detect.Result) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.begun {
		return errors.New("render: Write before Begin or after End")
	}
	return p.emit(r)
}

// End finishes the renderer. A second End is a no-op.
func (p *Pretty) End() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.begun {
		if p.ended {
			return nil
		}
		return errors.New("render: End before Begin")
	}
	p.begun = false
	p.ended = true
	restore := p.restore
	p.restore = nil
	if restore != nil {
		return restore()
	}
	return nil
}

func (p *Pretty) emit(r detect.Result) error {
	_, err := io.WriteString(p.w, p.format(r))
	return err
}

func (p *Pretty) format(r detect.Result) string {
	var b strings.Builder
	failed := r.Error != ""
	if p.icons != iconsNone {
		glyph, sgr := iconGlyph(p.icons, r.Label, r.Group, failed)
		b.WriteString(paint(p.color, sgr, glyph))
		b.WriteByte(' ')
	}

	path := fitMiddle(r.Path, p.lay.pathW)
	pathSGR := sgrBold
	if failed {
		pathSGR = sgrErr
	}
	b.WriteString(paint(p.color, pathSGR, path))
	b.WriteString(colSep)

	if failed {
		// Result.Error is the whole message, octal-escaped, with no prefix.
		b.WriteString(paint(p.color, sgrErr, termesc.Escape(r.Error)))
		b.WriteByte('\n')
		return b.String()
	}
	if !r.Classified {
		b.WriteString(unclassified(r))
		b.WriteByte('\n')
		return b.String()
	}

	sgr := groupSGR[r.Group]
	desc := fitEnd(r.Description, descWidth)
	b.WriteString(paint(p.color, sgr, desc))
	b.WriteString(colSep)
	b.WriteString(paint(p.color, sgr, fitEnd(r.Group, groupWidth)))
	b.WriteString(colSep)

	low := lowConfidence(r)
	b.WriteString(paint(p.color, sgrDim, scoreField(r.Score, low)))
	if p.lay.showSize {
		b.WriteString(colSep)
		b.WriteString(paint(p.color, sgrDim, sizeField(r.Size)))
	}
	if p.lay.showSource {
		b.WriteString(colSep)
		b.WriteString(paint(p.color, sgrDim, fitEnd(r.Source, sourceWidth)))
	}
	if p.lay.showLabel {
		b.WriteString(colSep)
		b.WriteString(paint(p.color, sgrDim, fitEnd(r.Label, labelWidth)))
	}
	if p.lay.showMIME {
		b.WriteString(colSep)
		// MIME is not padded, so a short type does not leave trailing spaces.
		b.WriteString(paint(p.color, sgrDim, truncEnd(r.MIME, mimeWidth)))
	}
	if low {
		b.WriteString(colSep)
		b.WriteString(paint(p.color, sgrLow, lowText))
	}
	b.WriteByte('\n')
	return b.String()
}

// unclassified describes a path detect did not classify. Kind is one of
// directory, symlink, fifo, socket, char device, block device, file, or other.
// A symlink with a target is shown as a link rather than the bare kind.
func unclassified(r detect.Result) string {
	switch {
	case r.Target != "":
		return "symbolic link to " + termesc.Escape(r.Target)
	case r.Kind != "":
		return termesc.Escape(r.Kind)
	default:
		return "unclassified"
	}
}

// enableVirtualTerminal is the platform hook for Windows console ANSI.
// colorOK is false only when enabling virtual-terminal processing fails.
// restore is nil when the writer is not a console or the mode is unchanged.
// Tests replace it to cover that fallback and the restore path.
var enableVirtualTerminal = platformEnableVirtualTerminal

var _ Renderer = (*Pretty)(nil)
