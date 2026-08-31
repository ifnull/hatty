// Package layout solves the cell grid.
//
// Pure: no I/O, no clock, no dependency on config or state. Everything it needs
// arrives as a Spec, which makes the whole responsive design testable as a
// table of inputs -- and the breakpoint matrix in Phase 8 is exactly that.
package layout

import (
	"errors"
	"fmt"
	"sort"
)

// ErrTooSmall means the grid cannot host the dashboard. The caller renders the
// refusal screen: drawing a corrupted layout is worse than declining to draw
// (D36, requirement F6).
var ErrTooSmall = errors.New("layout: terminal too small")

// TooSmallError carries what was needed, so the refusal can say so.
type TooSmallError struct{ HaveCols, HaveRows, WantCols, WantRows int }

func (e TooSmallError) Error() string {
	return fmt.Sprintf("layout: terminal too small: have %dx%d, need %dx%d",
		e.HaveCols, e.HaveRows, e.WantCols, e.WantRows)
}
func (e TooSmallError) Is(target error) bool { return target == ErrTooSmall }

// Spec describes one panel's requirements.
type Spec struct {
	Name string

	// Reserve marks a panel that NEVER collapses at any size.
	//
	// D34/D36: the alert strip reserves its line at every grid. An alert that
	// disappears when the window shrinks is worse than never having had one,
	// and a dashboard whose geometry jumps when something goes wrong trains the
	// reader to distrust it at the moment they most need to read it.
	Reserve bool

	// MinRows is the height below which this panel cannot render usefully.
	MinRows int

	// NatRows is the height the panel wants given its current data. A table
	// with nine rows of traffic asks for nine, not for the whole screen.
	NatRows int

	// Elastic is a RANK. Leftover rows go to the highest-ranked SURVIVING
	// panel; zero means not elastic.
	//
	// FINDING B1: revision 2 required exactly one elastic panel and then let
	// the solver drop it, leaving zero at precisely the small grids where the
	// rule matters. A rank means the next-best candidate inherits the job.
	//
	// D37 refined: on `radar` the detail pane outranks the table, because the
	// table's height is bounded by how much traffic is overhead -- it cannot
	// absorb slack -- whereas the ha-airspace detail record always has more to
	// show.
	Elastic int

	// DropRank orders removal when the grid is too short. Lower drops first.
	// Reserve panels are never eligible whatever their rank.
	DropRank int
}

// Region is a panel's assigned slice of the grid.
type Region struct {
	Panel int // index into the input specs
	Y, H  int
}

// Frame is a solved layout.
type Frame struct {
	Cols, Rows int
	Regions    []Region
	Dropped    []int
}

// Solve assigns every row of the grid.
//
// Steps (r3 §6):
//  1. refuse below the floor;
//  2. give reserved panels their natural height;
//  3. give the rest their natural height, capped at their data;
//  4. hand leftover rows to the highest-ranked surviving elastic panel;
//  5. if still over budget, drop panels by DropRank until it fits --
//     never a reserved one.
func Solve(specs []Spec, cols, rows, minCols, minRows int) (Frame, error) {
	if cols < minCols || rows < minRows {
		return Frame{}, TooSmallError{cols, rows, minCols, minRows}
	}

	live := make([]int, 0, len(specs))
	for i := range specs {
		live = append(live, i)
	}

	// Step 5, applied first so the surviving set is known before rows are
	// distributed: drop until the reserved+minimum budget fits.
	for {
		need := 0
		for _, i := range live {
			need += minOf(specs[i])
		}
		if need <= rows || len(live) == 0 {
			break
		}
		victim, rank := -1, 0
		for pos, i := range live {
			if specs[i].Reserve {
				continue
			}
			if victim < 0 || specs[i].DropRank < rank {
				victim, rank = pos, specs[i].DropRank
			}
		}
		if victim < 0 {
			// Only reserved panels remain and they still do not fit.
			return Frame{}, TooSmallError{cols, rows, minCols, sumMin(specs, live)}
		}
		live = append(live[:victim], live[victim+1:]...)
	}
	if len(live) == 0 {
		return Frame{}, TooSmallError{cols, rows, minCols, minRows}
	}

	// Steps 2-3: natural heights.
	h := make(map[int]int, len(live))
	used := 0
	for _, i := range live {
		v := specs[i].NatRows
		if v < minOf(specs[i]) {
			v = minOf(specs[i])
		}
		h[i] = v
		used += v
	}

	// Shrink over-budget panels back toward their minimums, LEAST VALUABLE
	// FIRST -- ascending DropRank, the same order in which they would be
	// dropped.
	//
	// Shrinking by largest-slack instead (the obvious implementation) is
	// rank-blind, and produces layouts where the panel that would be dropped
	// first holds the most rows: at 12 rows a rank-10 trend chart kept 4 rows
	// while the rank-30 table -- the content -- was squeezed to its 3-row
	// minimum. Ordering the squeeze by the same rank that orders removal makes
	// the two consistent, so importance decides both.
	if used > rows {
		order := append([]int(nil), live...)
		sort.SliceStable(order, func(a, b int) bool {
			return specs[order[a]].DropRank < specs[order[b]].DropRank
		})
		for _, i := range order {
			if used <= rows {
				break
			}
			slack := h[i] - minOf(specs[i])
			if slack <= 0 {
				continue
			}
			take := used - rows
			if take > slack {
				take = slack
			}
			h[i] -= take
			used -= take
		}
	}

	// Step 4: leftover rows to the highest-ranked surviving elastic panel.
	if used < rows {
		if e := bestElastic(specs, live); e >= 0 {
			h[e] += rows - used
			used = rows
		}
	}

	f := Frame{Cols: cols, Rows: rows}
	y := 0
	for _, i := range live {
		f.Regions = append(f.Regions, Region{Panel: i, Y: y, H: h[i]})
		y += h[i]
	}
	for i := range specs {
		if _, ok := h[i]; !ok {
			f.Dropped = append(f.Dropped, i)
		}
	}
	sort.Ints(f.Dropped)
	return f, nil
}

func minOf(s Spec) int {
	if s.MinRows > 0 {
		return s.MinRows
	}
	return 1
}

func sumMin(specs []Spec, live []int) int {
	n := 0
	for _, i := range live {
		n += minOf(specs[i])
	}
	return n
}

// bestElastic returns the surviving panel with the highest Elastic rank.
func bestElastic(specs []Spec, live []int) int {
	best, rank := -1, 0
	for _, i := range live {
		if specs[i].Elastic > rank {
			best, rank = i, specs[i].Elastic
		}
	}
	return best
}

// Column is the layout-facing view of a table column.
type Column struct {
	Name    string
	Width   int
	MinCols int
}

// ActiveColumns returns the columns surviving at the given width.
//
// D36: each column declares the narrowest terminal at which it survives, so the
// engine needs no per-breakpoint special cases. FLIGHT, DIST, ALT and BRG
// declare zero because they are the questions the screen exists to answer.
func ActiveColumns(cols []Column, width int) []Column {
	out := make([]Column, 0, len(cols))
	for _, c := range cols {
		if c.MinCols <= width {
			out = append(out, c)
		}
	}
	return out
}

// ColumnsFit reports the cells the active set needs, including one separator
// between columns.
func ColumnsFit(cols []Column, width int) (need int, ok bool) {
	active := ActiveColumns(cols, width)
	for i, c := range active {
		need += c.Width
		if i > 0 {
			need++
		}
	}
	return need, need <= width
}
