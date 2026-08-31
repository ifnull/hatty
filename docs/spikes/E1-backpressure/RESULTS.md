# E1 — backpressure, verified against real hardware

`internal/server`'s unit tests inject a `send` function, so they prove the
sink's *logic*. They say nothing about whether wish's real write path behaves as
finding E1 assumes. This probe answers that, on the actual deployment target.

Run 2026-08-31. `cmd/e1probe` cross-compiled to `linux/arm64`, deployed to
`control-panel-mini` (Raspberry Pi 3B), driven at 20 Hz × 16 KB frames from a
workstation client over the LAN.

## The assumption under test

E1 rests entirely on:

> a **stalled** client makes the write **block** (no error), and
> a **dead** client makes the write **return an error**.

If a stall also produced an error, hatty would tear down sessions on every
Wi-Fi hiccup — precisely the failure E1 was written to prevent, arriving from
underneath.

## Result 1 — a stalled client blocks. Confirmed.

Client `SIGSTOP`ped at 14:49:50:

```
14:49:53 [s1] sent=111 dropped=67  maxPush=302.652µs writeBlockedFor=3.1s
14:49:54 [s1] sent=111 dropped=86  maxPush=302.652µs writeBlockedFor=4.1s
14:49:55 [s1] sent=111 dropped=107 maxPush=302.652µs writeBlockedFor=5.099s
14:49:56 [s1] sent=111 dropped=126 maxPush=302.652µs writeBlockedFor=6.099s
14:49:57 [s1] sent=111 dropped=147 maxPush=302.652µs writeBlockedFor=7.1s
```

The write blocked for **7+ seconds with no error**, `sent` froze, and `dropped`
climbed as the replacing slot discarded superseded frames. Exactly as E1
assumed.

## Result 2 — the store is genuinely isolated. Confirmed.

`maxPush` is the longest any `Push` call took, measured continuously.

**It did not move during the stall**: 302.652 µs before, 302.652 µs after seven
seconds of a completely wedged consumer. Across the whole run it peaked at
**407 µs**.

This is the property r1's design did not have — there, the store goroutine
would have been parked for the entire stall, and permanently on a client that
never returned.

## Result 3 — a stalled session recovers. This kills r2's reaper twice over.

On `SIGCONT`, the session resumed cleanly:

```
14:50:23 [s1] sent=139 dropped=640 writeBlockedFor=-
14:50:24 [s1] sent=159 dropped=640 writeBlockedFor=0s
14:50:25 [s1] sent=179 dropped=640 writeBlockedFor=0s
```

r2 would have reaped this session after **750 ms** — nine times over during the
stall — and the session it killed was one that **recovers by itself**. The
dashboard would have blanked and reconnected for a condition that resolved
without intervention.

## Result 4 — CORRECTION: death is detected by context cancellation, not a write error

Client `SIGKILL`ed:

```
14:50:25 [s1] CLOSED after write error="" | sent=186 dropped=640 maxPush=406.974µs
```

**The error string is empty.** `s.Write()` never returned an error. The session
ended because wish cancelled the SSH session context, which the sink's parent
context propagates.

So the code comment and r3 §2's wording — *"only confirmed channel death tears a
session down… a write error"* — describe a backstop, not the mechanism. The real
teardown path is **parent context cancellation**, and the write-error branch may
never fire in practice.

This does not change the design's behaviour, which is correct in both cases. It
does mean:

- `TestWriteErrorTearsTheSessionDown` exercises a path production may not use.
  Worth keeping — a backstop that is never tested is not a backstop — but it
  must not be mistaken for coverage of the real path.
- **The sink's correctness depends on wish cancelling the session context.** That
  is now the load-bearing assumption, and it was not previously identified as
  one. It held across 11 session lifecycles here.
- A hypothetical wish that failed to cancel would leave a session blocked
  forever with no detection, since the write demonstrably does not error.

Comments and documentation corrected accordingly.

## Result 5 — no resource growth across session churn

Eleven connect/kill cycles:

```
CLOSED sessions logged: 11
probe RSS:     10488 kB
probe threads: 7
```

10 MB resident and 7 OS threads after 11 lifecycles including one 7-second
stall. No accumulation — the goroutine hygiene the unit tests assert holds
against the real SSH stack.

## Verdict

E1's core assumption **holds on the target hardware**, and the design that
follows from it is correct: slowness is tolerated, death is detected, the store
is never blocked, and a stalled session recovers rather than being killed.

One documented mechanism was wrong. The corrected statement:

> A session is torn down when its **context is cancelled** — which wish does on
> connection death — with a write error as a backstop. Render latency, of any
> magnitude, is never a teardown trigger.
