package state

import (
	"testing"
	"time"
)

// A4: a binding with no declared cadence can NEVER go stale. This is the
// default for event and binary_sensor entities -- a lightning event that has
// not fired in three days is correct, not stale.
func TestNoCadenceNeverGoesStale(t *testing.T) {
	v := Value{Kind: Valid, Seen: t0}
	got := v.Staleness(0, t0.Add(30*24*time.Hour))
	if got.Kind != Valid {
		t.Fatalf("Kind = %v after 30 days with no cadence, want valid", got.Kind)
	}
}

func TestStalenessAppliesWhenACadenceExists(t *testing.T) {
	v := Value{Kind: Valid, Seen: t0}
	if got := v.Staleness(time.Minute, t0.Add(30*time.Second)); got.Kind != Valid {
		t.Fatalf("Kind = %v within cadence, want valid", got.Kind)
	}
	if got := v.Staleness(time.Minute, t0.Add(2*time.Minute)); got.Kind != Stale {
		t.Fatalf("Kind = %v past cadence, want stale", got.Kind)
	}
}

// Staleness must not resurrect or mask a condition Home Assistant reported.
func TestStalenessLeavesNonValidKindsAlone(t *testing.T) {
	for _, k := range []Kind{Unknown, Unavailable, Fault} {
		v := Value{Kind: k, Seen: t0}
		if got := v.Staleness(time.Minute, t0.Add(time.Hour)); got.Kind != k {
			t.Errorf("Kind %v became %v", k, got.Kind)
		}
	}
}

// D42: only Valid and Stale may render as content. A guarded or faulted value
// rendering numerically is how "0.0 mi" comes to mean "lightning overhead".
func TestOnlyValidAndStaleAreUsable(t *testing.T) {
	want := map[Kind]bool{Valid: true, Stale: true, Unknown: false, Unavailable: false, Fault: false}
	for k, w := range want {
		if k.Usable() != w {
			t.Errorf("%v.Usable() = %v, want %v", k, k.Usable(), w)
		}
	}
}
