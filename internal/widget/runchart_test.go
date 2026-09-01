package widget

import (
	"math"
	"strings"
	"testing"

	"github.com/ifnull/hatty/internal/render"
)

func wave(n int, amp, off float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = off + amp*math.Sin(float64(i)/float64(n)*4*math.Pi)
	}
	return out
}

func windChart(th *Theme) *RunChart {
	return &RunChart{
		Unit:   "mph",
		Legend: true,
		Series: []Series{
			{Name: "max", Points: wave(48, 3, 9), Style: render.Style{FG: 110}},
			{Name: "mean", Points: wave(48, 2, 5), Style: render.Style{FG: 74}},
			{Name: "min", Points: wave(48, 1, 1), Style: render.Style{FG: 66}},
		},
	}
}

// The widget contract, at every verified breakpoint and in BOTH glyph tiers.
func TestRunChartFillsItsRegionExactly(t *testing.T) {
	for _, glyphs := range []string{"full", "block"} {
		th := Default()
		th.Glyphs = glyphs
		c := windChart(th)
		for _, g := range []struct{ w, h int }{
			{98, 8}, {98, 4}, {78, 6}, {62, 5}, {48, 3}, {42, 1},
		} {
			lines := c.Render(g.w, g.h, th)
			if len(lines) != g.h {
				t.Errorf("%s %dx%d: %d lines, want %d", glyphs, g.w, g.h, len(lines), g.h)
				continue
			}
			for i, l := range lines {
				if n := render.Width(vis(l)); n != g.w {
					t.Errorf("%s %dx%d: line %d is %d cells: %q", glyphs, g.w, g.h, i, n, vis(l))
				}
			}
		}
	}
}

// D23/S2: Braille is used when available, and NOT when the glyph tier says the
// client cannot render it -- a bare framebuffer console caps its font at
// 256/512 glyphs, so Braille cannot fit alongside everything else.
func TestGlyphTierSelectsTheImplementation(t *testing.T) {
	th := Default()
	c := windChart(th)

	th.Glyphs = "full"
	full := strings.Join(c.Render(80, 6, th), "")
	if !strings.ContainsFunc(vis(full), func(r rune) bool { return r >= 0x2800 && r <= 0x28FF }) {
		t.Error("full tier drew no Braille")
	}

	th.Glyphs = "block"
	block := vis(strings.Join(c.Render(80, 6, th), ""))
	if strings.ContainsFunc(block, func(r rune) bool { return r >= 0x2800 && r <= 0x28FF }) {
		t.Error("block tier drew Braille, which a console font cannot render")
	}
	if !strings.ContainsAny(block, "▁▂▃▄▅▆▇█") {
		t.Error("block tier drew no block elements either")
	}
}

// Braille gives 4x the vertical resolution of block elements. A chart that
// cannot distinguish two nearby values is a staircase, not a curve.
func TestBrailleResolvesFinerThanBlocks(t *testing.T) {
	th := Default()
	th.Glyphs = "full"
	fine := &RunChart{Series: []Series{{Name: "a", Points: []float64{0, 0.1, 0.2, 0.3, 0.4}}}}
	rows := fine.Render(40, 2, th)
	distinct := map[rune]bool{}
	for _, l := range rows {
		for _, r := range vis(l) {
			if r >= 0x2800 && r <= 0x28FF {
				distinct[r] = true
			}
		}
	}
	if len(distinct) < 3 {
		t.Errorf("only %d distinct Braille patterns; a gentle slope should produce several", len(distinct))
	}
}

// D16: three series share the axis, and each keeps its own hue so they read as
// one measurement rather than three unrelated things.
func TestOverlaidSeriesKeepTheirOwnStyle(t *testing.T) {
	th := Default()
	out := strings.Join(windChart(th).Render(90, 8, th), "")
	for _, fg := range []string{"38;5;110", "38;5;74", "38;5;66"} {
		if !strings.Contains(out, fg) {
			t.Errorf("series colour %s is missing; the overlay lost a series", fg)
		}
	}
}

func TestLegendShowsCurrentValuesAndUnit(t *testing.T) {
	th := Default()
	got := vis(windChart(th).Render(90, 8, th)[7])
	for _, want := range []string{"max", "mean", "min", "mph"} {
		if !strings.Contains(got, want) {
			t.Errorf("legend missing %q: %q", want, got)
		}
	}
}

// Degenerate inputs must not panic or misdraw: an empty series, a single point,
// and a flat line all have zero span.
func TestDegenerateSeriesAreSafe(t *testing.T) {
	th := Default()
	for name, c := range map[string]*RunChart{
		"empty":  {Series: []Series{{Name: "a"}}},
		"single": {Series: []Series{{Name: "a", Points: []float64{5}}}},
		"flat":   {Series: []Series{{Name: "a", Points: []float64{3, 3, 3, 3}}}},
		"none":   {},
	} {
		for _, glyphs := range []string{"full", "block"} {
			th.Glyphs = glyphs
			lines := c.Render(60, 4, th)
			if len(lines) != 4 {
				t.Errorf("%s/%s: %d lines, want 4", name, glyphs, len(lines))
			}
			for _, l := range lines {
				if n := render.Width(vis(l)); n != 60 {
					t.Errorf("%s/%s: line is %d cells", name, glyphs, n)
				}
			}
		}
	}
}
