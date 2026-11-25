package gui

import "unicode/utf8"

// glyphText returns what GDI should draw for a cell's text. GDI has no
// emoji shaping, so the parts of a cluster that only select a
// presentation or join emoji would show up as boxes: variation selectors,
// skin tone modifiers, tag characters and the keycap mark are dropped, and
// a ZWJ sequence is cut at its first joiner (its first emoji stands for
// it). Combining marks (accents, voiced marks) are kept.
func glyphText(s string) string {
	if len(s) == 1 {
		return s
	}
	r0, n := utf8.DecodeRuneInString(s)
	if n == len(s) {
		return s
	}
	out := make([]byte, 0, len(s))
	out = utf8.AppendRune(out, r0)
	for _, r := range s[n:] {
		switch {
		case r == 0x200D: // ZWJ: the rest is joined into the first emoji
			return string(out)
		case r >= 0xFE00 && r <= 0xFE0F, r >= 0xE0100 && r <= 0xE01EF, // variation selectors
			r >= 0x1F3FB && r <= 0x1F3FF, // skin tones
			r >= 0xE0020 && r <= 0xE007F, // tags
			r == 0x20E3:                  // combining enclosing keycap
			continue
		}
		out = utf8.AppendRune(out, r)
	}
	return string(out)
}

// solo reports whether text must be drawn by itself: a character outside
// the BMP (a surrogate pair in UTF-16) or several code points.
func solo(text string) bool {
	if len(text) < 4 {
		r, n := utf8.DecodeRuneInString(text)
		return n != len(text) || r > 0xFFFF
	}
	return true
}
