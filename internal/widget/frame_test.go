package widget

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ifnull/hatty/internal/layout"
	"github.com/ifnull/hatty/internal/render"
)

// aircraft mirrors what ha-airspace actually supplies (airspace-entity-model.md).
type aircraft struct {
	flight, typ, sqk string
	dist             float64
	alt, brg         int
	mil              bool
	stale            bool
}

var contacts = []aircraft{
	{"UAL1234", "B738", "1200", 9.1, 36000, 284, false, false},
	{"RCH451", "C17", "4231", 12.6, 24000, 195, true, false},
	{"N77148", "C172", "1200", 18.3, 4600, 270, false, false},
	{"SWA2584", "B737", "1200", 24.7, 34000, 312, false, false},
	{"AAL691", "A321", "1200", 31.2, 20875, 101, false, false},
	{"VTM300", "C560", "1200", 36.8, 29150, 38, false, false},
	{"DAL1508", "A320", "1200", 41.4, 27825, 263, false, true},
	{"JBU607", "A320", "1200", 46.6, 36025, 280, false, false},
	{"N556GA", "GLF6", "1200", 63.1, 41000, 340, false, false},
}

func buildTable(th *Theme) *Table {
	rows := make([][]Cell, 0, len(contacts))
	for _, a := range contacts {
		name := th.Value
		switch {
		case a.mil:
			name = render.Style{FG: th.Alert.FG, Bold: true}
		case a.stale:
			name = th.Stale
		}
		rows = append(rows, []Cell{
			{a.flight, name},
			{fmt.Sprintf("%.1f", a.dist), th.Value},
			{fmt.Sprintf("%d", a.alt), th.AltFor(float64(a.alt))},
			{fmt.Sprintf("%d", a.brg), th.Label},
			{a.typ, th.Label},
			{a.sqk, th.Muted},
		})
	}
	return &Table{Columns: cols(), Rows: rows, Selected: 1,
		Note: "9 flagged of 94 tracked · lists capped at 10 per flag"}
}

// Compose the radar screen through the real solver and assert the frame is
// exactly the grid -- every row, every cell. This is the shape Phase 8's
// golden-file matrix takes.
func TestRadarFrameIsExactAtEveryBreakpoint(t *testing.T) {
	th := Default()
	specs := []layout.Spec{
		{Name: "alert", Reserve: true, MinRows: 1, NatRows: 1},
		{Name: "table", MinRows: 3, NatRows: len(contacts) + 2, Elastic: 2, DropRank: 30},
		{Name: "detail", MinRows: 3, NatRows: 4, Elastic: 3, DropRank: 20},
		{Name: "status", Reserve: true, MinRows: 1, NatRows: 1},
	}
	widgets := map[string]Widget{
		"alert":  &AlertStrip{Nominal: "9 flagged · 94 tracked · last alert 6 d ago"},
		"table":  buildTable(th),
		"detail": &Detail{Title: "RCH451", Fields: []DetailField{{"Type", Cell{"C17 Globemaster III", th.Value}}, {"Operator", Cell{"US Air Force", th.Value}}, {"Squawk", Cell{"4231", th.Value}}, {"Vertical", Cell{"-1,200 ft/min", th.Value}}}},
		"status": &StatusBar{Left: "1090 ●  978 ●  RID ●", Keys: "↑↓ select  ⏎ pin  / search", Right: "14:23:07"},
	}

	for _, g := range []struct{ w, h int }{{100, 32}, {80, 25}, {80, 22}, {64, 20}, {50, 15}, {44, 12}} {
		f, err := layout.Solve(specs, g.w, g.h, 44, 12)
		if err != nil {
			t.Fatalf("%dx%d: %v", g.w, g.h, err)
		}
		var frame []string
		for _, r := range f.Regions {
			frame = append(frame, widgets[specs[r.Panel].Name].Render(g.w, r.H, th)...)
		}
		if len(frame) != g.h {
			t.Errorf("%dx%d: frame is %d rows, want %d", g.w, g.h, len(frame), g.h)
		}
		for i, l := range frame {
			if got := render.Width(vis(l)); got != g.w {
				t.Errorf("%dx%d: row %d is %d cells, want %d\n  %q", g.w, g.h, i, got, g.w, vis(l))
			}
		}
	}
}

// Print the real frame so it can be compared against the Phase 5 mockups.
func TestShowRadarFrame(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	th := Default()
	specs := []layout.Spec{
		{Name: "alert", Reserve: true, MinRows: 1, NatRows: 1},
		{Name: "table", MinRows: 3, NatRows: len(contacts) + 2, Elastic: 2, DropRank: 30},
		{Name: "detail", MinRows: 3, NatRows: 4, Elastic: 3, DropRank: 20},
		{Name: "status", Reserve: true, MinRows: 1, NatRows: 1},
	}
	widgets := map[string]Widget{
		"alert":  &AlertStrip{Nominal: "9 flagged · 94 tracked · last alert 6 d ago"},
		"table":  buildTable(th),
		"detail": &Detail{Title: "RCH451", Fields: []DetailField{{"Type", Cell{"C17 Globemaster III", th.Value}}, {"Operator", Cell{"US Air Force", th.Value}}, {"Squawk", Cell{"4231", th.Value}}, {"Vertical", Cell{"-1,200 ft/min", th.Value}}}},
		"status": &StatusBar{Left: "1090 ●  978 ●  RID ●", Keys: "↑↓ select  ⏎ pin  / search", Right: "14:23:07"},
	}
	f, _ := layout.Solve(specs, 100, 32, 44, 12)
	var out []string
	for _, r := range f.Regions {
		out = append(out, widgets[specs[r.Panel].Name].Render(100, r.H, th)...)
	}
	fmt.Println("\n" + strings.Join(out, "\n"))
}
