# Phase 6 (revision 3) — Engineering Specification

Supersedes [r2](phase-6-engineering-r2.md), which supersedes
[r1](phase-6-engineering.md). Both are retained as the record of what each adversarial round
attacked.

Closes all ten findings from [round 2](phase-7-adversarial-r2.md) and applies the MVP scope cut
in D44. **Sections unchanged from r2 are summarised with a pointer rather than restated**; the
detail is in the changed sections, which is where a third adversarial round should look.

## Changelog

| Change | Finding |
|---|---|
| Reap on confirmed channel death, never on render latency | E1 |
| Alert stickiness in the state model, so latest-wins cannot drop an alert | E2 |
| Ring samples carry provenance; statistics beat live, newer statistics beat older | E3 |
| Bucket derived from the backfill source, not configured | E4 |
| One addressing grammar — forecast is a pseudo-entity | E5 |
| `valid_when` referents auto-bound; unbound referents rejected at load | E6 |
| Sort hysteresis **deferred**, off by default, pending measurement | E7 |
| Single audited unwrap for the token; outbound frame redactor; a test that asserts it | E8 |
| `authorized_keys` reloadable on SIGHUP | E9 |
| Safety decisions render with staleness marked, never suppressed | E10 |
| "Gusty later" leaves MVP; the rest of the decision panel ships | D44 |

---

## 0. What `home` is for

Unchanged from r2 §0 — two decisions, not a sensor list: *how do I dress or plan*, and *what do
I need to do to prepare the house*.

**MVP trigger set (D44):**

| Trigger | Data | Status |
|---|---|---|
| Gusts high now → flag down, secure items | `sensor.st_00128663_wind_gust` | ships |
| Freezing tonight → cover water fixtures | `forecast.home:daily[0].templow` (Met.no, already in HA) | ships |
| Windy later → plan ahead | `forecast.home:daily[0].wind_speed` | ships, **caveat rendered**: average, not gust |
| ~~Gusty later → pre-emptively secure~~ | needs `wind_gust_speed`; no configured provider has it | **fast-follow todo** |

Wind bindings, settled: `wind_gust` (worst case now), `wind_speed_average` (typical),
`wind_speed` unbound — a 3-second rapid sample answering no decision.

Forecast transport verified live: `weather/subscribe_forecast`, 48 hourly (~10.1 KB) and 6 daily
(~1.2 KB) entries; daily carries `templow`.

---

## 1. Modules

Unchanged from r2 §1: `cmd/hatty`, `cmd/hatty-connect`, and `internal/{ha,forecast,state,config,
model,layout,widget,render,server,telemetry}`. Dependencies strictly downward.

---

## 2. Data flow and concurrency — **revised for E1**

Pipeline unchanged from r2 §2: single-writer store, coalescing 4 Hz ticker, per-session Bubble
Tea programs, depth-1 replacing sink with `atomic.Pointer[FrameMsg]` and context cancellation.
`FrameMsg.ResolveHint` retains r2's naming and its stated scope — Bubble Tea's `View()` returns
a whole string, so it saves view-model resolution and nothing else.

### E1 — slowness is not death

r2 reaped any session whose last consumed frame was older than `3 × tick` — **750 ms**. A Pi 3B
on 2.4 GHz Wi-Fi stalls longer than that routinely, so the reaper would blank the dashboard and
force a reconnect on every ordinary hiccup: strictly worse than the stale frame it was
protecting against.

The design already tolerates a slow consumer — that is what the depth-1 replacing sink is for.
Frames are *supposed* to be dropped.

**Reap only on confirmed channel death:**

| Trigger | Action |
|---|---|
| SSH write returns an error | cancel context, tear down |
| SSH keepalive fails | cancel context, tear down |
| Session context cancelled (client disconnect) | tear down |
| **Render latency, of any magnitude** | **nothing — drop frames, keep the session** |

The goroutine-leak fix from A1 is retained in full: the sender races `Send` against
`ctx.Done()`, so no goroutine outlives its session. Only the latency threshold is removed.

---

## 3. State model — **revised for E2, E3, E4**

Five value conditions unchanged (`Valid`, `Unknown`, `Unavailable`, `Stale`, `Fault`).
`stale_after` remains optional, absent by default for `event.*` and `binary_sensor.*` (A4).
`Value` remains immutable once published, which is what makes snapshots shallow and cheap (D3).

### E2 — alerts are events, and latest-wins drops events

The depth-1 sink keeps only the newest frame. That is right for state and wrong for an alert
that goes on and off between two consumed frames — the one element reserving a permanent line
(D34) would be the one element the transport can silently discard.

Fixed in the state model rather than the transport, so latest-wins stays correct everywhere:

```go
type Alert struct {
    Active     bool
    LastActive time.Time   // set whenever Active observed true
}
// Rendered when: Active || time.Since(LastActive) < hold
```

```toml
[[panel]]
type = "alert_strip"
hold = "90s"     # an alert stays on screen this long after clearing, then decays visibly
```

A transient alert now persists across dropped frames and fades rather than vanishing.

### E3 — buckets carry provenance

r2's "idempotent, first write wins" discarded two legitimate cases: HA **revises** statistics,
and reconnect **re-fetches** them into buckets that are already full, making the repair a no-op.

```go
type Source uint8
const (Live Source = iota; Stats)

type Sample struct {
    T   time.Time
    V   float64
    Src Source
    Rev uint64      // fetch sequence; monotonic per connection-lifetime
}

func (r *Ring) Put(s Sample) {
    i := r.slot(s.T)
    cur := r.slots[i]
    switch {
    case cur.Empty():                                    r.slots[i] = s   // fill
    case s.Src == Stats && cur.Src == Live:              r.slots[i] = s   // stats beat live
    case s.Src == Stats && cur.Src == Stats && s.Rev > cur.Rev:
                                                          r.slots[i] = s   // newer correction
    default:                                                               // ignore
    }
}
```

Backfill and live updates still commute (A3's goal), *and* corrections land, *and* reconnect
backfill repairs the gap it exists for.

### E4 — bucket size is derived, never configured

A bucket smaller than the statistics period leaves slots empty and renders the chart as a comb;
larger, and samples collide and are discarded. r2 let the two be configured independently with
nothing checking.

**The bucket is derived from the series' backfill source**: a series fed by
`period: "5minute"` gets 5-minute buckets, full stop. A live-only series takes its bucket from
its declared window and capacity. `bucket` is not a config key. If a dashboard declares a
backfill source and a window that cannot produce whole buckets, that is a **load-time error**.

---

## 4. Home Assistant communication — **revised for E5**

Protocol as r2 §4 — `subscribe_entities` with an entity filter, merge-never-replace diffs,
strictly-increasing per-connection ids, `recorder/statistics_during_period`, ping/pong liveness,
backoff reconnect marking bindings `Stale`. Dependency lane pinned and proven:
`bubbletea v1.3.10`, `lipgloss v1.1.0`, `wish v1.4.7`, `ntcharts v0.5.1`, and
`charmbracelet/ssh@v0.0.0-20250128164007-98fd5ae11894` — **mandatory**, or the build fails on a
module-path mismatch. Measured load: ~5.6 msg/s, ~7.2 KB/s.

### E5 — one addressing grammar

r2 defined bindings as `entity_id[':' path]` and then wrote `forecast.daily[0].templow`, which
is neither — its own loader could not parse its own example.

**The forecast is a pseudo-entity.** `forecast.home` is populated from
`weather/subscribe_forecast` on `weather.forecast_home`, and addressed by the ordinary grammar:

```text
forecast.home:daily[0].templow
forecast.home:daily[0].wind_speed
forecast.home:hourly[3].precipitation
```

One grammar, one parser, one validator. The `forecast` package populates the pseudo-entity in
the store like any other source; nothing downstream knows it is special.

Table binding is unchanged from D41: the union of `airspace_flag_*` collections deduplicated by
`hex`, with the header stating the subset and the ten-item cap.

---

## 5. Configuration — **revised for E6, E10**

Schema as r2 §5: TOML, one dashboard per file, inlined display profile, columns declaring
`min_cols`, alert-strip variables declared as bindings (D4). Validation as r2 including B2's
width-budget check at every breakpoint.

### E6 — guards cannot silently disable what they guard

```toml
valid_when = "sensor.st_00128663_lightning_last_distance is available"
```

If that referent is not itself bound, it is never subscribed, so its condition is permanently
`Unknown`, so the guard never passes — and the failure is indistinguishable from the real fault
it was written to detect.

**Guard referents are added to the subscription set automatically at load.** A referent that
cannot be subscribed at all — a malformed id, an entity absent from HA — is a **load-time
error**, not a runtime silence.

### E10 — safety decisions are never suppressed

r2 suppressed decisions depending on a stale forecast. If the subscription drops at 18:00 and a
freeze arrives at 02:00, the user gets nothing — inverting the spec's own rule that it must
*degrade to a legible status, never to a lie*. Silence is not a legible status.

```toml
[[panel.decision]]
when   = "forecast.home:daily[0].templow < 32"
say    = "Cover external water fixtures tonight"
level  = "warn"
safety = true      # render with staleness marked; NEVER suppress
```

A `safety = true` decision renders as `Cover external water fixtures tonight · forecast 4 h old`
rather than disappearing. Suppression remains available only for decisions explicitly marked
non-safety. The default is `safety = true` — silence must be opted into.

`when` remains one binding, one operator, one literal (D25). Conjunction belongs in an HA
template binary sensor.

---

## 6. Layout

Unchanged from r2 §6: elasticity as a ranked list, validated to survive at
`min_cols × min_rows` (B1); refusal below 44 × 12; drop order trend → detail → status hints →
scroll rows, with the alert strip never eligible.

---

## 7. Rendering — **revised for E7**

Width safety unchanged and unbroken across two adversarial rounds: `go-runewidth` pinned so
ambiguous glyphs are one cell (D39), and a `Row` builder that is the only sanctioned way to
assemble a line, panicking on width mismatch in tests and truncating with a log in production.

### E7 — hysteresis is deferred, not defaulted

r2 specified bucketed sort hysteresis as though it were free. It is not: two contacts in the
same 0.5 nm bucket order by `hex`, so the table can display **12.6 above 12.4** under a header
reading "sorted by distance."

And the concern driving it — A2, SSH-link bandwidth — **is still unmeasured**. Paying a
correctness cost for an unmeasured benefit is the wrong trade.

**Plain, correct sort is the default.** Hysteresis becomes opt-in:

```toml
[panel.sort]
key       = "distance_nm"
dir       = "asc"
hysteresis = "0.0"     # 0 = exact ordering (default). >0 buckets, trading order for stability.
```

Measure first (§11 requires per-session bytes from the first commit). If N3 is genuinely
threatened, enable it — and render the bucketed value so the displayed order matches the
displayed numbers.

---

## 8. Widget contracts

Unchanged from r2 §8: widgets are pure over their view-model, never touching `state` or
performing I/O. Widget set as r2, including `decisions`.

---

## 9. Error handling

As r2 §9, with two changes:

| Failure | Behaviour |
|---|---|
| Forecast subscription drops | last forecast retained and marked stale; `safety = true` decisions **still render**, staleness shown (E10) |
| Alert clears | held for `hold`, then decays visibly (E2) |

Through-line unchanged and now enforced in both directions: **degrade to a legible status,
never to a lie — and never to silence.**

---

## 10. Testing

As r2 §10, plus tests targeting each round-2 fix:

- **E1:** a session whose writer stalls for 5 s must survive; one whose writer errors must be
  torn down, with no goroutine left behind.
- **E2:** an alert active for one tick only must remain rendered for `hold`.
- **E3:** property test — live and statistics writes in any order converge to the same ring, and
  a higher-`Rev` statistics write always wins.
- **E4:** a dashboard whose window and backfill source cannot produce whole buckets must fail at
  load.
- **E5:** one parser test covering entity, attribute, nested-dict, collection, **and forecast**
  paths.
- **E6:** a `valid_when` referent absent from HA must fail at load, not render as unavailable.
- **E8:** capture all log output during a full connect/reconnect cycle and **assert the token
  string never appears**.

---

## 11. Observability

As r2 §11. Per-session bytes written remains **required from the first commit** — it is the only
measurement of N3, and both A2 and E7 stay unresolved until it exists.

---

## 12. Packaging and security — **revised for E8, E9**

### E8 — protect the path the token actually travels

r2's `Secret` guarded `%+v` dumps but not the JSON auth frame — where either `MarshalJSON`
redacts and **auth breaks**, or the code unwraps and the marshalled bytes carry the live token
into any protocol trace. Protocol tracing is exactly what gets enabled to debug reconnects.

Three parts, all required:

1. **Keep the redacting `Secret`**, `MarshalJSON` included. Accidental marshalling stays safe.
2. **One audited unwrap.** `Secret.reveal()` is unexported and called from exactly one place —
   the function building the auth frame, which constructs it explicitly rather than by
   marshalling a struct containing the secret. That call site carries a comment saying it is the
   only one, and a test asserts the count.
3. **Redact outbound frames before logging.** The trace path scrubs `access_token` from any
   frame it records, so enabling protocol tracing cannot leak the credential.

Verified by the E8 test above, not by inspection.

### E9 — keys are reloadable

`authorized_keys` is loaded at startup and the daemon **refuses to start** if it is missing or
empty (C1, unchanged — Wish's default accepts any key). Added: **SIGHUP reloads it**, so a key
can be added or revoked without dropping every live session. Revocation takes effect for new
connections; existing sessions are unaffected until they reconnect, which is stated rather than
implied.

Otherwise as r2 §12: a 4.8 MB static `linux/arm64` binary (proven), systemd with
`Restart=on-failure`, token via `LoadCredential` or a 0600 `EnvironmentFile`, `hatty-connect`
remaining a shell script.

---

## 13. What remains open

1. **A2 / E7 — SSH-link bandwidth is still unmeasured**, and now two decisions depend on it.
   It cannot be measured before a renderer exists; per-session byte instrumentation from the
   first commit is the mitigation.
2. **The rain discrepancy is undiagnosed** — both sources read 0.00 during the audit; it needs
   capturing during actual rain.
3. **`when` expressiveness** — one binding, one operator, one literal. Untested against a real
   trigger set.
4. **This revision has not been adversarially reviewed.** Round 1 found 13; round 2 found 10, of
   which **four were defects in round 1's own fixes**. That ratio has not yet converged, and the
   round-3 fixes above — provenance in `Put`, a pseudo-entity, an audited unwrap, sticky alerts
   — are exactly the kind of new code path that produced round 2's findings.
5. **Both rounds share an author with the spec.** The findings were real, but so are the shared
   blind spots. An independent reviewer remains the highest-value unrun step.
