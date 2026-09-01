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
	ws           *websocket.Conn
	ids          IDGen
	log          *slog.Logger
	forecastSeen bool

	statsIDs map[uint64]bool
	statsRev uint64
	onStats  OnStatistics
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

// SubscribeForecast subscribes to a weather forecast.
//
// Verified live: `daily` returns 6 entries (~1.2 KB) including templow, which
// is what makes the freeze decision possible, and `hourly` returns 48 (~10 KB).
// weather.forecast_home exposes NOTHING useful in its attributes -- modern Home
// Assistant moved forecasts to this subscription (D43).
func (c *Conn) SubscribeForecast(entityID, forecastType string) error {
	f, err := ForecastFrame(c.ids.Next(), entityID, forecastType)
	if err != nil {
		return err
	}
	return c.ws.WriteMessage(websocket.TextMessage, f)
}

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

// RequestStatistics asks for aggregated history and records the command id, so
// the matching result can be recognised when it arrives.
func (c *Conn) RequestStatistics(ids []string, start time.Time, period string) error {
	if len(ids) == 0 {
		return nil
	}
	id := c.ids.Next()
	f, err := StatisticsFrame(id, ids, start.UTC().Format(time.RFC3339), "", period)
	if err != nil {
		return err
	}
	if c.statsIDs == nil {
		c.statsIDs = map[uint64]bool{}
	}
	c.statsIDs[id] = true
	return c.ws.WriteMessage(websocket.TextMessage, f)
}

// OnStatistics is invoked when a statistics result arrives. The revision rises
// with each fetch so a later correction beats an earlier one in the ring
// (finding E3) -- Home Assistant REVISES statistics, and first-write-wins would
// discard the correction and make reconnect backfill a no-op.
type OnStatistics func(rev uint64, byID map[string][]StatPoint)

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
			ID     uint64          `json:"id"`
			Type   string          `json:"type"`
			Event  json.RawMessage `json:"event"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		// A statistics response is a `result`, not an `event`, which is why it
		// was invisible to a reader that only looked at events.
		if m.Type == "result" && c.statsIDs[m.ID] {
			delete(c.statsIDs, m.ID)
			if c.onStats != nil && len(m.Result) > 0 {
				c.statsRev++
				c.onStats(c.statsRev, decodeStatistics(m.Result))
			}
			continue
		}
		if m.Type != "event" || len(m.Event) == 0 {
			continue
		}
		// A forecast event carries `{"type":"daily","forecast":[...]}`, not the
		// a/c/r shape. It is translated into an ordinary entity event for the
		// pseudo-entity `forecast.home`, so there is ONE binding grammar and
		// nothing downstream knows the forecast is special (finding E5).
		if ev := c.forecastEvent(m.Event); ev != nil {
			return ev, nil
		}
		ev, err := DecodeEvent(m.Event)
		if err != nil {
			c.log.Warn("ha: undecodable event", "err", err)
			continue
		}
		return ev, nil
	}
}

// ForecastEntity is the pseudo-entity id the forecast is published under.
const ForecastEntity = "forecast.home"

// forecastEvent translates a forecast subscription event into an entity event.
// Returns nil if this is not a forecast event.
func (c *Conn) forecastEvent(raw json.RawMessage) *Event {
	var f struct {
		Type     string            `json:"type"`
		Forecast []json.RawMessage `json:"forecast"`
	}
	if json.Unmarshal(raw, &f) != nil || f.Forecast == nil || f.Type == "" {
		return nil
	}
	list := make([]any, 0, len(f.Forecast))
	for _, e := range f.Forecast {
		var m map[string]any
		if json.Unmarshal(e, &m) == nil {
			list = append(list, m)
		}
	}
	attrs, _ := json.Marshal(map[string]any{f.Type: list})

	// The first forecast type seen ADDS the entity; later types must MERGE, or
	// subscribing to both daily and hourly would leave only whichever arrived
	// last -- the same replace-versus-merge trap as D18.
	if !c.forecastSeen {
		c.forecastSeen = true
		full, _ := json.Marshal(map[string]any{"s": "ok", "a": json.RawMessage(attrs)})
		return &Event{Add: map[string]json.RawMessage{ForecastEntity: full}}
	}
	diff, _ := json.Marshal(map[string]any{"+": map[string]any{"a": json.RawMessage(attrs)}})
	return &Event{Change: map[string]json.RawMessage{ForecastEntity: diff}}
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
// StatsRequest describes the backfill a dashboard needs.
type StatsRequest struct {
	IDs    []string
	Window time.Duration
	Period string // "5minute" | "hour" | ...
	On     OnStatistics
}

func Run(ctx context.Context, url string, token Secret, entityIDs []string, forecastFor string, stats *StatsRequest, sink Sink, onConnect func(), log *slog.Logger) {
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
		if forecastFor != "" {
			for _, ft := range []string{"daily", "hourly"} {
				if err := c.SubscribeForecast(forecastFor, ft); err != nil {
					log.Warn("ha: forecast subscribe failed", "type", ft, "err", err)
				}
			}
		}
		// Backfill on EVERY connection, not just the first. A reconnect exists
		// to repair a gap, and the ring's provenance rules let a refetch land
		// on buckets that are already full (finding E3).
		if stats != nil && len(stats.IDs) > 0 {
			c.onStats = stats.On
			if err := c.RequestStatistics(stats.IDs, time.Now().Add(-stats.Window), stats.Period); err != nil {
				log.Warn("ha: statistics request failed", "err", err)
			} else {
				log.Info("ha: statistics requested", "ids", len(stats.IDs), "window", stats.Window)
			}
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
