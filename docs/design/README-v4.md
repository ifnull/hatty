# Phase 5, round four — colour and alerts

Colour cannot be designed in markdown. [`render_v4.py`](render_v4.py) prints **real ANSI
256-colour output** — run it in the terminal you will actually use:

```bash
python3 docs/design/render_v4.py            # all three states
python3 docs/design/render_v4.py alert      # just the alert state
```

Three states are rendered, because a dashboard's colour design is only testable across the
states it must distinguish. Plain-text versions below show structure; the colour is the point,
so run the script.

---

## The rules, in order of importance

### 1. Hues are reserved

**Red, amber and green mean STATE and nothing else.** Altitude is a continuous category, not a
severity, so it gets a **sequential blue→cyan ramp**.

This deliberately contradicts the existing Lovelace dashboard, which colours low altitude red
and high altitude green. On a card you read, that is a harmless category scale. On a screen you
*glance* at, red means "something is wrong" — and a Cessna at 4,600 ft is not wrong. Spending
the alarm hue on routine altitude banding means the actual alarm has nothing left to shout with.

### 2. Colour never carries meaning alone

Every coloured state also carries a glyph or a word: `●` nominal, `▲` warning, `⚠` alert,
`stale 47s` spelled out. Required three times over — for 8-colour terminals, for colour-blind
readability, and because the client's colour depth is not guaranteed (the Pi reports 256, a
different SSH client might not).

### 3. Muted, not saturated

This runs 24/7 at arm's length on a desk. Full-intensity red (`\033[91m`) glares and creates
afterimages; desaturated red at 256-colour index 167 still reads as alarming. Every hue in the
palette is a muted variant for the same reason.

### 4. Alerts reserve space, they do not reflow

Decision D26 gives panels a collapse-or-reserve choice. **The alert strip reserves.** It is
always present on line 2 — quiet when nominal, loud when not. A dashboard whose geometry jumps
when something goes wrong is exactly the "visually unstable" failure the brief warns about, and
it trains you to distrust the layout at the moment you most need to read it.

---

## The palette

| Role | 256-index | Use |
|---|---|---|
| `chrome` | 238 | borders, rules |
| `label` | 245 | column headers, field names, units |
| `value` | 252 | primary readings |
| `title` | 255 + bold | screen title |
| `muted` | 240 | de-emphasised, not-applicable |
| **`ok`** | **71** | nominal — receivers up, distance comfortable |
| **`warn`** | **179** | attention wanted, not urgent |
| **`alert`** | **167** | urgent |
| `alertbg` | bg 52 / fg 224 | the alert banner |
| `stale` | 96 | *our* judgement that an update is overdue |
| `unavail` | 240 | HA says the entity is unreachable |
| `sel` | bg 236 | selection cursor row |
| `alt0…alt3` | 60, 67, 74, 81 | altitude ramp — sequential, **not** a traffic light |

`stale` and `unavail` are distinct on purpose. Plan §7 requires three value conditions that are
never collapsed: `unavailable` (HA says so), `unknown` (HA has no value), and **stale** (the app
judges the update overdue). Stale is the app's own invention and the most important — a frozen
dashboard presented as live is the failure N7 exists to prevent.

---

## Nominal

```text
┌─ hatty · radar        AIRSPACE · 9 tracked ──────────────────────────────────────────────────────┐
│ ●  all nominal   9 tracked · 0 drones · 0 flagged · last alert 6 d ago                           │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│   FLIGHT     TYPE     DIST    RANGE            ALT    SPD   BRG    CPA  OP  FLAGS                │
│ ▸ UAL1234    B738      9.1  █▌░░░░░░░   36,000 →   446   284    8.2  US                          │
│   RCH451     C17      12.6  █▌░░░░░░░   24,000 ↓   402   195   11.8  US  MIL                     │
│   N77148     C172     18.3  ██▌░░░░░░    4,600 ↑   178   270   16.5  US                          │
│   SWA2584    B737     24.7  ███▌░░░░░   34,000 →   455   312   22.2  US                          │
│   AAL691     A321     31.2  ████▌░░░░   20,875 ↓   432   101   28.1  US                          │
│   VTM300     C560     36.8  █████░░░░   29,150 →   408    38   33.1  US  LADD                    │
│   DAL1508    A320     41.4  █████▌░░░   27,825 ↓   450   263   37.3  US   stale 47s              │
│   JBU607     A320     46.6  ██████▌░░   36,025 →   434   280   41.9  US                          │
│   N556GA     GLF6     63.1  ████████▌   41,000 ↓   488   340   60.4  US  PIA                     │
├─ UAL1234 ────────────────────────────────────────────────────────────────────────────────────────┤
│ Type          B738                              Operator      United Airlines                    │
│ Squawk        1200                              Vertical      level                              │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│ 1090 ●  978 ●  RID ●   412/s · 37/s    ↑↓ select  ⏎ pin  f filter  / search  14:23:07            │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

Quiet. The only colour doing work is the green nominal dot, the altitude ramp, and the near-range
bars. `DAL1508` is stale and recedes rather than shouting — degraded data should look degraded,
not alarming.

## Warning

```text
┌─ hatty · radar        AIRSPACE · 9 tracked ──────────────────────────────────────────────────────┐
│ ▲  2 warnings   tempest battery 2.63 V · 978 receiver degraded (3/s)                             │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│   FLIGHT     TYPE     DIST    RANGE            ALT    SPD   BRG    CPA  OP  FLAGS                │
│ ▸ UAL1234    B738      9.1  █▌░░░░░░░   36,000 →   446   284    8.2  US                          │
│   RCH451     C17      12.6  █▌░░░░░░░   24,000 ↓   402   195   11.8  US  MIL                     │
│   N77148     C172     18.3  ██▌░░░░░░    4,600 ↑   178   270   16.5  US                          │
│   SWA2584    B737     24.7  ███▌░░░░░   34,000 →   455   312   22.2  US                          │
│   AAL691     A321     31.2  ████▌░░░░   20,875 ↓   432   101   28.1  US                          │
│   VTM300     C560     36.8  █████░░░░   29,150 →   408    38   33.1  US  LADD                    │
│   DAL1508    A320     41.4  █████▌░░░   27,825 ↓   450   263   37.3  US   stale 47s              │
│   JBU607     A320     46.6  ██████▌░░   36,025 →   434   280   41.9  US                          │
│   N556GA     GLF6     63.1  ████████▌   41,000 ↓   488   340   60.4  US  PIA                     │
├─ UAL1234 ────────────────────────────────────────────────────────────────────────────────────────┤
│ Type          B738                              Operator      United Airlines                    │
│ Squawk        1200                              Vertical      level                              │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│ 1090 ● 412/s  978 ▲ 3/s  RID ● ▲ battery 2.63 V    ↑↓ select  ⏎ pin  f filter  / search  14:23:07│
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

Amber, two places, both spelled out. The 978 receiver drops to `▲ 3/s` in the status bar and the
strip summarises. Nothing moves.

## Alert

```text
┌─ hatty · radar        AIRSPACE · 9 tracked ──────────────────────────────────────────────────────┐
│  ⚠  MILITARY CONTACT   RCH451 · C17 · 12.6 nm · brg 195° · descending · CPA 11.8 nm              │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│   FLIGHT     TYPE     DIST    RANGE            ALT    SPD   BRG    CPA  OP  FLAGS                │
│   UAL1234    B738      9.1  █▌░░░░░░░   36,000 →   446   284    8.2  US                          │
│ ▸ RCH451     C17      12.6  █▌░░░░░░░   24,000 ↓   402   195   11.8  US  MIL                     │
│   N77148     C172     18.3  ██▌░░░░░░    4,600 ↑   178   270   16.5  US                          │
│   SWA2584    B737     24.7  ███▌░░░░░   34,000 →   455   312   22.2  US                          │
│   AAL691     A321     31.2  ████▌░░░░   20,875 ↓   432   101   28.1  US                          │
│   VTM300     C560     36.8  █████░░░░   29,150 →   408    38   33.1  US  LADD                    │
│   DAL1508    A320     41.4  █████▌░░░   27,825 ↓   450   263   37.3  US   stale 47s              │
│   JBU607     A320     46.6  ██████▌░░   36,025 →   434   280   41.9  US                          │
│   N556GA     GLF6     63.1  ████████▌   41,000 ↓   488   340   60.4  US  PIA                     │
├─ RCH451 ─────────────────────────────────────────────────────────────────────────────────────────┤
│ Type          C17 Globemaster III               Operator      US Air Force                       │
│ Squawk        4231                              Vertical      -1,200 ft/min                      │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│ 1090 ● 412/s   978 ● 37/s   RID ● 0  ⚠ 1 alert    ↑↓ select  ⏎ pin  f filter  / search  14:23:07 │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

The strip that said "all nominal" becomes a filled banner. `RCH451` is bold-red in the table,
selected, and its detail record is open below. **Line count is identical across all three
states** — that is the point of reserving.

---

## Known issue, unfixed

The sort indicator was removed from the header rather than fixed. With colour available the right
answer is to mark the sorted column by *hue*, not by a glyph that competes with the `↑ ↓ →`
vertical-speed arrows. That is a small change once colour exists, but it is not drawn here.

---

## What else ntcharts brings

Beyond the line charts already assumed, reviewing its component list turned up four things worth
having and one that changes a roadmap item.

**Streaming line chart** — data traverses right-to-left continuously. That is exactly the
"distance to nearest, 60 min" runchart, live, without hand-rolling a ring buffer and rescale.

**Time series with temporal axis, automatic scaling, and multi-series overlay** — a direct fit for
the 24 h wind chart, which needs `min`/`mean`/`max` from `recorder/statistics_during_period`
(decision D16) drawn as three series on one axis. This is the single most annoying chart to build
by hand and it is off the shelf.

**Bar charts, horizontal and vertical** — replaces the hand-rolled threshold bars for CO₂, VOC and
PM2.5 on the `home` screen.

**Heatmap** — no current requirement, but an hour×day grid of traffic density or lightning
activity is the kind of thing that is only worth building when it is free. Noted, not planned.

**BubbleZone mouse support — and this is the one that matters.** The DSI panel is a *touchscreen*.
Under X, touch is translated to pointer events; a terminal with mouse reporting enabled receives
them; and over SSH mouse events are simply in-band bytes, so they propagate to the daemon like
keystrokes. That means **tap-to-select-a-row is close to free** — the master/detail design in round
three becomes touch-driven with no new architecture.

Caveats before treating it as done: the app must enable mouse reporting (`tea.WithMouseCellMotion`),
the terminal must support it, touch→pointer translation depends on the X input stack, and grabbing
mouse events costs the user native text selection. So: **likely cheap, not proven.** Worth a short
spike rather than an assumption — but it turns "touch is a nice-to-have someday" into a concrete
path, which it was not before.

---

## Still open

- **The `home` screen**, where round two's runchart language belongs.
- **Responsive behaviour** below 100×30. The table has an obvious column drop order —
  flags, CPA, operator, speed — which makes this tractable.
- **Spike S2**, still unrun. It no longer gates the table (decision D33), only the trend chart.
