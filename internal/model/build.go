package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
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
		spec.Rule = true
		spec.MinRows = 3
		spec.NatRows = rows + 2 // header + the subset note (D41)
		// F1: a table cannot draw more rows than it has contacts, so it must
		// not absorb space it will render blank.
		spec.MaxRows = spec.NatRows
		return t, spec

	case "detail":
		d, n := buildDetail(p, r, th)
		spec.Rule, spec.Title = true, p.Follows
		spec.MinRows = 2
		spec.NatRows = n + 1
		spec.MaxRows = spec.NatRows // F1: bounded by the record, like the table
		return d, spec

	case "chart":
		c := buildChart(p, r, th)
		spec.MinRows = 3
		spec.NatRows = 8
		spec.Rule = true
		spec.Title = p.Follows
		spec.DropRank = 10 // D36: the trend is the first thing to go
		return c, spec

	case "decisions":
		d, n := buildDecisions(p, r, th)
		spec.MinRows = 1
		spec.NatRows = n
		spec.MaxRows = n
		return d, spec

	case "status_bar":
		spec.Reserve, spec.Rule = true, true
		return &widget.StatusBar{Left: p.Left, Keys: p.Keys}, spec
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
	binds := p.Binds
	if p.Bind != "" {
		binds = append([]string{p.Bind}, binds...)
	}
	if len(binds) == 0 {
		return t, 0
	}

	// F0: union the collections, deduplicating by the declared key. An
	// aircraft can be both military and interesting, and ha-airspace lists it
	// in both -- showing it twice would be wrong, and picking one collection
	// arbitrarily is what made the live screen look empty.
	var records []map[string]any
	seen := map[string]bool{}
	truncated := false
	for _, bs := range binds {
		b, err := config.ParseBinding(bs)
		if err != nil {
			continue
		}
		v := r.Resolve(b, 0)
		got := Rows(v)
		// The collection is capped at ten by ha-airspace while the entity
		// state carries the true count, so the two legitimately disagree (D41).
		if ent := r.Snap.Get(b.Entity); ent != nil {
			if n, err := strconv.Atoi(strings.TrimSpace(ent.State)); err == nil && n > len(got) {
				truncated = true
			}
		}
		for _, rec := range got {
			if p.Dedupe != "" {
				k, _ := rec[p.Dedupe].(string)
				if k != "" {
					if seen[k] {
						continue
					}
					seen[k] = true
				}
			}
			records = append(records, rec)
		}
	}
	t.Note = subsetNote(len(records), truncated, r)

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
	// A missing sort key is NOT zero.
	//
	// Verified against live data: most military contacts carry only
	// aircraft_type and hex -- Mode-S returns identified from the database but
	// not yet broadcasting position -- so distance_nm, bearing_deg, altitude
	// and flight are all null. Coercing nil to 0.0 (the obvious
	// `v, _ := m[key].(float64)`) sorts every positionless contact to the TOP,
	// presenting aircraft of unknown location as the nearest ones.
	//
	// That is the same class of error as rendering "0.0 mi" for absent
	// lightning: a plausible number standing in for no information.
	key := func(m map[string]any) (float64, bool) {
		v, ok := m[s.Key]
		if !ok || v == nil {
			return 0, false
		}
		f, ok := v.(float64)
		if !ok {
			return 0, false
		}
		if s.Hysteresis > 0 {
			return float64(int(f/s.Hysteresis)) * s.Hysteresis, true
		}
		return f, true
	}
	sort.SliceStable(recs, func(i, j int) bool {
		a, aOK := key(recs[i])
		b, bOK := key(recs[j])
		switch {
		case !aOK && !bOK:
			// Stable tiebreak on an unchanging field, so equal keys do not
			// shuffle between frames.
			ai, _ := recs[i]["hex"].(string)
			bj, _ := recs[j]["hex"].(string)
			return ai < bj
		case !aOK:
			return false // unknown position sorts LAST, never first
		case !bOK:
			return true
		case a == b:
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

// subsetNote states what the table is showing and what it is not.
//
// D41: the table can only ever show FLAGGED contacts, and the flag collections
// are capped at ten each, so both the subset and the cap must be visible.
// Silently showing ten of eleven military aircraft is the quiet wrongness this
// project exists to avoid.
func subsetNote(shown int, truncated bool, r Resolver) string {
	note := fmt.Sprintf("%d flagged", shown)
	if c := r.Snap.Get("sensor.airspace_aircraft_count"); c != nil && c.State != "" {
		note += " of " + c.State + " tracked"
	}
	if truncated {
		note += " · lists capped at 10 per flag"
	}
	return note
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

// buildDecisions evaluates each trigger.
//
// D43/D44: the MVP set is gusts now (Tempest), freezing tonight (Met.no daily
// templow, already in HA), and windy later. "Gusty later" is a fast-follow: no
// configured provider exposes wind_gust_speed in a forecast.
// buildChart assembles the overlaid series.
//
// D16: min, mean and max share one axis in ONE hue family, so they read as a
// single measurement rather than three unrelated things. The series come from
// the store's tracked rings, keyed by binding, so the widget stays pure over
// its view-model and never touches state.
func buildChart(p config.Panel, r Resolver, th *widget.Theme) *widget.RunChart {
	c := &widget.RunChart{Unit: p.Unit, Legend: true}
	for _, sp := range p.Series {
		pts := r.Snap.Series(sp.Bind)
		if len(pts) == 0 {
			continue
		}
		c.Series = append(c.Series, widget.Series{
			Name:   sp.Label,
			Points: pts,
			Style:  NamedStyle(sp.Color, th, th.Value),
		})
	}
	return c
}

// TrackedSeries returns the series a dashboard needs the store to accumulate,
// with the extractor for each. main wires these into the store; keeping the
// resolution here means `state` never learns the binding grammar.
func TrackedSeries(d *config.Dashboard, window time.Duration, points int) map[string]func(*state.Entity) (float64, bool) {
	out := map[string]func(*state.Entity) (float64, bool){}
	for _, p := range d.Panels {
		for _, sp := range p.Series {
			b, err := config.ParseBinding(sp.Bind)
			if err != nil {
				continue
			}
			bind := b
			out[sp.Bind] = func(e *state.Entity) (float64, bool) {
				if e.ID != bind.Entity {
					return 0, false
				}
				v := Resolver{Snap: state.NewSnapshotForTest(map[string]*state.Entity{e.ID: e}, time.Time{})}.
					Resolve(bind, 0)
				if !v.Kind.Usable() {
					return 0, false
				}
				return v.Num, true
			}
		}
	}
	return out
}

func buildDecisions(p config.Panel, r Resolver, th *widget.Theme) (*widget.Decisions, int) {
	d := &widget.Decisions{Nominal: p.Nominal}
	for _, dec := range p.Decisions {
		c, err := config.ParseCondition(dec.When)
		if err != nil {
			continue
		}
		v := r.Resolve(c.Bind, 0)

		// E10: a safety decision is NEVER suppressed for stale data. It renders
		// with its age shown, because a stale freeze warning beats none.
		if !v.Kind.Usable() {
			if v.Kind == state.Stale && dec.SafetyOrDefault() {
				// fall through and evaluate anyway
			} else {
				continue
			}
		}
		numeric := v.Str != "" || v.Num != 0
		if !c.Test(v.Num, v.Str, numeric) {
			continue
		}
		f := widget.Decision{Say: dec.Say, Level: dec.Level}
		if dec.Detail != "" {
			f.Detail = expand(dec.Detail, p.Vars, r, th)
		}
		if v.Kind == state.Stale {
			f.Stale = true
			f.Age = "data " + Relative(state.Value{Changed: v.Seen})
		}
		d.Fired = append(d.Fired, f)
	}
	n := len(d.Fired)
	if n == 0 {
		n = 1
	}
	return d, n
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
