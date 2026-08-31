# Phase 6 (revision 2) — Engineering Specification

Supersedes [`phase-6-engineering.md`](phase-6-engineering.md), which is retained as the record
of what Phase 7 attacked. This revision closes all thirteen Phase 7 findings and absorbs one
product change that arrived afterwards and matters more than any of them.

## Changelog

| Change | Source |
|---|---|
| Session sender owns a context; wedged-session reaper | A1 |
| Sort hysteresis on the aircraft table | A2 |
| Ring buffers are timestamp-bucketed, merge is idempotent | A3 |
| `stale_after` is optional; absent means "no cadence expected" | A4 |
| Elasticity is a ranked list, validated at the smallest grid | B1 |
| Column validation is a width-budget check at every breakpoint | B2 |
| `FrameMsg.Changed` → `ResolveHint`, with scope stated | B3 |
| `authorized_keys` required; fail closed | C1 |
| `Secret` type with redacting `String`/`GoString`/`MarshalJSON` | C2 |
| `Value` immutability stated as the invariant that makes snapshots cheap | D3 |
| Alert-strip template variables are declared bindings | D4 |
| Dependency lane pinned and proven to build | D2 (resolved) |
| Update rates measured; R15 quantified | D1 (resolved) |
| Table binds *flagged* aircraft, and says so | D41 |
| `valid_when` guard; corrected wind and battery bindings | D42 |
| **`home` is decision-oriented; forecast subscription added** | **D43** |

---

## 0. What `home` is for — the product change

The user's framing, which replaces "display the weather sensors":

> *"It breaks down into: how should I dress or plan when leaving the house, and what if anything
> do I need to do to make sure the house is prepared for the conditions."*

Two decisions, not a sensor list:

**Decision 1 — leaving the house.** Needs *typical* conditions: `wind_speed_average`, feels-like,
humidity, UV, and the next few hours of forecast.

**Decision 2 — preparing the house.** Needs *worst case*, now and ahead:

| Trigger | Action | Data |
|---|---|---|
| Gusts high now | take the flag down, secure loose items | `sensor.st_00128663_wind_gust` (live) |
| Windy later today | same, planned | `weather/subscribe_forecast` daily `wind_speed` |
| Freezing tonight | cover external water fixtures | daily forecast **`templow`** |
| Rain starting | — | `event.st_00128663_precipitation_start` |

This settles the wind question raised against D42: **bind all three, they answer different
questions.** `wind_gust` is the worst case (decision 2), `wind_speed_average` is the typical
case (decision 1), and `wind_speed` — the 3-second rapid sample — is bound by neither, because
it answers no decision and only flickers.

### Forecast — verified live, not assumed

`weather.forecast_home` (Met.no) exposes no forecast in its attributes; modern HA moved it to a
subscription. Tested against the live instance:

```jsonc
{"id": N, "type": "weather/subscribe_forecast",
 "entity_id": "weather.forecast_home", "forecast_type": "hourly" | "daily"}
```

| Type | Entries | Payload | Fields |
|---|---|---|---|
| `hourly` | 48 | ~10.1 KB | condition, datetime, temperature, precipitation, humidity, uv_index, wind_bearing, wind_speed, cloud_coverage |
| `daily` | 6 | ~1.2 KB | as above **plus `templow`** |

It is a *subscription*, so it pushes updates and fits the existing event pipeline with no new
transport.

**Two gaps, stated rather than papered over.** Met.no supplies no `wind_gust_speed` and no
`precipitation_probability`. So "will it be gusty enough to take the flag down *later*" is not
answerable from this provider — only "will average wind be high". If that decision matters
enough, a second provider is needed; that is a product call, not an engineering one.

`templow` on the daily forecast is present, so **the freeze warning works**, which is the
highest-consequence of the stated triggers.

---

## 1. Module structure

Unchanged from r1 §1, plus `internal/forecast` for the weather subscription (distinct from
`internal/ha` because its lifecycle and payload shape differ from entity subscriptions).

---

## 2. Data flow and concurrency — **revised for A1, B3, D3**

The pipeline is unchanged: single-writer store, coalescing ticker, per-session Bubble Tea
programs. Three corrections.

### A1 — the sender must be cancellable, and slow sessions must be reaped

`tea.Program.Send()` blocks when the program's event loop blocks, which happens whenever the
renderer's SSH write blocks — a suspended Pi or a Wi-Fi flap does exactly this. The r1 design
would park a goroutine per such session, forever.

```go
type sessionSink struct {
    prog   *tea.Program
    latest atomic.Pointer[FrameMsg]  // depth-1, replacing
    wake   chan struct{}             // capacity 1
    ctx    context.Context
    cancel context.CancelFunc
    lastConsumed atomic.Int64        // unix nanos, set from Update
}
```

- The store never calls `Send`. It stores the newest frame and pokes `wake` non-blockingly.
- One goroutine per session drains `wake` and calls `Send`, **racing it against `ctx.Done()`**.
- `Update` stamps `lastConsumed` on every `FrameMsg`.
- A reaper cancels any session whose `lastConsumed` is older than `3 × tick` and closes its SSH
  channel. A client that cannot keep up with 4 Hz has stopped being a client.

The store is now genuinely isolated: no store-side call can block on a session.

### B3 — `Changed` renamed, scope stated

```go
type FrameMsg struct {
    Snap        *state.Snapshot
    ResolveHint EntitySet   // skip view-model resolution for untouched bindings.
                            // NOT a render hint: Bubble Tea's View() returns a whole
                            // string and there is no partial render.
}
```

Requirement F5 ("redraw only changed regions") is satisfied by **Bubble Tea's line-diffing
renderer**, below the application. hatty cannot influence it and must not try; returning a
partial `View()` produces a corrupted frame.

### D3 — the invariant that makes snapshots cheap

**`Value` is immutable once published.** Updates replace the pointer; nothing mutates in place.
A `Snapshot` is therefore a shallow map of pointers — cheap to take at 4 Hz, and safe to hand
to any number of sessions. Any code that mutates a published `Value` is violating a written
rule.

---

## 3. State model — **revised for A3, A4, D42**

`Value`'s five conditions (`Valid`, `Unknown`, `Unavailable`, `Stale`, `Fault`) are unchanged.

### A4 — staleness is opt-in

```toml
stale_after = "5m"    # optional. ABSENT means this entity has no expected cadence.
```

Absent is the default for `event.*`, `binary_sensor.*` and the flag collections. A lightning
event entity that has not fired in three days is *correct*, not stale; marking it stale trains
the user to ignore the indicator, destroying the mechanism that protects N7.

Staleness applies only where a cadence genuinely exists — the weather and airspace sensors,
whose measured rates are in `S1-event-payload/RESULTS.md`.

### A3 — series are timestamp-bucketed

```go
type Ring struct {
    bucket  time.Duration   // e.g. 5m for statistics, 1s for live
    origin  time.Time
    slots   []Sample        // fixed capacity, allocated once
}
func (r *Ring) Put(t time.Time, v float64)  // idempotent; ignores an older sample for a filled slot
```

Backfill and live updates then **commute**. A statistics response landing after live samples
have arrived cannot overwrite newer data or push points out of order, so the chart can no
longer run backwards in time — which is what r1's append-ordered ring allowed.

### D42 — guarded bindings

```toml
valid_when = "sensor.st_00128663_lightning_last_distance is available"
```

A guard, not an expression language: it tests another binding's condition and nothing else.
D25's line holds. When the guard fails the value renders as `Unavailable`, never as its
numeric content — which is the difference between an honest blank and a rendered `0.0 mi`
that reads as *lightning overhead*.

---

## 4. Home Assistant communication — **revised for D2, D41, D43**

Protocol requirements from r1 §4 stand, with additions:

| Requirement | Detail |
|---|---|
| Forecast | `weather/subscribe_forecast`, `forecast_type: daily` and `hourly` — verified live (§0) |
| Dependency pin | `github.com/charmbracelet/ssh@v0.0.0-20250128164007-98fd5ae11894`, **mandatory**: without it `go mod tidy` resolves v0.4.3 which declares `charm.land/ssh` and the build fails |
| Measured load | ~5.6 msg/s, ~7.2 KB/s for the bound set; two flag sensors at ~4 KB each dominate |

The proven lane: `bubbletea v1.3.10`, `lipgloss v1.1.0`, `wish v1.4.7`, `ntcharts v0.5.1`.
Builds and cross-compiles to a 4.8 MB `linux/arm64` static binary.

### D41 — the aircraft table is a filtered subset

There is no full-aircraft-list entity. `aircraft_count` is 94; the only collections are
`airspace_flag_*`, each carrying `aircraft: LIST[10]` — **truncated**, since
`flag_interesting` reports `state='11'` against a ten-item list.

The table binds the **union of the flag collections**, deduplicated by `hex`. The header must
state the subset and the truncation, because the count and the list legitimately disagree:

```text
FLAGGED  17 of 94 tracked        (lists capped at 10 per flag)
```

Silently showing ten of eleven military contacts is the kind of quiet wrongness this project
exists to avoid.

---

## 5. Configuration — **revised for B2, D4, D42, D43**

### D4 — no undeclared template variables

r1 allowed `nominal = "{count} tracked · {flagged} flagged"` with `{count}` and `{flagged}`
bound to nothing — hard-coded behaviour smuggled inside the config system meant to prevent it.
Variables are now declared bindings:

```toml
[[panel]]
type    = "alert_strip"
reserve = true
source  = "binary_sensor.airspace_alert_military_close"
nominal = "{tracked} tracked · {flagged} flagged"

  [panel.vars]
  tracked = "sensor.airspace_aircraft_count"
  flagged = "sensor.airspace_flag_military"
```

### D43 — a decision panel

The `home` screen's reason to exist. A trigger, a threshold, and the action it implies:

```toml
[[panel]]
type = "decisions"

  [[panel.decision]]
  when   = "sensor.st_00128663_wind_gust > 25"
  say    = "Take the flag down · secure loose items"
  level  = "warn"

  [[panel.decision]]
  when   = "forecast.daily[0].templow < 32"
  say    = "Cover external water fixtures tonight"
  level  = "warn"
  detail = "low {forecast.daily[0].templow}°F"
```

`when` is a **threshold comparison against one binding** — an operator, a literal, nothing more.
It is deliberately not an expression language; if a condition needs `and`, it belongs in an HA
template binary sensor (D25). Phase 7 should test whether that restriction survives contact
with a real trigger set.

### B2 — the validation rule that actually checks something

r1's rule 3 compared `min_cols` to itself and could never fail. Replaced:

> **For every distinct `min_cols` value in a table's column set, evaluate the surviving columns
> at exactly that width; the sum of their widths plus separators must fit.**

Checkable, and it is the property that actually prevents a corrupted table. Remaining rules from
r1 §5 stand, with rule 4 replaced by B1 below.

---

## 6. Layout — **revised for B1**

r1 required *exactly one* elastic panel and then allowed the solver to drop it, leaving zero at
precisely the small grids where D37 matters.

**Elasticity is a ranked list.** Panels declare `elastic = <rank>`; the solver gives leftover
rows to the highest-ranked *surviving* panel. Validation checks that at least one
elastic-capable panel survives at `min_cols × min_rows`, not merely that one exists at the
authoring size.

On `radar`: detail pane rank 1, table rank 2. Below 24 rows the detail pane is dropped and the
table becomes elastic — leftover rows go to more aircraft, which is exactly right when there is
no room for a record.

Solver steps otherwise as r1 §6. Refusal floor unchanged at 44 × 12.

---

## 7. Rendering — **revised for A2**

Width safety, `go-runewidth` pinning and the `Row` builder are unchanged from r1 §7 — Phase 7
could not break them.

### A2 — sort hysteresis

Sorting the table by a continuously-changing value means any rank change rewrites every row
below it. Bubble Tea diffs line by line, so two aircraft crossing in range triggers a near-full
32-line repaint — roughly 6 KB, and at 4 Hz that is an order of magnitude over the Phase 1
estimate.

**Sort on a bucketed key**, and hold order otherwise:

```go
// distance bucketed to 0.5 nm; ties broken by hex, which never changes.
key := struct{ bucket int; hex string }{ int(distNM * 2), a.Hex }
```

A contact must move a full bucket to change rank, so noise cannot reorder rows. This is a
readability fix as much as a bandwidth one — a table whose rows swap continuously cannot be
read at a glance, which is the entire point of the screen.

**Still unmeasured.** S1 measured HA → daemon (7.2 KB/s, fine, same-host bridge). The
daemon → Pi SSH link is what N3 constrains and it cannot be measured until a renderer exists.
Instrument bytes-written-per-session from the first commit (§11) so the number arrives with the
first working build rather than after a redesign.

---

## 8. Widget contracts

Unchanged from r1 §8 — Phase 7 could not break widget purity. One addition: a `decisions` widget
(D43), rendering trigger/action pairs with `level` mapped to the D34 state palette.

---

## 9. Error handling

Unchanged from r1 §9, plus:

| Failure | Behaviour |
|---|---|
| `valid_when` guard fails | value renders `Unavailable`, never its numeric content |
| Forecast subscription drops | last forecast retained, marked stale by its own timestamp; decisions depending on it are suppressed, not evaluated against stale data |
| Flag list truncated | header states the subset and the cap (D41) |

---

## 10. Testing

r1 §10 stands. Additions:

- **Backfill/live commutativity (A3):** property test — apply statistics and live samples in
  both orders, assert identical ring contents.
- **Sender liveness (A1):** a session whose writer blocks must be reaped without the store
  stalling. Fake a wedged `io.Writer`.
- **Sort stability (A2):** feed jittering distances; assert row order changes only when a
  contact crosses a bucket.
- **Column budget (B2):** generated column sets, asserting the width budget holds at every
  breakpoint.
- **Elastic survival (B1):** assert some elastic panel survives at `min_cols × min_rows`.
- **Guard semantics (D42):** a guarded binding whose guard is unavailable must never render its
  numeric value.

---

## 11. Observability

r1 §11 stands. `bytes written per session` is promoted from useful to **required from the first
commit** — it is the only measurement of N3, and A2 remains open until it exists.

Logging never touches the rendered terminal. With C2's `Secret` type, a token cannot reach a log
line, a panic dump, or the diagnostics overlay.

---

## 12. Packaging and security — **revised for C1, C2**

### C1 — fail closed

```go
wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
    return authorized.Contains(key)   // loaded from authorized_keys at startup
})
```

Wish's default accepts **any** public key. The daemon holds a Home Assistant token and serves
home-relative aircraft bearings and distances — location-inferring data on a LAN service. It
**must refuse to start** if `authorized_keys` is missing or empty. r1's "authentication is
enforced by the app" described protection that did not exist.

### C2 — the token cannot be printed

```go
type Secret string
func (Secret) String() string        { return "[redacted]" }
func (Secret) GoString() string      { return "[redacted]" }
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"[redacted]"`), nil }
```

Total rather than discouraged. `%+v` on the client struct, a JSON debug dump and the
diagnostics overlay all become safe by construction.

Packaging otherwise as r1 §12: a 4.8 MB static `linux/arm64` binary (proven), a systemd unit
with `Restart=on-failure`, token via `LoadCredential` or a 0600 `EnvironmentFile`, and
`hatty-connect` remaining a shell script.

---

## 13. What remains open

1. **A2 is mitigated but unverified.** Sort hysteresis is specified; the SSH-link bandwidth it
   protects cannot be measured until a renderer exists.
2. **Forecast gaps (§0).** No `wind_gust_speed` and no `precipitation_probability` from Met.no.
   "Will it be gusty later" is unanswerable without a second provider — a product decision.
3. **The WeatherFlow rain discrepancy is undiagnosed.** Both sources read 0.00 during the audit;
   it needs capturing during actual rain (`weatherflow-data-audit.md` §4).
4. **`when` expressiveness (§5).** One binding, one operator, one literal. Phase 7 should try to
   find a decision that genuinely needs conjunction and see whether pushing it into an HA
   template binary sensor is acceptable.
5. **This revision has not been adversarially reviewed.** Phase 7 attacked r1. The fixes are
   new code paths — a reaper, a bucketed ring, a ranked solver — and new code paths are where
   bugs live.
