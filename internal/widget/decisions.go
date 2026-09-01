package widget

import "github.com/ifnull/hatty/internal/render"

// Decision is one trigger that has fired, and the action it implies.
type Decision struct {
	Say    string
	Detail string
	Level  string // "warn" | "alert" | ""
	// Stale marks a decision evaluated against data that has gone stale.
	//
	// FINDING E10: r2 SUPPRESSED such decisions. If the forecast subscription
	// drops at 18:00 and a freeze arrives at 02:00, suppression means the user
	// gets nothing and the fixtures freeze. Silence is not a legible status --
	// a stale freeze warning is far more useful than no freeze warning -- so
	// the decision renders with its staleness shown instead.
	Stale bool
	Age   string
}

// Decisions renders the actions a screen implies. This is what `home` exists
// for (D43): not a sensor list, but "how should I dress or plan when leaving
// the house, and what do I need to do to prepare it".
type Decisions struct {
	Fired   []Decision
	Nominal string // shown when nothing has fired
}

func (d *Decisions) Render(w, h int, th *Theme) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	if len(d.Fired) == 0 {
		r := render.NewRow(w)
		r.Cell(" ●  ", 4, render.Left, th.OK)
		r.Cell(d.Nominal, w-4, render.Left, th.Label)
		r.Fill(" ", render.Style{})
		return fit([]string{r.String()}, w, h)
	}

	out := make([]string, 0, h)
	for _, f := range d.Fired {
		if len(out) >= h {
			break
		}
		st, glyph := th.Warn, "▲"
		if f.Level == "alert" {
			st, glyph = th.Alert, "⚠"
		}
		r := render.NewRow(w)
		r.Cell(" "+glyph+"  ", 4, render.Left, st)
		// The action, not the reading. "Cover external water fixtures" rather
		// than "templow 29".
		say := f.Say
		r.Cell(say, minInt(w-4, render.Width(say)), render.Left, st)

		tail := ""
		if f.Detail != "" {
			tail = "  " + f.Detail
		}
		if f.Stale {
			// E10: shown, never withheld.
			tail += "  · " + f.Age
		}
		if tail != "" {
			style := th.Label
			if f.Stale {
				style = th.Stale
			}
			r.Cell(tail, maxInt(0, w-4-render.Width(say)), render.Left, style)
		}
		r.Fill(" ", render.Style{})
		out = append(out, r.String())
	}
	return fit(out, w, h)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
