package render

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DawnMagnet/mfile/internal/detect"
)

// ColorMode selects when pretty output uses ANSI color.
type ColorMode int

const (
	// ColorAuto enables color on a terminal unless NO_COLOR is set.
	ColorAuto ColorMode = iota
	// ColorAlways enables color even when stdout is not a terminal.
	// A non-empty NO_COLOR still disables it.
	ColorAlways
	// ColorNever disables color.
	ColorNever
)

// IconMode selects the icon glyph set.
type IconMode int

const (
	// IconAuto uses unicode glyphs on a terminal and no glyphs when stdout
	// is not a terminal or NO_COLOR is set.
	IconAuto IconMode = iota
	IconUnicode
	IconNerd
	IconNone
)

// Options configures a pretty renderer. The zero value is ColorAuto and
// IconAuto, which follow the terminal and NO_COLOR.
type Options struct {
	Color ColorMode
	Icons IconMode
	// IsTTY, when non-nil, overrides terminal detection on the writer.
	IsTTY *bool
	// Columns, when positive, is the terminal width in cells. It applies
	// only when the output is a TTY; a pipe or buffer keeps the full layout.
	// Zero asks the terminal for its width.
	Columns int
}

const (
	// Column widths are display cells, not bytes. The full layout uses these
	// bounds so a streaming writer can align rows without seeing later paths.
	// On a narrow terminal the path bound shrinks (down to pathWidthMin) and
	// then optional columns are dropped. Paths and descriptions longer than
	// the bound are truncated on rune boundaries.
	pathWidth    = 28
	pathWidthMin = 16
	descWidth    = 28
	groupWidth   = 12
	scoreWidth   = 12
	sizeWidth    = 14
	sourceWidth  = 9
	labelWidth   = 16
	mimeWidth    = 32

	// colSep separates columns. iconCols is a one-cell glyph plus a space.
	colSep   = "  "
	iconCols = 2
	lowText  = "low"

	// lowScoreThreshold marks a result as low confidence. Sources in
	// lowSources are also low confidence at any score. Any other source
	// string is shown as itself and is low only under this threshold.
	lowScoreThreshold = 0.5
)

// layout is the column plan chosen in Begin. cols is the terminal width
// when the output is a TTY; zero keeps the full layout and does not clip
// error text.
type layout struct {
	cols       int
	pathW      int
	showSize   bool
	showSource bool
	showLabel  bool
	showMIME   bool
}

func fullLayout() layout {
	return layout{
		pathW:      pathWidth,
		showSize:   true,
		showSource: true,
		showLabel:  true,
		showMIME:   true,
	}
}

// lineWidth is the display width of a low-confidence row, which is the
// widest shape: every shown column plus the "low" marker.
func (l layout) lineWidth(icons bool) int {
	fields := []int{l.pathW, descWidth, groupWidth, scoreWidth}
	if l.showSize {
		fields = append(fields, sizeWidth)
	}
	if l.showSource {
		fields = append(fields, sourceWidth)
	}
	if l.showLabel {
		fields = append(fields, labelWidth)
	}
	if l.showMIME {
		fields = append(fields, mimeWidth)
	}
	fields = append(fields, len(lowText))
	w := fields[0]
	if icons {
		w += iconCols
	}
	for _, f := range fields[1:] {
		w += len(colSep) + f
	}
	return w
}

// fitLayout sizes a TTY layout to width. The path budget scales with the
// terminal (pathWidth * width / fullWidth, at least pathWidthMin) before any
// column is removed. Columns then drop in order: label, MIME, source, size.
// Path, description, group, and score stay. The low marker is part of the
// budget, so it fits on the same line. If the core is still wider than the
// terminal, the path shrinks below pathWidthMin until it fits or reaches one
// cell.
func fitLayout(width int, icons bool) layout {
	l := fullLayout()
	l.cols = width
	full := l.lineWidth(icons)
	if width >= full {
		return l
	}
	path := pathWidth * width / full
	if path < pathWidthMin {
		path = pathWidthMin
	}
	if path > pathWidth {
		path = pathWidth
	}
	l.pathW = path
	for _, drop := range []*bool{&l.showLabel, &l.showMIME, &l.showSource, &l.showSize} {
		if l.lineWidth(icons) <= width {
			return l
		}
		*drop = false
	}
	for l.lineWidth(icons) > width && l.pathW > 1 {
		l.pathW--
	}
	return l
}

func resolveLayout(w io.Writer, opt Options, icons bool) layout {
	if !isTTY(w, opt) {
		return fullLayout()
	}
	cols := opt.Columns
	if cols <= 0 {
		cols = terminalColumns(w)
	}
	if cols <= 0 {
		return fullLayout()
	}
	return fitLayout(cols, icons)
}

const (
	ansiReset = "\033[0m"
	sgrBold   = "1"
	sgrDim    = "2"
	// sgrLow is bold and underlined in the default foreground, so the marker
	// stays readable on light and dark backgrounds without a risky hue.
	sgrLow = "1;4"
	sgrErr = "1;31"
)

// Group foregrounds are the normal and bright members of the 16-color set
// (SGR 30–37 and 90–97). Bright neon colors that disappear on a light
// background (yellow, bright green, bright cyan, white) are not used.
// The group name is always printed as text; color is only an extra cue.
var groupSGR = map[string]string{
	"code":        "36",
	"text":        "32",
	"document":    "94",
	"image":       "35",
	"video":       "95",
	"audio":       "91",
	"archive":     "31",
	"executable":  "1;32",
	"font":        "1;36",
	"database":    "1;35",
	"application": "94",
	"binary":      "90",
	"geometry":    "36",
	"gis":         "32",
	"scientific":  "1;36",
	"model":       "35",
	"inode":       "94",
	"unknown":     "90",
	"undefined":   "90",
}

type iconSet int

const (
	iconsNone iconSet = iota
	iconsUnicode
	iconsNerd
)

// Unicode icons are East Asian Width Neutral (single cell in every locale).
// Nerd icons are Font Awesome 4 codepoints from a Nerd Font. They are one
// Private Use Area rune, counted as one cell, and followed by a space so the
// path column stays put. A font that draws those glyphs two cells wide will
// shift that row by one cell.
var (
	unicodeGroupIcons = map[string]string{
		"code":        "⌘",
		"text":        "✎",
		"document":    "✑",
		"image":       "✿",
		"video":       "✦",
		"audio":       "♫",
		"archive":     "✉",
		"executable":  "⚙",
		"font":        "✒",
		"database":    "☰",
		"application": "⎈",
		"binary":      "✶",
		"geometry":    "◊",
		"gis":         "☀",
		"scientific":  "⚗",
		"model":       "⍟",
		"inode":       "⌂",
		"unknown":     "◌",
		"undefined":   "✕",
	}
	unicodeLabelIcons = map[string]string{
		"symlink": "⎋",
		"empty":   "◌",
		"unknown": "?",
		"shell":   "⏎",
		"json":    "✱",
	}

	// Nerd Font / Font Awesome 4 codepoints.
	nerdGroupIcons = map[string]string{
		"code":        "\uf121", // nf-fa-code
		"text":        "\uf0f6", // nf-fa-file_text_o
		"document":    "\uf15c", // nf-fa-file_text
		"image":       "\uf1c5", // nf-fa-file_image
		"video":       "\uf1c8", // nf-fa-file_video
		"audio":       "\uf1c7", // nf-fa-file_audio
		"archive":     "\uf1c6", // nf-fa-file_archive
		"executable":  "\uf013", // nf-fa-cog
		"font":        "\uf031", // nf-fa-font
		"database":    "\uf1c0", // nf-fa-database
		"application": "\uf085", // nf-fa-cogs
		"binary":      "\uf1b2", // nf-fa-cube
		"geometry":    "\uf1b2", // nf-fa-cube
		"gis":         "\uf041", // nf-fa-map_marker
		"scientific":  "\uf0c3", // nf-fa-flask
		"model":       "\uf1b3", // nf-fa-cubes
		"inode":       "\uf07b", // nf-fa-folder
		"unknown":     "\uf128", // nf-fa-question
		"undefined":   "\uf128", // nf-fa-question
	}
	nerdLabelIcons = map[string]string{
		"directory": "\uf07b", // nf-fa-folder
		"symlink":   "\uf0c1", // nf-fa-link
		"empty":     "\uf016", // nf-fa-file
		"unknown":   "\uf128", // nf-fa-question
		"shell":     "\uf120", // nf-fa-terminal
		"json":      "\uf1c9", // nf-fa-file_code
		"pdf":       "\uf1c1", // nf-fa-file_pdf
	}

	unicodeFallback = "✧"
	nerdFallback    = "\uf016" // nf-fa-file_o
	unicodeError    = "✖"
	nerdError       = "\uf00d" // nf-fa-times
)

func resolveAppearance(w io.Writer, opt Options) (color bool, icons iconSet) {
	tty := isTTY(w, opt)
	noColor := os.Getenv("NO_COLOR") != ""

	switch opt.Color {
	case ColorNever:
		color = false
	case ColorAlways:
		color = !noColor
	default:
		color = tty && !noColor
	}

	switch opt.Icons {
	case IconNone:
		icons = iconsNone
	case IconUnicode:
		icons = iconsUnicode
	case IconNerd:
		icons = iconsNerd
	default:
		if tty && !noColor {
			icons = iconsUnicode
		} else {
			icons = iconsNone
		}
	}
	return color, icons
}

func isTTY(w io.Writer, opt Options) bool {
	if opt.IsTTY != nil {
		return *opt.IsTTY
	}
	return isTerminal(w)
}

// lowSources are layers that are low confidence at any score. Values outside
// this set, including ones added later, are printed unchanged.
var lowSources = map[string]struct{}{
	detect.SourceFallback: {},
	"heuristic":           {},
}

func lowConfidence(r detect.Result) bool {
	if _, ok := lowSources[r.Source]; ok {
		return true
	}
	return r.Score < lowScoreThreshold
}

func paint(enabled bool, code, s string) string {
	if !enabled || code == "" || s == "" {
		return s
	}
	return "\033[" + code + "m" + s + ansiReset
}

func iconGlyph(set iconSet, label, group string, failed bool) (glyph, sgr string) {
	if failed {
		switch set {
		case iconsNerd:
			return nerdError, sgrErr
		default:
			return unicodeError, sgrErr
		}
	}
	sgr = groupSGR[group]
	if set == iconsNerd {
		if g, ok := nerdLabelIcons[label]; ok {
			return g, sgr
		}
		if g, ok := nerdGroupIcons[group]; ok {
			return g, sgr
		}
		return nerdFallback, sgr
	}
	if g, ok := unicodeLabelIcons[label]; ok {
		return g, sgr
	}
	if g, ok := unicodeGroupIcons[group]; ok {
		return g, sgr
	}
	return unicodeFallback, sgr
}

func scoreField(score float32, low bool) string {
	if low {
		// The tilde sits in the padding cell so the digits stay aligned.
		return fmt.Sprintf("score=%.3f~", score)
	}
	return fmt.Sprintf("score=%.3f ", score)
}

func sizeField(n int64) string {
	return fmt.Sprintf("size=%-9s", humanSize(n))
}

// sanitize matches internal/termesc.Escape. Printable UTF-8, including CJK,
// is kept. C0, DEL, C1, invalid UTF-8, bidi controls, zero-width characters,
// and U+2028/U+2029 are \ooo octal escapes of their UTF-8 bytes. Measure
// width on the returned string; \033 is four columns.
func sanitize(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if (r == utf8.RuneError && size == 1) || escapeRune(r) {
			for _, c := range []byte(s[i : i+size]) {
				b.WriteByte('\\')
				b.WriteByte('0' + (c>>6)&7)
				b.WriteByte('0' + (c>>3)&7)
				b.WriteByte('0' + c&7)
			}
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

func escapeRune(r rune) bool {
	switch {
	case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
		return true
	case r >= 0x200b && r <= 0x200f:
		return true
	case r == 0x2028 || r == 0x2029 || (r >= 0x202a && r <= 0x202e):
		return true
	case r >= 0x2066 && r <= 0x2069 || r == 0xfeff:
		return true
	default:
		return false
	}
}

// fitMiddle pads s to width, or middle-truncates it with "..." so a long
// path keeps its prefix and its file name. The result is exactly width
// display cells when width > 0.
func fitMiddle(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := displayWidth(s)
	if w <= width {
		return s + strings.Repeat(" ", width-w)
	}
	if width <= 3 {
		p, used := takePrefix(s, width)
		return p + strings.Repeat(" ", width-used)
	}
	budget := width - 3
	left := budget / 2
	right := budget - left
	head, hw := takePrefix(s, left)
	tail, tw := takeSuffix(s, right)
	return head + "..." + tail + strings.Repeat(" ", width-hw-3-tw)
}

// truncEnd shortens s to width with a trailing "..." and does not pad.
func truncEnd(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidth(s) <= width {
		return s
	}
	return fitEnd(s, width)
}

// fitEnd pads s to width, or truncates it with a trailing "...".
func fitEnd(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := displayWidth(s)
	if w <= width {
		return s + strings.Repeat(" ", width-w)
	}
	if width <= 3 {
		p, used := takePrefix(s, width)
		return p + strings.Repeat(" ", width-used)
	}
	head, hw := takePrefix(s, width-3)
	return head + "..." + strings.Repeat(" ", width-hw-3)
}

func takePrefix(s string, max int) (string, int) {
	if max <= 0 {
		return "", 0
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := runeWidth(r)
		if used+w > max {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String(), used
}

func takeSuffix(s string, max int) (string, int) {
	if max <= 0 {
		return "", 0
	}
	type cell struct {
		r rune
		w int
	}
	cells := make([]cell, 0, len(s))
	for _, r := range s {
		cells = append(cells, cell{r, runeWidth(r)})
	}
	used := 0
	i := len(cells)
	for i > 0 {
		w := cells[i-1].w
		if used+w > max {
			break
		}
		used += w
		i--
	}
	var b strings.Builder
	for _, c := range cells[i:] {
		b.WriteRune(c.r)
	}
	return b.String(), used
}

// displayWidth reports the terminal column width of s.
// Wide and fullwidth runes count as 2. Combining marks and a few format
// characters count as 0. Ambiguous-width runes count as 1, which matches a
// non-CJK terminal; the unicode icons chosen for this renderer are Neutral,
// so they stay one cell in a CJK terminal too.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}

func runeWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 0x20 || (r >= 0x7f && r < 0xa0):
		return 0
	case r >= 0x200b && r <= 0x200f:
		return 0
	case r >= 0x2028 && r <= 0x202e:
		return 0
	case r >= 0x2060 && r <= 0x206f:
		return 0
	case r >= 0xfe00 && r <= 0xfe0f || r == 0xfeff:
		return 0
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
		return 0
	case unicode.Is(wideRunes, r):
		return 2
	default:
		return 1
	}
}

// wideRunes is the East Asian Width W/F set used by typical terminals,
// without the ambiguous class.
var wideRunes = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x1100, Hi: 0x115F, Stride: 1},
		{Lo: 0x2329, Hi: 0x232A, Stride: 1},
		{Lo: 0x2E80, Hi: 0x303E, Stride: 1},
		{Lo: 0x3040, Hi: 0xA4CF, Stride: 1},
		{Lo: 0xAC00, Hi: 0xD7A3, Stride: 1},
		{Lo: 0xF900, Hi: 0xFAFF, Stride: 1},
		{Lo: 0xFE10, Hi: 0xFE19, Stride: 1},
		{Lo: 0xFE30, Hi: 0xFE6F, Stride: 1},
		{Lo: 0xFF00, Hi: 0xFF60, Stride: 1},
		{Lo: 0xFFE0, Hi: 0xFFE6, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0x1F300, Hi: 0x1F64F, Stride: 1},
		{Lo: 0x1F900, Hi: 0x1F9FF, Stride: 1},
		{Lo: 0x1FA00, Hi: 0x1FAFF, Stride: 1},
		{Lo: 0x20000, Hi: 0x3FFFD, Stride: 1},
	},
}
