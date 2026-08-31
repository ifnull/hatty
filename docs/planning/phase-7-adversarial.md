# Phase 7 — Adversarial Review

The brief: *"Assume previous reviewers are wrong until demonstrated otherwise… The goal is not
to defend the architecture. The goal is to find reasons it will fail."*

**Conflict of interest, stated plainly.** The same model wrote Phase 6 and this review. That is
exactly the arrangement the brief warns against, and this document should be treated as a
first pass rather than an independent one. A genuinely separate reviewer should redo it. What
follows is written to break the spec, not to defend it.

Thirteen findings. Four would ship a broken product; three are wrong invariants that pass
validation and fail at runtime; two are security holes.

---

## Critical — these ship broken

### A1. The backpressure design leaks goroutines until the daemon dies

Phase 6 §2 specifies "per-session send is non-blocking with a depth-1 mailbox that replaces
rather than queues." That cannot be implemented with `tea.Program.Send()` alone, because
**`Send` blocks when the program's event loop is blocked**, and the event loop blocks whenever
the renderer's write to the SSH channel blocks.

The failure chain is concrete:

1. The Pi's Wi-Fi drops mid-frame, or the client is suspended. TCP window fills.
2. `wish`'s channel write blocks. Bubble Tea's renderer is inside `View()` → the event loop
   stops consuming.
3. The store's per-session sender goroutine calls `p.Send()` and blocks forever.
4. SSH keepalive eventually reaps the session — but the sender goroutine is still parked on a
   send to a program that will never read.
5. The Pi reconnects. New session, new goroutine. Repeat.

Over a week of Wi-Fi flaps on a 2.4 GHz-only Pi 3B (`environment.md`), this is an unbounded
goroutine and memory leak in a daemon meant to run for months (N6). It also defeats the entire
point of R-1: the store is *not* isolated from a slow client.

**Required:** the sender goroutine must own a `context.Context` cancelled on session teardown,
and `Send` must be raced against it. Additionally a session reaper must terminate any
`tea.Program` that has not consumed a frame within a timeout — a client that cannot keep up
with 4 Hz is not a client any more.

### A2. Re-sorting the aircraft table forces a full-frame repaint, continuously

The table sorts by `distance_nm` ascending (Phase 6 §5). Aircraft distances change every update.
**Any change in sort order changes every row below the swap**, so Bubble Tea's line-diffing
renderer — which compares line by line — finds most lines different and rewrites them.

Phase 1 §9 estimated bandwidth on the assumption that a few cells change per frame. The real
steady state for the primary dashboard is closer to a full 32-line repaint whenever two aircraft
cross in range, which with 9–15 contacts converging and diverging is *most frames*.

At 100 columns × 32 rows with SGR escapes, a full repaint is roughly 6 KB. At 4 Hz that is
~24 KB/s sustained — about ten times the Phase 1 estimate, and precisely the "excessive redraw
traffic over SSH" the brief lists.

**Required:** sort hysteresis. Sort on a bucketed key (e.g. distance rounded to 0.5 nm) so that
noise does not reorder rows, or hold the previous order and only re-sort when a contact's rank
changes by more than one position. This is a product decision as much as a technical one: a
table whose rows swap continuously is also unreadable.

### A3. Statistics backfill and live updates race into the same ring buffer

Phase 6 §4 requires `recorder/statistics_during_period` backfill on connect, and §3 gives each
bound series a fixed `Ring`. **The interleaving is unspecified.**

The backfill is an async request returning ~288 points covering the last 24 h. While it is in
flight, live updates are appending to the same ring. When the response lands, a naive fill
either overwrites live samples that are newer, or appends historical points *after* newer ones,
producing a chart that goes backwards in time.

Worse on reconnect, where both happen at once and a second disconnect can arrive mid-fetch.

**Required:** the ring must be timestamp-bucketed rather than append-ordered, and merge must be
idempotent — writing a bucket that already holds a newer sample is a no-op. Then backfill and
live updates commute and the order stops mattering.

### A4. Staleness marks correct data as broken

Phase 6 §3 says `stale_after` is "derived from observed update cadence, not guessed." For
`event.st_00128663_lightning_strike`, the observed cadence is *four weeks* — because there has
been no lightning. There is no cadence to derive.

The same applies to `sensor.airspace_flag_emergency` (usually zero, may not change for months)
and any binary sensor that is correctly quiet.

Under the spec as written, the lightning panel renders `Stale` permanently, and the alert
entities — the ones that matter most — are the ones most likely to be wrongly marked. A
dashboard that cries stale at correct data trains the user to ignore the staleness indicator,
which destroys the one mechanism protecting against N7.

**Required:** `stale_after` is explicit per binding and **may be absent**, meaning "this entity
is not expected to update on a schedule." Staleness then applies only where a cadence genuinely
exists — the weather and airspace sensors — and never to event or alert entities.

---

## Wrong invariants — pass validation, fail at runtime

### B1. The elastic panel can be dropped, and then there is no elastic panel

Phase 6 §5 rule 4 requires *exactly one* panel with `elastic = true`, validated at load. Phase 6
§6 step 5 drops panels — including the detail pane — when the layout does not fit. D36 collapses
the detail pane below 24 rows.

So at 64 × 20 the `radar` screen validates fine and then has **zero** elastic panels, and step 4
distributes leftover rows to nothing. The trapped-space bug that D37 exists to fix returns at
exactly the sizes where space is tightest.

**Required:** elasticity is a ranked list, not a flag. The solver takes the highest-ranked
*surviving* panel. Validation checks that at least one elastic-capable panel survives at
`min_cols × min_rows`, not merely that one exists at the authoring size.

### B2. Config validation rule 3 is vacuous

Rule 3: *"Columns' `min_cols` are monotonic with the drop order — a column cannot outlive a
wider one."*

There is no separately declared drop order. `min_cols` **is** the drop order. The rule compares
a thing to itself and can never fail. It is not a weak check; it is not a check.

The invariant actually needed is different: **at every width where the active column set
changes, the sum of surviving column widths plus separators must fit that width.** That is
checkable — evaluate at each distinct `min_cols` value — and it is the property that actually
prevents a corrupted table.

**Required:** replace rule 3 with the width-budget check, evaluated at every breakpoint implied
by the column set.

### B3. `FrameMsg.Changed` promises partial rendering the framework does not do

Phase 6 §2 defines `Changed EntitySet` as "which bindings need re-resolving." Requirement F5
says "redraw only changed regions where the framework permits."

Bubble Tea's `View()` returns a **complete string** every frame. There is no partial render. The
line-level diffing happens *inside the renderer*, below the application, and the app cannot
influence it. `Changed` can therefore save view-model resolution and nothing else — a real but
small saving, and one that invites implementers to believe they are doing region updates.

**Required:** rename it to make the scope obvious, and state explicitly in §7 that F5 is
satisfied by Bubble Tea's renderer rather than by hatty. Otherwise someone will "optimise" by
returning a partial view and produce a corrupted frame.

---

## Security

### C1. Wish accepts any public key unless told otherwise

Phase 6 §12 and Phase 1 §8 both say "public-key authentication is enforced by the app." Wish's
default `PublicKeyHandler` is permissive — with no handler configured, **any key is accepted**.

The daemon is a network service holding a Home Assistant token, exposing every bound entity —
including home-relative aircraft bearings and distances, which are location-inferring data.
Default-open on a LAN is not acceptable for that, and the spec's phrasing implies protection
that does not exist by default.

**Required:** an explicit `PublicKeyHandler` checking an `authorized_keys` file, refusing to
start if that file is missing or empty. Fail closed.

### C2. The token can leak through panics and logging

The token is correctly kept out of the repository (D15, `.gitignore`) and off the command line.
It is not protected from:

- a panic dump that formats the client struct with `%+v`;
- a debug log of the connection config;
- the diagnostics overlay (Phase 6 §11) rendering "connection state" to a terminal that may be
  screen-shared or photographed — as this very project has demonstrated repeatedly.

**Required:** a `Secret` string type whose `String()`, `GoString()` and `MarshalJSON()` all
return `"[redacted]"`. Cheap, total, and it makes the leak impossible rather than merely
discouraged.

---

## Assumptions that may not hold

### D1. ~~The 1/s update bound rests on a README, not a measurement~~ — RESOLVED 2026-08-31

D21 downgraded R1 from critical to low on the basis that `ha-airspace` "throttles publishes to a
default 1/s." That came from reading the project's README.

The throttle is described as per-hex *and* summary. Whether the *aggregate* sensor HA sees
updates at 1 Hz, or at 1 Hz × number of aircraft as MQTT messages fan in, is not established.
If it is the latter, R1 returns and A2 becomes considerably worse.

**Resolved by running S1.** The README was accurate: no entity exceeds ~0.92/s. But the throttle
is *per entity*, and the bound set yields **~5.6 msg/s and 7.2 KB/s** in aggregate — dominated by
two flag sensors at ~4 KB each, re-sent ~0.9 times a second. R15 is confirmed exactly.

Two caveats that matter more than the resolution:

- That 7.2 KB/s is **HA → daemon**, over a same-host virtual bridge (D13). It is fine.
  **A2's concern is daemon → Pi over SSH, which S1 does not measure and which remains open.**
- S1 destroyed three design assumptions in passing — 94 aircraft tracked rather than 9, flag
  lists capped at 10, and no full-aircraft-list entity at all. See D41 and
  `S1-event-payload/RESULTS.md`.

### D2. ~~The bubbletea v1 / v2 split is an unexamined dependency trap~~ — RESOLVED 2026-08-31

The build already broke once on the `charmbracelet/ssh` → `charm.land/ssh` rename (R10, spike
S8). There is a second version fault line: **ntcharts publishes separate v1 and v2 branches**,
the v2 branch targeting Bubble Tea v2, while `wish` v1.4.7 pins the pre-rename `ssh` module.

Three libraries, two major-version lanes, and one already-observed rename. If `wish` stays on
Bubble Tea v1 while ntcharts development moves to v2, the chart library and the SSH server
cannot both be current.

**Resolved by doing it.** A throwaway module importing all four builds, links and runs:

```
github.com/charmbracelet/bubbletea  v1.3.10
github.com/charmbracelet/lipgloss   v1.1.0
github.com/charmbracelet/wish       v1.4.7
github.com/NimbleMarkets/ntcharts   v0.5.1        (main branch, Bubble Tea v1)
github.com/charmbracelet/ssh        v0.0.0-20250128164007-98fd5ae11894  (pinned pre-rename)
```

`timeserieslinechart` and `sparkline` — the two ntcharts components the design actually needs
(D32) — instantiate alongside a `wish` server in one binary. Cross-compiles clean to
`linux/arm64`: a **4.8 MB static binary**, which is the deployment story D29 promised.

The `charmbracelet/ssh` pin remains mandatory; without it `go mod tidy` resolves v0.4.3, which
declares itself as `charm.land/ssh`, and the build fails on a module-path mismatch. That pin
belongs in the real `go.mod` with a comment, not rediscovered later.

The Bubble Tea v1/v2 fault line is real but not yet a problem: ntcharts' **v1 branch is
current at v0.5.1** and works with the wish that exists. Revisit only if wish moves to v2.

### D3. Snapshot immutability is required but never stated

Phase 6 §2 rule R-2 hands sessions a `*Snapshot`. Whether that is a deep copy, a shallow map of
pointers, or a persistent structure is unspecified.

If it is a deep copy, 4 Hz × attribute-heavy entities (the aircraft list is a slice of maps) is
real GC churn on a machine also running Home Assistant. If it is a shallow map of pointers, it
is cheap and correct — **but only if `Value` is never mutated after publication.**

That invariant is the load-bearing one and the spec does not state it.

**Required:** state it. `Value` is immutable once published; updates replace rather than mutate;
snapshots are shallow maps of pointers. Then R-2 is cheap and safe, and a future contributor who
mutates in place is violating a written rule rather than an unwritten assumption.

### D4. The alert strip smuggles in hard-coded behaviour

Phase 6 §5:

```toml
nominal = "{count} tracked · {flagged} flagged"
```

`{count}` and `{flagged}` are bound to nothing. They are implicit references resolved by
widget-specific code — exactly the "hard-coded entity behavior" the brief lists, arriving inside
the config system that was supposed to prevent it.

It also breaks D25's line: this is the config layer deriving values rather than mapping them.

**Required:** template variables are declared bindings like everything else.

---

## What I could not break

Recorded so the review is not just a list of complaints, and so a later reviewer knows where I
already looked.

- **Width safety (§7).** The `Row` builder with a test-mode panic is sound, and pinning
  `go-runewidth`'s ambiguous-width behaviour (D39) addresses the real hazard. I could not
  construct a composition failure that the per-cell check misses.
- **Value conditions (§3).** Separating `Fault` from `Unknown` is correct and I found no case
  where five conditions collapse to fewer.
- **Attribute-path grammar (§3).** Deliberately expression-free (D25); I could not find a real
  dashboard requirement it cannot express, given HA template sensors.
- **Service-call safety.** MVP is read-only (D9) and there is no service-call code path, so
  there is nothing to trigger accidentally. This is the right shape.
- **tmux edge cases.** Option C (D7) removes tmux entirely; the D8 conditional is honest about
  reverting if option A is chosen.
- **Refusal floor (§6).** Refusing below 44 × 12 rather than drawing corruption is right, and
  the measured ladder (D38) confirms nothing legitimate lands below it.

---

## Verdict

The architecture survives. The **shape** — single-writer store, immutable snapshots, coalesced
frames, pure widgets, declarative config, HA-side derivation — holds up under attack, and none
of the thirteen findings requires rethinking it.

But four findings (A1–A4) would produce a product that leaks memory, saturates the link,
corrupts its own charts, and cries wolf about staleness. Three more (B1–B3) are invariants that
pass validation and fail in production. Those seven are not polish; they are the difference
between a spec and a working system.

**Recommended: revise Phase 6 against A1–A4, B1–B3, C1–C2 before any implementation, and run
spike S1 first** — D1 shows it is now load-bearing for two separate findings.

And the standing caveat: this review shares an author with the thing reviewed. The findings are
real, but the blind spots are shared too.
