package widget

import "github.com/ifnull/hatty/internal/render"

// Chrome draws the frame the Phase 5 mockups specified: a titled top border,
// side rails, titled rules between panels, and a bottom border.
//
// FINDING F2: the widgets rendered bare content while the approved design drew
// borders. That is not only cosmetic -- chrome CONSUMES ROWS AND COLUMNS, so
// the solver must be given the reduced grid or the frame overflows. Two columns
// for the rails, two rows for the borders, and one row per rule.

// Top renders the top border with a title.
func Top(w int, title string, th *Theme) string {
	r := render.NewRow(w)
	r.Cell("┌", 1, render.Left, th.Chrome)
	if title != "" && w > 6 {
		lab := "─ " + title + " "
		if render.Width(lab) > w-2 {
			lab = render.Truncate(lab, w-2)
		}
		r.Cell(lab, render.Width(lab), render.Left, th.Title)
	}
	r.Fill("─", th.Chrome)
	// Fill claimed the whole remainder; overwrite its last cell with the corner.
	return replaceLast(r.String(), "┐", th)
}

// Bottom renders the closing border.
func Bottom(w int, th *Theme) string {
	r := render.NewRow(w)
	r.Cell("└", 1, render.Left, th.Chrome)
	r.Fill("─", th.Chrome)
	return replaceLast(r.String(), "┘", th)
}

// Rule renders a titled divider between panels.
func Rule(w int, title string, th *Theme, style render.Style) string {
	r := render.NewRow(w)
	r.Cell("├", 1, render.Left, th.Chrome)
	if title != "" && w > 6 {
		lab := "─ " + title + " "
		if render.Width(lab) > w-2 {
			lab = render.Truncate(lab, w-2)
		}
		r.Cell(lab, render.Width(lab), render.Left, style)
	}
	r.Fill("─", th.Chrome)
	return replaceLast(r.String(), "┤", th)
}

// Rail wraps one content line in the side rails.
func Rail(line string, th *Theme) string {
	v := th.Chrome.Wrap("│")
	return v + line + v
}

// replaceLast swaps the final visible cell for a corner glyph. Chrome is built
// with Fill, which claims the whole remainder, so the corner is applied after.
func replaceLast(s, corner string, th *Theme) string {
	runes := []rune(s)
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == '─' {
			return string(runes[:i]) + th.Chrome.Wrap(corner) + string(runes[i+1:])
		}
	}
	return s
}
