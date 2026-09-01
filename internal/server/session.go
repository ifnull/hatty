package server

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ifnull/hatty/internal/config"
	"github.com/ifnull/hatty/internal/layout"
	"github.com/ifnull/hatty/internal/model"
	"github.com/ifnull/hatty/internal/render"
	"github.com/ifnull/hatty/internal/state"
	"github.com/ifnull/hatty/internal/widget"
)

// FrameMsg carries a published snapshot to one session's Bubble Tea program.
//
// There is no ResolveHint here, and that is deliberate. Bubble Tea's View()
// returns a COMPLETE string every frame; the line-level diffing happens inside
// the renderer, below the application, and hatty cannot influence it (finding
// B3). Carrying a hint would invite an implementer to return a partial view and
// produce a corrupted frame.
type FrameMsg struct{ Snap *state.Snapshot }

// Session is one client's Bubble Tea model.
type Session struct {
	Dash  *config.Dashboard
	Theme *widget.Theme

	snap   *state.Snapshot
	w, h   int
	frames int
}

func (s Session) Init() tea.Cmd { return nil }

func (s Session) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = m.Width, m.Height
	case FrameMsg:
		s.snap = m.Snap
		s.frames++
	case tea.KeyMsg:
		switch m.String() {
		case "q", "ctrl+c", "esc":
			return s, tea.Quit
		}
	}
	return s, nil
}

// View renders the whole screen.
//
// Every line is exactly s.w cells and there are exactly s.h of them, because
// the widgets guarantee it and the solver assigns every row. A frame that is
// the wrong shape corrupts the terminal, not just one panel.
func (s Session) View() string {
	if s.w <= 0 || s.h <= 0 {
		return ""
	}
	d := s.Dash
	screen := model.Build(d, s.snap, s.Theme, time.Now())

	// F2: chrome consumes grid. Two columns for the side rails, two rows for
	// the borders, and one row per titled rule -- all subtracted BEFORE
	// solving, or the frame overflows the terminal.
	rules := 0
	for _, sp := range screen.Specs {
		if sp.Rule {
			rules++
		}
	}
	innerW, innerH := s.w-2, s.h-2-rules
	if innerW < d.Display.MinCols || innerH < d.Display.MinRows {
		return refusal(s.w, s.h, d, s.Theme)
	}

	f, err := layout.Solve(screen.Specs, innerW, innerH, d.Display.MinCols, d.Display.MinRows)
	if err != nil {
		return refusal(s.w, s.h, d, s.Theme)
	}

	lines := []string{widget.Top(s.w, d.Title, s.Theme)}
	for _, reg := range f.Regions {
		var body []string
		switch {
		case reg.Panel == layout.GapPanel:
			// Deliberate blank space (finding F1), drawn where it belongs.
			for n := 0; n < reg.H; n++ {
				body = append(body, render.NewRow(innerW).Gap(innerW).String())
			}
		case reg.Panel < len(screen.Widgets):
			sp := screen.Specs[reg.Panel]
			if sp.Rule {
				st := s.Theme.Chrome
				if sp.Name == "detail" {
					st = s.Theme.Title
				}
				lines = append(lines, widget.Rule(s.w, sp.Title, s.Theme, st))
			}
			body = screen.Widgets[reg.Panel].Render(innerW, reg.H, s.Theme)
		default:
			continue
		}
		for _, b := range body {
			lines = append(lines, widget.Rail(b, s.Theme))
		}
	}
	lines = append(lines, widget.Bottom(s.w, s.Theme))

	var out []byte
	for i, l := range lines {
		if i > 0 {
			out = append(out, '\n')
		}
		out = append(out, l...)
	}
	return string(out)
}

// refusal is what a too-small terminal gets. Drawing a corrupted layout is
// worse than declining to draw (D36, requirement F6).
func refusal(w, h int, d *config.Dashboard, th *widget.Theme) string {
	lines := []string{
		"",
		"  terminal too small",
		"",
	}
	need := render.NewRow(w)
	need.Cell("  need ", 7, render.Left, th.Label)
	need.Cell(itoa(d.Display.MinCols)+"×"+itoa(d.Display.MinRows), 12, render.Left, th.Value)
	need.Cell("  have ", 7, render.Left, th.Label)
	need.Cell(itoa(w)+"×"+itoa(h), 12, render.Left, th.Value)
	need.Fill(" ", render.Style{})

	var out []byte
	for i := 0; i < h; i++ {
		var line string
		switch {
		case i == 1 && len(lines) > 1:
			r := render.NewRow(w)
			r.Cell(lines[1], w, render.Left, th.Alert)
			line = r.String()
		case i == 3:
			line = need.String()
		default:
			line = render.NewRow(w).Gap(w).String()
		}
		out = append(out, line...)
		if i < h-1 {
			out = append(out, '\n')
		}
	}
	return string(out)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
