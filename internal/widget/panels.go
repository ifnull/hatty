package widget

import (
	"strings"

	"github.com/ifnull/hatty/internal/render"
)

// AlertStrip is the reserved line. It NEVER collapses at any size (D34/D36):
// an alert that disappears when the window shrinks is worse than never having
// had one, and a dashboard whose geometry jumps when something goes wrong
// trains the reader to distrust it exactly when they most need to read it.
type AlertStrip struct {
	// Active is true while the alert condition holds, OR while it is being
	// held after clearing.
	//
	// FINDING E2: alerts are EVENTS, and the depth-1 replacing frame sink keeps
	// only the newest frame -- correct for state, wrong for events. An alert
	// that goes on and off between two consumed frames would never be seen, so
	// the state model holds it for a configured window and lets it decay
	// visibly instead of vanishing.
	Active  bool
	Message string // shown while active
	Nominal string // shown otherwise
	Held    bool   // active only because of the hold window
}

func (a *AlertStrip) Render(w, h int, th *Theme) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	var line string
	switch {
	case a.Active:
		st := th.AlertBanner
		if a.Held {
			st = render.Style{FG: th.Alert.FG, Bold: true}
		}
		line = render.NewRow(w).Cell("  ⚠  "+a.Message, w, render.Left, st).String()
	default:
		r := render.NewRow(w)
		r.Cell("●  ", min(3, w), render.Left, th.OK)
		r.Cell(a.Nominal, w-min(3, w), render.Left, th.Label)
		r.Fill(" ", render.Style{})
		line = r.String()
	}
	return fit([]string{line}, w, h)
}

// Detail is the key/value record beneath the table. It carries the elastic role
// on `radar` (D37 refined): the table's height is bounded by how much traffic
// is overhead, so it cannot absorb slack, whereas the ha-airspace detail record
// always has more to show.
type Detail struct {
	Title  string
	Fields []DetailField
}

// DetailField is one labelled value.
type DetailField struct {
	Label string
	Value Cell
}

func (d *Detail) Render(w, h int, th *Theme) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	// The title rule is drawn by the frame chrome (finding F2), not here, so a
	// panel cannot disagree with its own divider.
	out := make([]string, 0, h)
	// Two columns of label/value when there is room; one when there is not.
	perCol := 2
	if w < 62 {
		perCol = 1
	}
	// One cell wider than the longest label expected, so a label that exactly
	// fills its field cannot run into its value -- "Closest approach" is 16
	// characters and did exactly that.
	labelW, colW := 18, w/perCol
	for i := 0; i < len(d.Fields) && len(out) < h; i += perCol {
		r := render.NewRow(w)
		for c := 0; c < perCol; c++ {
			if i+c >= len(d.Fields) {
				r.Gap(colW)
				continue
			}
			f := d.Fields[i+c]
			r.Cell(" "+f.Label, labelW, render.Left, th.Label)
			r.Cell(f.Value.Text, colW-labelW, render.Left, f.Value.Style)
		}
		r.Fill(" ", render.Style{})
		out = append(out, r.String())
	}
	return fit(out, w, h)
}

// Bar is a threshold bar. Half-block characters give double precision at no
// extra width -- verified single-cell by spike S2.
type Bar struct {
	Label      string
	ValueText  string
	Fraction   float64 // 0..1
	State      string  // "ok" | "warn" | "alert"
	ShowsState bool
}

func (b *Bar) Render(w, h int, th *Theme) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	st := th.OK
	word := "GOOD"
	switch b.State {
	case "warn":
		st, word = th.Warn, "WARN"
	case "alert":
		st, word = th.Alert, "HIGH"
	}
	labelW, valueW := 16, 12
	tail := 0
	if b.ShowsState {
		tail = 6
	}
	barW := w - labelW - valueW - tail
	if barW < 4 {
		barW = 4
	}
	r := render.NewRow(w)
	r.Cell(" "+b.Label, labelW, render.Left, th.Label)
	r.Cell(b.ValueText+" ", valueW, render.Right, th.Value)
	r.Cell(halfBlocks(b.Fraction, barW), barW, render.Left, st)
	if b.ShowsState {
		r.Cell("  "+word, tail, render.Left, st)
	}
	r.Fill(" ", render.Style{})
	return fit([]string{r.String()}, w, h)
}

// halfBlocks renders a bar at half-cell resolution.
func halfBlocks(f float64, w int) string {
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	halves := int(f*float64(w)*2 + 0.5)
	full, rem := halves/2, halves%2
	var b strings.Builder
	b.WriteString(strings.Repeat("█", full))
	if rem == 1 && full < w {
		b.WriteString("▌")
		full++
	}
	if full < w {
		b.WriteString(strings.Repeat("░", w-full))
	}
	return b.String()
}

// StatusBar abbreviates before it vanishes (D36).
type StatusBar struct {
	Left  string
	Keys  string
	Right string
}

func (s *StatusBar) Render(w, h int, th *Theme) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	r := render.NewRow(w)
	lw := render.Width(s.Left) + 1
	if lw > w {
		lw = w
	}
	r.Cell(" "+s.Left, lw, render.Left, th.OK)
	rest := w - lw
	rw := render.Width(s.Right) + 1
	if rw > rest {
		rw = rest
	}
	keysW := rest - rw
	if keysW > 0 {
		keys := s.Keys
		if render.Width(keys)+2 > keysW {
			keys = "" // abbreviate away before overflowing
		}
		r.Cell("  "+keys, keysW, render.Left, th.Muted)
	}
	if rw > 0 {
		r.Cell(s.Right+" ", rw, render.Right, th.Label)
	}
	r.Fill(" ", render.Style{})
	return fit([]string{r.String()}, w, h)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
