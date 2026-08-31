package render

import (
	"fmt"
	"strings"
)

// Align controls how a cell's content sits within its width.
type Align uint8

const (
	Left Align = iota
	Right
)

// Style is applied to a cell's visible content. Zero value is unstyled.
// Styling wraps content AFTER width is computed, so escape sequences can never
// affect the measured width -- which is how Phase 7 finding "width safety under
// composition" is closed structurally rather than by care.
type Style struct {
	FG   int // 256-colour index; 0 means unset
	BG   int // 256-colour index; 0 means unset
	Bold bool
	Dim  bool
}

func (s Style) wrap(text string) string {
	if s.FG == 0 && s.BG == 0 && !s.Bold && !s.Dim {
		return text
	}
	var b strings.Builder
	if s.Bold {
		b.WriteString("\x1b[1m")
	}
	if s.Dim {
		b.WriteString("\x1b[2m")
	}
	if s.FG != 0 {
		fmt.Fprintf(&b, "\x1b[38;5;%dm", s.FG)
	}
	if s.BG != 0 {
		fmt.Fprintf(&b, "\x1b[48;5;%dm", s.BG)
	}
	b.WriteString(text)
	b.WriteString("\x1b[0m")
	return b.String()
}

// Row assembles one terminal line of an exact declared width.
//
// It is the ONLY sanctioned way to build a rendered line (r3 §7). Widgets do
// not concatenate strings themselves, because a row assembled from correct
// cells plus a stray escape sequence still drifts -- and a drifted row corrupts
// the whole frame, not one character.
type Row struct {
	want  int
	used  int
	parts []string
}

// NewRow starts a row that must end up exactly w cells wide.
func NewRow(w int) *Row { return &Row{want: w, parts: make([]string, 0, 12)} }

// Cell appends content occupying exactly w cells, truncating or padding as
// needed. Style is applied after measurement.
func (r *Row) Cell(s string, w int, a Align, st Style) *Row {
	if w <= 0 {
		return r
	}
	if r.used+w > r.want {
		w = r.want - r.used // never overflow the row, whatever the caller asked for
		if w <= 0 {
			return r
		}
	}
	var text string
	if a == Right {
		text = PadLeft(s, w)
	} else {
		text = Pad(s, w)
	}
	r.parts = append(r.parts, st.wrap(text))
	r.used += w
	return r
}

// Gap appends w blank cells.
func (r *Row) Gap(w int) *Row { return r.Cell("", w, Left, Style{}) }

// Fill appends s repeated to occupy the remainder of the row.
func (r *Row) Fill(s string, st Style) *Row {
	if s == "" {
		s = " "
	}
	rem := r.want - r.used
	if rem <= 0 {
		return r
	}
	var b strings.Builder
	for Width(b.String()) < rem {
		b.WriteString(s)
	}
	return r.Cell(b.String(), rem, Left, st)
}

// String renders the row, padding any shortfall.
//
// In Strict mode (tests) a row that does not reach its declared width panics,
// so a layout bug fails loudly in CI. In production it pads, because a
// slightly-wrong frame beats a dead daemon.
func (r *Row) String() string {
	if r.used != r.want {
		if Strict {
			panic(fmt.Sprintf("render: row width %d, declared %d", r.used, r.want))
		}
		r.Gap(r.want - r.used)
	}
	return strings.Join(r.parts, "")
}
