package widget

import "github.com/ifnull/hatty/internal/render"

// Widget renders one panel into an exact region.
//
// The contract every widget must honour, asserted for all of them by
// TestAllWidgetsFillTheirRegionExactly: Render returns EXACTLY h lines, each
// EXACTLY w display cells. A widget that returns the wrong shape corrupts the
// whole frame, not its own panel.
type Widget interface {
	Render(w, h int, th *Theme) []string
}

// Cell is one resolved value ready to draw. The model layer produces these;
// widgets never resolve bindings themselves.
type Cell struct {
	Text  string
	Style render.Style
}

// blank returns n empty rows of width w.
func blank(w, n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, render.NewRow(w).Gap(w).String())
	}
	return out
}

// fit forces lines to exactly h entries, padding or truncating.
func fit(lines []string, w, h int) []string {
	if len(lines) > h {
		return lines[:h]
	}
	return append(lines, blank(w, h-len(lines))...)
}
