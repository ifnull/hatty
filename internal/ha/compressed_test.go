package ha

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
)

func ev(t *testing.T, s string) *Event {
	t.Helper()
	e, err := DecodeEvent(json.RawMessage(s))
	if err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return e
}

// D18, failure mode one: a change carries only what changed. Treating it as a
// whole state drops every attribute not mentioned -- silently, and the result
// looks merely empty rather than wrong.
func TestChangeMustNotDropUnmentionedAttributes(t *testing.T) {
	c := NewCache()
	if err := c.Apply(ev(t, `{"a":{"sensor.x":{"s":"1","a":{"unit":"nm","friendly_name":"X","icon":"mdi:a"},"lc":1788200771.5,"lu":1788200771.5}}}`)); err != nil {
		t.Fatal(err)
	}
	// A state-only change.
	if err := c.Apply(ev(t, `{"c":{"sensor.x":{"+":{"s":"2","lu":1788200800.0}}}}`)); err != nil {
		t.Fatal(err)
	}
	got, _ := c.Get("sensor.x")
	if got.State != "2" {
		t.Fatalf("state = %q, want 2", got.State)
	}
	for _, k := range []string{"unit", "friendly_name", "icon"} {
		if _, ok := got.Attrs[k]; !ok {
			t.Errorf("attribute %q was dropped by a state-only change", k)
		}
	}
}

// D18, failure mode two: "-" means attributes DISAPPEAR. Ignoring it retains
// values Home Assistant has dropped, and the dashboard looks plausible.
func TestRemovalsActuallyRemove(t *testing.T) {
	c := NewCache()
	c.Apply(ev(t, `{"a":{"sensor.x":{"s":"1","a":{"keep":"y","drop":"z"}}}}`))
	c.Apply(ev(t, `{"c":{"sensor.x":{"-":{"a":["drop"]}}}}`))
	got, _ := c.Get("sensor.x")
	if _, ok := got.Attrs["drop"]; ok {
		t.Error(`attribute "drop" survived a removal`)
	}
	if _, ok := got.Attrs["keep"]; !ok {
		t.Error(`removal took "keep" with it`)
	}
}

// An attribute removed and re-added in the same diff must end up present.
func TestRemoveThenAddInOneDiff(t *testing.T) {
	c := NewCache()
	c.Apply(ev(t, `{"a":{"sensor.x":{"s":"1","a":{"v":"old"}}}}`))
	c.Apply(ev(t, `{"c":{"sensor.x":{"+":{"a":{"v":"new"}},"-":{"a":["v"]}}}}`))
	got, _ := c.Get("sensor.x")
	if got.Attrs["v"] != "new" {
		t.Fatalf(`attrs["v"] = %v, want "new"`, got.Attrs["v"])
	}
}

func TestRemoveDeletesEntity(t *testing.T) {
	c := NewCache()
	c.Apply(ev(t, `{"a":{"sensor.x":{"s":"1"}}}`))
	c.Apply(ev(t, `{"r":["sensor.x"]}`))
	if _, ok := c.Get("sensor.x"); ok {
		t.Fatal("entity survived removal")
	}
}

// The cache must never invent state for an entity it never saw added.
func TestChangeForUnknownEntityIsIgnored(t *testing.T) {
	c := NewCache()
	if err := c.Apply(ev(t, `{"c":{"sensor.ghost":{"+":{"s":"1"}}}}`)); err != nil {
		t.Fatal(err)
	}
	if c.Len() != 0 {
		t.Fatalf("cache invented %d entities", c.Len())
	}
}

// Replay the real capture. Shapes no hand-written mock would produce:
// list-valued attributes, nested watchpoint dicts, unavailable states,
// truncated collections.
func TestReplayLiveCapture(t *testing.T) {
	f, err := os.Open("testdata/frames-busy.jsonl")
	if err != nil {
		t.Skip("fixture missing")
	}
	defer f.Close()

	c := NewCache()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	frames, changes := 0, 0
	for sc.Scan() {
		var rec struct {
			E json.RawMessage `json:"e"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("frame %d: %v", frames, err)
		}
		e, err := DecodeEvent(rec.E)
		if err != nil {
			t.Fatalf("frame %d: %v", frames, err)
		}
		changes += len(e.Change)
		if err := c.Apply(e); err != nil {
			t.Fatalf("frame %d: %v", frames, err)
		}
		frames++
	}
	if frames == 0 {
		t.Fatal("fixture contained no frames")
	}
	if changes == 0 {
		t.Fatal("fixture contained no change diffs -- it is not exercising the merge path")
	}
	t.Logf("replayed %d frames (%d change diffs), %d entities cached", frames, changes, c.Len())

	// After replaying real traffic every cached entity must still carry the
	// attributes it was added with. This is the regression the merge path
	// exists to prevent, checked against real diffs rather than synthetic ones.
	empty := 0
	for _, id := range c.IDs() {
		es, _ := c.Get(id)
		if len(es.Attrs) == 0 {
			empty++
			t.Errorf("%s lost all attributes during replay", id)
		}
	}
	if empty > 0 {
		t.Fatalf("%d entities were emptied by the merge path", empty)
	}

	// The flag collections must still hold their list-valued attribute.
	if es, ok := c.Get("sensor.airspace_flag_military"); ok {
		if _, ok := es.Attrs["aircraft"].([]any); !ok {
			t.Errorf("flag_military lost its aircraft list: %T", es.Attrs["aircraft"])
		}
	}
}
