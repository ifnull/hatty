package ha

import (
	"encoding/json"
	"testing"
	"time"
)

// The real response shape, with epoch-millisecond starts.
func TestDecodeStatistics(t *testing.T) {
	raw := json.RawMessage(`{"sensor.wind":[
		{"start":1788200700000,"min":0.5,"mean":3.2,"max":8.1},
		{"start":1788201000000,"min":0.0,"mean":2.8,"max":6.4}]}`)
	got := decodeStatistics(raw)
	pts := got["sensor.wind"]
	if len(pts) != 2 {
		t.Fatalf("%d points, want 2", len(pts))
	}
	if pts[0].Start.IsZero() || pts[1].Start.Sub(pts[0].Start) != 5*time.Minute {
		t.Errorf("bucket spacing = %v, want 5m", pts[1].Start.Sub(pts[0].Start))
	}
	for stat, want := range map[string]float64{"min": 0.5, "mean": 3.2, "max": 8.1} {
		if v, ok := pts[0].Value(stat); !ok || v != want {
			t.Errorf("%s = %v, want %v", stat, v, want)
		}
	}
	if _, ok := pts[0].Value("nonsense"); ok {
		t.Error("an unknown aggregate was accepted")
	}
}

func TestDecodeStatisticsAcceptsISOTimestamps(t *testing.T) {
	raw := json.RawMessage(`{"sensor.x":[{"start":"2026-08-30T12:00:00+00:00","mean":1.5}]}`)
	pts := decodeStatistics(raw)["sensor.x"]
	if len(pts) != 1 || pts[0].Start.IsZero() {
		t.Fatalf("ISO start not parsed: %+v", pts)
	}
}

// A bucket with no aggregates is genuinely absent, not zero. Substituting 0
// would draw the line to the floor -- the same error as "0.0 mi" for absent
// lightning.
func TestBucketsWithNoAggregatesAreDropped(t *testing.T) {
	raw := json.RawMessage(`{"sensor.x":[
		{"start":1788200700000,"min":null,"mean":null,"max":null},
		{"start":1788201000000,"mean":4.0}]}`)
	pts := decodeStatistics(raw)["sensor.x"]
	if len(pts) != 1 {
		t.Fatalf("%d points, want 1 -- an empty bucket must be dropped, not zeroed", len(pts))
	}
	if v, _ := pts[0].Value("mean"); v != 4.0 {
		t.Errorf("surviving point = %v, want 4.0", v)
	}
}

func TestMalformedStatisticsDoNotPanic(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"a":"b"}`, `{"x":[{"start":"nope"}]}`, ``} {
		_ = decodeStatistics(json.RawMessage(raw))
	}
}
