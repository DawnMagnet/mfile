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

func TestHumanSize(t *testing.T) {
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
	if displayWidth(plain) != displayWidth(low) {
		t.Errorf("score fields differ: %q %q", plain, low)
	}
	if sizeField(555) != "size=555B     " {
		t.Errorf("size field %q", sizeField(555))
	}
}
