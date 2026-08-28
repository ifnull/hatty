# S4 — Textual over SSH

**Question.** Can a Textual app be served over a programmatically owned SSH connection,
in-process, with no login shell?

**Why it matters.** This is the framework decision. Go + Bubble Tea has a documented path to
§8 option C via Wish (spike S8, already shown to serve). Textual has no documented
equivalent. If Textual can be served this way, its layout and testing advantages are strong
enough to reopen the Phase 1 leaning; if it cannot, Textual is limited to options A and B
and Phase 4 becomes straightforward.

**A negative result is a useful result.** Timebox this — four hours is suggested. Do not
sink days into making it work; record what failed and move on. That *is* the finding.

## Run it

```bash
pip install asyncssh textual
cd docs/spikes/S4-textual-over-ssh
ssh-keygen -t ed25519 -f ./spike_host_key -N ''
./serve_textual.py               # listens on :2223
ssh -p 2223 localhost
```

The probe app deliberately mirrors the S8 Bubble Tea harness — same grid readout, same
resize history, same column ruler — so the two frameworks can be compared like for like.

> The spike server has **no authentication** (`begin_auth` returns `False`). Never expose
> this port beyond localhost or a trusted LAN.

## Two tests

**Test A — single client (implemented).** Binds one Textual app to a pty whose master end is
pumped to the SSH channel. It uses `dup2` on fds 0 and 1, which are process-global, so it
serves one client at a time by construction. If Test A fails, Textual over SSH is dead and
Test B is moot.

**Test B — multiple clients (not implemented; this is the work).** The real option C: N
renderers in one process, each bound to its own channel, sharing one HA state cache. The
extension point is Textual's `Driver` class — subclass it to write into a specific asyncssh
channel instead of the process's stdout. A sketch is at the bottom of `serve_textual.py`.

Verify the `Driver` signature against your installed Textual before trusting that sketch; it
is written from working knowledge rather than documentation, and Textual's API moves quickly
(risk R10).

## Go / no-go

| Outcome | Conclusion |
|---|---|
| Test A renders, resizes, exits cleanly | Textual viable for §8 options A and B |
| Test B serves 2+ clients at different sizes | **Textual viable for option C** — reopen the framework decision |
| Test B not working inside the timebox | Textual limited to A/B; Phase 4 should prefer Go + Wish |

## Record

Write `RESULTS.md` with which tests passed, the Textual and asyncssh versions, and — if Test
B failed — precisely where it failed. "Could not make it work" is not a finding; "the Driver
subclass could not X because Y" is.
