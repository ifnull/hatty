package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Errors is a collection of validation failures. All rules run, so a single
// load reports every problem rather than one at a time.
type Errors []error

func (e Errors) Error() string {
	b := make([]string, len(e))
	for i, err := range e {
		b[i] = "  - " + err.Error()
	}
	return fmt.Sprintf("%d configuration error(s):\n%s", len(e), strings.Join(b, "\n"))
}

// Load reads and validates a dashboard.
//
// Validation failures are FATAL at startup. A dashboard that silently drops a
// misconfigured panel is worse than one that refuses to start, because the
// missing panel looks like an absent condition rather than a broken config.
func Load(path string) (*Dashboard, error) {
	var d Dashboard
	if _, err := toml.DecodeFile(path, &d); err != nil {
		return nil, err
	}
	if errs := Validate(&d); len(errs) > 0 {
		return nil, errs
	}
	return &d, nil
}

// Validate applies every rule and returns all failures.
func Validate(d *Dashboard) Errors {
	var errs Errors
	add := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }

	if strings.TrimSpace(d.Name) == "" {
		add("dashboard has no name")
	}
	if d.Display.MinCols <= 0 || d.Display.MinRows <= 0 {
		add("display.min_cols and display.min_rows must be positive")
	}
	if d.Display.Cols < d.Display.MinCols || d.Display.Rows < d.Display.MinRows {
		add("display target %dx%d is smaller than the declared minimum %dx%d",
			d.Display.Cols, d.Display.Rows, d.Display.MinCols, d.Display.MinRows)
	}
	if g := d.Display.Glyphs; g != "" && g != "full" && g != "block" {
		add("display.glyphs = %q, want \"full\" or \"block\"", g)
	}
	if len(d.Panels) == 0 {
		add("dashboard has no panels")
	}

	elasticSeen := false
	for i, p := range d.Panels {
		where := fmt.Sprintf("panel[%d] (%s)", i, p.Type)
		if p.Type == "" {
			add("%s: missing type", where)
		}
		if p.Elastic > 0 {
			elasticSeen = true
		}
		for _, b := range panelBindings(p) {
			if _, err := ParseBinding(b); err != nil {
				add("%s: %v", where, err)
			}
		}
		// D4: template variables are declared bindings, not magic. Revision 1
		// allowed "{count} tracked" with count bound to nothing -- hard-coded
		// behaviour smuggled into the system meant to prevent it.
		for _, name := range templateVars(p.Nominal) {
			if _, ok := p.Vars[name]; !ok {
				add("%s: template variable {%s} is not declared in [panel.vars]", where, name)
			}
		}
		// E6: a guard whose referent is never subscribed can never pass, so the
		// guarded value never renders -- a silence indistinguishable from the
		// fault the guard was written to detect. The fix is to AUTO-ADD the
		// referent to the subscription set (see Subscriptions), so a guard
		// cannot disable what it guards. Only a malformed guard is a load
		// error; whether the entity exists in Home Assistant is a runtime
		// condition, resolving to Unavailable.
		for _, f := range p.Fields {
			if f.ValidWhen == "" {
				continue
			}
			if _, err := parseGuard(f.ValidWhen); err != nil {
				add("%s field %q: %v", where, f.Label, err)
			}
		}
		for _, dec := range p.Decisions {
			if _, err := ParseCondition(dec.When); err != nil {
				add("%s decision %q: %v", where, dec.Say, err)
			}
			if strings.TrimSpace(dec.Say) == "" {
				add("%s: a decision with no `say` states no action", where)
			}
		}
		for _, dec := range p.Decisions {
			if _, err := ParseCondition(dec.When); err != nil {
				add("%s decision %q: %v", where, dec.Say, err)
			}
			if strings.TrimSpace(dec.Say) == "" {
				add("%s: a decision with no `say` states no action", where)
			}
		}
		for _, c := range p.Columns {
			if c.Ramp != nil && len(c.Ramp.Palette) != len(c.Ramp.Thresholds)+1 {
				add("%s column %q: ramp has %d thresholds and %d palette entries, want %d",
					where, c.Header, len(c.Ramp.Thresholds), len(c.Ramp.Palette),
					len(c.Ramp.Thresholds)+1)
			}
		}
		if p.Type == "table" {
			if len(p.Binds) > 1 && p.Dedupe == "" {
				add("%s: unions %d collections but declares no dedupe key; "+
					"the same aircraft appears in several flag lists", where, len(p.Binds))
			}
			for _, e := range validateWidthBudget(p, d.Display.MinCols, where) {
				errs = append(errs, e)
			}
		}
	}

	// B1: elasticity must SURVIVE at the smallest grid, not merely exist at the
	// authoring size. Revision 2 validated existence and then let the solver
	// drop the only elastic panel.
	if !elasticSeen {
		add("no panel declares elastic > 0; leftover rows would be trapped (D37)")
	} else if !elasticSurvives(d) {
		add("no elastic panel survives at the declared minimum %dx%d (finding B1)",
			d.Display.MinCols, d.Display.MinRows)
	}
	return errs
}

// validateWidthBudget is the rule that replaces revision 1's vacuous one.
//
// FINDING B2: r1 required "min_cols monotonic with the drop order" -- but
// min_cols IS the drop order, so the rule compared a thing to itself and could
// never fail. The property that actually prevents a corrupted table is:
//
//	at every width where the active column set changes, the surviving
//	columns plus separators must FIT that width.
//
// Checkable, because the breakpoints are exactly the distinct min_cols values.
func validateWidthBudget(p Panel, floor int, where string) []error {
	if len(p.Columns) == 0 {
		return nil
	}
	const sep = 1 // one space between columns

	widths := map[int]bool{floor: true}
	for _, c := range p.Columns {
		if c.MinCols > 0 {
			widths[c.MinCols] = true
		}
	}
	points := make([]int, 0, len(widths))
	for w := range widths {
		points = append(points, w)
	}
	sort.Ints(points)

	var errs []error
	for _, w := range points {
		need, n := 0, 0
		var names []string
		for _, c := range p.Columns {
			if c.MinCols > w {
				continue
			}
			need += c.Width
			names = append(names, c.Header)
			n++
		}
		if n > 0 {
			need += (n - 1) * sep
		}
		if need > w {
			errs = append(errs, fmt.Errorf(
				"%s: at %d columns the surviving set %v needs %d cells -- it does not fit",
				where, w, names, need))
		}
	}
	return errs
}

// elasticSurvives reports whether some elastic panel is still present once
// panels that collapse at the minimum grid are removed.
func elasticSurvives(d *Dashboard) bool {
	for _, p := range d.Panels {
		if p.Elastic <= 0 {
			continue
		}
		// The detail panel collapses below 24 rows (D36); anything else with a
		// rank survives at any size.
		if p.Type == "detail" && d.Display.MinRows < 24 {
			continue
		}
		return true
	}
	return false
}

// Subscriptions returns every entity the dashboard must subscribe to.
//
// FINDING E6: this INCLUDES guard referents. A guard whose referent is not
// subscribed is permanently Unknown, so the guard never passes and the value it
// protects never renders -- silently, and looking exactly like the fault it
// exists to detect. Auto-adding makes that impossible rather than merely
// detectable.
func (d *Dashboard) Subscriptions() []string {
	seen := map[string]bool{}
	for _, p := range d.Panels {
		for _, b := range panelBindings(p) {
			if pb, err := ParseBinding(b); err == nil {
				seen[pb.Entity] = true
			}
		}
		for _, f := range p.Fields {
			if f.ValidWhen == "" {
				continue
			}
			if ref, err := parseGuard(f.ValidWhen); err == nil {
				seen[ref] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func panelBindings(p Panel) []string {
	var out []string
	if p.Bind != "" {
		out = append(out, p.Bind)
	}
	out = append(out, p.Binds...)
	if p.Source != "" {
		out = append(out, p.Source)
	}
	for _, v := range p.Vars {
		out = append(out, v)
	}
	for _, f := range p.Fields {
		if f.Bind != "" {
			out = append(out, f.Bind)
		}
	}
	for _, d := range p.Decisions {
		if c, err := ParseCondition(d.When); err == nil {
			out = append(out, c.Bind.String())
		}
	}
	return out
}

func templateVars(s string) []string {
	var out []string
	for {
		i := strings.IndexByte(s, '{')
		if i < 0 {
			return out
		}
		j := strings.IndexByte(s[i:], '}')
		if j < 0 {
			return out
		}
		if name := strings.TrimSpace(s[i+1 : i+j]); name != "" {
			out = append(out, name)
		}
		s = s[i+j+1:]
	}
}

// parseGuard reads `<binding> is available`. Deliberately not an expression
// language: it tests one binding's condition and nothing else (D25).
func parseGuard(s string) (string, error) {
	f := strings.Fields(strings.TrimSpace(s))
	if len(f) != 3 || f[1] != "is" || f[2] != "available" {
		return "", fmt.Errorf("valid_when must read `<binding> is available`, got %q", s)
	}
	b, err := ParseBinding(f[0])
	if err != nil {
		return "", err
	}
	return b.Entity, nil
}
