package state

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifnull/hatty/internal/ha"
)

func event(t *testing.T, s string) *ha.Event {
	t.Helper()
	e, err := ha.DecodeEvent(json.RawMessage(s))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func runStore(t *testing.T, tick time.Duration) (*Store, context.CancelFunc) {
	t.Helper()
	st := NewStore()
	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx, tick)
	t.Cleanup(cancel)
	return st, cancel
}

// R1/R15: snapshots are published on the TICK, not per event. With the measured
// ~5.6 msg/s this collapses bursts into one frame; without it an event storm
// becomes a redraw storm.
func TestEventsAreCoalescedIntoOneSnapshotPerTick(t *testing.T) {
	st, _ := runStore(t, 50*time.Millisecond)
	st.Heartbeat = time.Hour // isolate coalescing from the heartbeat

	var published atomic.Int64
	st.Subscribe(func(*Snapshot) { published.Add(1) })

	st.Ingest(event(t, `{"a":{"sensor.x":{"s":"0","a":{}}}}`))
	time.Sleep(120 * time.Millisecond)
	base := published.Load()

	// 200 events inside a single tick window.
	for i := 0; i < 200; i++ {
		st.Ingest(event(t, `{"c":{"sensor.x":{"+":{"s":"1"}}}}`))
	}
	time.Sleep(120 * time.Millisecond)

	if got := published.Load() - base; got > 3 {
		t.Errorf("200 events produced %d snapshots; coalescing is not engaging", got)
	}
	if got := published.Load() - base; got == 0 {
		t.Error("200 events produced no snapshot at all")
	}
}

// An idle store must not churn frames -- but it must still publish on the
// heartbeat, or time-based conditions like staleness would never re-evaluate.
func TestIdleStorePublishesOnlyOnTheHeartbeat(t *testing.T) {
	st, _ := runStore(t, 20*time.Millisecond)
	st.Heartbeat = 200 * time.Millisecond

	var n atomic.Int64
	st.Subscribe(func(*Snapshot) { n.Add(1) })
	time.Sleep(500 * time.Millisecond)

	got := n.Load()
	if got == 0 {
		t.Fatal("an idle store never published; staleness would never be re-evaluated")
	}
	if got > 6 {
		t.Errorf("idle store published %d times in 500ms with a 200ms heartbeat", got)
	}
}

// D3: a snapshot taken before an update must be UNAFFECTED by it. This is the
// invariant that makes snapshots shallow and safe to share across sessions.
func TestSnapshotsAreImmutable(t *testing.T) {
	st, _ := runStore(t, 20*time.Millisecond)
	st.Ingest(event(t, `{"a":{"sensor.x":{"s":"first","a":{"k":"v1"}}}}`))
	time.Sleep(100 * time.Millisecond)

	before := st.Snapshot()
	e := before.Get("sensor.x")
	if e == nil || e.State != "first" {
		t.Fatalf("setup failed: %+v", e)
	}

	st.Ingest(event(t, `{"c":{"sensor.x":{"+":{"s":"second","a":{"k":"v2"}}}}}`))
	time.Sleep(100 * time.Millisecond)

	if e.State != "first" {
		t.Errorf("a published entity mutated under a held snapshot: %q", e.State)
	}
	if e.Attrs["k"] != "v1" {
		t.Errorf("published attributes mutated: %v", e.Attrs["k"])
	}
	if after := st.Snapshot().Get("sensor.x"); after.State != "second" {
		t.Errorf("the new snapshot did not see the update: %q", after.State)
	}
}

// Falling behind Home Assistant is recoverable; deadlocking is not. Ingest must
// never block the protocol reader.
func TestIngestNeverBlocks(t *testing.T) {
	st := NewStore() // deliberately NOT running: nothing drains the queue
	// Built here, not inside the goroutine: t.Fatal from a non-test goroutine
	// does not stop it, and the test then hangs on the very timeout it is
	// trying to measure.
	ev := event(t, `{"a":{"sensor.x":{"s":"1"}}}`)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			st.Ingest(ev)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Ingest blocked when the writer was not draining")
	}
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	st, _ := runStore(t, 20*time.Millisecond)
	st.Heartbeat = 20 * time.Millisecond

	var a, b atomic.Int64
	unsubA := st.Subscribe(func(*Snapshot) { a.Add(1) })
	st.Subscribe(func(*Snapshot) { b.Add(1) })

	time.Sleep(120 * time.Millisecond)
	if a.Load() == 0 || b.Load() == 0 {
		t.Fatal("subscribers did not receive snapshots")
	}
	unsubA()
	time.Sleep(60 * time.Millisecond)
	mid := a.Load()
	time.Sleep(120 * time.Millisecond)
	if a.Load() > mid+1 {
		t.Errorf("unsubscribed callback still fired: %d -> %d", mid, a.Load())
	}
	if b.Load() == 0 {
		t.Error("the remaining subscriber stopped receiving")
	}
}

// The single-writer claim, checked by the race detector against concurrent
// producers and readers.
func TestConcurrentIngestAndReadIsRaceFree(t *testing.T) {
	st, _ := runStore(t, 10*time.Millisecond)
	ev := event(t, `{"a":{"sensor.x":{"s":"1","a":{"k":"v"}}}}`)
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					st.Ingest(ev)
				}
			}
		}()
	}
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if s := st.Snapshot(); s != nil {
						if e := s.Get("sensor.x"); e != nil {
							_ = e.State
							_ = e.Attrs["k"]
						}
					}
				}
			}
		}()
	}
	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// Replaying the real capture through the store must leave a coherent view.
func TestReplayLiveCaptureThroughTheStore(t *testing.T) {
	raw, err := readLines("../ha/testdata/frames-busy.jsonl")
	if err != nil {
		t.Skip("fixture missing")
	}
	st, _ := runStore(t, 20*time.Millisecond)
	for _, line := range raw {
		var rec struct {
			E json.RawMessage `json:"e"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		e, err := ha.DecodeEvent(rec.E)
		if err != nil {
			t.Fatal(err)
		}
		st.Ingest(e)
	}
	time.Sleep(200 * time.Millisecond)

	snap := st.Snapshot()
	if snap.Len() == 0 {
		t.Fatal("nothing in the store after replaying the capture")
	}
	for _, id := range snap.IDs() {
		if len(snap.Get(id).Attrs) == 0 {
			t.Errorf("%s has no attributes after replay", id)
		}
	}
	t.Logf("replayed capture: %d entities", snap.Len())
}
