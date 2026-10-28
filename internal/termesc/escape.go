// Package termesc escapes text for a terminal.
//
// Escape keeps every rune that escapeRune in the plain renderer keeps,
// including CJK and spaces such as U+00A0 and U+3000. It writes \ooo for
// each UTF-8 byte of C0 controls, DEL, C1 controls, U+200B–U+200F,
// U+2028–U+202E, U+2066–U+2069, U+FEFF, and for every byte of invalid
// UTF-8. Callers add the record separator; this function does not.
package termesc

import (
	"strings"
	"unicode/utf8"
)

// Escape returns s with the same runes escaped as render.escapeRune.
// An escaped rune is written byte-by-byte as \ooo of its UTF-8 encoding.
func Escape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
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

// escapeRune is the same set as internal/render.escapeRune on PR #6
// (554fd7d): C0, DEL, C1, zero-width U+200B–U+200F, U+2028/U+2029,
// bidi U+202A–U+202E, U+2066–U+2069, and U+FEFF.
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
