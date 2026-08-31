package widget

import (
	"regexp"
	"strings"
	"testing"

	"github.com/ifnull/hatty/internal/layout"
	"github.com/ifnull/hatty/internal/render"
)

func TestMain(m *testing.M) { render.Strict = true; m.Run() }

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func vis(s string) string { return ansi.ReplaceAllString(s, "") }

func cols() []TableColumn {
	return []TableColumn{
		{layout.Column{Name: "FLIGHT", Width: 9}, render.Left},
		{layout.Column{Name: "DIST", Width: 6}, render.Right},
		{layout.Column{Name: "ALT", Width: 8}, render.Right},
		{layout.Column{Name: "BRG", Width: 4}, render.Right},
		{layout.Column{Name: "TYPE", Width: 6, MinCols: 56}, render.Left},
		{layout.Column{Name: "SQK", Width: 5, MinCols: 80}, render.Left},
	}
}

func rows(th *Theme, n int) [][]Cell {
	out := make([][]Cell, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, []Cell{
			{Text: "UAL1234", Style: th.Value},
			{Text: "9.1", Style: th.Value},
			{Text: "36,000", Style: th.AltFor(36000)},
			{Text: "284", Style: th.Label},
			{Text: "B738", Style: th.Label},
			{Text: "1200", Style: th.Muted},
		})
	}
	return out
}

func all(th *Theme) map[string]Widget {
	return map[string]Widget{
		"table":      &Table{Columns: cols(), Rows: rows(th, 6), Selected: 1, Note: "17 of 94 tracked"},
		"alert-ok":   &AlertStrip{Nominal: "9 tracked · 0 flagged"},
		"alert-fire": &AlertStrip{Active: true, Message: "MILITARY  RCH451 · 12.6 nm"},
		"alert-held": &AlertStrip{Active: true, Held: true, Message: "MILITARY  cleared 20s ago"},
		"detail":     &Detail{Title: "RCH451", Fields: []DetailField{{"Type", Cell{Text: "C17"}}, {"Squawk", Cell{Text: "4231"}}, {"Vertical", Cell{Text: "-1,200 ft/min"}}}},
		"bar":        &Bar{Label: "CO₂", ValueText: "436 ppm", Fraction: 0.02, State: "ok", ShowsState: true},
		"bar-warn":   &Bar{Label: "Battery", ValueText: "2.63 V", Fraction: 0.55, State: "warn", ShowsState: true},
		"status":     &StatusBar{Left: "1090 ●  978 ●  RID ●", Keys: "↑↓ select  ⏎ pin  / search", Right: "14:23:07"},
	}
}

// THE contract. A widget returning the wrong shape corrupts the whole frame,
// not just its own panel -- so it is asserted for every widget at every
// verified breakpoint.
func TestAllWidgetsFillTheirRegionExactly(t *testing.T) {
	th := Default()
	grids := []struct{ w, h int }{
		{100, 32}, {100, 8}, {80, 25}, {80, 4}, {64, 20}, {50, 15}, {44, 12}, {44, 1},
	}
	for name, wdg := range all(th) {
		for _, g := range grids {
			lines := wdg.Render(g.w, g.h, th)
			if len(lines) != g.h {
				t.Errorf("%s at %dx%d: %d lines, want %d", name, g.w, g.h, len(lines), g.h)
				continue
			}
			for i, l := range lines {
				if got := render.Width(vis(l)); got != g.w {
					t.Errorf("%s at %dx%d: line %d is %d cells, want %d\n  %q",
						name, g.w, g.h, i, got, g.w, vis(l))
				}
			}
		}
	}
}

// D36: columns drop by declared minimum width, and the header must drop with
// the data or every row misaligns.
func TestTableDropsColumnsWithItsHeader(t *testing.T) {
	th := Default()
	tbl := &Table{Columns: cols(), Rows: rows(th, 3)}
	for _, c := range []struct {
		w    int
		want []string
		gone []string
	}{
		{100, []string{"FLIGHT", "DIST", "ALT", "BRG", "TYPE", "SQK"}, nil},
		{70, []string{"FLIGHT", "DIST", "ALT", "BRG", "TYPE"}, []string{"SQK"}},
		{50, []string{"FLIGHT", "DIST", "ALT", "BRG"}, []string{"TYPE", "SQK"}},
	} {
		hdr := vis(tbl.Render(c.w, 6, th)[0])
		for _, w := range c.want {
			if !strings.Contains(hdr, w) {
				t.Errorf("at %d cols header lacks %q: %q", c.w, w, hdr)
			}
		}
		for _, g := range c.gone {
			if strings.Contains(hdr, g) {
				t.Errorf("at %d cols header still has %q: %q", c.w, g, hdr)
			}
		}
	}
}

// The selection cursor must be legible without colour, so the design does not
// depend on a palette the client may not have.
func TestSelectionCursorWorksWithoutColour(t *testing.T) {
	th := Default()
	tbl := &Table{Columns: cols(), Rows: rows(th, 4), Selected: 2}
	lines := tbl.Render(100, 8, th)
	if !strings.HasPrefix(vis(lines[3]), "▸") {
		t.Errorf("selected row lacks the cursor glyph: %q", vis(lines[3]))
	}
	if strings.HasPrefix(vis(lines[1]), "▸") {
		t.Errorf("unselected row has a cursor: %q", vis(lines[1]))
	}
}

// D41: the count and the list legitimately disagree, so the subset must be
// stated rather than hidden.
func TestTableStatesItsSubset(t *testing.T) {
	th := Default()
	tbl := &Table{Columns: cols(), Rows: rows(th, 3), Note: "17 of 94 tracked · lists capped at 10 per flag"}
	joined := strings.Join(tbl.Render(100, 10, th), "\n")
	if !strings.Contains(vis(joined), "of 94 tracked") {
		t.Error("the table did not state that it shows a subset")
	}
}

// E2: a held alert must still read as an alert, and must differ from a live one
// so a decaying alert is visibly decaying rather than indistinguishable.
func TestHeldAlertRendersDifferentlyFromLive(t *testing.T) {
	th := Default()
	live := (&AlertStrip{Active: true, Message: "X"}).Render(60, 1, th)[0]
	held := (&AlertStrip{Active: true, Held: true, Message: "X"}).Render(60, 1, th)[0]
	if live == held {
		t.Error("a held alert renders identically to a live one")
	}
	for _, l := range []string{live, held} {
		if !strings.Contains(vis(l), "⚠") {
			t.Errorf("alert lacks its glyph: %q", vis(l))
		}
	}
}

// Colour never carries meaning alone (D34): every state also has a word.
func TestBarStatesCarryAWordNotJustAColour(t *testing.T) {
	th := Default()
	for state, word := range map[string]string{"ok": "GOOD", "warn": "WARN", "alert": "HIGH"} {
		got := vis((&Bar{Label: "X", ValueText: "1", Fraction: 0.5, State: state, ShowsState: true}).Render(60, 1, th)[0])
		if !strings.Contains(got, word) {
			t.Errorf("state %q did not render the word %q: %q", state, word, got)
		}
	}
}

// Half-block bars must be exactly the requested width whatever the fraction.
func TestHalfBlockBarsAreExactWidth(t *testing.T) {
	for _, w := range []int{4, 9, 24} {
		for i := 0; i <= 40; i++ {
			f := float64(i) / 40
			if got := render.Width(halfBlocks(f, w)); got != w {
				t.Fatalf("halfBlocks(%.3f, %d) is %d cells", f, w, got)
			}
		}
	}
}

// D36: status hints abbreviate before they vanish, rather than overflowing.
func TestStatusBarShedsKeysBeforeOverflowing(t *testing.T) {
	th := Default()
	s := &StatusBar{Left: "1090 ●  978 ●  RID ●", Keys: "↑↓ select  ⏎ pin  / search", Right: "14:23:07"}
	wide := vis(s.Render(100, 1, th)[0])
	if !strings.Contains(wide, "select") {
		t.Error("keys should be present at 100 columns")
	}
	narrow := vis(s.Render(44, 1, th)[0])
	if strings.Contains(narrow, "select") {
		t.Error("keys should have been shed at 44 columns")
	}
	if !strings.Contains(narrow, "1090") {
		t.Error("connection state must survive; it is the important half")
	}
}
