package ha

import (
	"encoding/json"
	"math"
	"time"
)

// Compressed-state and entity-event keys, verified against
// homeassistant/const.py and websocket_api/messages.py (plan §7).
//
//	compressed state:  s=state  a=attributes  c=context  lc=last_changed  lu=last_updated
//	entity event:      a=add    c=change      r=remove
//	within a change:   +=additions  -=removals
const (
	keyState       = "s"
	keyAttributes  = "a"
	keyLastChanged = "lc"
	keyLastUpdated = "lu"

	evAdd    = "a"
	evChange = "c"
	evRemove = "r"

	diffAdd    = "+"
	diffRemove = "-"
)

// EntityState is the cached state of one entity.
//
// Attrs is owned by the cache and mutated only by Apply, which runs on the
// store's single writer goroutine. Callers receive it through an immutable
// snapshot (r3 §2 rule R-2).
type EntityState struct {
	ID          string
	State       string
	Attrs       map[string]any
	LastChanged time.Time
	LastUpdated time.Time
}

// Event is one decoded `subscribe_entities` event.
type Event struct {
	Add    map[string]json.RawMessage
	Change map[string]json.RawMessage
	Remove []string
}

// DecodeEvent parses the `event` object of a subscribe_entities message.
func DecodeEvent(raw json.RawMessage) (*Event, error) {
	var e struct {
		A map[string]json.RawMessage `json:"a"`
		C map[string]json.RawMessage `json:"c"`
		R json.RawMessage            `json:"r"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, err
	}
	ev := &Event{Add: e.A, Change: e.C}
	if len(e.R) > 0 {
		// `r` is a list of entity ids in practice; tolerate an object form too.
		if err := json.Unmarshal(e.R, &ev.Remove); err != nil {
			var m map[string]json.RawMessage
			if json.Unmarshal(e.R, &m) == nil {
				for k := range m {
					ev.Remove = append(ev.Remove, k)
				}
			}
		}
	}
	return ev, nil
}

func haTime(v any) time.Time {
	f, ok := v.(float64)
	if !ok || f == 0 || math.IsNaN(f) {
		return time.Time{}
	}
	sec, frac := math.Modf(f)
	return time.Unix(int64(sec), int64(frac*1e9)).UTC()
}

// applyAdd builds a fresh entity from a full compressed state.
func applyAdd(id string, raw json.RawMessage) (*EntityState, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	es := &EntityState{ID: id, Attrs: map[string]any{}}
	if s, ok := m[keyState].(string); ok {
		es.State = s
	}
	if a, ok := m[keyAttributes].(map[string]any); ok {
		for k, v := range a {
			es.Attrs[k] = v
		}
	}
	es.LastChanged = haTime(m[keyLastChanged])
	es.LastUpdated = haTime(m[keyLastUpdated])
	return es, nil
}

// ApplyChange merges a diff into an existing entity.
//
// FINDING D18 -- the two failure modes this exists to prevent, both silent:
//
//   - A change message carries ONLY the fields that changed. Treating it as a
//     whole state drops every attribute not mentioned; the dashboard then shows
//     an entity with no attributes and looks merely empty.
//   - The "-" key means attributes DISAPPEAR. Ignoring it retains attributes
//     Home Assistant has dropped, so the dashboard shows values that no longer
//     exist and looks perfectly plausible.
//
// Both render as stale-looking data rather than as an error, which is why they
// are specified here rather than left to the implementer.
func ApplyChange(cur *EntityState, raw json.RawMessage) error {
	if cur == nil {
		return nil
	}
	var d struct {
		Add map[string]any `json:"+"`
		// The trailing comma is REQUIRED and is not a typo. encoding/json
		// treats the tag `json:"-"` as "never encode this field", so a plain
		// `json:"-"` silently decodes to nil and every attribute removal is
		// dropped -- a data-loss bug with no error and no symptom except a
		// dashboard showing values Home Assistant deleted. `json:"-,"` means
		// "the field literally named -". See TestRemovalsActuallyRemove.
		Rem map[string]any `json:"-,"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return err
	}

	// Removals first: an attribute removed and re-added in one diff must end
	// up present.
	if rem, ok := d.Rem[keyAttributes]; ok {
		if names, ok := rem.([]any); ok {
			for _, n := range names {
				if s, ok := n.(string); ok {
					delete(cur.Attrs, s)
				}
			}
		}
	}

	for k, v := range d.Add {
		switch k {
		case keyState:
			if s, ok := v.(string); ok {
				cur.State = s
			}
		case keyAttributes:
			// MERGE by key. Replacing the map here is failure mode one.
			if a, ok := v.(map[string]any); ok {
				for ak, av := range a {
					cur.Attrs[ak] = av
				}
			}
		case keyLastChanged:
			cur.LastChanged = haTime(v)
		case keyLastUpdated:
			cur.LastUpdated = haTime(v)
		}
	}
	return nil
}

// Cache is the entity map maintained from a subscription.
type Cache struct{ m map[string]*EntityState }

func NewCache() *Cache { return &Cache{m: map[string]*EntityState{}} }

func (c *Cache) Get(id string) (*EntityState, bool) { es, ok := c.m[id]; return es, ok }
func (c *Cache) Len() int                           { return len(c.m) }

// IDs returns the cached entity ids, unordered.
func (c *Cache) IDs() []string {
	out := make([]string, 0, len(c.m))
	for k := range c.m {
		out = append(out, k)
	}
	return out
}

// Apply folds one event into the cache.
func (c *Cache) Apply(ev *Event) error {
	for id, raw := range ev.Add {
		es, err := applyAdd(id, raw)
		if err != nil {
			return err
		}
		c.m[id] = es
	}
	for id, raw := range ev.Change {
		cur, ok := c.m[id]
		if !ok {
			// A change for an entity we never saw added: ignore rather than
			// synthesise, so the cache never invents state.
			continue
		}
		if err := ApplyChange(cur, raw); err != nil {
			return err
		}
	}
	for _, id := range ev.Remove {
		delete(c.m, id)
	}
	return nil
}
