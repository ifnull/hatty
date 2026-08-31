// Package server owns the SSH front end and per-session delivery.
package server

import (
	"context"
	"sync/atomic"
)

// Frame is whatever the renderer needs for one paint. The sink is agnostic.
type Frame any

// Sink delivers frames to one session.
//
// FINDING A1, then E1. Two designs were wrong before this one:
//
// r1 said the store would call tea.Program.Send() directly. But Send BLOCKS
// when the program's event loop is blocked, and the event loop blocks whenever
// the renderer's write to the SSH channel blocks -- a suspended Pi or a Wi-Fi
// flap does exactly that. Every such session parked a store goroutine forever:
// an unbounded leak in a daemon meant to run for months.
//
// r2 fixed the leak with a context, then added a reaper that killed any session
// whose last consumed frame was older than 3 x tick -- 750 ms. A Raspberry Pi
// 3B on 2.4 GHz Wi-Fi stalls longer than that routinely, so the reaper would
// blank the dashboard and force a reconnect on every ordinary hiccup: strictly
// worse than the stale frame it was protecting against.
//
// The resolution is that SLOWNESS IS NOT DEATH. A slow consumer is a condition
// this design already handles -- frames are SUPPOSED to be dropped, which is
// what the depth-1 replacing slot is for.
//
// VERIFIED ON HARDWARE (docs/spikes/E1-backpressure/RESULTS.md). Against a Pi 3B
// over the LAN, a SIGSTOPped client blocked the write for 7+ seconds with NO
// error while Push stayed at 302 microseconds, and the session recovered
// cleanly on resume -- a session r2's reaper would have killed nine times over.
//
// That probe also CORRECTED this comment. A killed client did not produce a
// write error either: wish cancels the SSH session context, and that -- not the
// write -- is what ends the session. So teardown happens on CONTEXT
// CANCELLATION, with the write-error branch as a backstop that may never fire
// in practice. The load-bearing assumption is therefore that wish cancels the
// session context on connection death; it did, across 11 lifecycles.
//
// Render latency, of any magnitude, is never a teardown trigger.
type Sink struct {
	latest atomic.Pointer[Frame]
	wake   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc

	// send delivers one frame. It may block for as long as it likes; that is
	// the client's problem, not the store's. It returns an error only on
	// CONFIRMED channel death -- a write error or a closed connection.
	send func(context.Context, Frame) error

	dropped atomic.Int64
	sent    atomic.Int64
	done    chan struct{}
}

// NewSink starts a delivery goroutine. Call Close to stop it.
func NewSink(parent context.Context, send func(context.Context, Frame) error) *Sink {
	ctx, cancel := context.WithCancel(parent)
	s := &Sink{
		wake:   make(chan struct{}, 1),
		ctx:    ctx,
		cancel: cancel,
		send:   send,
		done:   make(chan struct{}),
	}
	go s.loop()
	return s
}

// Push offers a frame. It NEVER blocks.
//
// This is the property that isolates the store: no matter how wedged a session
// is, the single writer goroutine returns immediately. A frame that arrives
// while an older one is still undelivered REPLACES it, because a stale frame is
// worthless -- only the newest matters.
func (s *Sink) Push(f Frame) {
	if s.latest.Swap(&f) != nil {
		s.dropped.Add(1)
	}
	select {
	case s.wake <- struct{}{}:
	default: // a wake is already pending; the consumer will see the new frame
	}
}

func (s *Sink) loop() {
	defer close(s.done)
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
			p := s.latest.Swap(nil)
			if p == nil {
				continue
			}
			if err := s.send(s.ctx, *p); err != nil {
				// A write error means the channel is gone. In practice the
				// context is usually cancelled first (see the package comment
				// and the E1 probe results), so this is a backstop rather than
				// the primary path -- but an untested backstop is not one.
				s.cancel()
				return
			}
			s.sent.Add(1)
		}
	}
}

// Close tears the sink down and waits for its goroutine to exit, so a session
// that ends cannot leave one behind.
func (s *Sink) Close() {
	s.cancel()
	<-s.done
}

// Done is closed when the sink has stopped, for whatever reason.
func (s *Sink) Done() <-chan struct{} { return s.ctx.Done() }

// Stats reports frames delivered and frames superseded before delivery.
// Dropped is expected and healthy on a slow link; it is the mechanism working.
func (s *Sink) Stats() (sent, dropped int64) { return s.sent.Load(), s.dropped.Load() }
