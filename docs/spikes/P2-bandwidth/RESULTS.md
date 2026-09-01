# P2 — SSH bandwidth, measured

Measured 2026-08-31 with the real daemon running on `control-panel-mini` against
live Home Assistant, `radar` at 100×32, client attached over the LAN for 45 s.

```
bytes over SSH: 26,171 in 45s
sustained:      582 B/s   (4.7 kbps)
frames sent:    45        dropped: 0
daemon RSS:     13,984 kB
```

| Criterion | Budget | Measured | |
|---|---|---|---|
| **P2** sustained SSH bytes | ≤ 8,000 B/s | **582 B/s** | **PASS**, 14× margin |
| **P6** daemon RSS | ≤ 64 MB | **14 MB** | on track (7-day soak still required) |

## This closes A2 and E7

**Finding A2** predicted that sorting the table by a continuously-changing value
would cause near-continuous full-frame repaints — *"roughly 6 KB per repaint at
4 Hz, ~24 KB/s sustained, about ten times the Phase 1 estimate."*

The real figure is **582 B/s: A2 overstated the cost by roughly 40×.**

Two reasons. Bubble Tea's line-diffing renderer works as documented — only
changed lines are rewritten. And the store publishes on change, not on the tick:
45 frames in 45 seconds, matching `ha-airspace`'s ~1/s publish rate rather than
the 4 Hz tick ceiling.

**Finding E7** is therefore settled. r3 made sort hysteresis opt-in and
`hysteresis = 0` the default, on the reasoning that *"paying a known correctness
cost for an unmeasured benefit is the wrong trade"*. The measurement vindicates
that: there is no bandwidth problem to trade sort correctness for. **Hysteresis
stays off**, and the table sorts exactly.

That is the clearest vindication in the project of deferring a mitigation until
its motivating number exists. r2 would have shipped the correctness cost.

## Caveat

The screen currently has ~22 blank rows (finding F1), so this is close to a best
case. A full table of flagged contacts would move more bytes — but with 14×
margin and line-diffing doing the work, not by enough to threaten P2.

Worth re-measuring once F1 is resolved and the table is populated.
