// Package widget renders panels.
//
// Widgets are PURE over their view-model: they never touch state, never
// perform I/O, and never know which entity produced a number. That is what
// makes the whole rendering path testable without a live Home Assistant, and
// it is the contract an adversarial review should try hardest to violate.
package widget

import "github.com/ifnull/hatty/internal/render"

// Theme is the D34 palette.
//
// The rule that shapes it: HUES ARE RESERVED. Red, amber and green mean STATE
// and nothing else. Altitude is a continuous category, not a severity, so it
// gets a sequential blue-cyan ramp -- the existing Lovelace dashboard colours
// low altitude red, which is harmless on a card you read and wrong on a screen
// you glance at, because spending the alarm hue on routine banding leaves the
// real alarm nothing to shout with.
//
// Every colour here is a MUTED variant. This runs 24/7 at arm's length on a
// desk; full-intensity red glares and leaves afterimages, while index 167 still
// reads as alarming.
type Theme struct {
	Chrome, Label, Value, Title, Muted render.Style
	OK, Warn, Alert                    render.Style
	Stale, Unavailable                 render.Style
	Selected                           render.Style
	AlertBanner                        render.Style
	Alt                                [4]render.Style

	// Glyphs selects the visual vocabulary. "block" drops Braille, which a
	// bare framebuffer console cannot render -- the tables are unaffected
	// either way (D33).
	Glyphs string
}

// Default is the verified palette. Spike S2 confirmed 256 colours on the panel.
func Default() *Theme {
	return &Theme{
		Chrome:      render.Style{FG: 238},
		Label:       render.Style{FG: 245},
		Value:       render.Style{FG: 252},
		Title:       render.Style{FG: 255, Bold: true},
		Muted:       render.Style{FG: 240},
		OK:          render.Style{FG: 71},
		Warn:        render.Style{FG: 179},
		Alert:       render.Style{FG: 167},
		Stale:       render.Style{FG: 96},
		Unavailable: render.Style{FG: 240},
		Selected:    render.Style{BG: 236},
		AlertBanner: render.Style{FG: 224, BG: 52, Bold: true},
		Alt: [4]render.Style{
			{FG: 60}, {FG: 67}, {FG: 74}, {FG: 81},
		},
		Glyphs: "full",
	}
}

// AltFor maps an altitude to its ramp entry. Sequential, deliberately not a
// traffic light.
func (t *Theme) AltFor(ft float64) render.Style {
	switch {
	case ft < 10000:
		return t.Alt[0]
	case ft < 20000:
		return t.Alt[1]
	case ft < 30000:
		return t.Alt[2]
	default:
		return t.Alt[3]
	}
}

// Braille reports whether sub-cell plotting is available.
func (t *Theme) Braille() bool { return t.Glyphs != "block" }
