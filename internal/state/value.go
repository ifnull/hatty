// Package state owns the live mirror of Home Assistant.
//
// One goroutine writes it; everything else reads immutable snapshots (r3 §2).
// A Value is immutable once published: updates replace the pointer rather than
// mutating in place, which is the invariant that makes a snapshot a cheap
// shallow map instead of a deep copy (finding D3).
package state

import "time"

// Kind distinguishes the five value conditions, which must never collapse
// into a single "no data" case (r3 §3).
type Kind uint8

const (
	// Valid: a usable value.
	Valid Kind = iota
	// Unknown: Home Assistant has no value for this entity.
	Unknown
	// Unavailable: Home Assistant says the entity is unreachable. Also the
	// condition a guarded binding takes when its guard fails (D42).
	Unavailable
	// Stale: OUR judgement that an update is overdue. Home Assistant does not
	// report this. It is the condition that stops a frozen dashboard being
	// presented as live, which is the failure requirement N7 exists to prevent.
	Stale
	// Fault: the value arrived but did not parse as its declared type. Kept
	// separate from Unknown because their causes differ -- Unknown is Home
	// Assistant's state, Fault is our parser rejecting it, and rendering them
	// identically would hide our own bugs behind theirs.
	Fault
)

func (k Kind) String() string {
	switch k {
	case Valid:
		return "valid"
	case Unknown:
		return "unknown"
	case Unavailable:
		return "unavailable"
	case Stale:
		return "stale"
	case Fault:
		return "fault"
	}
	return "?"
}

// Usable reports whether the value may be rendered as its content. Everything
// else must render as an indicator, never as a number -- rendering a guarded
// or faulted value numerically is how "0.0 mi" comes to mean "lightning
// overhead" (D42).
func (k Kind) Usable() bool { return k == Valid || k == Stale }

// Value is one entity state, or one resolved attribute path.
//
// Immutable once published.
type Value struct {
	Kind Kind
	Str  string  // raw state as Home Assistant sent it; states are strings
	Num  float64 // only meaningful when Kind is Valid or Stale and the binding is numeric
	Attr any     // decoded attribute subtree, when the binding addresses one

	Changed time.Time // HA's last_changed
	Updated time.Time // HA's last_updated
	Seen    time.Time // when WE received it; drives Stale
}

// Staleness decides whether v has gone stale, given a cadence and a clock.
//
// A zero staleAfter means this binding has NO expected cadence and can never
// go stale (finding A4). That is the default for event and binary_sensor
// entities: a lightning event that has not fired for three days is correct,
// not stale, and marking it stale trains the reader to ignore the indicator --
// destroying the mechanism that protects N7.
func (v Value) Staleness(staleAfter time.Duration, now time.Time) Value {
	if staleAfter <= 0 || v.Kind != Valid || v.Seen.IsZero() {
		return v
	}
	if now.Sub(v.Seen) > staleAfter {
		v.Kind = Stale
	}
	return v
}
