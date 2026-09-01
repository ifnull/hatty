package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
)

// Conn is one WebSocket connection to Home Assistant.
type Conn struct {
	ws  *websocket.Conn
	ids IDGen
	log *slog.Logger
}

// Dial connects and authenticates.
func Dial(ctx context.Context, url string, token Secret, log *slog.Logger) (*Conn, error) {
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	c := &Conn{ws: ws, log: log}

	var greet Greeting
	if err := ws.ReadJSON(&greet); err != nil {
		ws.Close()
		return nil, err
	}
	if greet.Type != "auth_required" {
		ws.Close()
		return nil, fmt.Errorf("ha: unexpected greeting %q", greet.Type)
	}

	frame, err := authFrame(token)
	if err != nil {
		ws.Close()
		return nil, err
	}
	// The frame carries the live credential, so it is redacted before it can
	// reach a log line (finding E8).
	log.Debug("ha: sending auth", "frame", string(RedactFrame(frame)))
	if err := ws.WriteMessage(websocket.TextMessage, frame); err != nil {
		ws.Close()
		return nil, err
	}
	_, reply, err := ws.ReadMessage()
	if err != nil {
		ws.Close()
		return nil, err
	}
	if err := CheckAuthResult(reply); err != nil {
		ws.Close()
		return nil, err
	}
	// D17: the server tracks the highest id PER CONNECTION, so a reconnect that
	// does not reset gets ERR_ID_REUSE on every subsequent message.
	c.ids.Reset()
	log.Info("ha: connected", "version", greet.HAVersion)
	return c, nil
}

func (c *Conn) Close() error { return c.ws.Close() }

// SubscribeEntities subscribes with a server-side entity filter (D6).
func (c *Conn) SubscribeEntities(ids []string) error {
	f, err := SubscribeEntitiesFrame(c.ids.Next(), ids)
	if err != nil {
		return err
	}
	return c.ws.WriteMessage(websocket.TextMessage, f)
}

// Ping sends a liveness probe, which detects a half-open socket before the
// cache silently goes stale (risk R6).
func (c *Conn) Ping() error {
	f, _ := json.Marshal(map[string]any{"id": c.ids.Next(), "type": "ping"})
	return c.ws.WriteMessage(websocket.TextMessage, f)
}

// Read returns the next entity event, skipping anything else.
func (c *Conn) Read(deadline time.Duration) (*Event, error) {
	for {
		if deadline > 0 {
			_ = c.ws.SetReadDeadline(time.Now().Add(deadline))
		}
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			return nil, err
		}
		var m struct {
			Type  string          `json:"type"`
			Event json.RawMessage `json:"event"`
		}
		if json.Unmarshal(raw, &m) != nil || m.Type != "event" || len(m.Event) == 0 {
			continue
		}
		ev, err := DecodeEvent(m.Event)
		if err != nil {
			c.log.Warn("ha: undecodable event", "err", err)
			continue
		}
		return ev, nil
	}
}

// Sink receives events from the connection runner.
type Sink interface{ Ingest(*Event) }

// Run maintains a connection, reconnecting with backoff until ctx is done.
//
// On every (re)connection the ids reset (D17), the subscription is re-issued,
// and onConnect fires so callers can mark bindings Stale until fresh values
// arrive -- not Unavailable, because we do not know that, and not Valid,
// because the cached number may be minutes old.
//
// The daemon and Home Assistant share a Proxmox host (D13), so a host reboot
// takes both down at once: this must start cleanly against an HA that is not
// yet listening rather than exiting.
func Run(ctx context.Context, url string, token Secret, entityIDs []string, sink Sink, onConnect func(), log *slog.Logger) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for ctx.Err() == nil {
		c, err := Dial(ctx, url, token, log)
		if err != nil {
			log.Warn("ha: connect failed", "err", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff *= 2; backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = time.Second

		if err := c.SubscribeEntities(entityIDs); err != nil {
			log.Warn("ha: subscribe failed", "err", err)
			c.Close()
			continue
		}
		if onConnect != nil {
			onConnect()
		}

		for ctx.Err() == nil {
			ev, err := c.Read(90 * time.Second)
			if err != nil {
				log.Warn("ha: read failed, reconnecting", "err", err)
				break
			}
			sink.Ingest(ev)
		}
		c.Close()
	}
}
