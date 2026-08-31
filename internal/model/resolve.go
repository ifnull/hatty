// Package model turns configuration plus a state snapshot into the view-models
// widgets draw. It is where config, state and widget finally meet -- and where
// the value conditions, guards and staleness rules are actually applied.
package model

import (
	"strconv"
	"strings"
	"time"

	"github.com/ifnull/hatty/internal/config"
	"github.com/ifnull/hatty/internal/state"
)

// Resolver reads a snapshot through bindings.
type Resolver struct {
	Snap *state.Snapshot
	Now  time.Time
}

// Resolve turns a binding into a Value, assigning the right condition.
//
// The order matters, and each branch exists because of something real:
//
//	entity absent         -> Unavailable  (bound but never sent)
//	state "unavailable"   -> Unavailable  (Home Assistant says so)
//	state "unknown"       -> Unknown      (HA has no value)
//	path missing          -> Unavailable  (attribute not present)
//	otherwise             -> Valid, then staleness applied
//
// A value that is not Valid or Stale must NEVER render as its content. The
// WeatherFlow audit found lightning_average_distance reporting "0.0" while the
// vendor showed 23-25 mi: `unavailable` is honest, but a rendered 0.0 reads as
// lightning directly overhead.
func (r Resolver) Resolve(b config.Binding, staleAfter time.Duration) state.Value {
	ent := r.Snap.Get(b.Entity)
	if ent == nil {
		return state.Value{Kind: state.Unavailable}
	}
	switch strings.ToLower(strings.TrimSpace(ent.State)) {
	case "unavailable":
		return state.Value{Kind: state.Unavailable, Seen: ent.Seen}
	case "unknown", "":
		return state.Value{Kind: state.Unknown, Seen: ent.Seen}
	}

	v := state.Value{
		Kind:    state.Valid,
		Str:     ent.State,
		Changed: ent.LastChanged,
		Updated: ent.LastUpdated,
		Seen:    ent.Seen,
	}
	if f, err := strconv.ParseFloat(ent.State, 64); err == nil {
		v.Num = f
	}

	if len(b.Path) > 0 {
		got, err := b.Resolve(any(ent.Attrs))
		if err != nil {
			return state.Value{Kind: state.Unavailable, Seen: ent.Seen}
		}
		v.Attr = got
		v.Str = ""
		switch t := got.(type) {
		case float64:
			v.Num, v.Str = t, strconv.FormatFloat(t, 'f', -1, 64)
		case string:
			v.Str = t
			if f, err := strconv.ParseFloat(t, 64); err == nil {
				v.Num = f
			}
		case bool:
			v.Str = strconv.FormatBool(t)
		}
	}
	return v.Staleness(staleAfter, r.Now)
}

// Guarded resolves a binding behind a `valid_when` guard.
//
// D42: the guard tests another binding's availability and nothing else, so
// D25's no-expressions line holds. When the guard fails the value becomes
// Unavailable -- never its numeric content, which is the whole point.
func (r Resolver) Guarded(b config.Binding, guard string, staleAfter time.Duration) state.Value {
	if strings.TrimSpace(guard) != "" {
		ref, err := config.ParseGuardBinding(guard)
		if err != nil {
			return state.Value{Kind: state.Fault}
		}
		g := r.Resolve(ref, 0)
		if !g.Kind.Usable() {
			return state.Value{Kind: state.Unavailable}
		}
	}
	return r.Resolve(b, staleAfter)
}

// Numbers extracts a numeric series from a collection binding, skipping
// elements whose field is missing rather than substituting zero.
func Numbers(v state.Value) []float64 {
	list, ok := v.Attr.([]any)
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(list))
	for _, el := range list {
		switch t := el.(type) {
		case float64:
			out = append(out, t)
		case string:
			if f, err := strconv.ParseFloat(t, 64); err == nil {
				out = append(out, f)
			}
		}
	}
	return out
}

// Rows extracts a collection of records from a binding like `x:aircraft[]`.
func Rows(v state.Value) []map[string]any {
	list, ok := v.Attr.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, el := range list {
		if m, ok := el.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
