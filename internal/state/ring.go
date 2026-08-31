package state

import "time"

// Source records where a sample came from. Provenance is what lets backfill
// and live updates share a buffer safely (finding E3).
type Source uint8

const (
	// Live: an update that arrived over the entity subscription.
	Live Source = iota
	// Stats: a point from recorder/statistics_during_period.
	Stats
)

// Sample is one point in a series.
type Sample struct {
	T   time.Time
	V   float64
	Src Source
	Rev uint64 // fetch sequence for Stats writes; ignored for Live
}

// Ring is a fixed-capacity, timestamp-bucketed series buffer.
//
// Bucketing rather than appending is what makes backfill and live updates
// COMMUTE (finding A3): a statistics response landing after live samples have
// arrived cannot overwrite newer data or push points out of order, so a chart
// can no longer run backwards in time.
//
// Capacity is allocated once and never grows, which is the memory bound in
// requirement N4 and risk R8.
type Ring struct {
	bucket time.Duration
	slots  []Sample
	idx    []int64 // bucket index occupying each slot; -1 means empty
	newest int64
}

// NewRing creates a buffer of n buckets of the given duration.
//
// The bucket is DERIVED from the series' backfill source, never configured
// independently (finding E4): a bucket smaller than the statistics period
// leaves slots empty and renders the chart as a comb, and a larger one makes
// samples collide and be discarded.
func NewRing(bucket time.Duration, n int) *Ring {
	r := &Ring{bucket: bucket, slots: make([]Sample, n), idx: make([]int64, n), newest: -1}
	for i := range r.idx {
		r.idx[i] = -1
	}
	return r
}

func (r *Ring) bucketOf(t time.Time) int64 { return t.UnixNano() / int64(r.bucket) }

// Put writes s, resolving conflicts by provenance.
//
// The rules, in order (finding E3):
//
//	empty slot                      -> write
//	Stats over Live                 -> write; statistics are authoritative
//	Stats over Stats, newer Rev     -> write; Home Assistant REVISES statistics,
//	                                   and first-write-wins would discard the
//	                                   correction permanently -- and would make
//	                                   reconnect backfill a no-op, since every
//	                                   bucket is already full
//	otherwise                       -> ignore
func (r *Ring) Put(s Sample) {
	if len(r.slots) == 0 {
		return
	}
	b := r.bucketOf(s.T)
	if b > r.newest {
		r.newest = b
	}
	// Older than the window we retain.
	if b <= r.newest-int64(len(r.slots)) {
		return
	}
	i := int(((b % int64(len(r.slots))) + int64(len(r.slots))) % int64(len(r.slots)))

	if r.idx[i] != b {
		// The slot belongs to a different (now-evicted) bucket: claim it.
		r.slots[i], r.idx[i] = s, b
		return
	}
	cur := r.slots[i]
	switch {
	case s.Src == Stats && cur.Src == Live:
		r.slots[i] = s
	case s.Src == Stats && cur.Src == Stats && s.Rev > cur.Rev:
		r.slots[i] = s
	}
}

// Series returns the retained samples in ascending time order, skipping
// buckets that were never filled.
func (r *Ring) Series() []Sample {
	if r.newest < 0 {
		return nil
	}
	out := make([]Sample, 0, len(r.slots))
	lo := r.newest - int64(len(r.slots)) + 1
	for b := lo; b <= r.newest; b++ {
		if b < 0 {
			continue
		}
		i := int(((b % int64(len(r.slots))) + int64(len(r.slots))) % int64(len(r.slots)))
		if r.idx[i] == b {
			out = append(out, r.slots[i])
		}
	}
	return out
}

// Len reports how many buckets currently hold a sample.
func (r *Ring) Len() int { return len(r.Series()) }
