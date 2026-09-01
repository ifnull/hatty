package config

import "time"

// Dashboard is one screen. A dashboard is a screen, not a container of pages
// (D30): the CLI argument selects the whole thing, and switching costs a
// reconnect -- which is free under the daemon architecture (D7).
type Dashboard struct {
	Name    string  `toml:"name"`
	Title   string  `toml:"title"`
	Display Display `toml:"display"`
	Panels  []Panel `toml:"panel"`
}

// Display is the deployment profile, inlined per dashboard rather than shared
// (D3): at three dashboards the redundancy is a dozen lines, and inlining keeps
// each file self-contained and portable.
//
// Cols/Rows are the AUTHORING target. The app only ever sees cells; font and
// resolution are client-side and cannot be enforced (D4), so they are recorded
// for provisioning and to warn on mismatch.
type Display struct {
	Resolution string `toml:"resolution"`
	Font       string `toml:"font"`
	Cols       int    `toml:"cols"`
	Rows       int    `toml:"rows"`
	MinCols    int    `toml:"min_cols"`
	MinRows    int    `toml:"min_rows"`
	Glyphs     string `toml:"glyphs"` // "full" (Braille available) | "block"
	Colors     int    `toml:"colors"`
}

// Panel is one region of a screen.
type Panel struct {
	Type string `toml:"type"`
	Bind string `toml:"bind"`

	// Binds unions several collections into one table.
	//
	// FINDING F0: D41 specifies the table as "the union of the flag
	// collections, deduplicated by hex", because ha-airspace has no
	// full-aircraft-list entity -- only airspace_flag_* collections, each
	// capped at ten. Binding one of them showed two contacts against 71
	// tracked, which is why the live screen looked empty.
	Binds   []string `toml:"binds"`
	Dedupe  string   `toml:"dedupe"`  // record key that identifies a duplicate
	Reserve bool     `toml:"reserve"` // never collapses at any size (alert strip)

	// Elastic is a RANK, not a flag (finding B1). Revision 2 required exactly
	// one elastic panel and then let the solver drop it, leaving zero at
	// precisely the small grids where the rule matters. Leftover rows go to the
	// highest-ranked SURVIVING panel. Zero means not elastic.
	Elastic int `toml:"elastic"`

	Follows string `toml:"follows"`

	// Left and Keys are LITERAL text for the status bar, not bindings. Reusing
	// Source/Nominal for them made validation reject "hatty · radar" as a
	// malformed entity id -- correctly, since Source is a binding.
	Left    string            `toml:"left"`
	Keys    string            `toml:"keys"`
	Source  string            `toml:"source"`
	Nominal string            `toml:"nominal"`
	Hold    duration          `toml:"hold"` // alert stickiness (finding E2)
	Vars    map[string]string `toml:"vars"` // declared template bindings (finding D4)

	// Chart panels overlay several series on one axis (D16).
	Series []SeriesSpec `toml:"series"`
	Unit   string       `toml:"unit"`
	Window duration     `toml:"window"`

	Sort      *Sort      `toml:"sort"`
	Columns   []Column   `toml:"column"`
	Fields    []Field    `toml:"field"`
	Decisions []Decision `toml:"decision"`
}

// Sort controls table ordering.
type Sort struct {
	Key string `toml:"key"`
	Dir string `toml:"dir"`
	// Hysteresis buckets the sort key to stop noise reordering rows.
	//
	// FINDING E7: this trades SORT CORRECTNESS for stability -- two contacts in
	// one bucket order by tiebreak, so the table can show 12.6 above 12.4 under
	// a header saying "sorted by distance". Revision 2 presented it as free.
	// It defaults to 0 (exact ordering) because the concern driving it -- SSH
	// link bandwidth -- is still unmeasured, and paying a known cost for an
	// unmeasured benefit is the wrong trade.
	Hysteresis float64 `toml:"hysteresis"`
}

// Column is one table column.
type Column struct {
	Header string `toml:"header"`
	Path   string `toml:"path"`
	Width  int    `toml:"width"`
	Align  string `toml:"align"`
	Format string `toml:"format"`
	Render string `toml:"render"`
	// MinCols is the narrowest terminal at which this column survives. It IS
	// the drop order; there is no separate declaration (finding B2).
	MinCols int       `toml:"min_cols"`
	Range   []float64 `toml:"range"`
	Ramp    *Ramp     `toml:"ramp"`
}

// Ramp maps a value to a palette entry. Sequential, for continuous categories
// like altitude -- never a traffic light, because red/amber/green are reserved
// for state (D34).
type Ramp struct {
	Thresholds []float64 `toml:"thresholds"`
	Palette    []string  `toml:"palette"`
}

// SeriesSpec is one line on a chart.
type SeriesSpec struct {
	Label string `toml:"label"`
	Bind  string `toml:"bind"`
	Color string `toml:"color"` // palette name; sequential family, not state hues
}

// Field is one key/value line in a detail panel.
type Field struct {
	Label  string `toml:"label"`
	Bind   string `toml:"bind"`
	Format string `toml:"format"`
	// ValidWhen guards a value that is only trustworthy under a condition.
	//
	// D42: the WeatherFlow integration reports lightning_average_distance as
	// "0.0 mi" while the vendor app shows 23-25 mi. `unavailable` is honest;
	// a rendered 0.0 reads as lightning OVERHEAD. The guard tests another
	// binding's availability and nothing else, so D25's no-expressions line
	// holds.
	ValidWhen string `toml:"valid_when"`
}

// Decision is a trigger, a threshold, and the action it implies. The reason
// `home` exists (D43).
type Decision struct {
	When   string `toml:"when"`
	Say    string `toml:"say"`
	Level  string `toml:"level"`
	Detail string `toml:"detail"`
	// Safety decisions render with staleness marked rather than being
	// suppressed (finding E10). Silence is not a legible status: a freeze
	// warning withheld because the forecast went stale is how the fixtures
	// freeze. Defaults to TRUE -- silence must be opted into.
	Safety *bool `toml:"safety"`
}

// SafetyOrDefault reports whether this decision is safety-relevant.
func (d Decision) SafetyOrDefault() bool { return d.Safety == nil || *d.Safety }

type duration struct{ time.Duration }

func (d *duration) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}
