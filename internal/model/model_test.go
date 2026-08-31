package model

import (
	"strings"
	"testing"
	"time"

	"github.com/ifnull/hatty/internal/config"
	"github.com/ifnull/hatty/internal/state"
	"github.com/ifnull/hatty/internal/widget"
)

var now = time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

func snapOf(entities map[string]*state.Entity) *state.Snapshot {
	return state.NewSnapshotForTest(entities, now)
}

func bind(t *testing.T, s string) config.Binding {
	t.Helper()
	b, err := config.ParseBinding(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Every non-usable condition must render as an INDICATOR, never as content.
// The WeatherFlow audit is the reason: lightning_average_distance reported
// "0.0" while the vendor showed 23-25 mi, and a rendered 0.0 reads as lightning
// directly overhead.
func TestNonUsableConditionsNeverRenderContent(t *testing.T) {
	th := widget.Default()
	cases := []struct {
		name string
		ent  *state.Entity
		want string
	}{
		{"absent", nil, IndicatorUnavailable},
		{"unavailable", &state.Entity{ID: "sensor.x", State: "unavailable"}, IndicatorUnavailable},
		{"unknown", &state.Entity{ID: "sensor.x", State: "unknown"}, IndicatorUnknown},
		{"empty", &state.Entity{ID: "sensor.x", State: ""}, IndicatorUnknown},
	}
	for _, c := range cases {
		m := map[string]*state.Entity{}
		if c.ent != nil {
			m["sensor.x"] = c.ent
		}
		r := Resolver{Snap: snapOf(m), Now: now}
		got := Cell(r.Resolve(bind(t, "sensor.x"), 0), "%.1f", th)
		if got.Text != c.want {
			t.Errorf("%s: rendered %q, want the indicator %q", c.name, got.Text, c.want)
		}
		if strings.Contains(got.Text, "0.0") {
			t.Errorf("%s: rendered a number for a non-usable value: %q", c.name, got.Text)
		}
	}
}

// D42: a guarded value must not render numerically when its guard is
// unavailable -- exactly the lightning case.
func TestGuardSuppressesTheValueNotJustTheStyle(t *testing.T) {
	th := widget.Default()
	r := Resolver{Now: now, Snap: snapOf(map[string]*state.Entity{
		"sensor.lightning_average_distance": {ID: "sensor.lightning_average_distance", State: "0.0"},
		"sensor.lightning_last_distance":    {ID: "sensor.lightning_last_distance", State: "unavailable"},
	})}
	v := r.Guarded(bind(t, "sensor.lightning_average_distance"),
		"sensor.lightning_last_distance is available", 0)
	if v.Kind.Usable() {
		t.Fatalf("guarded value is usable despite an unavailable guard: %v", v.Kind)
	}
	if got := Cell(v, "%.1f", th).Text; got != IndicatorUnavailable {
		t.Errorf("guarded value rendered %q, want %q -- 0.0 mi reads as lightning overhead",
			got, IndicatorUnavailable)
	}

	// With the guard satisfied the value renders normally.
	r.Snap = snapOf(map[string]*state.Entity{
		"sensor.lightning_average_distance": {ID: "sensor.lightning_average_distance", State: "23.4"},
		"sensor.lightning_last_distance":    {ID: "sensor.lightning_last_distance", State: "23.4"},
	})
	v = r.Guarded(bind(t, "sensor.lightning_average_distance"),
		"sensor.lightning_last_distance is available", 0)
	if got := Cell(v, "%.1f", th).Text; got != "23.4" {
		t.Errorf("satisfied guard rendered %q, want 23.4", got)
	}
}

// A4: a zero cadence means "no cadence expected"; the value never goes stale.
func TestStalenessOnlyAppliesWhenACadenceIsDeclared(t *testing.T) {
	old := now.Add(-30 * 24 * time.Hour)
	r := Resolver{Now: now, Snap: snapOf(map[string]*state.Entity{
		"event.lightning": {ID: "event.lightning", State: "2026-08-01T00:00:00Z", Seen: old},
	})}
	if v := r.Resolve(bind(t, "event.lightning"), 0); v.Kind != state.Valid {
		t.Errorf("no declared cadence gave %v after 30 days, want valid", v.Kind)
	}
	if v := r.Resolve(bind(t, "event.lightning"), time.Minute); v.Kind != state.Stale {
		t.Errorf("declared cadence gave %v, want stale", v.Kind)
	}
}

func TestNestedAndCollectionBindings(t *testing.T) {
	r := Resolver{Now: now, Snap: snapOf(map[string]*state.Entity{
		"sensor.nearest": {ID: "sensor.nearest", State: "9.1", Attrs: map[string]any{
			"bearing_to": map[string]any{"home": 284.5},
		}},
		"sensor.flag": {ID: "sensor.flag", State: "3", Attrs: map[string]any{
			"aircraft": []any{
				map[string]any{"flight": "A", "distance_nm": 9.1},
				map[string]any{"flight": "B", "distance_nm": 12.6},
			},
		}},
	})}
	if v := r.Resolve(bind(t, "sensor.nearest:bearing_to.home"), 0); v.Num != 284.5 {
		t.Errorf("nested dict resolved to %v", v.Num)
	}
	rows := Rows(r.Resolve(bind(t, "sensor.flag:aircraft[]"), 0))
	if len(rows) != 2 {
		t.Fatalf("collection resolved to %d rows, want 2", len(rows))
	}
	nums := Numbers(r.Resolve(bind(t, "sensor.flag:aircraft[].distance_nm"), 0))
	if len(nums) != 2 || nums[0] != 9.1 {
		t.Errorf("projection gave %v", nums)
	}
}

func TestFormatting(t *testing.T) {
	for _, c := range []struct{ in, format, want string }{
		{"36000", "%,d", "36,000"},
		{"9.14159", "%.1f", "9.1"},
		{"284", "%d", "284"},
		{"hello", "", "hello"},
	} {
		v := state.Value{Kind: state.Valid, Str: c.in}
		if f, err := parseNum(c.in); err == nil {
			v.Num = f
		}
		if got := Format(v, c.format); got != c.want {
			t.Errorf("Format(%q, %q) = %q, want %q", c.in, c.format, got, c.want)
		}
	}
	if got := Thousands(-1234567); got != "-1,234,567" {
		t.Errorf("Thousands(-1234567) = %q", got)
	}
}

// D34: the altitude ramp is sequential, and must never reach for a state hue.
func TestAltitudeRampIsSequentialNotATrafficLight(t *testing.T) {
	th := widget.Default()
	ramp := &config.Ramp{
		Thresholds: []float64{10000, 20000, 30000},
		Palette:    []string{"alt0", "alt1", "alt2", "alt3"},
	}
	for _, c := range []struct {
		alt  float64
		want int
	}{{4600, th.Alt[0].FG}, {12000, th.Alt[1].FG}, {24000, th.Alt[2].FG}, {36000, th.Alt[3].FG}} {
		if got := RampStyle(c.alt, ramp, th, th.Value).FG; got != c.want {
			t.Errorf("altitude %v mapped to colour %d, want %d", c.alt, got, c.want)
		}
	}
	for _, alt := range []float64{4600, 12000, 24000, 36000} {
		got := RampStyle(alt, ramp, th, th.Value).FG
		for name, reserved := range map[string]int{"alert": th.Alert.FG, "warn": th.Warn.FG, "ok": th.OK.FG} {
			if got == reserved {
				t.Errorf("altitude %v used the reserved %q hue -- hues are reserved for state (D34)", alt, name)
			}
		}
	}
}

func parseNum(s string) (float64, error) {
	var f float64
	var err error
	f, err = strconvParseFloat(s)
	return f, err
}
