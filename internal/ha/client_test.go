package ha

import (
	"encoding/json"
	"strings"
	"testing"
)

// D17: strictly increasing, and reset per connection or the server rejects
// every message after the first reconnect with ERR_ID_REUSE.
func TestIDsAreStrictlyIncreasing(t *testing.T) {
	var g IDGen
	prev := uint64(0)
	for i := 0; i < 1000; i++ {
		n := g.Next()
		if n <= prev {
			t.Fatalf("id %d followed %d -- not strictly increasing", n, prev)
		}
		prev = n
	}
	g.Reset()
	if n := g.Next(); n != 1 {
		t.Fatalf("after Reset first id = %d, want 1", n)
	}
}

func TestIDGenIsConcurrencySafe(t *testing.T) {
	var g IDGen
	const n = 200
	seen := make(chan uint64, n)
	done := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() { seen <- g.Next(); done <- struct{}{} }()
	}
	for i := 0; i < n; i++ {
		<-done
	}
	close(seen)
	uniq := map[uint64]bool{}
	for id := range seen {
		if uniq[id] {
			t.Fatalf("id %d issued twice", id)
		}
		uniq[id] = true
	}
}

// The auth frame must carry the real token (or authentication fails) and must
// be safe to log once redacted.
func TestAuthFrameCarriesTokenAndRedacts(t *testing.T) {
	f, err := authFrame(Secret(fakeToken))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(f, &m); err != nil {
		t.Fatalf("auth frame is not valid JSON: %v", err)
	}
	if m["access_token"] != fakeToken {
		t.Fatalf("auth frame does not carry the real token -- authentication would fail")
	}
	if strings.Contains(string(RedactFrame(f)), "THIS_MUST_NEVER_APPEAR") {
		t.Fatal("redacted auth frame still leaks the token")
	}
}

func TestAuthFrameRejectsEmptyToken(t *testing.T) {
	if _, err := authFrame(Secret("   ")); err == nil {
		t.Fatal("expected an error for an empty token")
	}
}

func TestCheckAuthResult(t *testing.T) {
	if err := CheckAuthResult([]byte(`{"type":"auth_ok","ha_version":"2026.8"}`)); err != nil {
		t.Fatalf("auth_ok returned %v", err)
	}
	err := CheckAuthResult([]byte(`{"type":"auth_invalid","message":"bad"}`))
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("auth_invalid returned %v", err)
	}
}

// D6: an empty entity set would silently subscribe to nothing.
func TestSubscribeRefusesEmptySet(t *testing.T) {
	if _, err := SubscribeEntitiesFrame(1, nil); err == nil {
		t.Fatal("expected an error for an empty entity set")
	}
}

func TestFramesAreWellFormed(t *testing.T) {
	subs, err := SubscribeEntitiesFrame(7, []string{"sensor.a", "sensor.b"})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := StatisticsFrame(8, []string{"sensor.wind"}, "2026-08-30T00:00:00Z", "", "5minute")
	if err != nil {
		t.Fatal(err)
	}
	fc, err := ForecastFrame(9, "weather.forecast_home", "daily")
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string][]byte{"subscribe": subs, "statistics": stats, "forecast": fc} {
		var m map[string]any
		if err := json.Unmarshal(f, &m); err != nil {
			t.Errorf("%s frame invalid: %v", name, err)
		}
		if _, ok := m["id"]; !ok {
			t.Errorf("%s frame has no id", name)
		}
	}
	var sm map[string]any
	json.Unmarshal(stats, &sm)
	types, _ := sm["types"].([]any)
	if len(types) != 3 {
		t.Errorf("statistics types = %v, want min/mean/max (D16)", sm["types"])
	}
}
