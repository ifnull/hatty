package model

import (
	"sort"
	"time"

	"github.com/ifnull/hatty/internal/config"
	"github.com/ifnull/hatty/internal/layout"
	"github.com/ifnull/hatty/internal/render"
	"github.com/ifnull/hatty/internal/state"
	"github.com/ifnull/hatty/internal/widget"
)

// Screen is a dashboard resolved against one snapshot: the widgets and the
// layout specs that place them, ready to render.
type Screen struct {
	Specs   []layout.Spec
	Widgets []widget.Widget
}

// Build resolves a dashboard against a snapshot.
func Build(d *config.Dashboard, snap *state.Snapshot, th *widget.Theme, now time.Time) *Screen {
	r := Resolver{Snap: snap, Now: now}
	sc := &Screen{}
	for _, p := range d.Panels {
		w, spec := buildPanel(p, r, th)
		if w == nil {
			continue
		}
		sc.Specs = append(sc.Specs, spec)
		sc.Widgets = append(sc.Widgets, w)
	}
	return sc
}

func buildPanel(p config.Panel, r Resolver, th *widget.Theme) (widget.Widget, layout.Spec) {
	spec := layout.Spec{
		Name:     p.Type,
		Reserve:  p.Reserve,
		Elastic:  p.Elastic,
		MinRows:  1,
		NatRows:  1,
		DropRank: dropRank(p.Type),
	}
	switch p.Type {
	case "alert_strip":
		return buildAlertStrip(p, r, th), spec

	case "table":
		t, rows := buildTable(p, r, th)
		spec.MinRows = 3
		spec.NatRows = rows + 1
		if p.Bind != "" {
			spec.NatRows++ // the subset note (D41)
		}
		return t, spec

	case "detail":
		d, n := buildDetail(p, r, th)
		spec.MinRows = 2
		spec.NatRows = n + 1
		return d, spec

	case "status_bar":
		spec.Reserve = true
		return &widget.StatusBar{Left: p.Source, Keys: p.Nominal}, spec
	}
	return nil, spec
}

// dropRank encodes D36's order: the trend goes first, then the detail pane,
// then status hints. The table is the content and outlives them.
func dropRank(typ string) int {
	switch typ {
	case "trend", "runchart":
		return 10
	case "detail":
		return 20
	case "table":
		return 30
	}
	return 40
}

func buildAlertStrip(p config.Panel, r Resolver, th *widget.Theme) *widget.AlertStrip {
	a := &widget.AlertStrip{Nominal: expand(p.Nominal, p.Vars, r, th)}
	if p.Source == "" {
		return a
	}
	b, err := config.ParseBinding(p.Source)
	if err != nil {
		return a
	}
	v := r.Resolve(b, 0)
	if v.Kind.Usable() && (v.Str == "on" || v.Str == "true") {
		a.Active = true
		a.Message = expand(p.Nominal, p.Vars, r, th)
		if ent := r.Snap.Get(b.Entity); ent != nil {
			if s, ok := ent.Attrs["friendly_name"].(string); ok {
				a.Message = s
			}
		}
	}
	return a
}

// expand substitutes declared template variables. D4: every {name} must be a
// declared binding, checked at load, so no widget-specific magic resolves them.
func expand(tmpl string, vars map[string]string, r Resolver, th *widget.Theme) string {
	if tmpl == "" {
		return ""
	}
	out := tmpl
	for name, bind := range vars {
		b, err := config.ParseBinding(bind)
		if err != nil {
			continue
		}
		out = replaceAll(out, "{"+name+"}", Cell(r.Resolve(b, 0), "", th).Text)
	}
	return out
}

func replaceAll(s, old, new string) string {
	for {
		i := indexOf(s, old)
		if i < 0 {
			return s
		}
		s = s[:i] + new + s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func buildTable(p config.Panel, r Resolver, th *widget.Theme) (*widget.Table, int) {
	t := &widget.Table{}
	for _, c := range p.Columns {
		al := render.Left
		if c.Align == "right" {
			al = render.Right
		}
		t.Columns = append(t.Columns, widget.TableColumn{
			Column: layout.Column{Name: c.Header, Width: c.Width, MinCols: c.MinCols},
			Align:  al,
		})
	}
	if p.Bind == "" {
		return t, 0
	}
	b, err := config.ParseBinding(p.Bind)
	if err != nil {
		return t, 0
	}
	v := r.Resolve(b, 0)
	records := Rows(v)

	if p.Sort != nil && p.Sort.Key != "" {
		sortRecords(records, p.Sort)
	}
	for _, rec := range records {
		row := make([]widget.Cell, 0, len(p.Columns))
		for _, c := range p.Columns {
			row = append(row, recordCell(rec, c, th))
		}
		t.Rows = append(t.Rows, row)
	}
	return t, len(records)
}

// sortRecords orders rows. Hysteresis defaults to zero -- EXACT ordering.
//
// FINDING E7: bucketing trades sort correctness for stability, so two contacts
// in one bucket can display out of order under a header saying "sorted by
// distance". Revision 2 presented that as free. It is opt-in until the SSH-link
// bandwidth that motivates it has actually been measured.
func sortRecords(recs []map[string]any, s *config.Sort) {
	key := func(m map[string]any) float64 {
		f, _ := m[s.Key].(float64)
		if s.Hysteresis > 0 {
			return float64(int(f/s.Hysteresis)) * s.Hysteresis
		}
		return f
	}
	sort.SliceStable(recs, func(i, j int) bool {
		a, b := key(recs[i]), key(recs[j])
		if a == b {
			// A stable tiebreak on an unchanging field, so equal keys do not
			// shuffle between frames.
			ai, _ := recs[i]["hex"].(string)
			bj, _ := recs[j]["hex"].(string)
			return ai < bj
		}
		if s.Dir == "desc" {
			return a > b
		}
		return a < b
	})
}

func recordCell(rec map[string]any, c config.Column, th *widget.Theme) widget.Cell {
	raw, ok := rec[c.Path]
	if !ok || raw == nil {
		return widget.Cell{Text: IndicatorUnavailable, Style: th.Unavailable}
	}
	v := state.Value{Kind: state.Valid}
	switch t := raw.(type) {
	case float64:
		v.Num, v.Str = t, Format(state.Value{Num: t}, c.Format)
	case string:
		v.Str = t
	case bool:
		v.Str = boolStr(t)
	default:
		v.Str = IndicatorUnavailable
	}
	st := th.Value
	if c.Ramp != nil {
		st = RampStyle(v.Num, c.Ramp, th, th.Value)
	}
	text := v.Str
	if _, isNum := raw.(float64); isNum && c.Format != "" {
		text = Format(v, c.Format)
	}
	return widget.Cell{Text: text, Style: st}
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func buildDetail(p config.Panel, r Resolver, th *widget.Theme) (*widget.Detail, int) {
	d := &widget.Detail{Title: p.Follows}
	for _, f := range p.Fields {
		b, err := config.ParseBinding(f.Bind)
		if err != nil {
			continue
		}
		v := r.Guarded(b, f.ValidWhen, 0)
		d.Fields = append(d.Fields, widget.DetailField{
			Label: f.Label,
			Value: Cell(v, f.Format, th),
		})
	}
	n := (len(d.Fields) + 1) / 2
	return d, n
}
