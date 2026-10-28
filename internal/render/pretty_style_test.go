package render

import (
	"strings"
	"testing"
)

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"go.mod", 6},
		{"照片.png", 8},
		{"cafe\u0301.txt", 8},
		{"Ａ", 2},
		{"A", 1},
		{"한", 2},
		{"\u3000", 2},
		{"😀", 2},
		{"\u200b", 0},
		{"⌘✎✑✿✦♫✉⚙✒☰⎈✶◊☀⚗⍟⌂◌✧✖✱⏎⎋", 23},
	}
	for _, tt := range tests {
		if got := displayWidth(tt.in); got != tt.want {
			t.Errorf("displayWidth(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
	for _, r := range "⌘✎✑✿✦♫✉⚙✒☰⎈✶◊☀⚗⍟⌂◌✧✖✱⏎⎋?" {
		if got := runeWidth(r); got != 1 {
			t.Errorf("icon %q width %d, want 1", string(r), got)
		}
	}
}

func TestFitPreservesWidth(t *testing.T) {
	samples := []string{
		"",
		"go.mod",
		"照片.png",
		"cafe\u0301.txt",
		strings.Repeat("a", 50) + "/readme.md",
		strings.Repeat("照", 30) + ".png",
		strings.Repeat("Ａ", 10),
		"x",
	}
	for _, s := range samples {
		for _, width := range []int{1, 2, 3, 4, 8, 28, 40} {
			got := fitMiddle(s, width)
			if w := displayWidth(got); w != width {
				t.Errorf("fitMiddle(%q, %d) width %d (%q)", s, width, w, got)
			}
			got = fitEnd(s, width)
			if w := displayWidth(got); w != width {
				t.Errorf("fitEnd(%q, %d) width %d (%q)", s, width, w, got)
			}
		}
	}
	long := strings.Repeat("dir/", 10) + "readme.md"
	got := fitMiddle(long, 28)
	if !strings.Contains(got, "...") || !strings.Contains(got, "readme.md") {
		t.Errorf("middle truncate dropped the name: %q", got)
	}
	// A wide rune must not be split, and "..." must not overlap the ends.
	wide := strings.Repeat("照", 10)
	got = fitMiddle(wide, 9)
	if strings.Contains(got, "\uFFFD") || displayWidth(got) != 9 {
		t.Errorf("wide fit: %q width %d", got, displayWidth(got))
	}
}

func TestFitLayout(t *testing.T) {
	full := fullLayout().lineWidth(false)
	if full != 170 {
		t.Fatalf("full layout width %d, want 170", full)
	}
	if got := fullLayout().lineWidth(true); got != full+iconCols {
		t.Fatalf("icon layout width %d, want %d", got, full+iconCols)
	}
	tests := []struct {
		width                     int
		icons                     bool
		path                      int
		size, source, label, mime bool
	}{
		{width: 200, path: 28, size: true, source: true, label: true, mime: true},
		{width: 170, path: 28, size: true, source: true, label: true, mime: true},
		{width: 169, path: 27, size: true, source: true, label: true, mime: true},
		{width: 120, path: 19, size: true, source: true},
		{width: 100, path: 16, size: true},
		{width: 80, path: 16},
		{width: 120, icons: true, path: 19, size: true, source: true},
		{width: 100, icons: true, path: 16, size: true},
		{width: 80, icons: true, path: 15},
	}
	for _, tt := range tests {
		got := fitLayout(tt.width, tt.icons)
		if got.pathW != tt.path || got.showSize != tt.size || got.showSource != tt.source || got.showLabel != tt.label || got.showMIME != tt.mime {
			t.Errorf("width %d icons %v: path %d size %v source %v label %v mime %v, want path %d size %v source %v label %v mime %v",
				tt.width, tt.icons, got.pathW, got.showSize, got.showSource, got.showLabel, got.showMIME,
				tt.path, tt.size, tt.source, tt.label, tt.mime)
		}
		if w := got.lineWidth(tt.icons); w > tt.width {
			t.Errorf("width %d icons %v: layout is %d cells", tt.width, tt.icons, w)
		}
	}
}

func TestTerminalEscapes(t *testing.T) {
	// Replace sanitize with termesc.Escape when that package lands.
	escape := sanitize
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "ESC", in: "\x1b", want: `\033`},
		{name: "BEL", in: "\x07", want: `\007`},
		{name: "newline", in: "\n", want: `\012`},
		{name: "invalid byte", in: "\xff", want: `\377`},
		{name: "U+202E", in: "\u202e", want: `\342\200\256`},
		{name: "U+2028", in: "\u2028", want: `\342\200\250`},
		{name: "Chinese filename", in: "照片.png", want: "照片.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escape(tt.in)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if strings.IndexByte(got, 0x1b) >= 0 || strings.IndexByte(got, 0x07) >= 0 {
				t.Fatalf("raw control in %q", got)
			}
		})
	}
	if w := displayWidth(escape("\x1b")); w != 4 {
		t.Fatalf(`display width of \033 = %d, want 4`, w)
	}
}

func TestSizeUnits(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0B"},
		{555, "555B"},
		{1023, "1023B"},
		{1024, "1.0KiB"},
		{1536, "1.5KiB"},
		{1 << 20, "1.0MiB"},
		{1536 << 10, "1.5MiB"},
	}
	for _, tt := range tests {
		if got := humanSize(tt.n); got != tt.want {
			t.Errorf("humanSize(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestScoreFieldWidth(t *testing.T) {
	plain := scoreField(0.745, false)
	low := scoreField(0.25, true)
	if plain != "score=0.745 " {
		t.Errorf("plain score %q", plain)
	}
	if low != "score=0.250~" {
		t.Errorf("low score %q", low)
	}
	if displayWidth(plain) != scoreWidth || displayWidth(low) != scoreWidth {
		t.Errorf("score fields differ from budget %d: %q %q", scoreWidth, plain, low)
	}
	if got := sizeField(555); got != "size=555B     " || displayWidth(got) != sizeWidth {
		t.Errorf("size field %q width %d, budget %d", got, displayWidth(got), sizeWidth)
	}
}
