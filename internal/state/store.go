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
	At time.Time
	m  map[string]*Entity
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

	clock func() time.Time
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
// returned function unsubscribes.
func (s *Store) Subscribe(fn func(*Snapshot)) func() {
	resp := make(chan int, 1)
	s.subs <- subReq{fn: fn, resp: resp}
	id := <-resp
	return func() {
		select {
		case s.subs <- subReq{id: id, drop: true, resp: make(chan int, 1)}:
		default:
		}
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
		snap := &Snapshot{At: now, m: m}
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
