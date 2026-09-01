package state

import (
	"context"
	"sync"
	"time"

	"github.com/ifnull/hatty/internal/ha"
)

// Entity is one entity's published state. Immutable once published (D3): an
// update replaces the pointer, nothing mutates in place. That is what lets a
// Snapshot be a shallow map instead of a deep copy, and it is why several
// sessions can read concurrently without a lock.
type Entity struct {
	ID          string
	State       string
	Attrs       map[string]any
	LastChanged time.Time
	LastUpdated time.Time
	Seen        time.Time // when WE received it; drives staleness
}

// Snapshot is an immutable view of the store, valid for one frame.
type Snapshot struct {
	At     time.Time
	m      map[string]*Entity
	series map[string][]float64
}

// Get returns an entity, or nil.
func (s *Snapshot) Get(id string) *Entity {
	if s == nil {
		return nil
	}
	return s.m[id]
}

// Len reports how many entities the snapshot holds.
func (s *Snapshot) Len() int {
	if s == nil {
		return 0
	}
	return len(s.m)
}

// IDs returns the entity ids, unordered.
func (s *Snapshot) IDs() []string {
	out := make([]string, 0, len(s.m))
	for k := range s.m {
		out = append(out, k)
	}
	return out
}

// Store owns the live mirror.
//
// SINGLE WRITER (r3 §2 rule R-1). One goroutine mutates; everything else talks
// by channel and reads snapshots. There are no mutex-guarded shared maps,
// because the failure mode of a forgotten lock here is a silently corrupted
// dashboard rather than a crash -- and a corrupted dashboard that still looks
// plausible is the worst outcome this project can produce.
type Store struct {
	in   chan *ha.Event
	subs chan subReq

	mu   sync.RWMutex
	snap *Snapshot // published; readers take it under RLock and then never block

	// Heartbeat forces a snapshot even with no events, so time-based
	// conditions -- staleness above all -- are re-evaluated. Without it a
	// dashboard whose source went quiet would never notice.
	Heartbeat time.Duration

	// stopped is closed when Run returns, so Subscribe and the unsubscribe
	// function cannot block forever against a store that has shut down.
	stopped chan struct{}

	clock func() time.Time

	trackMu sync.Mutex
	tracked map[string]*tracker
}

type subReq struct {
	fn   func(*Snapshot)
	id   int
	drop bool
	resp chan int
}

// NewStore creates a store. Call Run to start its writer goroutine.
func NewStore() *Store {
	return &Store{
		in:        make(chan *ha.Event, 64),
		subs:      make(chan subReq),
		stopped:   make(chan struct{}),
		snap:      &Snapshot{m: map[string]*Entity{}},
		Heartbeat: time.Second,
		clock:     time.Now,
	}
}

// Ingest queues an event for the writer.
//
// Buffered, and DROPS OLDEST when full rather than blocking the protocol
// reader. A store that cannot keep up must not stall the WebSocket, because
// falling behind Home Assistant is recoverable and deadlocking is not.
func (s *Store) Ingest(ev *ha.Event) {
	select {
	case s.in <- ev:
	default:
		select {
		case <-s.in:
		default:
		}
		select {
		case s.in <- ev:
		default:
		}
	}
}

// Subscribe registers a callback invoked with each published snapshot. The
// returned function unsubscribes, and BLOCKS until the writer has acknowledged
// it.
//
// The first implementation made unsubscribe best-effort -- a select with a
// default on an unbuffered channel. When the writer happened to be publishing,
// the send fell through and the unsubscribe was SILENTLY DROPPED, leaving a
// dead session's callback wired to a live store. That is a leak of exactly the
// kind A1 was about, and it is invisible: nothing errors, the callback simply
// keeps firing forever. Unsubscribing must not be best-effort.
func (s *Store) Subscribe(fn func(*Snapshot)) func() {
	resp := make(chan int, 1)
	select {
	case s.subs <- subReq{fn: fn, resp: resp}:
	case <-s.stopped:
		return func() {}
	}
	var id int
	select {
	case id = <-resp:
	case <-s.stopped:
		return func() {}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			ack := make(chan int, 1)
			select {
			case s.subs <- subReq{id: id, drop: true, resp: ack}:
				select {
				case <-ack:
				case <-s.stopped:
				}
			case <-s.stopped:
			}
		})
	}
}

// Snapshot returns the most recently published view.
func (s *Store) Snapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

// Run is the single writer. It returns when ctx is cancelled.
//
// Events are folded in as they arrive; snapshots are published on the TICK, not
// per event. That coalescing is the mitigation for R1/R15: with the measured
// load of ~5.6 msg/s (spike S1) a 4 Hz tick collapses bursts into one frame,
// and the mechanism must exist before a chattier source appears.
func (s *Store) Run(ctx context.Context, tick time.Duration) {
	defer close(s.stopped)
	cache := ha.NewCache()
	subs := map[int]func(*Snapshot){}
	nextID := 1

	t := time.NewTicker(tick)
	defer t.Stop()

	dirty := false
	lastPublish := s.clock()

	publish := func() {
		now := s.clock()
		m := make(map[string]*Entity, cache.Len())
		for _, id := range cache.IDs() {
			es, _ := cache.Get(id)
			// A new Entity per publish for changed data; unchanged entities
			// still get a fresh pointer, which is cheap at this scale and
			// keeps "immutable once published" trivially true.
			attrs := make(map[string]any, len(es.Attrs))
			for k, v := range es.Attrs {
				attrs[k] = v
			}
			m[id] = &Entity{
				ID: es.ID, State: es.State, Attrs: attrs,
				LastChanged: es.LastChanged, LastUpdated: es.LastUpdated,
				Seen: now,
			}
		}
		snap := &Snapshot{At: now, m: m, series: s.accumulate(m, now)}
		s.mu.Lock()
		s.snap = snap
		s.mu.Unlock()
		for _, fn := range subs {
			fn(snap)
		}
		dirty = false
		lastPublish = now
	}

	for {
		select {
		case <-ctx.Done():
			return

		case ev := <-s.in:
			if err := cache.Apply(ev); err == nil {
				dirty = true
			}

		case r := <-s.subs:
			if r.drop {
				delete(subs, r.id)
				r.resp <- r.id
				continue
			}
			id := nextID
			nextID++
			subs[id] = r.fn
			r.resp <- id

		case <-t.C:
			if dirty || s.clock().Sub(lastPublish) >= s.Heartbeat {
				publish()
			}
		}
	}
}

// NewSnapshotForTest builds a snapshot directly. Test-only: production
// snapshots are published by the store's single writer.
func NewSnapshotForTest(m map[string]*Entity, at time.Time) *Snapshot {
	return &Snapshot{At: at, m: m}
}

// NewSnapshotWithSeriesForTest builds a snapshot carrying series data.
func NewSnapshotWithSeriesForTest(m map[string]*Entity, series map[string][]float64, at time.Time) *Snapshot {
	return &Snapshot{At: at, m: m, series: series}
}

// Track registers a numeric series the store should accumulate.
//
// The extractor is supplied by the caller rather than the store resolving
// bindings itself, because `state` must not import `config` -- the dependency
// runs the other way (r3 §1). It keeps the store ignorant of the binding
// grammar and keeps this package testable without any of it.
//
// Capacity is allocated once and never grows (N4, R8): a 24-hour window at
// 5-minute buckets is 288 points, a 60-minute live chart at 1 Hz is 3600.
func (s *Store) Track(key string, bucket time.Duration, n int, extract func(*Entity) (float64, bool)) {
	s.trackMu.Lock()
	defer s.trackMu.Unlock()
	if s.tracked == nil {
		s.tracked = map[string]*tracker{}
	}
	s.tracked[key] = &tracker{ring: NewRing(bucket, n), extract: extract}
}

type tracker struct {
	ring    *Ring
	extract func(*Entity) (float64, bool)
}

// accumulate folds the current values into the tracked series. Called by the
// writer on every publish, so a series advances at the publish rate rather
// than per event -- the same coalescing that protects the render path.
func (s *Store) accumulate(m map[string]*Entity, now time.Time) map[string][]float64 {
	s.trackMu.Lock()
	defer s.trackMu.Unlock()
	if len(s.tracked) == 0 {
		return nil
	}
	out := make(map[string][]float64, len(s.tracked))
	for key, t := range s.tracked {
		if t.extract == nil {
			// Statistic-backed: filled by backfill only.
			pts := t.ring.Series()
			vals := make([]float64, len(pts))
			for i, p := range pts {
				vals[i] = p.V
			}
			out[key] = vals
			continue
		}
		for _, e := range m {
			if v, ok := t.extract(e); ok {
				t.ring.Put(Sample{T: now, V: v, Src: Live})
				break
			}
		}
		pts := t.ring.Series()
		vals := make([]float64, len(pts))
		for i, p := range pts {
			vals[i] = p.V
		}
		out[key] = vals
	}
	return out
}

// PutStatistics injects backfilled points into a tracked series.
//
// Provenance does the work (finding E3): statistics beat live, and a newer
// revision beats an older one -- so a refetch after a reconnect LANDS on
// buckets that are already full, instead of being discarded as a duplicate.
func (s *Store) PutStatistics(key string, rev uint64, pts []struct {
	T time.Time
	V float64
}) {
	s.trackMu.Lock()
	defer s.trackMu.Unlock()
	t, ok := s.tracked[key]
	if !ok {
		return
	}
	for _, p := range pts {
		t.ring.Put(Sample{T: p.T, V: p.V, Src: Stats, Rev: rev})
	}
}

// Series returns the accumulated points for a tracked key. The slice belongs to
// the snapshot and is never mutated afterwards (D3).
func (s *Snapshot) Series(key string) []float64 {
	if s == nil {
		return nil
	}
	return s.series[key]
}
