package config

import (
	"strings"
	"testing"
)

func TestLoadRealDashboard(t *testing.T) {
	d, err := Load("testdata/radar.toml")
	if err != nil {
		t.Fatalf("valid dashboard rejected:\n%v", err)
	}
	if d.Name != "radar" || d.Display.Cols != 100 || d.Display.Rows != 32 {
		t.Fatalf("unexpected load: %+v", d.Display)
	}
	if len(d.Panels) != 3 {
		t.Fatalf("%d panels, want 3", len(d.Panels))
	}
	if got := d.Panels[0].Hold.Duration.String(); got != "1m30s" {
		t.Errorf("hold = %s, want 1m30s", got)
	}
}

func mustFail(t *testing.T, d *Dashboard, want string) {
	t.Helper()
	errs := Validate(d)
	if len(errs) == 0 {
		t.Fatalf("expected a validation error containing %q, got none", want)
	}
	if !strings.Contains(errs.Error(), want) {
		t.Fatalf("errors did not mention %q:\n%v", want, errs)
	}
}

func base(t *testing.T) *Dashboard {
	t.Helper()
	d, err := Load("testdata/radar.toml")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// B2: the rule r1 got wrong. It required "min_cols monotonic with the drop
// order", but min_cols IS the drop order -- the rule compared a thing to
// itself and could never fail. This one can.
func TestWidthBudgetIsCheckedAtEveryBreakpoint(t *testing.T) {
	d := base(t)
	// Widen a column so the always-present set no longer fits the 44-cell floor.
	for i := range d.Panels[1].Columns {
		if d.Panels[1].Columns[i].Header == "FLIGHT" {
			d.Panels[1].Columns[i].Width = 40
		}
	}
	mustFail(t, d, "does not fit")
}

func TestWidthBudgetCatchesAWiderBreakpoint(t *testing.T) {
	d := base(t)
	// Fits at 44 and 56, but the set surviving at 80 overflows.
	for i := range d.Panels[1].Columns {
		if d.Panels[1].Columns[i].Header == "SQK" {
			d.Panels[1].Columns[i].Width = 60
		}
	}
	errs := Validate(d)
	if len(errs) == 0 || !strings.Contains(errs.Error(), "at 80 columns") {
		t.Fatalf("expected a failure at the 80-column breakpoint:\n%v", errs)
	}
}

// D4: template variables must be declared bindings, not magic resolved by
// widget-specific code.
func TestUndeclaredTemplateVariableIsRejected(t *testing.T) {
	d := base(t)
	d.Panels[0].Nominal = "{tracked} tracked · {undeclared} flagged"
	mustFail(t, d, "{undeclared} is not declared")
}

// E6: the fix is AUTO-ADDING guard referents, not rejecting them. A guard whose
// referent is not subscribed is permanently Unknown, so the guard never passes
// and the guarded value never renders -- silently, and looking exactly like the
// fault it exists to detect.
func TestGuardReferentsAreAutoSubscribed(t *testing.T) {
	d := base(t)
	subs := d.Subscriptions()
	want := "sensor.st_00128663_lightning_last_distance" // referenced ONLY by a guard
	found := false
	for _, s := range subs {
		if s == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("guard referent %s was not auto-subscribed; subscriptions = %v", want, subs)
	}
}

func TestSubscriptionsCoverEveryBindingKind(t *testing.T) {
	d := base(t)
	subs := d.Subscriptions()
	for _, want := range []string{
		"binary_sensor.airspace_alert_military_close",   // panel source
		"sensor.airspace_aircraft_count",                // declared template var
		"sensor.airspace_flag_military",                 // table bind
		"event.st_00128663_lightning_strike",            // detail field
		"sensor.st_00128663_lightning_average_distance", // guarded field
		"sensor.st_00128663_lightning_last_distance",    // guard referent
	} {
		found := false
		for _, s := range subs {
			if s == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s missing from subscriptions %v", want, subs)
		}
	}
}

func TestGuardSyntaxIsChecked(t *testing.T) {
	d := base(t)
	d.Panels[2].Fields[1].ValidWhen = "sensor.x > 3"
	mustFail(t, d, "valid_when must read")
}

// B1: elasticity must survive at the SMALLEST grid, not merely exist at the
// authoring size. The detail panel collapses below 24 rows (D36).
func TestElasticMustSurviveAtTheMinimumGrid(t *testing.T) {
	d := base(t)
	// Make the detail panel the only elastic one; it collapses at min_rows=12.
	for i := range d.Panels {
		if d.Panels[i].Type == "table" {
			d.Panels[i].Elastic = 0
		}
	}
	mustFail(t, d, "no elastic panel survives")
}

func TestNoElasticPanelAtAllIsRejected(t *testing.T) {
	d := base(t)
	for i := range d.Panels {
		d.Panels[i].Elastic = 0
	}
	mustFail(t, d, "leftover rows would be trapped")
}

func TestRampPaletteArityIsChecked(t *testing.T) {
	d := base(t)
	d.Panels[1].Columns[2].Ramp = &Ramp{
		Thresholds: []float64{10000, 20000, 30000},
		Palette:    []string{"alt0", "alt1"}, // want 4
	}
	mustFail(t, d, "want 4")
}

func TestMalformedBindingFailsAtLoad(t *testing.T) {
	d := base(t)
	d.Panels[1].Bind = "not_an_entity"
	mustFail(t, d, "domain.object_id")
}

// E10: silence must be opted into. A decision with no explicit safety flag is
// safety-relevant, so a stale forecast marks it rather than withholding it.
func TestSafetyDefaultsToTrue(t *testing.T) {
	if !(Decision{Say: "x"}).SafetyOrDefault() {
		t.Fatal("a decision with no safety flag must default to safety = true")
	}
	no := false
	if (Decision{Say: "x", Safety: &no}).SafetyOrDefault() {
		t.Fatal("safety = false was not honoured")
	}
}

// E7: hysteresis defaults to exact ordering, because the concern driving it is
// still unmeasured.
func TestSortHysteresisDefaultsToExact(t *testing.T) {
	d := base(t)
	if got := d.Panels[1].Sort.Hysteresis; got != 0 {
		t.Fatalf("hysteresis = %v, want 0 (exact ordering by default)", got)
	}
}

func TestAllErrorsAreReportedAtOnce(t *testing.T) {
	d := base(t)
	d.Name = ""
	d.Panels[1].Bind = "bad"
	d.Panels[0].Nominal = "{nope}"
	if errs := Validate(d); len(errs) < 3 {
		t.Fatalf("got %d errors, want at least 3 -- one load should report every problem:\n%v", len(errs), errs)
	}
}
