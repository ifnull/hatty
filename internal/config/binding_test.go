package config

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// E5: ONE grammar must cover every shape the real data actually uses --
// entities, nested watchpoint dicts, collections, projections, and the
// forecast pseudo-entity. Revision 2 had two grammars and documented one.
func TestGrammarCoversEveryRealShape(t *testing.T) {
	cases := []struct {
		in     string
		entity string
		segs   int
		coll   bool
	}{
		{"sensor.airspace_aircraft_count", "sensor.airspace_aircraft_count", 0, false},
		{"binary_sensor.airspace_alert_military_close", "binary_sensor.airspace_alert_military_close", 0, false},
		{"event.st_00128663_lightning_strike", "event.st_00128663_lightning_strike", 0, false},
		{"sensor.airspace_nearest_aircraft:bearing_to.home", "sensor.airspace_nearest_aircraft", 2, false},
		{"sensor.airspace_nearest_aircraft:db_metadata.ownop", "sensor.airspace_nearest_aircraft", 2, false},
		{"sensor.airspace_flag_military:aircraft[]", "sensor.airspace_flag_military", 1, true},
		{"sensor.airspace_flag_military:aircraft[].distance_nm", "sensor.airspace_flag_military", 2, true},
		{"forecast.home:daily[0].templow", "forecast.home", 2, false},
		{"forecast.home:hourly[3].precipitation", "forecast.home", 2, false},
	}
	for _, c := range cases {
		b, err := ParseBinding(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if b.Entity != c.entity {
			t.Errorf("%s: entity = %q, want %q", c.in, b.Entity, c.entity)
		}
		if len(b.Path) != c.segs {
			t.Errorf("%s: %d segments, want %d", c.in, len(b.Path), c.segs)
		}
		if b.IsCollection() != c.coll {
			t.Errorf("%s: IsCollection = %v, want %v", c.in, b.IsCollection(), c.coll)
		}
		if b.String() != c.in {
			t.Errorf("%s: String() = %q", c.in, b.String())
		}
	}
}

func TestMalformedBindingsAreRejected(t *testing.T) {
	for _, in := range []string{
		"", "   ", "nodot", "sensor.", ".object", "sensor.x:", "sensor.x:.",
		"sensor.x:a[", "sensor.x:a]", "sensor.x:[0]", "sensor.x:a[-1]", "sensor.x:a[abc]",
	} {
		if _, err := ParseBinding(in); err == nil {
			t.Errorf("%q was accepted; it should be a load-time error", in)
		}
	}
}

func tree(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestResolveNestedDict(t *testing.T) {
	root := tree(t, `{"bearing_to":{"home":284.5},"distance_to":{"home":9.1}}`)
	b, _ := ParseBinding("sensor.x:bearing_to.home")
	got, err := b.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != 284.5 {
		t.Fatalf("got %v, want 284.5", got)
	}
}

func TestResolveIndexedElement(t *testing.T) {
	root := tree(t, `{"daily":[{"templow":31},{"templow":45}]}`)
	b, _ := ParseBinding("forecast.home:daily[0].templow")
	got, err := b.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != float64(31) {
		t.Fatalf("got %v, want 31", got)
	}
}

func TestResolveProjectsAcrossACollection(t *testing.T) {
	root := tree(t, `{"aircraft":[{"distance_nm":9.1},{"distance_nm":12.6},{"distance_nm":18.3}]}`)
	b, _ := ParseBinding("sensor.x:aircraft[].distance_nm")
	got, err := b.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{9.1, 12.6, 18.3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A row missing a field must show NOTHING there. Substituting a zero is how
// "0.0 mi" comes to mean "lightning overhead" (D42) -- the same failure in a
// different place.
func TestProjectionSkipsMissingFieldsRatherThanZeroing(t *testing.T) {
	root := tree(t, `{"aircraft":[{"squawk":"1200"},{"flight":"X"},{"squawk":"7700"}]}`)
	b, _ := ParseBinding("sensor.x:aircraft[].squawk")
	got, err := b.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"1200", "7700"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v -- a missing field must be skipped, never zeroed", got, want)
	}
}

func TestResolveMissingPathIsNotFound(t *testing.T) {
	root := tree(t, `{"a":1}`)
	b, _ := ParseBinding("sensor.x:nope.deeper")
	if _, err := b.Resolve(root); err == nil {
		t.Fatal("expected ErrNotFound")
	} else if _, ok := err.(ErrNotFound); !ok {
		t.Fatalf("got %T, want ErrNotFound", err)
	}
}

// The audit found lightning_average_distance reporting a NUMBER where the
// truth was unavailable. A path that expects a collection and finds a scalar
// must error, not coerce.
func TestCollectionPathOnAScalarErrors(t *testing.T) {
	root := tree(t, `{"aircraft":"unavailable"}`)
	b, _ := ParseBinding("sensor.x:aircraft[].distance_nm")
	if _, err := b.Resolve(root); err == nil {
		t.Fatal("expected an error when a collection path meets a scalar")
	}
}

// Resolve every binding the real dashboards need against the real capture.
func TestResolveAgainstLiveCapture(t *testing.T) {
	raw, err := os.ReadFile("../ha/testdata/frames-busy.jsonl")
	if err != nil {
		t.Skip("fixture missing")
	}
	// The first line is the initial "add" carrying full state for every entity.
	var rec struct {
		E struct {
			A map[string]struct {
				S string         `json:"s"`
				A map[string]any `json:"a"`
			} `json:"a"`
		} `json:"e"`
	}
	first := raw
	if i := indexByte(raw, '\n'); i > 0 {
		first = raw[:i]
	}
	if err := json.Unmarshal(first, &rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.E.A) == 0 {
		t.Skip("first frame is not an add")
	}
	for _, bind := range []string{
		"sensor.airspace_flag_military:aircraft[].distance_nm",
		"sensor.airspace_flag_military:aircraft[].bearing_deg",
		"sensor.airspace_flag_military:aircraft[].flight",
		"sensor.airspace_nearest_aircraft:bearing_to.home",
	} {
		b, err := ParseBinding(bind)
		if err != nil {
			t.Fatalf("%s: %v", bind, err)
		}
		ent, ok := rec.E.A[b.Entity]
		if !ok {
			continue // entity not in this capture
		}
		got, err := b.Resolve(any(ent.A))
		if err != nil {
			t.Errorf("%s: %v", bind, err)
			continue
		}
		if l, ok := got.([]any); ok && len(l) == 0 {
			t.Errorf("%s resolved to an empty collection", bind)
		}
		t.Logf("%s -> %T", bind, got)
	}
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}
