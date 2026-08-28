# S8 — Wish + Bubble Tea over SSH

**Question.** Does Wish serve a Bubble Tea app over SSH, with the app seeing the client's
real terminal size, live resizes, and — critically — a *different* grid on reconnect?

**Why it matters.** This gates §8 option C, the recommended session architecture (decision
D7): a daemon owning HA state with an SSH front-end, so a disconnect drops a renderer rather
than the state. It also probes risk R7 (resize / detach / reattach).

## Status

**Partially answered 2026-08-27.** Builds, serves, renders. The interactive matrix below is
outstanding and needs the Pi.

## Run it

```bash
cd docs/spikes/S8-wish-bubbletea
go mod tidy
go run .                 # listens on :2222, generates a host key on first run
```

Then from the Pi:

```bash
ssh -p 2222 <server>
```

No account or login shell is involved — the app *is* the SSH server.

## The dependency trap, already hit

`go mod tidy` on a clean checkout will fail with:

```
module declares its path as: charm.land/ssh
        but was required as: github.com/charmbracelet/ssh
```

`charmbracelet/ssh` renamed its module path at `v0.4.3`; `wish v1.4.7` still depends on the
old path. `go.mod` here already pins the pre-rename pseudo-version wish uses. If you
regenerate `go.mod`, re-apply:

```bash
go mod edit -require=github.com/charmbracelet/ssh@v0.0.0-20250128164007-98fd5ae11894
```

Record in `RESULTS.md` if this changes — it is live evidence for risk R10.

## What to check, in order

The app displays its grid, a resize history, and a **column ruler**. Do not trust the
reported numbers alone: if the ruler does not end flush with the right edge, the app's idea
of the width is wrong, which is the class of bug risk R3 is about.

| # | Check | Pass |
|---|---|---|
| 1 | Connect from the Pi | App renders; no login shell |
| 2 | Reported grid | Matches the panel's real `stty size`; ruler ends flush |
| 3 | 100×30 target | Banner shows the target is met (decision D1) |
| 4 | Live resize | Resize the window; history gains an entry; ruler re-flows flush |
| 5 | **Reconnect at a different grid** | Disconnect, `setfont` to a different size, reconnect. App renders correctly at the *new* grid — not the old one. **This is the R7 test and the one that matters.** |
| 6 | Two clients, different sizes | Connect from the Pi *and* a desktop simultaneously. Each renders at its own size. This is option C's claim that multiple clients "fall out naturally" |
| 7 | Ticks advance | Proves a live render loop rather than a one-shot paint |
| 8 | Clean exit | `q` exits; terminal is restored, not left in alt-screen |

Check 5 is the one to run twice. Check 6 is what distinguishes option C from options A and B.

## Record

Write `RESULTS.md` here with the outcome of each check, the exact `stty size` values used,
and any API drift encountered. Raw results, not just conclusions.
