# Spikes

Measurement-only harnesses. These answer questions the Phase 1 plan could otherwise only
guess at. **None of this is application code** — decision D12 keeps a source tree out of the
repository until Phase 4 selects a framework, and spike code is exempt because it is
throwaway by design.

Repository convention: **spikes record raw results, not just conclusions.** A spike whose
numbers are gone cannot be re-examined when a later phase disputes what was concluded from
them. Each spike writes a `RESULTS.md` next to its harness, and machine-readable output
where it has any.

| Spike | Question | Runs on | Status |
|---|---|---|---|
| [S1](S1-event-payload/) | What do HA state updates actually cost in bytes/sec, and how much does `subscribe_entities` save? | server (or any machine that can reach HA) | ready to run |
| [S4](S4-textual-over-ssh/) | Can Textual be served over a programmatically owned SSH connection? | server / dev machine | ready to run |
| [S8](S8-wish-bubbletea/) | Does Wish serve a Bubble Tea app over SSH with correct resize? | server; **Pi as the client** | ✅ partially answered — see below |
| [S2](S2-glyph-coverage/) | Glyph coverage **and width** at 8×16 on the panel | **Pi** | ready to run — `bash check.sh` |
| S3 | *(merged into S1)* | — | — |
| ~~S9~~ | ~~Browser cost on the Pi~~ | — | withdrawn — see decision D24 |

## Which of these need the Pi

Worth being explicit, because it is easy to get backwards: **S1 and S4 do not involve the Pi
at all.** S8's server half does not either. The Pi matters for exactly two things:

- **S2** — glyph coverage, which is purely a property of the Pi's terminal and font.
- **S8's interactive half** — connecting as a real client to verify the app sees 100×30,
  that `SIGWINCH` propagates, and above all that **reconnecting at a different grid works**
  (decision D5 puts font-per-dashboard in the `hatty-connect` launcher, so a dashboard
  switch changes the console font and the app gets a different grid on reattach). That is
  risk R7 and it cannot be tested with a synthetic terminal.

## Already learned, 2026-08-27

Running S8 far enough to build and serve produced two findings before anyone touched the Pi:

1. **Wish serves a Bubble Tea program over SSH.** The harness builds clean against
   `wish v1.4.7` / `bubbletea v1.3.10`, listens, accepts a connection with no login shell,
   renders the app, and propagates `TERM`. The core premise of §8 option C holds.

2. **A live dependency rename is mid-flight, and it bites.** `github.com/charmbracelet/ssh`
   has renamed its module path to `charm.land/ssh` as of `v0.4.3`, but `wish v1.4.7` still
   depends on the *old* path. So `go mod tidy` resolves `charmbracelet/ssh` to `v0.4.3`,
   which declares itself as `charm.land/ssh`, and the build fails with a module-path
   mismatch. The fix is to pin the pre-rename pseudo-version that wish itself uses:

   ```
   go mod edit -require=github.com/charmbracelet/ssh@v0.0.0-20250128164007-98fd5ae11894
   ```

   This is concrete evidence for **risk R10 (framework API churn)**, and notably it landed
   on the Go side — the plan had flagged churn as a Textual concern. Phase 4 should weigh it
   against both candidates, not one.

**Still unanswered for S8:** real terminal dimensions, live resize, reconnect-at-a-different-
grid, and multiple simultaneous clients at different sizes. All need an interactive terminal.
