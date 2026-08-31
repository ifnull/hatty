package model

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ifnull/hatty/internal/config"
	"github.com/ifnull/hatty/internal/render"
	"github.com/ifnull/hatty/internal/state"
	"github.com/ifnull/hatty/internal/widget"
)

// Indicators for the conditions that must never render as content.
//
// Each is a GLYPH, not a colour. D34 requires colour never to carry meaning
// alone -- for 8-colour terminals, for colour-blind readability, and because
// the client's depth is not guaranteed.
const (
	IndicatorUnavailable = "—"
	IndicatorUnknown     = "?"
	IndicatorFault       = "!"
)

// Cell renders a value for display, choosing the indicator when the value must
// not be shown as content.
func Cell(v state.Value, format string, th *widget.Theme) widget.Cell {
	switch v.Kind {
	case state.Unavailable:
		return widget.Cell{Text: IndicatorUnavailable, Style: th.Unavailable}
	case state.Unknown:
		return widget.Cell{Text: IndicatorUnknown, Style: th.Unavailable}
	case state.Fault:
		return widget.Cell{Text: IndicatorFault, Style: th.Alert}
	}
	st := th.Value
	if v.Kind == state.Stale {
		st = th.Stale
	}
	return widget.Cell{Text: Format(v, format), Style: st}
}

// Format applies a format directive to a value.
func Format(v state.Value, format string) string {
	switch {
	case format == "":
		return v.Str
	case format == "relative":
		return Relative(v)
	case format == "%,d":
		return Thousands(v.Num)
	case strings.HasPrefix(format, "%"):
		if strings.ContainsAny(format, "dxX") {
			return fmt.Sprintf(format, int64(math.Round(v.Num)))
		}
		return fmt.Sprintf(format, v.Num)
	default:
		return v.Str
	}
}

// Relative renders a timestamp as an age.
//
// Required by the lightning panel, whose `event` entity carries a timestamp
// rather than a scalar -- and whose age is currently three days, which is
// CORRECT rather than stale (A4).
func Relative(v state.Value) string {
	t, err := time.Parse(time.RFC3339, v.Str)
	if err != nil {
		if !v.Changed.IsZero() {
			t = v.Changed
		} else {
			return v.Str
		}
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dwk ago", int(d.Hours()/24/7))
	}
}

// Thousands groups an integer with commas.
func Thousands(f float64) string {
	s := strconv.FormatInt(int64(math.Round(f)), 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// RampStyle maps a value to a palette entry.
//
// Sequential, for continuous categories like altitude. Deliberately NOT a
// traffic light: red, amber and green mean STATE (D34), and spending the alarm
// hue on routine banding leaves the real alarm nothing to shout with.
func RampStyle(n float64, r *config.Ramp, th *widget.Theme, fallback render.Style) render.Style {
	if r == nil || len(r.Palette) != len(r.Thresholds)+1 {
		return fallback
	}
	idx := 0
	for _, t := range r.Thresholds {
		if n < t {
			break
		}
		idx++
	}
	return NamedStyle(r.Palette[idx], th, fallback)
}

// NamedStyle resolves a palette name from the theme.
func NamedStyle(name string, th *widget.Theme, fallback render.Style) render.Style {
	switch name {
	case "chrome":
		return th.Chrome
	case "label":
		return th.Label
	case "value":
		return th.Value
	case "muted":
		return th.Muted
	case "ok":
		return th.OK
	case "warn":
		return th.Warn
	case "alert":
		return th.Alert
	case "stale":
		return th.Stale
	case "alt0":
		return th.Alt[0]
	case "alt1":
		return th.Alt[1]
	case "alt2":
		return th.Alt[2]
	case "alt3":
		return th.Alt[3]
	}
	return fallback
}
