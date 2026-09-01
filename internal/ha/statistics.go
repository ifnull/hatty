package ha

import (
	"encoding/json"
	"math"
	"time"
)

// StatPoint is one bucket from recorder/statistics_during_period.
//
// D16: `types` of min/mean/max at a 5-minute period is what feeds the 24-hour
// wind chart -- 288 buckets, already aggregated server-side, so the client does
// no aggregation at all.
type StatPoint struct {
	Start          time.Time
	Min, Mean, Max float64
}

// Value selects one of the three aggregates.
func (p StatPoint) Value(stat string) (float64, bool) {
	switch stat {
	case "min":
		return p.Min, true
	case "max":
		return p.Max, true
	case "mean", "":
		return p.Mean, true
	}
	return 0, false
}

// decodeStatistics parses a statistics result payload, keyed by statistic id.
func decodeStatistics(raw json.RawMessage) map[string][]StatPoint {
	var res map[string][]struct {
		Start any      `json:"start"`
		Min   *float64 `json:"min"`
		Mean  *float64 `json:"mean"`
		Max   *float64 `json:"max"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return nil
	}
	out := make(map[string][]StatPoint, len(res))
	for id, rows := range res {
		pts := make([]StatPoint, 0, len(rows))
		for _, r := range rows {
			t := statTime(r.Start)
			if t.IsZero() {
				continue
			}
			p := StatPoint{Start: t}
			// A nil aggregate is genuinely absent, not zero. Substituting 0
			// would draw a line to the floor -- the same error as rendering
			// "0.0 mi" for absent lightning.
			if r.Min == nil && r.Mean == nil && r.Max == nil {
				continue
			}
			if r.Min != nil {
				p.Min = *r.Min
			}
			if r.Mean != nil {
				p.Mean = *r.Mean
			}
			if r.Max != nil {
				p.Max = *r.Max
			}
			pts = append(pts, p)
		}
		out[id] = pts
	}
	return out
}

// statTime accepts either epoch milliseconds or an ISO timestamp; Home
// Assistant has used both across versions.
func statTime(v any) time.Time {
	switch t := v.(type) {
	case float64:
		if t == 0 || math.IsNaN(t) {
			return time.Time{}
		}
		return time.UnixMilli(int64(t)).UTC()
	case string:
		if p, err := time.Parse(time.RFC3339, t); err == nil {
			return p
		}
	}
	return time.Time{}
}
