// Package render owns every byte that reaches a terminal.
//
// Width safety is a hard invariant, not a convention. Almost every glyph the
// design uses -- box drawing, block elements, half blocks, arrows, the status
// glyphs -- has East Asian Width "Ambiguous", meaning the terminal decides
// whether it occupies one cell or two (decision D39). Spike S2 verified they
// render single-width in foot with DejaVu Sans Mono; go-runewidth must be
// configured to agree, or a single mis-measured rune corrupts an entire frame.
package render

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

func init() {
	// Ambiguous-width runes occupy ONE cell. This must match the client
	// terminal's behaviour (D39). It is set here, once, rather than relied
	// upon as a default that a dependency might change.
	runewidth.DefaultCondition.EastAsianWidth = false
}

// Strict makes width violations panic instead of truncating. Tests set it;
// production leaves it false, because a panic in a daemon serving several
// sessions must not take them all down (r3 §9).
var Strict bool

// Width reports the display width of s in terminal cells.
func Width(s string) int { return runewidth.StringWidth(s) }

// Truncate returns the longest prefix of s that fits within w cells. It never
// splits a multi-cell rune: if the next rune would overflow, it stops.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if used+rw > w {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String()
}

// Pad returns s padded with spaces to exactly w cells, truncating if longer.
func Pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	cur := Width(s)
	switch {
	case cur == w:
		return s
	case cur < w:
		return s + strings.Repeat(" ", w-cur)
	default:
		t := Truncate(s, w)
		// A truncation that lands mid-wide-rune leaves a one-cell gap.
		if d := w - Width(t); d > 0 {
			t += strings.Repeat(" ", d)
		}
		return t
	}
}

// PadLeft is Pad, right-aligned. Numeric columns use it so the eye compares
// columns rather than values (design round three).
func PadLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	cur := Width(s)
	if cur >= w {
		return Pad(s, w)
	}
	return strings.Repeat(" ", w-cur) + s
}
