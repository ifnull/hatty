package widget

import (
	"github.com/ifnull/hatty/internal/layout"
	"github.com/ifnull/hatty/internal/render"
)

// Table is the airspace panel, and the design's centrepiece (D33).
//
// The polar radar scope was descoped: a terminal is genuinely excellent at
// dense tabular data and the airspace feed is tabular, so this is a better fit
// than a scope, not a consolation. It also removed the design's dependency on
// Braille entirely -- the table renders identically whether or not the client
// font has it.
// TableColumn is a layout column plus its presentation. Alignment is a render
// concern, so it lives here rather than in `layout`, which stays pure.
type TableColumn struct {
	layout.Column
	Align render.Align
}

type Table struct {
	Columns  []TableColumn
	Rows     [][]Cell
	Selected int

	// Note states the subset when the source is truncated.
	//
	// D41: `sensor.airspace_flag_interesting` reports state "11" against a
	// ten-item list, because ha-airspace caps the collections. And
	// aircraft_count is 94 while the table can only ever show flagged
	// contacts. Silently showing ten of eleven military aircraft is exactly
	// the quiet wrongness this project exists to avoid, so the count and the
	// cap are rendered, not hidden.
	Note string
}

func (t *Table) Render(w, h int, th *Theme) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	// Index of each surviving column in the original set, so a row's cells
	// still line up after columns drop (D36).
	keep := make([]int, 0, len(t.Columns))
	for i, c := range t.Columns {
		if c.MinCols <= w {
			keep = append(keep, i)
		}
	}
	if len(keep) == 0 {
		return blank(w, h)
	}

	out := make([]string, 0, h)

	hdr := render.NewRow(w).Cell("", 1, render.Left, render.Style{})
	for n, ci := range keep {
		if n > 0 {
			hdr.Gap(1)
		}
		hdr.Cell(t.Columns[ci].Name, t.Columns[ci].Width, t.Columns[ci].Align, th.Label)
	}
	// The active column set is narrower than the grid; the panel still owns
	// every cell of its region, so the remainder is filled explicitly rather
	// than left for String() to pad. Strict mode makes the difference fatal.
	hdr.Fill(" ", render.Style{})
	out = append(out, hdr.String())

	body := h - 1
	if t.Note != "" {
		body--
	}
	for r := 0; r < body && r < len(t.Rows); r++ {
		row := render.NewRow(w)
		sel := r == t.Selected
		// The cursor works WITHOUT colour, so the design does not depend on a
		// palette the client may not have.
		mark, ms := " ", render.Style{}
		if sel {
			mark, ms = "▸", th.Value
		}
		if sel {
			ms.BG = th.Selected.BG
		}
		row.Cell(mark, 1, render.Left, ms)
		for n, ci := range keep {
			if n > 0 {
				row.Cell(" ", 1, render.Left, render.Style{BG: bgIf(sel, th)})
			}
			var c Cell
			if ci < len(t.Rows[r]) {
				c = t.Rows[r][ci]
			}
			st := c.Style
			if sel {
				st.BG = th.Selected.BG
			}
			row.Cell(c.Text, t.Columns[ci].Width, t.Columns[ci].Align, st)
		}
		row.Fill(" ", render.Style{BG: bgIf(sel, th)})
		out = append(out, row.String())
	}
	if t.Note != "" {
		out = fit(out, w, h-1)
		out = append(out, render.NewRow(w).Cell(" "+t.Note, w, render.Left, th.Muted).String())
	}
	return fit(out, w, h)
}

func bgIf(sel bool, th *Theme) int {
	if sel {
		return th.Selected.BG
	}
	return 0
}
