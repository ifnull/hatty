package server

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func settle() { time.Sleep(30 * time.Millisecond); runtime.GC() }

// THE property that isolates the store. r1's design blocked here, which is how
// a Wi-Fi flap parked a store goroutine forever.
func TestPushNeverBlocksOnAWedgedSession(t *testing.T) {
	block := make(chan struct{})
	s := NewSink(context.Background(), func(ctx context.Context, f Frame) error {
		<-block // wedged for the whole test
		return nil
	})
	defer func() { close(block); s.Close() }()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			s.Push(i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Push blocked on a wedged session -- the store is not isolated")
	}
}

// A slow consumer must see only the newest frame. Intermediate frames are
// SUPPOSED to be dropped; that is the mechanism, not a failure.
func TestSlowConsumerGetsOnlyTheLatestFrame(t *testing.T) {
	var mu sync.Mutex
	var got []int
	release := make(chan struct{})
	s := NewSink(context.Background(), func(ctx context.Context, f Frame) error {
		<-release
		mu.Lock()
		got = append(got, f.(int))
		mu.Unlock()
		return nil
	})
	defer s.Close()

	for i := 1; i <= 100; i++ {
		s.Push(i)
	}
	settle()
	close(release)
	settle()

	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatal("nothing was delivered")
	}
	if len(got) > 3 {
		t.Errorf("delivered %d frames; a depth-1 replacing slot should collapse them", len(got))
	}
	if last := got[len(got)-1]; last != 100 {
		t.Errorf("last delivered frame = %d, want 100 (the newest)", last)
	}
	if _, dropped := s.Stats(); dropped == 0 {
		t.Error("no frames recorded as superseded; the replacing slot is not engaging")
	}
}

// E1: a five-second stall is an ordinary Wi-Fi hiccup on a Pi 3B. It must NOT
// tear the session down -- r2's 750 ms reaper would have blanked the dashboard.
func TestALongStallDoesNotKillTheSession(t *testing.T) {
	stalled := make(chan struct{})
	var once sync.Once
	s := NewSink(context.Background(), func(ctx context.Context, f Frame) error {
		once.Do(func() { close(stalled) })
		time.Sleep(300 * time.Millisecond) // stand-in for a much longer stall
		return nil
	})
	defer s.Close()

	s.Push(1)
	<-stalled
	select {
	case <-s.Done():
		t.Fatal("the session was torn down because it was slow -- slowness is not death")
	case <-time.After(200 * time.Millisecond):
	}
}

// The ONLY teardown trigger: a write error, meaning confirmed channel death.
func TestWriteErrorTearsTheSessionDown(t *testing.T) {
	s := NewSink(context.Background(), func(ctx context.Context, f Frame) error {
		return errors.New("broken pipe")
	})
	s.Push(1)
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("a write error did not tear the session down")
	}
	s.Close()
}

// A1: no session may outlive its sink. This is the leak that would have
// accumulated across reconnects over weeks of Wi-Fi flaps.
func TestSinksLeakNoGoroutines(t *testing.T) {
	settle()
	before := runtime.NumGoroutine()

	for i := 0; i < 200; i++ {
		block := make(chan struct{})
		s := NewSink(context.Background(), func(ctx context.Context, f Frame) error {
			select {
			case <-block:
			case <-ctx.Done():
			}
			return nil
		})
		s.Push(i)
		s.Close() // must return only once the goroutine has exited
		close(block)
	}
	settle()

	if after := runtime.NumGoroutine(); after > before+5 {
		t.Fatalf("goroutines grew from %d to %d across 200 session lifecycles", before, after)
	}
}

// Cancelling the parent must stop the sink, so a daemon shutdown cannot hang.
func TestParentCancellationStopsTheSink(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := NewSink(ctx, func(context.Context, Frame) error { return nil })
	cancel()
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("parent cancellation did not stop the sink")
	}
	s.Close()
}

func TestConcurrentPushIsSafe(t *testing.T) {
	s := NewSink(context.Background(), func(context.Context, Frame) error { return nil })
	defer s.Close()
	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				s.Push(w*1000 + i)
			}
		}(w)
	}
	wg.Wait()
	settle()
	if sent, _ := s.Stats(); sent == 0 {
		t.Error("nothing was delivered under concurrent push")
	}
}
