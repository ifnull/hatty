package widget

import (
	"fmt"

	"github.com/ifnull/hatty/internal/render"
)

// Braille dot bit for a sub-cell position. A Braille cell is 2 columns by 4
// rows of dots, so one character carries eight plottable points -- eight times
// the vertical resolution of block elements, which is what makes a curve read
// as a curve rather than a staircase.
//
// Spike S2 confirmed Braille renders single-width on the panel in foot with
// DejaVu Sans Mono, so this is available rather than assumed (D23).
func brailleDot(dx, dy int) rune {
	if dy == 3 {
		return rune(0x40 << dx)
	}
	return rune((0x01 << dy) << (dx * 3))
}

// Series is one line on a chart.
type Series struct {
	Name   string
	Points []float64
	Style  render.Style
}

// RunChart plots one or more series over a shared axis.
//
// D16: the 24-hour wind chart overlays min, mean and max from
// recorder/statistics_during_period. They are drawn in one hue family so they
// read as ONE measurement rather than three unrelated things.
type RunChart struct {
	Series []Series
	Unit   string
	Lo, Hi float64 // fixed bounds; when equal, computed from the data
	Legend bool
}

func (c *RunChart) Render(w, h int, th *Theme) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	body := h
	if c.Legend && h > 1 {
		body = h - 1
	}
	lo, hi := c.bounds()

	var lines []string
	if th.Braille() {
		lines = c.braille(w, body, lo, hi, th)
	} else {
		// A bare framebuffer console caps its font at 256/512 glyphs, so
		// Braille cannot fit alongside everything else. Block elements are
		// coarser but always available -- and the TABLES are unaffected either
		// way (D33), which is why the design does not depend on this.
		lines = c.blocks(w, body, lo, hi, th)
	}
	if c.Legend && h > 1 {
		lines = append(lines, c.legend(w, th))
	}
	return fit(lines, w, h)
}

func (c *RunChart) bounds() (float64, float64) {
	if c.Hi > c.Lo {
		return c.Lo, c.Hi
	}
	lo, hi := 0.0, 0.0
	first := true
	for _, s := range c.Series {
		for _, v := range s.Points {
			if first {
				lo, hi, first = v, v, false
				continue
			}
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
	}
	if first || hi == lo {
		return lo, lo + 1
	}
	return lo, hi
}

// braille plots at 2x4 sub-cell resolution.
func (c *RunChart) braille(w, h int, lo, hi float64, th *Theme) []string {
	dw, dh := w*2, h*4
	// bits[cell] holds the Braille pattern; owner[cell] the style that drew the
	// last dot there, so overlapping series stay distinguishable.
	bits := make([][]rune, h)
	owner := make([][]render.Style, h)
	for i := range bits {
		bits[i] = make([]rune, w)
		owner[i] = make([]render.Style, w)
	}

	span := hi - lo
	if span <= 0 {
		span = 1
	}
	for _, s := range c.Series {
		if len(s.Points) == 0 {
			continue
		}
		prevX, prevY := -1, -1
		for i, v := range s.Points {
			x := 0
			if len(s.Points) > 1 {
				x = i * (dw - 1) / (len(s.Points) - 1)
			}
			y := int((1 - (v-lo)/span) * float64(dh-1))
			if y < 0 {
				y = 0
			}
			if y >= dh {
				y = dh - 1
			}
			if prevX >= 0 {
				plotLine(bits, owner, prevX, prevY, x, y, s.Style)
			} else {
				plotDot(bits, owner, x, y, s.Style)
			}
			prevX, prevY = x, y
		}
	}

	out := make([]string, 0, h)
	for row := 0; row < h; row++ {
		r := render.NewRow(w)
		for col := 0; col < w; col++ {
			if bits[row][col] == 0 {
				r.Cell(" ", 1, render.Left, render.Style{})
				continue
			}
			r.Cell(string(rune(0x2800)+bits[row][col]), 1, render.Left, owner[row][col])
		}
		out = append(out, r.String())
	}
	return out
}

func plotDot(bits [][]rune, owner [][]render.Style, x, y int, st render.Style) {
	cx, cy := x/2, y/4
	if cy < 0 || cy >= len(bits) || cx < 0 || cx >= len(bits[cy]) {
		return
	}
	bits[cy][cx] |= brailleDot(x%2, y%4)
	owner[cy][cx] = st
}

func plotLine(bits [][]rune, owner [][]render.Style, x0, y0, x1, y1 int, st render.Style) {
	steps := abs(x1 - x0)
	if d := abs(y1 - y0); d > steps {
		steps = d
	}
	if steps == 0 {
		plotDot(bits, owner, x0, y0, st)
		return
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		plotDot(bits, owner,
			int(float64(x0)+(float64(x1-x0))*t+0.5),
			int(float64(y0)+(float64(y1-y0))*t+0.5), st)
	}
}

// blocks is the fallback: one column per sample, eight levels per cell.
func (c *RunChart) blocks(w, h int, lo, hi float64, th *Theme) []string {
	// []rune, not a string: block elements are multi-byte, so indexing the
	// string yields a BYTE and rune(byte) renders 0xE2 -- the first byte of
	// every block glyph -- as "â". The chart drew a wall of mojibake.
	ramp := []rune(" ▁▂▃▄▅▆▇█")
	grid := make([][]rune, h)
	styles := make([][]render.Style, h)
	for i := range grid {
		grid[i] = make([]rune, w)
		styles[i] = make([]render.Style, w)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}
	span := hi - lo
	if span <= 0 {
		span = 1
	}
	for _, s := range c.Series {
		for col := 0; col < w && len(s.Points) > 0; col++ {
			idx := col * len(s.Points) / w
			v := s.Points[idx]
			frac := (v - lo) / span
			total := frac * float64(h)
			row := h - 1 - int(total)
			if row < 0 {
				row = 0
			}
			if row >= h {
				row = h - 1
			}
			level := int((total - float64(int(total))) * 8)
			grid[row][col] = rune(ramp[clamp(level+1, 1, 8)])
			styles[row][col] = s.Style
		}
	}
	out := make([]string, 0, h)
	for row := 0; row < h; row++ {
		r := render.NewRow(w)
		for col := 0; col < w; col++ {
			r.Cell(string(grid[row][col]), 1, render.Left, styles[row][col])
		}
		out = append(out, r.String())
	}
	return out
}

func (c *RunChart) legend(w int, th *Theme) string {
	r := render.NewRow(w)
	used := 0
	for _, s := range c.Series {
		if len(s.Points) == 0 {
			continue
		}
		cur := s.Points[len(s.Points)-1]
		txt := fmt.Sprintf(" %s %.1f", s.Name, cur)
		if used+render.Width(txt) > w-render.Width(c.Unit)-2 {
			break
		}
		r.Cell(txt, render.Width(txt), render.Left, s.Style)
		used += render.Width(txt)
	}
	if c.Unit != "" {
		r.Cell(" "+c.Unit, render.Width(c.Unit)+1, render.Left, th.Label)
	}
	r.Fill(" ", render.Style{})
	return r.String()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
