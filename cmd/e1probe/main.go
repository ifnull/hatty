// Command e1probe answers the one question the Sink's unit tests cannot.
//
// internal/server's tests inject a `send` function, so they prove the sink's
// LOGIC is right: Push never blocks, a slow consumer sees only the newest
// frame, a write error tears the session down. They say nothing about whether
// wish's real write path BEHAVES that way -- and finding E1 rests entirely on
// the assumption that:
//
//	a stalled client makes the write BLOCK (no error), and
//	a dead client makes the write RETURN AN ERROR.
//
// If a stalled client also produced an error, hatty would tear down sessions on
// every Wi-Fi hiccup -- exactly the failure E1 was written to prevent, arriving
// from underneath instead.
//
// Run on the Pi; connect from elsewhere; then SIGSTOP the client (stall) and
// SIGKILL it (death), and watch which branch each takes.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/ifnull/hatty/internal/server"
)

var (
	addr     = flag.String("addr", "0.0.0.0:2222", "listen address")
	authKeys = flag.String("authorized-keys", "./authorized_keys", "allow-list path")
	hostKey  = flag.String("host-key", "./probe_host_key", "host key path")
	hz       = flag.Int("hz", 4, "frame rate")
	frameLen = flag.Int("frame", 2048, "bytes per frame")
)

func main() {
	flag.Parse()

	keys, err := server.LoadAuthorizedKeys(*authKeys)
	if err != nil {
		log.Fatalf("C1 check: %v", err)
	}
	log.Printf("authorized_keys: %d key(s)", keys.Len())

	var sessions atomic.Int64

	srv, err := wish.NewServer(
		wish.WithAddress(*addr),
		wish.WithHostKeyPath(*hostKey),
		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			ok := keys.Contains(key)
			log.Printf("auth from %s: %v", ctx.RemoteAddr(), ok)
			return ok
		}),
		wish.WithMiddleware(
			func(next ssh.Handler) ssh.Handler {
				return func(s ssh.Session) { handle(s, &sessions); next(s) }
			},
			activeterm.Middleware(),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("e1probe listening on %s, %d Hz, %d B/frame", *addr, *hz, *frameLen)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func handle(s ssh.Session, sessions *atomic.Int64) {
	id := sessions.Add(1)
	log.Printf("[s%d] open from %s", id, s.RemoteAddr())

	payload := make([]byte, *frameLen)
	for i := range payload {
		payload[i] = byte('a' + i%26)
	}

	var (
		writeStarted atomic.Int64 // unix nanos of the write currently in flight
		writeErr     atomic.Value
	)

	sink := server.NewSink(s.Context(), func(ctx context.Context, f server.Frame) error {
		writeStarted.Store(time.Now().UnixNano())
		_, err := s.Write(payload)
		writeStarted.Store(0)
		if err != nil {
			writeErr.Store(err.Error())
		}
		return err
	})
	defer sink.Close()

	// The "store": pushes at a fixed rate and MEASURES whether Push blocks.
	// If Push ever takes meaningfully long, the store is not isolated.
	var maxPush atomic.Int64
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second / time.Duration(*hz))
		defer t.Stop()
		n := 0
		for {
			select {
			case <-stop:
				return
			case <-sink.Done():
				return
			case <-t.C:
				n++
				start := time.Now()
				sink.Push(n)
				if d := time.Since(start).Nanoseconds(); d > maxPush.Load() {
					maxPush.Store(d)
				}
			}
		}
	}()

	// Report every second until the session ends.
	rep := time.NewTicker(time.Second)
	defer rep.Stop()
	for {
		select {
		case <-sink.Done():
			close(stop)
			sent, dropped := sink.Stats()
			e, _ := writeErr.Load().(string)
			log.Printf("[s%d] CLOSED after write error=%q | sent=%d dropped=%d maxPush=%s",
				id, e, sent, dropped, time.Duration(maxPush.Load()))
			return
		case <-rep.C:
			sent, dropped := sink.Stats()
			inflight := "-"
			if t := writeStarted.Load(); t != 0 {
				inflight = time.Since(time.Unix(0, t)).Truncate(time.Millisecond).String()
			}
			log.Printf("[s%d] sent=%d dropped=%d maxPush=%s writeBlockedFor=%s",
				id, sent, dropped, time.Duration(maxPush.Load()), inflight)
			fmt.Fprintf(os.Stderr, "")
		}
	}
}
