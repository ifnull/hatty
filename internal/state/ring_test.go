package state

import (
	"math/rand"
	"testing"
	"time"
)

var t0 = time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

func at(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }

// A3: backfill and live updates must COMMUTE. Applying the same writes in any
// order must converge to the same buffer, or a statistics response landing
// after live samples makes the chart run backwards in time.
func TestPutIsOrderIndependent(t *testing.T) {
	writes := []Sample{
		{T: at(0), V: 1, Src: Live},
		{T: at(5), V: 2, Src: Live},
		{T: at(10), V: 3, Src: Live},
		{T: at(0), V: 10, Src: Stats, Rev: 1},
		{T: at(5), V: 20, Src: Stats, Rev: 1},
		{T: at(10), V: 30, Src: Stats, Rev: 1},
	}
	ref := NewRing(5*time.Minute, 12)
	for _, w := range writes {
		ref.Put(w)
	}
	want := ref.Series()

	for trial := 0; trial < 200; trial++ {
		shuffled := append([]Sample(nil), writes...)
		rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		got := NewRing(5*time.Minute, 12)
		for _, w := range shuffled {
			got.Put(w)
		}
		g, w := got.Series(), want
		if len(g) != len(w) {
			t.Fatalf("trial %d: %d samples, want %d", trial, len(g), len(w))
		}
		for i := range g {
			if g[i].V != w[i].V || !g[i].T.Equal(w[i].T) {
				t.Fatalf("trial %d: slot %d = %v, want %v", trial, i, g[i], w[i])
			}
		}
	}
}

// E3: statistics are authoritative over live samples for the same bucket.
func TestStatsBeatLive(t *testing.T) {
	r := NewRing(5*time.Minute, 12)
	r.Put(Sample{T: at(0), V: 1, Src: Live})
	r.Put(Sample{T: at(0), V: 99, Src: Stats, Rev: 1})
	if got := r.Series()[0].V; got != 99 {
		t.Fatalf("stats did not override live: got %v, want 99", got)
	}
	// ...and live must not claw the bucket back.
	r.Put(Sample{T: at(0), V: 2, Src: Live})
	if got := r.Series()[0].V; got != 99 {
		t.Fatalf("live overwrote stats: got %v, want 99", got)
	}
}

// E3: Home Assistant REVISES statistics. First-write-wins would discard the
// correction, and would make reconnect backfill a no-op because every bucket
// is already full.
func TestNewerStatsRevisionWins(t *testing.T) {
	r := NewRing(5*time.Minute, 12)
	r.Put(Sample{T: at(0), V: 10, Src: Stats, Rev: 1})
	r.Put(Sample{T: at(0), V: 11, Src: Stats, Rev: 2}) // a correction
	if got := r.Series()[0].V; got != 11 {
		t.Fatalf("correction discarded: got %v, want 11", got)
	}
	r.Put(Sample{T: at(0), V: 9, Src: Stats, Rev: 1}) // a stale refetch
	if got := r.Series()[0].V; got != 11 {
		t.Fatalf("stale revision won: got %v, want 11", got)
	}
}

// The reconnect case explicitly: a full refetch into a full buffer must land.
func TestReconnectBackfillIsNotANoOp(t *testing.T) {
	r := NewRing(5*time.Minute, 6)
	for i := 0; i < 6; i++ {
		r.Put(Sample{T: at(i * 5), V: float64(i), Src: Stats, Rev: 1})
	}
	for i := 0; i < 6; i++ {
		r.Put(Sample{T: at(i * 5), V: float64(i) + 100, Src: Stats, Rev: 2})
	}
	for i, s := range r.Series() {
		if s.V != float64(i)+100 {
			t.Fatalf("bucket %d = %v, want %v -- refetch was discarded", i, s.V, float64(i)+100)
		}
	}
}

// N4 / R8: capacity is fixed. Old buckets are evicted, never accumulated.
func TestCapacityIsBounded(t *testing.T) {
	const n = 8
	r := NewRing(time.Minute, n)
	for i := 0; i < 1000; i++ {
		r.Put(Sample{T: at(i), V: float64(i), Src: Live})
	}
	if got := r.Len(); got != n {
		t.Fatalf("retained %d buckets, want %d", got, n)
	}
	s := r.Series()
	if s[len(s)-1].V != 999 {
		t.Fatalf("newest sample = %v, want 999", s[len(s)-1].V)
	}
	if s[0].V != 992 {
		t.Fatalf("oldest retained = %v, want 992", s[0].V)
	}
}

func TestSamplesOlderThanTheWindowAreDropped(t *testing.T) {
	r := NewRing(time.Minute, 4)
	r.Put(Sample{T: at(100), V: 1, Src: Live})
	r.Put(Sample{T: at(0), V: 2, Src: Live}) // far outside the window
	for _, s := range r.Series() {
		if s.V == 2 {
			t.Fatal("accepted a sample older than the retained window")
		}
	}
}
