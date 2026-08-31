# Phase 6 — Detailed Engineering Review

Implementation-ready specification, reconciling the approved design (`docs/design/`) with the
architecture in `phase-1-plan.md` §5 and the forty decisions in `docs/decisions/`.

Written against **Go + Bubble Tea** (D29), the **100 × 32** grid (D40), and the **`ha-airspace`
entity contract** (`airspace-entity-model.md`).

This document is the input to Phase 7, whose job is to break it. Sections marked
**⚠ ATTACK THIS** are where I think it is weakest.

---

## 1. Module structure

```text
cmd/
  hatty/                  daemon: owns HA connection + state, serves SSH
  hatty-connect/          Pi-side launcher: setfont, then ssh (D5)
internal/
  ha/                     WebSocket client, protocol, reconnect, statistics
  state/                  entity store, series buffers, staleness, snapshots
  config/                 load + validate dashboards and display profiles
  model/                  resolved dashboard: panels, widgets, bindings
  layout/                 cell-grid solver: breakpoints, drop order, elastic
  widget/                 table, detail, bar, sparkline, runchart, strip, status
  render/                 theme, palette, glyph tier, width-safe assembly
  server/                 wish SSH server, session lifecycle
  telemetry/              structured logging, diagnostics
```

**Dependency direction is strictly downward.** `widget` may not import `state`; `layout` may not
import `ha`. Enforced by review, and cheaply by an import-cycle test.

---

## 2. Data flow and concurrency

This is the part most likely to contain the real bugs, so it is specified first.

```text
  HA WebSocket
       │  goroutine: ha.Client.readLoop
       ▼
  normalised Event
       │  channel (buffered, drop-oldest on overflow)
       ▼
  state.Store            ◄── SINGLE WRITER. One goroutine. Owns all mutable state.
       │
       │  marks entities dirty; does NOT push per-event
       ▼
  coalescing ticker      ◄── one FrameMsg per session per tick, not per event
       │
       ├──► session A: tea.Program.Send(FrameMsg{Snapshot, Changed})
       └──► session B: …
                │  Bubble Tea Update (single-threaded per program)
                ▼
          model → layout → widgets → View() string
                ▼
          wish → SSH → foot on the Pi
```

### The three rules that make this safe

**R-1. One writer.** `state.Store` runs a single goroutine. Everything else communicates by
channel. No mutex-guarded shared maps, because the failure mode of a forgotten lock here is a
silently corrupted dashboard rather than a crash.

**R-2. Sessions receive immutable snapshots.** `Store` hands out a `*Snapshot` — a
copy-on-write view valid for one frame. Bubble Tea models never dereference live store state.
This is what makes multiple simultaneous clients at different sizes (option C, D7) safe by
construction rather than by discipline.

**R-3. Redraws are coalesced, never per-event.** A ticker (default 4 Hz, configurable) converts
"N entities changed" into one `FrameMsg`. This is the mitigation for R1/R15 — with
`ha-airspace` throttled to 1/s (D21) the tick is usually idle, but the mechanism must exist
before a chattier source appears.

```go
type FrameMsg struct {
    Snap    *state.Snapshot
    Changed EntitySet   // which bindings need re-resolving; empty = full redraw
}
```

**⚠ ATTACK THIS.** A session that is slow to render will have `Send` block or its channel fill.
Specified behaviour: per-session send is non-blocking with a depth-1 mailbox that *replaces*
rather than queues — a stale frame is worthless, only the newest matters. Phase 7 should
check that a wedged SSH client cannot stall the store.

---

## 3. State model

### Value conditions — never collapsed

Plan §7 requires four shapes and three conditions. One type carries both:

```go
type Kind uint8
const (
    Valid Kind = iota
    Unknown       // HA has no value
    Unavailable   // HA says the entity is unreachable
    Stale         // OUR judgement: update overdue
    Fault         // the value did not parse as its declared type
)

type Value struct {
    Kind    Kind
    Str     string
    Num     float64          // valid only when Kind == Valid and the binding is numeric
    Attrs   map[string]any   // decoded attribute tree
    Changed time.Time        // HA's last_changed
    Updated time.Time        // HA's last_updated
    Seen    time.Time        // when WE last received it -- drives Stale
}
```

`Stale` is the app's own invention and the most important (N7): a frozen dashboard presented as
live is the failure the project exists to avoid. Staleness is per-binding, from a configured
`stale_after`; the default is derived from observed update cadence, not guessed.

`Fault` is separate from `Unknown` because their causes differ — `Unknown` is HA's state,
`Fault` is our parser rejecting it. Rendering them identically would hide our own bugs.

### Entity storage

```go
type Entity struct {
    ID     string
    Domain string           // sensor | binary_sensor | event
    State  Value
    Series map[string]*Ring // bounded, per bound attribute path (R8)
}
```

Only **bound** entities are stored (D6). Only **bound** attribute paths get a `Ring`. The memory
bound is therefore a function of the dashboard, not of the 981-entity instance.

`Ring` is a fixed-capacity circular buffer sized from the widget's need — a 24 h chart at
5-minute statistics is 288 points, a live 60-minute runchart at 1 Hz is 3600. Capacity is
allocated once at config load and never grows.

### Attribute paths

Bindings address nested structures, forced by the real data (`airspace-entity-model.md`):

```text
sensor.airspace_nearest_aircraft                       → state (distance nm)
sensor.airspace_nearest_aircraft:bearing_to.home       → nested dict, watchpoint-keyed
sensor.airspace_flag_military:aircraft[]               → list of dicts, drives the table
sensor.airspace_flag_military:aircraft[].distance_nm   → column within that list
```

Grammar: `entity_id` `[':' path]`, path segments dotted, `[]` marks a collection. **No
expressions, no arithmetic, no conditionals** — computation lives in HA template sensors (D25).
The line is: *hatty maps values to appearance; Home Assistant derives values.*

---

## 4. Home Assistant communication

Protocol verified against primary sources in `phase-1-plan.md` §7. Restated here as
requirements.

| Requirement | Detail |
|---|---|
| Connect | `ws://host:8123/api/websocket` |
| Auth | `auth_required` → `auth` → `auth_ok`/`auth_invalid`; token from env or a 0600 file, **never** a CLI argument (D15) |
| Message IDs | strictly increasing, **reset per connection** — the server enforces this with `ERR_ID_REUSE` (D17) |
| Subscription | `subscribe_entities` with an `entity_ids` filter (D6) |
| Diff handling | **merge, never replace** — `+` carries changes, `-` removes attributes (D18) |
| Initial state | none needed; the subscription opens with a full snapshot, so `get_states` is not called |
| History | `recorder/statistics_during_period`, `period: 5minute`, `types: [min,mean,max]` (D16) |
| Liveness | `ping`/`pong` on an interval — detects a half-open socket before the cache goes stale (R6) |

### Reconnect

Exponential backoff with jitter, capped. On reconnect: reset the ID counter, re-subscribe,
re-fetch statistics, and **mark every binding `Stale` until its first fresh value arrives** —
not `Unavailable`, because we do not know that yet, and not `Valid`, because the cached number
may be minutes old.

Because HA and the daemon share a Proxmox host (D13), the daemon must start cleanly against an
HA that is not yet listening and present a waiting state rather than exiting.

---

## 5. Configuration

The brief forbade designing this schema before the UI model existed. The UI model now exists,
so this is that schema. TOML.

```toml
# dashboards/radar.toml
name = "radar"
title = "hatty · radar"

[display]                      # inlined per dashboard, not shared (D3)
resolution = "800x480"
font       = "DejaVu Sans Mono:size=8"
cols       = 100               # authoring target (D40)
rows       = 32
min_cols   = 44                # refusal floor (D36)
min_rows   = 12
glyphs     = "full"            # full | block   -- Braille available? (D23, S2)
colors     = 256

[[panel]]
type    = "alert_strip"        # never collapses at any size (D34/D36)
reserve = true
source  = "binary_sensor.airspace_alert_military_close"
nominal = "{count} tracked · {flagged} flagged"

[[panel]]
type    = "table"
bind    = "sensor.airspace_flag_all:aircraft[]"
sort    = { key = "distance_nm", dir = "asc" }

  [[panel.column]]
  header = "FLIGHT"; path = "flight"; width = 9; min_cols = 0
  [[panel.column]]
  header = "TYPE";   path = "aircraft_type"; width = 6; min_cols = 56
  [[panel.column]]
  header = "DIST";   path = "distance_nm"; format = "%.1f"; align = "right"; min_cols = 0
  [[panel.column]]
  header = "RANGE";  path = "distance_nm"; render = "bar"; range = [0, 65]; min_cols = 68
  [[panel.column]]
  header = "ALT";    path = "alt_baro_ft"; format = "%,d"; align = "right"
  ramp   = { thresholds = [10000, 20000, 30000], palette = ["alt0","alt1","alt2","alt3"] }

[[panel]]
type    = "detail"
elastic = true                 # absorbs leftover rows on this screen (D37, refined)
follows = "table"              # renders the table's selection
```

### Validation, at load, fatal

1. Every `bind` parses as a valid attribute path.
2. Every panel's `min_cols`/`min_rows` are satisfiable within the display profile.
3. Columns' `min_cols` are monotonic with the drop order — a column cannot outlive a wider one.
4. **Exactly one** panel per dashboard has `elastic = true`.
5. Ramps have one more palette entry than thresholds.
6. Palette names resolve in the theme.

Config errors are fatal at startup with a line number. They are never tolerated at runtime,
because a dashboard that silently drops a misconfigured panel is worse than one that refuses to
start.

**⚠ ATTACK THIS.** Rule 3 is asserted, not proven. Phase 7 should construct a column set where
the drop order and the width budget disagree.

---

## 6. Layout

Pure function, no I/O, fully testable:

```go
func Solve(d *model.Dashboard, cols, rows int) (Frame, error)
```

**Algorithm.**

1. If `cols < min_cols || rows < min_rows` → return `ErrTooSmall`; caller renders the refusal
   screen (D36).
2. Select the active column set: every column whose `min_cols <= cols`.
3. Assign each panel its natural height: reserved panels (alert strip) first, then fixed panels,
   then data-driven panels capped at their data (a nine-row table asks for nine rows).
4. Distribute leftover rows to the single `elastic` panel.
5. If the total still exceeds `rows`, drop panels in declared order — trend, detail, status
   hints — until it fits. **The alert strip is never eligible.**

Step 3's cap is the D37 refinement: the table cannot absorb slack because its height is bounded
by how much traffic is overhead, so the detail pane absorbs it instead.

---

## 7. Rendering

### Width safety is a hard invariant

Every glyph the design uses except Braille and `µ` has East Asian Width **Ambiguous** (D39).
`go-runewidth` must be configured explicitly and consistently:

```go
runewidth.DefaultCondition.EastAsianWidth = false   // ambiguous = 1 cell
```

`render` exposes the only sanctioned way to build a row. Widgets do not concatenate strings
themselves:

```go
type Row struct{ /* ... */ }
func (r *Row) Cell(s string, w int, st Style) *Row   // truncates or pads to exactly w cells
func (r *Row) String() string                        // panics if != declared width in tests
```

**In test builds the width assertion panics; in production it truncates and logs.** A corrupted
frame must never reach the user, and a panic in a daemon serving several sessions must never
take them all down.

### Theme and glyph tier

`glyphs = "full" | "block"` selects the sparkline and chart implementation at render time from
the same widget (D23/D34). A dashboard definition does not change between tiers.

Colour follows D34 exactly: hues reserved for state, altitude on a sequential ramp, and every
coloured state also carrying a glyph or word so an 8-colour client degrades legibly.

---

## 8. Widget contracts

```go
type Widget interface {
    Constraints() layout.Constraints
    Render(region layout.Region, data ViewModel, th *render.Theme) []string
}
```

**Widgets are pure over their view-model.** They never touch `state`, never perform I/O, never
know what entity produced a number. This is what makes the whole rendering path testable without
a live Home Assistant, and it is the contract Phase 7 should try hardest to violate.

The MVP set, all required by the approved design:

| Widget | Used by |
|---|---|
| `alert_strip` | both screens — reserved, never collapses |
| `table` | radar — column drop, sort, selection cursor, inline bar cells |
| `detail` | radar — key/value grid, elastic |
| `bar` | home — threshold bar with half-block precision |
| `runchart` | home — multi-series Braille, or block fallback |
| `sparkline` | home |
| `hero` | home — block-digit number |
| `status_bar` | both — abbreviates before it vanishes |

---

## 9. Error handling

| Failure | Behaviour |
|---|---|
| HA unreachable at start | waiting screen, backoff retry, never exit (D13) |
| HA disconnects | bindings → `Stale`, connection indicator changes, retry |
| Auth rejected | fatal, clear message — a bad token will not fix itself |
| Value fails to parse | that binding → `Fault`, fault glyph rendered, logged once per binding per hour |
| Entity missing from HA | `Unavailable`, panel still renders |
| Terminal too small | refusal screen with the required size |
| Widget panics | recover at the session boundary, render an error panel in that region, keep the other sessions alive |
| Config invalid | fatal at startup, with a line number |

The through-line: **degrade to a legible status, never to a lie.** Every path that could show a
stale number as current instead shows it as stale.

---

## 10. Testing

| Layer | Method |
|---|---|
| Protocol | fake HA WebSocket server replaying recorded frames, including malformed ones |
| Diff merge | property test: random `+`/`-` sequences vs. a reference implementation. Targets D18 |
| State/staleness | table-driven over a fake clock |
| Layout | golden files across the full breakpoint matrix — 100×32, 80×25, 80×22, 64×20, 50×15, 43×11 |
| Width safety | every golden file asserted to exact cell width, ambiguous-width glyphs included |
| Widgets | pure input → expected `[]string` |
| Update | Bubble Tea's `Update` is a pure function; table-driven over message sequences |
| Session | Wish server + scripted SSH client: connect, resize, reconnect at a different grid (R7) |
| Soak | multi-day run with synthetic churn, watching RSS for R8 |

The breakpoint matrix and the width assertion are the two that would have caught real bugs
already — the mockup generators found four width errors during design.

---

## 11. Observability

**Logging never touches the rendered terminal.** Structured logs to a file or stderr, which for
a systemd unit means the journal. A log line written to stdout would corrupt the frame.

Emit: connection lifecycle, reconnect attempts with backoff, per-tick changed-entity count,
frame render duration, bytes written per session, binding faults (rate-limited), and config load
results.

`bytes written per session` is the direct measurement of N3, and makes S1's spike a permanent
instrument rather than a one-off.

A diagnostics overlay (a keybinding) shows connection state, last frame time, subscription
count, and the ring-buffer high-water mark, without leaving the dashboard.

---

## 12. Packaging

Single static binary, cross-compiled to the server's architecture. Deployment is a file copy
(D29). A systemd unit with `Restart=on-failure`, the token supplied via `LoadCredential` or an
`EnvironmentFile` with mode 0600, never on the command line (D15).

`hatty-connect` on the Pi is a shell script: `setfont` for the dashboard's declared font, then
`ssh`. It stays a script deliberately — it is the one component that must be trivially editable
on a machine with no Go toolchain.

---

## 13. What Phase 7 should attack

Ranked by where I think this specification is actually weak:

1. **Backpressure (§2).** A wedged SSH client and a depth-1 replacing mailbox — is the store
   genuinely isolated, or can a slow session stall it?
2. **Reconnect state machine (§4).** The `Stale`-on-reconnect rule reads well; the interleaving
   of a reconnect with an in-flight statistics fetch is unspecified.
3. **Column drop monotonicity (§5, rule 3).** Asserted, not proven.
4. **Elastic-panel solver (§6).** One elastic panel is a simplification. What happens when the
   elastic panel's own minimum exceeds the leftover?
5. **Width safety under composition (§7).** Individual cells are checked; a row assembled from
   correct cells plus a style escape could still drift.
6. **The `Fault` path (§9).** Rate-limited logging plus a fault glyph may hide a systematic
   parsing bug behind a plausible-looking dashboard.
