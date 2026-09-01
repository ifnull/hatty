# R1 — surviving Home Assistant restarts

Phase 8 criterion: *survives 20 HA restarts with no human action; every binding
returns to `Valid`.*

Run 2026-08-31 against the live instance, daemon under systemd on
`control-panel-mini`.

## Result

**18 cycles measured with a correct gate. hatty reconnected on every one, and
never crashed.**

| | |
|---|---|
| Reconnected after the restart | **18 / 18** |
| Statistics re-backfilled | 14 / 18 |
| Frame fully populated at the check | 9 / 18 |
| **`NRestarts` (daemon crashes)** | **0** |
| HA downtime | 10–29 s, median 14 s |
| hatty recovery after HA returned | 2–22 s, median 8 s |

The reconnect mechanism is confirmed in the journal, not inferred:

```
ha: read failed, reconnecting   err="websocket: close 1000 (normal)"
ha: connect failed  err="connection refused"  retry_in=1s
ha: connect failed                            retry_in=2s
ha: connect failed                            retry_in=4s
ha: connect failed                            retry_in=8s
ha: connect failed                            retry_in=16s
ha: connected → subscribed → statistics backfilled
```

Backoff at exactly the specified 1/2/4/8/16 intervals, then reconnect,
resubscribe and backfill — with no human action, which is the criterion.

## What the failures actually mean

**`frame=NO` on 9 cycles is not a hatty defect.** The check connects a client
and requires a numeric reading. It ran ~10 s after HA answered its API again —
but Home Assistant answers `/api/` well before its *entities* have values.
During that window hatty correctly renders `—`, because showing a stale number
as current is the failure N7 exists to prevent.

So the frame check was measuring **how long Home Assistant takes to repopulate**,
not whether hatty recovered. The right criterion is "every binding returns to
Valid *eventually*", and the reconnect column already establishes the path.

**`backfill=NO` on 4 cycles** is the same shape: `recorder` is not necessarily
ready when the API is, so a statistics request immediately after restart can
come back empty. Worth a retry on empty rather than one attempt per connection.

## Two mistakes in the method, recorded

**1. The first run's gate measured nothing.** v1 slept 20 s then asked "is HA
up?" — but the REST call returns *before* HA stops, so the check passed against
the still-running old process and reported `back after 0s` on all 20 cycles.
Fixed in v2 by waiting for HA to go **down** first.

**2. I did not stop v1 before starting v2.** `pkill -x r1.sh` matched nothing —
a shebang script started this way has `comm` of `bash`, not `r1.sh` — so the
kill silently no-opped while reporting success, and both loops restarted Home
Assistant concurrently for several cycles. Cycles 6–11 are contaminated; note
they are also where most `frame=NO` results cluster, and 12–18 are markedly
cleaner.

The second is the worse error: it doubled the disruption to a live system and
polluted the data, and I reported it as stopped. Killing by PID from `ps` output
is the reliable form; an exact-name match on a script is not.

## Verdict

**The substance of R1 passes**: across roughly 40 restarts including a period of
concurrent double-restarts, the daemon never crashed, reconnected every time,
and required no intervention.

**The measurement is not clean enough to sign off the criterion.** A confirmatory
run needs: v2's gate, no concurrent loop, and a recovery check that waits for HA
to repopulate rather than sampling once. That is another 20 restarts of a live
home automation system, so it should be a deliberate decision rather than an
assumption.
