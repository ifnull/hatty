// Package config loads and validates dashboard definitions.
//
// The binding grammar is deliberately small. Home Assistant derives values;
// hatty maps them to appearance (D25). There are no expressions, no arithmetic
// and no conditionals -- if a dashboard needs a computed value, it belongs in
// an HA template sensor, where the template engine already exists and where the
// user already keeps such things.
package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Binding addresses one value in the state tree.
//
//	sensor.airspace_nearest_aircraft
//	sensor.airspace_nearest_aircraft:bearing_to.home     nested dict (watchpoint-keyed)
//	sensor.airspace_flag_military:aircraft[]             a collection
//	sensor.airspace_flag_military:aircraft[].distance_nm a field across a collection
//	forecast.home:daily[0].templow                       the forecast pseudo-entity
//
// FINDING E5: there is exactly ONE grammar. Revision 2 defined bindings as
// entity+path and then wrote `forecast.daily[0].templow`, which is neither --
// its own loader could not have parsed its own example. The forecast is
// populated into the store as the pseudo-entity `forecast.home`, so nothing
// downstream knows it is special.
type Binding struct {
	Entity string
	Path   []Segment
	raw    string
}

// Segment is one step of a path.
type Segment struct {
	Name    string
	Index   int  // valid when Indexed is true
	All     bool // true for "[]": project across the whole collection
	Indexed bool
}

func (b Binding) String() string { return b.raw }

// IsCollection reports whether resolving this binding yields many values.
func (b Binding) IsCollection() bool {
	for _, s := range b.Path {
		if s.All {
			return true
		}
	}
	return false
}

// ParseBinding parses the grammar above. Errors are fatal at load: a binding
// that cannot be parsed is a configuration bug, and tolerating it at runtime
// would mean a panel that silently renders nothing.
func ParseBinding(s string) (Binding, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return Binding{}, fmt.Errorf("empty binding")
	}
	entity, rest, hasPath := strings.Cut(raw, ":")
	entity = strings.TrimSpace(entity)

	dom, obj, ok := strings.Cut(entity, ".")
	if !ok || dom == "" || obj == "" {
		return Binding{}, fmt.Errorf("%q: entity id must be domain.object_id", raw)
	}
	b := Binding{Entity: entity, raw: raw}
	if !hasPath {
		return b, nil
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return Binding{}, fmt.Errorf("%q: trailing ':' with no path", raw)
	}
	for _, part := range strings.Split(rest, ".") {
		seg, err := parseSegment(part, raw)
		if err != nil {
			return Binding{}, err
		}
		b.Path = append(b.Path, seg)
	}
	return b, nil
}

func parseSegment(part, raw string) (Segment, error) {
	part = strings.TrimSpace(part)
	if part == "" {
		return Segment{}, fmt.Errorf("%q: empty path segment", raw)
	}
	open := strings.IndexByte(part, '[')
	if open < 0 {
		if strings.ContainsAny(part, "]") {
			return Segment{}, fmt.Errorf("%q: unmatched ']' in %q", raw, part)
		}
		return Segment{Name: part}, nil
	}
	if !strings.HasSuffix(part, "]") {
		return Segment{}, fmt.Errorf("%q: unterminated '[' in %q", raw, part)
	}
	name := part[:open]
	if name == "" {
		return Segment{}, fmt.Errorf("%q: index with no field name in %q", raw, part)
	}
	inner := part[open+1 : len(part)-1]
	if inner == "" {
		return Segment{Name: name, All: true}, nil
	}
	n, err := strconv.Atoi(inner)
	if err != nil || n < 0 {
		return Segment{}, fmt.Errorf("%q: index %q must be a non-negative integer or empty", raw, inner)
	}
	return Segment{Name: name, Index: n, Indexed: true}, nil
}

// ErrNotFound means the path does not exist in the supplied tree. It is a
// normal condition -- an attribute Home Assistant has not sent -- and resolves
// to Unavailable, never to a zero value.
type ErrNotFound struct{ At string }

func (e ErrNotFound) Error() string { return "not found: " + e.At }

// Resolve walks the binding's path through a decoded attribute tree.
//
// A "[]" segment projects the remaining path across every element, returning
// []any. Elements whose path is missing are SKIPPED rather than filled with a
// zero -- a table row missing a field must show nothing there, not 0.
func (b Binding) Resolve(root any) (any, error) {
	cur := root
	for i, seg := range b.Path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, ErrNotFound{At: seg.Name}
		}
		next, ok := m[seg.Name]
		if !ok {
			return nil, ErrNotFound{At: seg.Name}
		}
		if !seg.All && !seg.Indexed {
			cur = next
			continue
		}
		list, ok := next.([]any)
		if !ok {
			return nil, fmt.Errorf("%s: expected a collection, got %T", seg.Name, next)
		}
		if seg.Indexed {
			if seg.Index >= len(list) {
				return nil, ErrNotFound{At: fmt.Sprintf("%s[%d]", seg.Name, seg.Index)}
			}
			cur = list[seg.Index]
			continue
		}
		// "[]": project the remaining path across every element.
		rest := Binding{Path: b.Path[i+1:]}
		out := make([]any, 0, len(list))
		for _, el := range list {
			if len(rest.Path) == 0 {
				out = append(out, el)
				continue
			}
			v, err := rest.Resolve(el)
			if err != nil {
				continue // missing field: skip, never substitute a zero
			}
			out = append(out, v)
		}
		return out, nil
	}
	return cur, nil
}

// ParseGuardBinding extracts the binding from a `<binding> is available` guard.
// Exported so the model layer can resolve the referent without duplicating the
// grammar.
func ParseGuardBinding(s string) (Binding, error) {
	f := strings.Fields(strings.TrimSpace(s))
	if len(f) != 3 || f[1] != "is" || f[2] != "available" {
		return Binding{}, fmt.Errorf("valid_when must read `<binding> is available`, got %q", s)
	}
	return ParseBinding(f[0])
}
