package layout

import (
	"errors"
	"testing"
)

// The radar screen as designed: a reserved alert strip, a table, a detail pane
// that outranks the table for leftover rows (D37 refined), and a status bar.
func radar(tableRows int) []Spec {
	return []Spec{
		{Name: "alert_strip", Reserve: true, MinRows: 1, NatRows: 1},
		{Name: "table", MinRows: 3, NatRows: tableRows, Elastic: 2, DropRank: 30},
		{Name: "detail", MinRows: 3, NatRows: 4, Elastic: 3, DropRank: 20},
		{Name: "trend", MinRows: 4, NatRows: 6, DropRank: 10},
		{Name: "status", Reserve: true, MinRows: 1, NatRows: 1},
	}
}

func total(f Frame) int {
	n := 0
	for _, r := range f.Regions {
		n += r.H
	}
	return n
}

func byName(f Frame, specs []Spec, name string) (Region, bool) {
	for _, r := range f.Regions {
		if specs[r.Panel].Name == name {
			return r, true
		}
	}
	return Region{}, false
}

// Every row of the grid must be assigned. A layout that leaves rows unowned is
// the trapped-space bug D37 exists to prevent.
func TestEveryRowIsAssignedAtEveryBreakpoint(t *testing.T) {
	specs := radar(9)
	for _, g := range []struct{ c, r int }{
		{100, 32}, {100, 30}, {80, 25}, {80, 22}, {64, 20}, {50, 15}, {44, 12},
	} {
		f, err := Solve(specs, g.c, g.r, 44, 12)
		if err != nil {
			t.Fatalf("%dx%d: %v", g.c, g.r, err)
		}
		if got := total(f); got != g.r {
			t.Errorf("%dx%d: rows assigned = %d, want %d", g.c, g.r, got, g.r)
		}
		y := 0
		for _, reg := range f.Regions {
			if reg.Y != y {
				t.Errorf("%dx%d: region at Y=%d, expected %d -- a gap or overlap", g.c, g.r, reg.Y, y)
			}
			y += reg.H
		}
	}
}

// D34/D36: the alert strip reserves its line at EVERY size.
func TestReservedPanelsNeverDrop(t *testing.T) {
	specs := radar(9)
	for r := 12; r <= 32; r++ {
		f, err := Solve(specs, 100, r, 44, 12)
		if err != nil {
			t.Fatalf("rows=%d: %v", r, err)
		}
		for _, name := range []string{"alert_strip", "status"} {
			if _, ok := byName(f, specs, name); !ok {
				t.Errorf("rows=%d: %s was dropped -- reserved panels never drop", r, name)
			}
		}
	}
}

// D36's drop order. MinRows is the contract: a panel that can still have its
// minimum is kept and squeezed, and only one that cannot is dropped. So panels
// drop when the sum of minimums exceeds the grid, lowest DropRank first.
func TestPanelsDropInDeclaredOrder(t *testing.T) {
	specs := radar(9)
	f, _ := Solve(specs, 100, 32, 44, 12)
	if _, ok := byName(f, specs, "trend"); !ok {
		t.Fatal("at 32 rows everything should fit")
	}
	// Minimums sum to 12, so force a drop by shrinking the grid below that.
	// The floor is lowered here to exercise the drop path rather than refusal.
	f, err := Solve(specs, 100, 10, 44, 8)
	if err != nil {
		t.Fatalf("10 rows: %v", err)
	}
	if _, ok := byName(f, specs, "trend"); ok {
		t.Error("trend (lowest DropRank) should drop first")
	}
	if _, ok := byName(f, specs, "table"); !ok {
		t.Error("the table is the content; it must outlive the trend")
	}
	// Minimums after losing the trend are 1+3+3+1 = 8, so the detail pane only
	// drops below that.
	f, _ = Solve(specs, 100, 7, 44, 6)
	if _, ok := byName(f, specs, "detail"); ok {
		t.Error("detail should drop once the trend is gone and minimums still do not fit")
	}
	for _, name := range []string{"alert_strip", "status", "table"} {
		if _, ok := byName(f, specs, name); !ok {
			t.Errorf("%s should survive to the last", name)
		}
	}
}

// The squeeze must respect the same ranking as the drop. Shrinking by largest
// slack is rank-blind and gives the panel that would be dropped FIRST the most
// rows -- observed as table:3 detail:3 trend:4 at a 12-row grid, where the
// table is the content and the trend is the first thing anyone would sacrifice.
func TestShrinkingRespectsDropRank(t *testing.T) {
	specs := radar(9)
	f, err := Solve(specs, 100, 16, 44, 12)
	if err != nil {
		t.Fatal(err)
	}
	table, _ := byName(f, specs, "table")
	trend, _ := byName(f, specs, "trend")
	if table.H <= trend.H {
		t.Errorf("table has %d rows and trend has %d -- the least valuable panel "+
			"must be squeezed before the content", table.H, trend.H)
	}
	if trend.H != 4 {
		t.Errorf("trend H = %d, want its 4-row minimum", trend.H)
	}
}

// FINDING B1: elasticity must survive. When the highest-ranked elastic panel is
// dropped, the next one inherits the leftover rows -- otherwise they are
// trapped at exactly the small grids where space is tightest.
func TestElasticityIsInheritedWhenTheTopRankIsDropped(t *testing.T) {
	specs := radar(3) // little data, so there is definitely leftover
	f, err := Solve(specs, 100, 30, 44, 12)
	if err != nil {
		t.Fatal(err)
	}
	detail, ok := byName(f, specs, "detail")
	if !ok {
		t.Fatal("detail should survive at 30 rows")
	}
	if detail.H <= 4 {
		t.Errorf("detail H = %d; the top-ranked elastic panel should have absorbed leftover", detail.H)
	}

	// Now a grid where the detail pane is dropped: the table must inherit its
	// elastic role, or the leftover rows are trapped -- which is exactly the
	// B1 failure.
	f, err = Solve(specs, 100, 7, 44, 6)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := byName(f, specs, "detail"); ok {
		t.Fatal("expected the detail pane to be dropped at 7 rows")
	}
	if total(f) != 7 {
		t.Fatalf("rows assigned = %d, want 7 -- leftover was trapped when the top elastic dropped", total(f))
	}
	tbl, ok := byName(f, specs, "table")
	if !ok {
		t.Fatal("the table must survive")
	}
	if tbl.H <= 3 {
		t.Errorf("table H = %d; it should have inherited the elastic role and absorbed leftover", tbl.H)
	}
}

// A table with nine rows of traffic asks for nine. It must not silently swell
// to fill the screen unless it is the elastic panel.
func TestNonElasticPanelsKeepTheirNaturalHeight(t *testing.T) {
	specs := []Spec{
		{Name: "strip", Reserve: true, MinRows: 1, NatRows: 1},
		{Name: "fixed", MinRows: 2, NatRows: 2},
		{Name: "grow", MinRows: 2, NatRows: 2, Elastic: 1},
	}
	f, err := Solve(specs, 100, 20, 44, 5)
	if err != nil {
		t.Fatal(err)
	}
	fixed, _ := byName(f, specs, "fixed")
	grow, _ := byName(f, specs, "grow")
	if fixed.H != 2 {
		t.Errorf("fixed panel H = %d, want 2", fixed.H)
	}
	if grow.H != 17 {
		t.Errorf("elastic panel H = %d, want 17 (absorbing all leftover)", grow.H)
	}
}

// D36: refuse below the floor rather than draw a corrupted layout.
func TestRefusesBelowTheFloor(t *testing.T) {
	specs := radar(9)
	for _, g := range []struct{ c, r int }{{43, 12}, {44, 11}, {20, 5}} {
		_, err := Solve(specs, g.c, g.r, 44, 12)
		if err == nil {
			t.Errorf("%dx%d was accepted; it is below the floor", g.c, g.r)
			continue
		}
		if !errors.Is(err, ErrTooSmall) {
			t.Errorf("%dx%d: got %v, want ErrTooSmall", g.c, g.r, err)
		}
		var tse TooSmallError
		if errors.As(err, &tse) && (tse.WantCols != 44) {
			t.Errorf("%dx%d: refusal does not state what is needed: %v", g.c, g.r, err)
		}
	}
}

// D36 column drop order, at the breakpoints the design actually declares.
func TestColumnDropOrder(t *testing.T) {
	cols := []Column{
		{Name: "FLIGHT", Width: 9},
		{Name: "DIST", Width: 6},
		{Name: "ALT", Width: 8},
		{Name: "BRG", Width: 4},
		{Name: "TYPE", Width: 6, MinCols: 56},
		{Name: "RANGE", Width: 9, MinCols: 68},
		{Name: "SPD", Width: 5, MinCols: 80},
		{Name: "CPA", Width: 6, MinCols: 88},
		{Name: "OP", Width: 4, MinCols: 96},
	}
	want := map[int]int{50: 4, 56: 5, 68: 6, 80: 7, 88: 8, 96: 9, 100: 9}
	for w, n := range want {
		if got := len(ActiveColumns(cols, w)); got != n {
			t.Errorf("at %d columns: %d survive, want %d", w, got, n)
		}
	}
	// The four that answer the screen's question survive everywhere.
	for _, c := range ActiveColumns(cols, 44) {
		_ = c
	}
	if got := len(ActiveColumns(cols, 44)); got != 4 {
		t.Errorf("at the floor: %d columns, want the 4 unconditional ones", got)
	}
	// And the surviving set must actually fit at every breakpoint.
	for _, w := range []int{44, 56, 68, 80, 88, 96, 100} {
		if need, ok := ColumnsFit(cols, w); !ok {
			t.Errorf("at %d columns the surviving set needs %d cells", w, need)
		}
	}
}

// F1: a data-bounded panel must not absorb rows it cannot draw. Observed live
// as 22 blank rows inside the detail pane at 100x32.
func TestCappedPanelsDoNotSwallowUnusableRows(t *testing.T) {
	specs := []Spec{
		{Name: "alert", Reserve: true, MinRows: 1, NatRows: 1},
		{Name: "table", MinRows: 3, NatRows: 5, MaxRows: 5, Elastic: 2, DropRank: 30},
		{Name: "detail", MinRows: 2, NatRows: 3, MaxRows: 3, Elastic: 3, DropRank: 20},
		{Name: "status", Reserve: true, MinRows: 1, NatRows: 1},
	}
	f, err := Solve(specs, 100, 32, 44, 12)
	if err != nil {
		t.Fatal(err)
	}
	if got := total(f); got != 32 {
		t.Fatalf("rows assigned = %d, want 32", got)
	}
	for _, r := range f.Regions {
		if r.Panel == GapPanel {
			continue
		}
		if m := specs[r.Panel].MaxRows; m > 0 && r.H > m {
			t.Errorf("%s got %d rows but can only use %d", specs[r.Panel].Name, r.H, m)
		}
	}
	var gap int
	for _, r := range f.Regions {
		if r.Panel == GapPanel {
			gap += r.H
		}
	}
	if gap != 32-10 {
		t.Errorf("gap = %d rows, want %d as an explicit region", gap, 22)
	}
	// The status bar must still be the last thing on screen.
	last := f.Regions[len(f.Regions)-1]
	if last.Panel == GapPanel || specs[last.Panel].Name != "status" {
		t.Error("the gap displaced the status bar from the bottom")
	}
}

// Leftover must fall THROUGH a capped panel to the next elastic rank.
func TestLeftoverFallsThroughToTheNextElasticRank(t *testing.T) {
	specs := []Spec{
		{Name: "capped", MinRows: 2, NatRows: 2, MaxRows: 3, Elastic: 5},
		{Name: "roomy", MinRows: 2, NatRows: 2, Elastic: 1},
		{Name: "status", Reserve: true, MinRows: 1, NatRows: 1},
	}
	f, err := Solve(specs, 100, 20, 44, 5)
	if err != nil {
		t.Fatal(err)
	}
	capped, _ := byName(f, specs, "capped")
	roomy, _ := byName(f, specs, "roomy")
	if capped.H != 3 {
		t.Errorf("capped panel got %d rows, want its 3-row cap", capped.H)
	}
	if roomy.H <= 2 {
		t.Errorf("uncapped panel got %d rows; leftover did not fall through", roomy.H)
	}
	if total(f) != 20 {
		t.Errorf("rows assigned = %d, want 20", total(f))
	}
}
