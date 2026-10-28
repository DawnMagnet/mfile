package termesc

import (
	"strings"
	"testing"
)

func TestEscape(t *testing.T) {
	// Same cases, in the same order, as TestTerminalEscapes in
	// internal/render on PR #6 (554fd7d).
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

		{name: "empty", in: "", want: ""},
		{name: "printable ascii", in: "a b/c.txt", want: "a b/c.txt"},
		{name: "backslash stays", in: `a\b`, want: `a\b`},
		{name: "CR", in: "\r", want: `\015`},
		{name: "TAB", in: "\t", want: `\011`},
		{name: "DEL", in: "\x7f", want: `\177`},
		{name: "ESC BEL LF name", in: "\x1b]0;x\x07\n", want: `\033]0;x\007\012`},
		{name: "invalid bytes mid string", in: "a\xff\xfeb", want: `a\377\376b`},
		{name: "lone continuation", in: "\x80", want: `\200`},
		{name: "C1 U+0080", in: "\u0080", want: `\302\200`},
		{name: "C1 U+009F", in: "\u009f", want: `\302\237`},
		{name: "U+202E inside text", in: "a\u202eb", want: `a\342\200\256b`},
		{name: "U+2029", in: "\u2029", want: `\342\200\251`},
		{name: "U+200B zero width space", in: "a\u200bb", want: `a\342\200\213b`},
		{name: "U+200F", in: "\u200f", want: `\342\200\217`},
		{name: "U+202A", in: "\u202a", want: `\342\200\252`},
		{name: "U+2066", in: "\u2066", want: `\342\201\246`},
		{name: "U+2069", in: "\u2069", want: `\342\201\251`},
		{name: "U+FEFF", in: "\ufeff", want: `\357\273\277`},
		{name: "U+00A0 not escaped", in: "a\u00a0b", want: "a\u00a0b"},
		{name: "U+3000 not escaped", in: "a\u3000b", want: "a\u3000b"},
		{name: "chinese around control", in: "测\x1b试", want: "测" + `\033` + "试"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Escape(tc.in)
			if got != tc.want {
				t.Fatalf("Escape(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.IndexByte(got, 0x1b) >= 0 || strings.IndexByte(got, 0x07) >= 0 {
				t.Fatalf("raw control in %q", got)
			}
		})
	}
}
