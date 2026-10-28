package render

import (
	"bytes"
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/DawnMagnet/mfile/internal/detect"
)

func TestPrettyColumns(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	rows := []detect.Result{
		rowGoMod(),
		rowLow(),
		{
			Index: 2, Path: strings.Repeat("dir/", 12) + "readme.md",
			Label: "txt", Description: "ASCII text", MIME: "text/plain",
			Group: "text", Score: 0.9, Source: "model", Size: 4096, Classified: true,
		},
		{Index: 3, Path: "missing.bin", Error: strings.Repeat("e", 90)},
	}
	full := renderAll(t, Options{Color: ColorNever, Icons: IconNone}, rows)
	// A non-TTY keeps the full layout even if a width is supplied.
	piped := renderAll(t, Options{Color: ColorNever, Icons: IconNone, IsTTY: tty(false), Columns: 80}, rows)
	if piped != full || full != goldenColumnsFull {
		t.Fatalf("non-TTY layout:\n got %q\nwant %q", piped, goldenColumnsFull)
	}

	tests := []struct {
		name  string
		width int
		want  string
	}{
		{name: "80", width: 80, want: goldenColumns80},
		{name: "100", width: 100, want: goldenColumns100},
		{name: "120", width: 120, want: goldenColumns120},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := Options{Color: ColorNever, Icons: IconNone, IsTTY: tty(true), Columns: tt.width}
			got := renderAll(t, opt, rows)
			if got != tt.want {
				t.Errorf("width %d:\n got %q\nwant %q", tt.width, got, tt.want)
			}
			errText := strings.Repeat("e", 90)
			for i, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
				if strings.Contains(line, errText) {
					continue
				}
				if w := displayWidth(line); w > tt.width {
					t.Errorf("line %d width %d > %d: %q", i, w, tt.width, line)
				}
			}
			if !strings.Contains(got, "low") || strings.Count(got, errText) != 1 || strings.Contains(got, "error:") {
				t.Errorf("error text not verbatim once:\n%s", got)
			}
		})
	}
	unicode := renderAll(t, Options{Color: ColorAlways, Icons: IconUnicode, IsTTY: tty(true), Columns: 80}, rows)
	plain := stripANSI(unicode)
	errText := strings.Repeat("e", 90)
	if !strings.Contains(plain, "⌘") || !strings.Contains(plain, "low") || strings.Count(plain, errText) != 1 {
		t.Fatalf("unicode 80 dropped a required field:\n%s", plain)
	}
	if !strings.Contains(plain, "✖") {
		t.Fatalf("unicode 80 missing error icon:\n%s", plain)
	}
	for i, line := range strings.Split(strings.TrimSuffix(plain, "\n"), "\n") {
		if strings.Contains(line, errText) {
			continue
		}
		if w := displayWidth(line); w > 80 {
			t.Errorf("unicode line %d width %d: %q", i, w, line)
		}
	}
}

func TestPrettyGolden(t *testing.T) {
	// Ambient NO_COLOR must not leak into these cases.
	t.Setenv("NO_COLOR", "")

	off := Options{Color: ColorNever, Icons: IconNone}
	on := Options{Color: ColorAlways, Icons: IconNone}
	rows := []detect.Result{rowGoMod(), rowLow(), rowError()}

	gotOff := renderAll(t, off, rows)
	gotOn := renderAll(t, on, rows)

	if gotOff != goldenOff {
		t.Errorf("color off:\n got: %q\nwant: %q", gotOff, goldenOff)
	}
	if strings.Contains(gotOff, "\x1b") {
		t.Errorf("color off contains an escape: %q", gotOff)
	}
	if gotOn != goldenOn {
		t.Errorf("color on:\n got: %q\nwant: %q", gotOn, goldenOn)
	}
	if stripANSI(gotOn) != gotOff {
		t.Errorf("color on stripped:\n got: %q\nwant: %q", stripANSI(gotOn), gotOff)
	}
	if strings.Contains(gotOn, "\x1b[38") || strings.Contains(gotOn, "\x1b[48") {
		t.Errorf("color on uses extended color: %q", gotOn)
	}

	// Icon glyphs take the group color, and errors stay red, with color on.
	unicode := renderAll(t, Options{Color: ColorAlways, Icons: IconUnicode}, rows)
	if !strings.Contains(unicode, "\x1b[36m⌘\x1b[0m") {
		t.Errorf("code icon not cyan: %q", unicode)
	}
	if !strings.Contains(unicode, "\x1b[32m✎\x1b[0m") {
		t.Errorf("text icon not green: %q", unicode)
	}
	if !strings.Contains(unicode, "\x1b[1;31m✖\x1b[0m") {
		t.Errorf("error icon not red: %q", unicode)
	}
	if stripANSI(unicode) == gotOff {
		t.Fatal("unicode icons stripped to the no-icon golden")
	}
}

func TestPrettyIcons(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	row := rowGoMod()
	low := rowLow()
	bad := rowError()

	tests := []struct {
		name    string
		icons   IconMode
		wantHas []string
		wantNot []string
	}{
		{
			name:    "none",
			icons:   IconNone,
			wantNot: []string{"⌘", "✖", "\uf121", "\uf00d"},
		},
		{
			name:    "unicode",
			icons:   IconUnicode,
			wantHas: []string{"⌘", "✎", "✖", "✱"},
			wantNot: []string{"\uf121", "\uf00d"},
		},
		{
			name:    "nerd",
			icons:   IconNerd,
			wantHas: []string{"\uf121", "\uf0f6", "\uf00d", "\uf1c9"},
			wantNot: []string{"⌘", "✖"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := []string{row.Path, low.Path, rowJSON().Path, bad.Path}
			got := renderAll(t, Options{Color: ColorNever, Icons: tt.icons}, []detect.Result{row, low, rowJSON(), bad})
			if strings.Contains(got, "\x1b") {
				t.Errorf("icons with color off leaked an escape: %q", got)
			}
			for _, s := range tt.wantHas {
				if !strings.Contains(got, s) {
					t.Errorf("missing %q in:\n%s", s, got)
				}
			}
			for _, s := range tt.wantNot {
				if strings.Contains(got, s) {
					t.Errorf("unexpected %q in:\n%s", s, got)
				}
			}
			// Icon column is one cell plus a space, so paths still share a column.
			if tt.icons != IconNone {
				lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
				var col int
				for i, line := range lines {
					w := leadingWidth(line)
					// glyph + space
					if w < 2 {
						t.Fatalf("line %d icon column too narrow: %q", i, line)
					}
					pathCol := widthBefore(line, paths[i])
					if pathCol < 0 {
						t.Fatalf("line %d missing path %q: %q", i, paths[i], line)
					}
					if i == 0 {
						col = pathCol
					} else if pathCol != col {
						t.Errorf("path column %d, want %d on %q", pathCol, col, line)
					}
				}
			}
		})
	}
}

func TestPrettyNoColorAndTTY(t *testing.T) {
	rows := []detect.Result{rowGoMod()}
	tests := []struct {
		name      string
		noColor   string
		opt       Options
		wantColor bool
		wantIcon  bool
	}{
		{
			name:      "auto tty",
			opt:       Options{IsTTY: tty(true)},
			wantColor: true,
			wantIcon:  true,
		},
		{
			name:      "auto pipe",
			opt:       Options{IsTTY: tty(false)},
			wantColor: false,
			wantIcon:  false,
		},
		{
			name:      "NO_COLOR=1 on a tty",
			noColor:   "1",
			opt:       Options{IsTTY: tty(true)},
			wantColor: false,
			wantIcon:  false,
		},
		{
			name:      "NO_COLOR=0 is still set",
			noColor:   "0",
			opt:       Options{Color: ColorAlways, IsTTY: tty(true)},
			wantColor: false,
			wantIcon:  false,
		},
		{
			name:      "empty NO_COLOR does not disable",
			noColor:   "",
			opt:       Options{IsTTY: tty(true)},
			wantColor: true,
			wantIcon:  true,
		},
		{
			name:      "explicit unicode icons survive NO_COLOR",
			noColor:   "1",
			opt:       Options{Color: ColorAlways, Icons: IconUnicode, IsTTY: tty(true)},
			wantColor: false,
			wantIcon:  true,
		},
		{
			name:      "explicit color never",
			opt:       Options{Color: ColorNever, Icons: IconUnicode, IsTTY: tty(true)},
			wantColor: false,
			wantIcon:  true,
		},
		{
			name:      "explicit color always off a tty",
			opt:       Options{Color: ColorAlways, Icons: IconNone, IsTTY: tty(false)},
			wantColor: true,
			wantIcon:  false,
		},
		{
			name:      "explicit nerd icons off a tty",
			opt:       Options{Color: ColorNever, Icons: IconNerd, IsTTY: tty(false)},
			wantColor: false,
			wantIcon:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColor)
			got := renderAll(t, tt.opt, rows)
			hasColor := strings.Contains(got, "\x1b")
			if hasColor != tt.wantColor {
				t.Errorf("color=%v, want %v: %q", hasColor, tt.wantColor, got)
			}
			hasIcon := strings.Contains(got, "⌘")
			if tt.opt.Icons == IconNerd {
				hasIcon = strings.Contains(got, "\uf121")
			}
			if hasIcon != tt.wantIcon {
				t.Errorf("icon=%v, want %v: %q", hasIcon, tt.wantIcon, got)
			}
		})
	}
}

func TestPrettyWidth(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	long := strings.Repeat("dir/", 20) + "readme.md"
	cjk := strings.Repeat("照", 20) + ".png"
	rows := []detect.Result{
		{Index: 0, Path: "go.mod", Description: "Verilog source", Label: "verilog", MIME: "text/x-verilog", Group: "code", Score: 0.745, Source: "model", Size: 555, Classified: true},
		{Index: 1, Path: "照片.png", Description: "JPEG image", Label: "jpeg", MIME: "image/jpeg", Group: "image", Score: 1, Source: "magic", Size: 2048, Classified: true},
		{Index: 2, Path: "cafe\u0301.txt", Description: "ASCII text", Label: "txt", MIME: "text/plain", Group: "text", Score: 1, Source: "magic", Size: 3, Classified: true},
		{Index: 3, Path: long, Description: strings.Repeat("长", 20), Label: "txt", MIME: strings.Repeat("m", 50), Group: "text", Score: 0.9, Source: "model", Size: 9, Classified: true},
		{Index: 4, Path: cjk, Description: "PNG image", Label: "png", MIME: "image/png", Group: "image", Score: 0.9, Source: "model", Size: 9, Classified: true},
		{Index: 5, Path: "a\nb\x1b[31m", Description: "line\nbreak", Label: "x", MIME: "text/plain", Group: "text", Score: 1, Source: "name", Size: 1, Classified: true},
	}
	got := renderAll(t, Options{Color: ColorNever, Icons: IconNone}, rows)
	if strings.Contains(got, "\x1b") {
		t.Fatalf("escape leaked into a sanitized line:\n%s", got)
	}
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != len(rows) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(rows), got)
	}
	for i, line := range lines {
		if got := displayWidth(takeFixed(line, pathWidth)); got != pathWidth {
			t.Errorf("line %d path width %d, want %d: %q", i, got, pathWidth, line)
		}
		rest := dropFixed(line, pathWidth)
		if !strings.HasPrefix(rest, "  ") {
			t.Errorf("line %d missing separator after path: %q", i, line)
		}
		desc := dropFixed(rest, 2)
		if got := displayWidth(takeFixed(desc, descWidth)); got != descWidth {
			t.Errorf("line %d description width %d, want %d: %q", i, got, descWidth, line)
		}
	}
	if !strings.Contains(lines[1], "照片.png") {
		t.Errorf("cjk path was altered: %q", lines[1])
	}
	if !strings.Contains(lines[2], "cafe\u0301.txt") {
		t.Errorf("combining mark was altered: %q", lines[2])
	}
	if !strings.Contains(lines[3], "...") || !strings.Contains(lines[3], "readme.md") {
		t.Errorf("long path should keep the file name: %q", lines[3])
	}
	if !strings.Contains(lines[4], "...") || !strings.Contains(lines[4], ".png") {
		t.Errorf("long cjk path should keep the suffix: %q", lines[4])
	}
	if strings.Contains(lines[4], "照照照") && displayWidth(takeFixed(lines[4], pathWidth)) != pathWidth {
		t.Errorf("cjk truncation split a column: %q", lines[4])
	}
	if strings.Contains(lines[5], "\n") || strings.Contains(got, "\x1b[31m") {
		t.Errorf("control characters were not sanitized: %q", lines[5])
	}
	if !strings.Contains(lines[5], "a?b?[31m") {
		t.Errorf("sanitized path: %q", lines[5])
	}
	// A wide description is truncated on a rune boundary and still fills the column.
	if !strings.Contains(lines[3], "...") {
		t.Errorf("wide description was not truncated: %q", lines[3])
	}
}

func TestPrettyErrorSanitize(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	const inj = "\x1b]0;x\x07"
	row := detect.Result{
		Index: 0,
		Path:  "bad" + inj + ".txt",
		Error: "lstat bad" + inj + ".txt: no such file",
	}
	off := renderAll(t, Options{Color: ColorNever, Icons: IconNone}, []detect.Result{row})
	if strings.IndexByte(off, 0x1b) >= 0 || strings.IndexByte(off, 0x07) >= 0 {
		t.Fatalf("color-off leaked controls: %q", off)
	}
	if !strings.Contains(off, "lstat bad?]0;x?.txt: no such file") {
		t.Fatalf("sanitized error text: %q", off)
	}
	on := renderAll(t, Options{Color: ColorAlways, Icons: IconNone, IsTTY: tty(true), Columns: 80}, []detect.Result{row})
	if strings.Contains(on, inj) || strings.IndexByte(on, 0x07) >= 0 {
		t.Fatalf("color-on kept the OSC sequence: %q", on)
	}
	for i := 0; i < len(on); i++ {
		if on[i] != 0x1b {
			continue
		}
		if i+1 >= len(on) || on[i+1] != '[' {
			t.Fatalf("non-SGR escape at %d in %q", i, on)
		}
	}
}

func TestVirtualTerminalFailureDisablesColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	orig := enableVirtualTerminal
	t.Cleanup(func() { enableVirtualTerminal = orig })
	enableVirtualTerminal = func(io.Writer) bool { return false }

	got := renderAll(t, Options{Color: ColorAlways, Icons: IconUnicode, IsTTY: tty(true), Columns: 80}, []detect.Result{rowGoMod()})
	if strings.IndexByte(got, 0x1b) >= 0 {
		t.Fatalf("color stayed on after VT enable failed: %q", got)
	}
	if !strings.Contains(got, "⌘") {
		t.Fatalf("icons were cleared with color: %q", got)
	}
}

func TestPrettyErrors(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	const errText = "cannot open 'missing.bin' (No such file or directory)"
	row := detect.Result{Index: 0, Path: "missing.bin", Error: errText}
	ok := rowGoMod()
	ok.Index = 1

	off := renderAll(t, Options{Color: ColorNever, Icons: IconNone}, []detect.Result{row, ok})
	if strings.Count(off, errText) != 1 {
		t.Fatalf("non-TTY error text count: %q", off)
	}
	if strings.Contains(off, "\x1b") || strings.Contains(off, "error: "+errText) {
		t.Fatalf("non-TTY rewrote the error row: %q", off)
	}
	if strings.Count(off, "\n") != 2 {
		t.Fatalf("error and ok row: %q", off)
	}
	if strings.Contains(strings.Split(off, "\n")[0], "score=") {
		t.Fatalf("error row should not look like a detection: %q", off)
	}

	on := renderAll(t, Options{Color: ColorAlways, Icons: IconUnicode, IsTTY: tty(true), Columns: 80}, []detect.Result{row})
	if strings.Count(stripANSI(on), errText) != 1 {
		t.Fatalf("TTY error text count: %q", on)
	}
	if !strings.Contains(on, "\x1b[1;31m"+errText+"\x1b[0m") {
		t.Fatalf("TTY error text is not the verbatim red string: %q", on)
	}
	if !strings.Contains(on, "✖") {
		t.Fatalf("unicode error icon missing: %q", on)
	}
	nerd := renderAll(t, Options{Color: ColorNever, Icons: IconNerd}, []detect.Result{row})
	if strings.Count(nerd, errText) != 1 || !strings.Contains(nerd, "\uf00d ") {
		t.Fatalf("nerd error row: %q", nerd)
	}
	none := renderAll(t, Options{Color: ColorNever, Icons: IconNone, IsTTY: tty(true), Columns: 80}, []detect.Result{row})
	if none != goldenError80 {
		t.Fatalf("80-col error:\n got %q\nwant %q", none, goldenError80)
	}
	if strings.Contains(none, "✖") || strings.Contains(none, "\uf00d") {
		t.Fatalf("icons=none printed an error icon: %q", none)
	}

	// Path columns of the error row and the following row match.
	lines := strings.Split(strings.TrimSuffix(stripANSI(on), "\n"), "\n")
	okLine := renderAll(t, Options{Color: ColorNever, Icons: IconUnicode, IsTTY: tty(true), Columns: 80}, []detect.Result{ok})
	if widthBefore(lines[0], "missing.bin") != widthBefore(strings.TrimSuffix(okLine, "\n"), "go.mod") {
		t.Fatalf("error path column drifted:\n%s\n%s", lines[0], okLine)
	}
}

func TestPrettyStreamsInCallOrder(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	a := rowGoMod()
	b := rowLow()
	c := rowJSON()
	opt := Options{Color: ColorNever, Icons: IconNone}

	var buf bytes.Buffer
	p := NewPretty(&buf, opt)
	if err := p.Begin(); err != nil {
		t.Fatal(err)
	}
	if err := p.Write(c); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "\n") != 1 || !strings.Contains(buf.String(), "data.json") {
		t.Fatalf("Write did not stream: %q", buf.String())
	}
	if err := p.Write(a); err != nil {
		t.Fatal(err)
	}
	if err := p.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := p.End(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	want := renderAll(t, opt, []detect.Result{c, a, b})
	if got != want {
		t.Errorf("call order:\n got: %q\nwant: %q", got, want)
	}
	sorted := renderAll(t, opt, []detect.Result{a, b, c})
	if got == sorted {
		t.Fatal("Write reordered results by Index")
	}

	if err := p.Write(a); err == nil {
		t.Fatal("Write after End succeeded")
	}
	if err := p.End(); err != nil {
		t.Fatal(err)
	}
	if err := NewPretty(&buf, opt).End(); err == nil {
		t.Fatal("End before Begin succeeded")
	}
}

func TestPrettyConcurrentWrite(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	const n = 32
	rows := make([]detect.Result, n)
	for i := range rows {
		rows[i] = detect.Result{
			Index: i, Path: "f", Description: "text", Label: "txt",
			MIME: "text/plain", Group: "text", Score: 1,
			Source: "model", Size: int64(i), Classified: true,
		}
		rows[i].Path = "f" + strconv.Itoa(i)
	}
	var buf bytes.Buffer
	p := NewPretty(&buf, Options{Color: ColorNever, Icons: IconNone})
	if err := p.Begin(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := n - 1; i >= 0; i-- {
		wg.Add(1)
		go func(r detect.Result) {
			defer wg.Done()
			if err := p.Write(r); err != nil {
				t.Errorf("write %d: %v", r.Index, err)
			}
		}(rows[i])
	}
	wg.Wait()
	if err := p.End(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("got %d lines, want %d", len(lines), n)
	}
	for _, r := range rows {
		if !strings.Contains(got, r.Path) {
			t.Errorf("missing %q", r.Path)
		}
	}
}

func TestPrettyWriterError(t *testing.T) {
	p := NewPretty(failWriter{}, Options{Color: ColorNever, Icons: IconNone})
	if err := p.Begin(); err != nil {
		t.Fatal(err)
	}
	err := p.Write(rowGoMod())
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("Write err: %v", err)
	}
	if err := p.End(); err != nil {
		t.Fatal(err)
	}
	if err := NewPretty(nil, Options{}).Begin(); err == nil {
		t.Fatal("nil writer Begin succeeded")
	}
}

func TestPipeIsNotTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		r.Close()
		w.Close()
	})
	if isTerminal(w) {
		t.Fatal("pipe writer reported as a terminal")
	}
	if isTerminal(&bytes.Buffer{}) {
		t.Fatal("bytes.Buffer reported as a terminal")
	}
}

func TestLowConfidenceThreshold(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	base := rowGoMod()
	base.Score = 0.5
	base.Source = "model"
	if lowConfidence(base) {
		t.Fatal("score 0.5 should not be low")
	}
	base.Score = 0.49
	if !lowConfidence(base) {
		t.Fatal("score 0.49 should be low")
	}
	base.Score = 0.99
	base.Source = detect.SourceFallback
	if !lowConfidence(base) {
		t.Fatal("fallback should be low at any score")
	}
	base.Source = "heuristic"
	if !lowConfidence(base) {
		t.Fatal("heuristic should be low at any score")
	}
	got := renderAll(t, Options{Color: ColorNever, Icons: IconNone}, []detect.Result{base})
	if !strings.Contains(got, "heuristic") || !strings.Contains(got, "score=0.990~") || !strings.Contains(got, " low") {
		t.Fatalf("high-score heuristic row: %q", got)
	}
	base.Source = detect.SourceRule
	if lowConfidence(base) {
		t.Fatal("rule with a high score should not be low")
	}
	base.Source = "special"
	if lowConfidence(base) {
		t.Fatal("unknown source with a high score should not be low")
	}
}

func TestPrettyUnclassified(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	rows := []detect.Result{
		{Index: 0, Path: "subdir", Kind: "directory"},
		{Index: 1, Path: "link", Kind: "symlink", Target: "hello.txt"},
		{Index: 2, Path: "pipe", Kind: "fifo"},
		{Index: 3, Path: "sock", Kind: "socket"},
		{Index: 4, Path: "tty", Kind: "char device"},
		{Index: 5, Path: "disk", Kind: "block device"},
		{Index: 6, Path: "odd", Kind: "other"},
	}
	got := renderAll(t, Options{Color: ColorNever, Icons: IconNone}, rows)
	for _, want := range []string{
		"directory",
		"symbolic link to hello.txt",
		"fifo",
		"socket",
		"char device",
		"block device",
		"other",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "score=") || strings.Contains(got, " low") {
		t.Fatalf("unclassified row looked classified:\n%s", got)
	}
}

func TestUnknownSource(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	r := rowGoMod()
	r.Source = "special"
	r.Score = 0.91
	got := renderAll(t, Options{Color: ColorNever, Icons: IconNone}, []detect.Result{r})
	if strings.Contains(got, " low") || strings.Contains(got, "~") {
		t.Fatalf("unknown source marked low: %q", got)
	}
	if !strings.Contains(got, "special") {
		t.Fatalf("unknown source not printed: %q", got)
	}
	r.Source = "plugin"
	got = renderAll(t, Options{Color: ColorNever, Icons: IconNone}, []detect.Result{r})
	if !strings.Contains(got, "plugin") || strings.Contains(got, " low") {
		t.Fatalf("arbitrary source: %q", got)
	}
}

func rowGoMod() detect.Result {
	return detect.Result{
		Index: 0, Path: "go.mod", Label: "verilog", Description: "Verilog source",
		MIME: "text/x-verilog", Group: "code", Score: 0.745,
		Source: "model", Size: 555, Classified: true,
	}
}

func rowLow() detect.Result {
	return detect.Result{
		Index: 1, Path: "notes.txt", Label: "txt", Description: "ASCII text",
		MIME: "text/plain", Group: "text", Score: 0.25,
		Source: detect.SourceFallback, Size: 12, Classified: true,
	}
}

func rowJSON() detect.Result {
	return detect.Result{
		Index: 2, Path: "data.json", Label: "json", Description: "JSON document",
		MIME: "application/json", Group: "code", Score: 0.91,
		Source: "model", Size: 128, Classified: true,
	}
}

func rowError() detect.Result {
	return detect.Result{Index: 3, Path: "missing.bin", Error: "no such file"}
}

func renderAll(t *testing.T, opt Options, rows []detect.Result) string {
	t.Helper()
	var buf bytes.Buffer
	p := NewPretty(&buf, opt)
	if err := p.Begin(); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if err := p.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.End(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func tty(v bool) *bool { return &v }

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

func takeFixed(s string, width int) string {
	p, _ := takePrefix(s, width)
	return p
}

func dropFixed(s string, width int) string {
	p, _ := takePrefix(s, width)
	return strings.TrimPrefix(s, p)
}

func leadingWidth(s string) int { return displayWidth(takeFixed(s, 2)) }

// widthBefore reports the display width of s before the first match.
func widthBefore(s, needle string) int {
	i := strings.Index(s, needle)
	if i < 0 {
		return -1
	}
	return displayWidth(s[:i])
}

// goldenError80 is one icons=none error row at 80 columns. The error text is verbatim.
const goldenError80 = "missing.bin       cannot open 'missing.bin' (No such file or directory)\n"

// goldenOff is the color-off rendering of rowGoMod, rowLow, and rowError.
const goldenColumnsFull = "go.mod                        Verilog source                code          score=0.745   size=555B       model      verilog           text/x-verilog\n" +
	"notes.txt                     ASCII text                    text          score=0.250~  size=12B        fallback   txt               text/plain  low\n" +
	"dir/dir/dir/...dir/readme.md  ASCII text                    text          score=0.900   size=4.0KiB     model      txt               text/plain\n" +
	"missing.bin                   eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\n"

const goldenColumns80 = "go.mod            Verilog source                code          score=0.745 \n" +
	"notes.txt         ASCII text                    text          score=0.250~  low\n" +
	"dir/di...adme.md  ASCII text                    text          score=0.900 \n" +
	"missing.bin       eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\n"

const goldenColumns100 = "go.mod            Verilog source                code          score=0.745   size=555B     \n" +
	"notes.txt         ASCII text                    text          score=0.250~  size=12B        low\n" +
	"dir/di...adme.md  ASCII text                    text          score=0.900   size=4.0KiB   \n" +
	"missing.bin       eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\n"

const goldenColumns120 = "go.mod               Verilog source                code          score=0.745   size=555B       model    \n" +
	"notes.txt            ASCII text                    text          score=0.250~  size=12B        fallback   low\n" +
	"dir/dir/...eadme.md  ASCII text                    text          score=0.900   size=4.0KiB     model    \n" +
	"missing.bin          eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\n"

const goldenOff = "go.mod                        Verilog source                code          score=0.745   size=555B       model      verilog           text/x-verilog\n" +
	"notes.txt                     ASCII text                    text          score=0.250~  size=12B        fallback   txt               text/plain  low\n" +
	"missing.bin                   no such file\n"

// goldenOn is the same rows with ColorAlways and IconsNone.
const goldenOn = "\x1b[1mgo.mod                      \x1b[0m  \x1b[36mVerilog source              \x1b[0m  \x1b[36mcode        \x1b[0m  \x1b[2mscore=0.745 \x1b[0m  \x1b[2msize=555B     \x1b[0m  \x1b[2mmodel    \x1b[0m  \x1b[2mverilog         \x1b[0m  \x1b[2mtext/x-verilog\x1b[0m\n" +
	"\x1b[1mnotes.txt                   \x1b[0m  \x1b[32mASCII text                  \x1b[0m  \x1b[32mtext        \x1b[0m  \x1b[2mscore=0.250~\x1b[0m  \x1b[2msize=12B      \x1b[0m  \x1b[2mfallback \x1b[0m  \x1b[2mtxt             \x1b[0m  \x1b[2mtext/plain\x1b[0m  \x1b[1;4mlow\x1b[0m\n" +
	"\x1b[1;31mmissing.bin                 \x1b[0m  \x1b[1;31mno such file\x1b[0m\n"
