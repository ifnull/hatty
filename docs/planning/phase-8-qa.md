# Phase 8 — QA and Validation Strategy

Written against [r3](phase-6-engineering-r3.md). The brief's requirement is *"measurable
acceptance criteria for the MVP"* — so §5 is the point of this document, and everything before
it exists to make those numbers obtainable.

Several targets have been deferred since Phase 1 as "to be set from measurement." The
measurements now exist (`S1-event-payload/RESULTS.md`, `S2-glyph-coverage/RESULTS.md`,
`environment.md`), so they are set here.

---

## 1. Test levels

| Level | Runs | Needs | Speed |
|---|---|---|---|
| **L1 pure** | `Update`, layout solver, widgets, ring, parser | nothing | ms |
| **L2 golden** | full render at every breakpoint | fixtures | ms |
| **L3 fake HA** | protocol, reconnect, diff merge, backfill | recorded frames | seconds |
| **L4 session** | Wish + scripted SSH client | loopback | seconds |
| **L5 live** | against the real instance | HA + token | minutes |
| **L6 soak** | 7-day stability | deployed daemon | days |

L1–L4 run in CI on every commit. L5 runs on demand. L6 runs before declaring MVP done.

---

## 2. Fixtures

**Recorded frames are the foundation.** A live capture of `subscribe_entities` traffic — the
real instance, real aircraft, real weather — makes L3 realistic instead of imagined. S1's
harness already connects and decodes; extending it to record raw frames costs almost nothing and
yields a fixture that no hand-written mock can match.

Required fixtures:

| Fixture | Contents | Source |
|---|---|---|
| `frames-busy.jsonl` | ~5 min of live traffic, aircraft overhead | live capture |
| `frames-quiet.jsonl` | ~5 min, empty sky | live capture |
| `frames-malformed.jsonl` | hand-edited: wrong types, missing keys, `null` where a number is expected, an attribute list that becomes a scalar | derived |
| `stats-24h.json` | a real `recorder/statistics_during_period` response | live capture |
| `forecast-daily.json` | a real `weather/subscribe_forecast` response, incl. a sub-freezing `templow` | live capture, or edited |
| `config-invalid/*.toml` | one file per validation rule, each failing exactly one | hand-written |

### Fixture privacy — captures are location data

A raw capture carries `distance_nm`, `bearing_deg`, `hex` and `flight` per aircraft. Against
public ADS-B history those **triangulate the watchpoint** — D27 applies to test fixtures exactly
as it applies to config.

`fixtures/sanitize.py` rotates bearings by a constant, scales distances by a factor,
pseudonymises identifiers via a salted hash, and drops lat/lon outright — while preserving frame
count, timing, payload sizes, diff structure, list truncation, value conditions and every type
shape. Realistic where the tests care; useless for locating anyone.

**Captured and committed 2026-08-31:** `fixtures/frames-busy.jsonl`, 120 real frames, sanitised.

The malformed set matters most. HA states are strings, this instance has 46 template sensors
(`environment.md`), and the WeatherFlow audit already found `unavailable` where a number was
expected and `0.0` where the truth was 23–25 mi. **Bad data is the normal case, not the edge
case.**

---

## 3. Coverage by the brief's categories

| Brief category | Level | Approach |
|---|---|---|
| Unit tests | L1 | `Update` is a pure function; layout and widgets take data and return strings |
| Integration against HA | L5 | connect, subscribe, backfill, verify against `/api/states` |
| Simulated state changes | L3 | replay recorded frames at 1×, 10×, 100× |
| Disconnect / reconnect | L3 | kill the fake mid-stream; assert `Stale`, backoff, resubscribe, id reset (D17) |
| Terminal resize | L4 | scripted SIGWINCH across the breakpoint matrix |
| Different dimensions | L2 | golden files at 100×32, 80×25, 80×22, 64×20, 50×15, 43×11 |
| Slow / high-latency SSH | L4 | `tc netem` — 200 ms RTT, 2 % loss, and a hard 5 s stall (tests E1) |
| Unsupported terminal capabilities | L2 | render with `glyphs = "block"` and 8-colour theme; assert legibility invariants |
| Missing entities | L3 | bind an entity the fake never sends; assert `Unavailable`, panel still renders |
| Invalid configuration | L1 | one fixture per rule; assert load fails with a line number |
| HA restarts | L5 | restart HA, assert unattended recovery |
| Server / Pi reboot | L5 | reboot both; assert the dashboard returns with no human action |
| Long-running stability | L6 | 7-day soak, RSS and goroutine count sampled every minute |
| **Interaction safety** | L1 | **MVP is read-only (D9): assert the binary constructs no `call_service` message.** A compile-time-adjacent test — grep the built binary for the string, and assert no code path builds one |

The last is the honest version of "interaction safety tests" for a read-only MVP: the strongest
guarantee is that the capability is absent, and that is directly assertable.

---

## 4. Tests targeting specific findings

Every adversarial finding that survived into r3 gets a test. Without this, the review rounds
were an essay.

| Finding | Test |
|---|---|
| A1 / E1 | writer stalls 5 s → session survives; writer errors → torn down, zero goroutines leaked |
| A2 / E7 | measure bytes/frame under recorded busy traffic; assert against the N3 budget |
| A3 / E3 | property: live and statistics writes in any order converge; higher `Rev` always wins |
| A4 | an entity with no `stale_after` never renders `Stale`, however long it is quiet |
| B1 | some elastic panel survives at `min_cols × min_rows` for every shipped dashboard |
| B2 | generated column sets; width budget holds at every breakpoint |
| B3 | `View()` output length equals the declared grid, always |
| C1 / E9 | daemon refuses to start with no `authorized_keys`; SIGHUP reloads |
| **C2 / E8** | **capture all log output across connect → disconnect → reconnect with tracing at maximum verbosity; assert the token string appears zero times** |
| D18 | property: random `+`/`-` diff sequences match a reference implementation |
| D41 | header states the subset when `count > len(list)` |
| D42 / E6 | a guarded value never renders numerically when its guard is unavailable; an unbound referent fails at load |
| E2 | an alert active for one tick remains rendered for `hold`, then decays |
| E4 | window and backfill source that cannot produce whole buckets → load error |
| E5 | one parser test covering entity, attribute, nested dict, collection, and forecast paths |
| E10 | forecast goes stale → a `safety = true` decision still renders, staleness shown |

---

## 5. Acceptance criteria — MVP is done when all of these hold

### Performance

| ID | Criterion | Target | Basis |
|---|---|---|---|
| **P1** | Daemon CPU, steady state | **< 5 % of one core**, mean over 1 h | 5.6 msg/s measured (S1), 4 Hz render |
| **P2** | SSH bytes to the Pi, sustained | **≤ 8 KB/s** mean over 10 min on `radar` under busy traffic | parity with the measured 7.2 KB/s HA link; 64 kbps is negligible on the Pi's Wi-Fi |
| **P3** | SSH bytes, peak | **≤ 32 KB/s** over any 1 s | allows ~5 full repaints/s before hysteresis (E7) is needed |
| **P4** | Reattach to a warm daemon → first paint | **< 200 ms** | the project's whole premise: the browser's ~10 s launch (D24) |
| **P5** | Cold daemon start → first paint | **< 10 s** incl. HA connect and backfill | must not be worse than the thing it replaces |
| **P6** | Daemon RSS after 7 days | **≤ 64 MB**, growth < 5 % over the final 5 days | bounded rings + 26 bound entities |
| **P7** | Pi-side application memory | **0** — no hatty process on the Pi | thin client (N1). `foot` measured at 14.5 MB |

P2 and P3 are the numbers N3 has lacked since Phase 1. They are budgets to validate against, not
measurements — A2/E7 remain open until P2 is measured on a real renderer.

### Correctness

| ID | Criterion |
|---|---|
| **C1** | Every rendered line is exactly the terminal width, at every breakpoint, with ambiguous-width glyphs present |
| **C2** | Golden files match at 100×32, 80×25, 80×22, 64×20, 50×15; refusal screen at 43×11 |
| **C3** | The five value conditions never collapse: `Unavailable`, `Unknown`, `Stale`, `Fault` and `Valid` are each visually distinct |
| **C4** | A guarded binding never renders a numeric value while its guard fails |
| **C5** | No malformed-fixture input causes a panic that reaches the user or kills another session |

### Reliability

| ID | Criterion |
|---|---|
| **R1** | Survives 20 HA restarts with no human action; every binding returns to `Valid` |
| **R2** | Survives Pi reboot, daemon-host reboot, and simultaneous reboot (D13) unattended |
| **R3** | Survives 100 SSH connect/disconnect cycles with zero goroutine growth |
| **R4** | Survives 5 s and 60 s network stalls without disconnecting (E1) |
| **R5** | 7-day soak: no crash, no unbounded growth, no stuck `Stale` on a live entity |

### Security

| ID | Criterion |
|---|---|
| **S1** | Token appears zero times in all log output at maximum verbosity, across a full reconnect cycle |
| **S2** | Daemon refuses to start with missing or empty `authorized_keys` |
| **S3** | An unauthorised public key is rejected |
| **S4** | No `call_service` message can be constructed (D9) |
| **S5** | No home coordinates, LAN addresses, or tokens in the repository (D27) |

### Product

| ID | Criterion |
|---|---|
| **U1** | `radar` and `home` render correctly against the live instance at 100×32 |
| **U2** | The aircraft table states its subset and cap when `count > len(list)` (D41) |
| **U3** | The freeze decision fires against a fixture with sub-freezing `templow` |
| **U4** | The battery indicator agrees with the vendor's assessment (D42) — no standing warning on a healthy battery |
| **U5** | A dashboard change requires no source edit or rebuild (F10) |

---

## 6. What cannot be tested, and the compensation

**The SSH bandwidth budget (P2/P3) cannot be validated before a renderer exists.** Compensation:
per-session byte instrumentation is required from the first commit (r3 §11), so the number
arrives with the first working build rather than after a redesign.

**Readability at arm's length is not automatable.** Compensation: the grid was chosen by the
user reading the real panel across four measured font sizes (D40), and the fallback (80×25) is
already a verified breakpoint.

**The WeatherFlow rain discrepancy cannot be reproduced without rain.** Compensation: an
instrumented capture during the next rainfall, recording `precipitation`,
`precipitation_intensity` and the `precipitation_start` event alongside the vendor app.

**Independent adversarial review.** Two rounds have shared an author with the spec. No test
substitutes; it remains the highest-value unrun step.

---

## 7. CI

L1–L4 on every commit: build, cross-compile to `linux/arm64`, unit, golden, fake-HA, session.
Plus two gates that would each have caught a real defect already:

1. **Clean-checkout build**, which catches dependency drift — the `charmbracelet/ssh` rename
   broke this exact thing during spike S8.
2. **Width assertion on every golden file**, which caught four errors while the mockups were
   being drawn.

---

## 8. Definition of done

MVP ships when **P1–P7, C1–C5, R1–R5, S1–S5 and U1–U5 all pass**, the 7-day soak completes, and
Phase 7's open items are either closed or explicitly accepted with a recorded rationale.

Not required for MVP, and tracked as fast-follows: the "gusty later" decision (D44), touch
interaction (D35), the second and third dashboards (O2), and the WeatherFlow rain diagnosis.
