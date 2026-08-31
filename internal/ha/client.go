package ha

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// ErrAuthFailed is returned when Home Assistant rejects the token. It is fatal:
// a bad credential will not fix itself, so the daemon reports it rather than
// retrying forever (r3 §9).
var ErrAuthFailed = errors.New("ha: authentication rejected")

// IDGen issues WebSocket message ids.
//
// FINDING D17: ids must be STRICTLY increasing. This is not a convention --
// connection.go rejects any id <= the highest seen with ERR_ID_REUSE
// ("Identifier values have to increase"). The developer documentation
// understates it, saying only that ids correlate messages to responses.
//
// The server tracks the highest id PER CONNECTION, so the counter must reset
// on reconnect. Reset() exists to make that explicit rather than implied.
type IDGen struct {
	mu   sync.Mutex
	next uint64
}

func (g *IDGen) Next() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next
}

// Reset returns the generator to its initial state. MUST be called on every new
// connection; the server's last-seen id is per-connection.
func (g *IDGen) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next = 0
}

// authFrame builds the authentication message.
//
// This function contains the ONLY call to Secret.reveal() in the package, and
// TestExactlyOneReveal asserts that. The frame is assembled explicitly rather
// than by marshalling a struct containing the Secret, because Secret's
// MarshalJSON redacts -- marshalling a struct would produce a frame that fails
// authentication (finding E8).
//
// Anything logging this frame MUST pass it through RedactFrame first.
func authFrame(token Secret) ([]byte, error) {
	if token.Empty() {
		return nil, errors.New("ha: empty token")
	}
	tok, err := json.Marshal(token.reveal())
	if err != nil {
		return nil, err
	}
	return []byte(`{"type":"auth","access_token":` + string(tok) + `}`), nil
}

// SubscribeEntitiesFrame builds a subscribe_entities command.
//
// FINDING D6: the entity filter is applied SERVER-SIDE. Subscribing to the
// bound set rather than the whole instance is what makes the memory bound a
// function of the dashboard rather than of the 981-entity instance -- and what
// keeps the measured load at ~7.2 KB/s instead of the full state_changed
// firehose (spike S1).
func SubscribeEntitiesFrame(id uint64, entityIDs []string) ([]byte, error) {
	if len(entityIDs) == 0 {
		return nil, errors.New("ha: refusing to subscribe with an empty entity set")
	}
	return json.Marshal(map[string]any{
		"id":         id,
		"type":       "subscribe_entities",
		"entity_ids": entityIDs,
	})
}

// StatisticsFrame builds a recorder/statistics_during_period command.
//
// D16: `types` of min/mean/max at a 5-minute period is what feeds the 24-hour
// wind chart. The bucket the results land in is derived from `period`, never
// configured separately (finding E4).
func StatisticsFrame(id uint64, statisticIDs []string, start, end, period string) ([]byte, error) {
	m := map[string]any{
		"id":            id,
		"type":          "recorder/statistics_during_period",
		"start_time":    start,
		"statistic_ids": statisticIDs,
		"period":        period,
		"types":         []string{"min", "mean", "max"},
	}
	if end != "" {
		m["end_time"] = end
	}
	return json.Marshal(m)
}

// ForecastFrame builds a weather/subscribe_forecast command.
//
// D43: verified against the live instance -- `daily` returns 6 entries
// (~1.2 KB) including templow, which is what makes the freeze decision
// possible. The forecast is addressed downstream as a pseudo-entity so there is
// only one binding grammar (finding E5).
func ForecastFrame(id uint64, entityID, forecastType string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":            id,
		"type":          "weather/subscribe_forecast",
		"entity_id":     entityID,
		"forecast_type": forecastType,
	})
}

// Greeting is the server's opening message.
type Greeting struct {
	Type      string `json:"type"`
	HAVersion string `json:"ha_version"`
}

// CheckAuthResult interprets the reply to an auth frame.
func CheckAuthResult(raw []byte) error {
	var r struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	switch r.Type {
	case "auth_ok":
		return nil
	case "auth_invalid":
		return fmt.Errorf("%w: %s", ErrAuthFailed, r.Message)
	default:
		return fmt.Errorf("ha: unexpected auth reply %q", r.Type)
	}
}
