# Phase 5, round five — the `home` screen and responsive layout

[`render_v5.py`](render_v5.py) parameterises every screen by `(width, height)` rather than
drawing at one size. That is the only honest way to design responsive layout: the rules have to
*produce* the small versions, not be reverse-engineered from them afterwards. This file is
effectively a prose specification of the layout engine in plan §5.

```bash
python3 docs/design/render_v5.py              # every screen at every breakpoint, in colour
python3 docs/design/render_v5.py radar 64 20  # one screen at one size
```

Every render is asserted to be exactly `width` columns and exactly `height` rows.

---

## The `home` screen

Round two's language — hero number, full-width multi-series runchart — applied to the sparse,
trend-heavy environment data.

```text
┌─ hatty · home        OUTSIDE 90.1 °F · INSIDE 68.4 °F ───────────────────────────────────────────┐
│ ▲  2 warnings   battery 2.63 V · 978 receiver degraded                                           │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│   ███ ███       █                                                                                │
│   █ █ █ █       █    feels like                                                                  │
│   ███ █ █       █    46.68 % humidity                                                            │
│     █ █ █       █    5.28 mph  ESE                                                               │
│   ███ ███   █   █    0.00 in rain today                                                          │
├─ WIND · 24 H · min / mean / max ─────────────────────────────────────────────────────────────────┤
│               ⢀⠤⠔⠒⠤⢄⣀⡀          ⣀⠤⠔⠒⠉⠉⠒⠤⣀               ⣀⡠⠤⢄⣀               ⢀⣀⠤⠤⠤⣄⣀⣀      ⢀⣀⣀⡀   │
│      ⣀⠔⠊⠉⠑⠒⠤⠤⠊⠁      ⠈⠑⠤⣀⣀⣀  ⢀⡠⠊         ⠉⠉⠒⠢⠤⢄⡀   ⢀⡠⠔⠊⠉⠁    ⠉⠉⠒⠦⢄⣀⡀    ⢀⠤⠒⠊⠉       ⠉⠓⠢⠤⠒⠋⠁  ⠉⠉⠉ │
│  ⢀⡤⠒⠉         ⢀⣀⠤⠤⢤⣀⣀⡀     ⠉⠉⠁  ⣀⠤⠤⠤⠤⠤⢄⣀⡀      ⠈⠒⠤⠒⠉   ⣀⣀⠤⠤⢄⣀      ⠈⠉⠑⠒⠊⠁   ⢀⣀⣀⣀⣀                │
│ ⠊⠁   ⣀⠔⠊⠉⠉⠓⠒⠒⠉⠁      ⠈⠙⠒⠤⠤⠤⣄⣀⡠⠔⠋        ⠈⠉⠉⠙⠒⠒⠢⢤⣀ ⢀⡠⠒⠉⠉      ⠉⠉⠑⠒⠤⠤⠤⢄⣀⣀⡠⠴⠒⠉⠉⠉    ⠉⠉⠉⠉⠓⠒⠒⠒⠋⠉⠉⠉⠉⠉⠉ │
│ ⠤⠔⠒⠉⠉                                            ⠉⠁                                              │
│ ⣀⣀⡤⠤⠤⠤⠖⠒⠒⠒⠦⠤⠤⠔⠒⠊⠉⠉⠙⠒⠒⠒⠲⠤⠤⠤⠤⠤⠤⠤⠔⠒⠉⠉⠉⠉⠉⠉⠉⠑⠒⠒⠒⠲⠤⠤⠤⢤⣀⣀⣀⠤⠔⠒⠒⠒⠉⠉⠉⠉⠓⠒⠒⠒⠦⠤⠤⠤⠤⠤⠤⠤⠴⠒⠒⠒⠚⠉⠉⠉⠉⠓⠒⠒⠒⠦⠤⠤⠤⠖⠒⠒⠒⠒⠒⠒ │
│ max 11.2  mean 4.8  min 0.0 mph                                                                  │
├─ INSIDE ─────────────────────────────────────────────────────────────────────────────────────────┤
│ Awair score            100  ████████████████████████  GOOD                                       │
│ Temperature        68.4 °F  ██████████░░░░░░░░░░░░░░  GOOD                                       │
│ Humidity            49.3 %  ████████████░░░░░░░░░░░░  GOOD                                       │
│ CO₂                436 ppm  ▌░░░░░░░░░░░░░░░░░░░░░░░  GOOD                                       │
│ VOC                117 ppb  ██░░░░░░░░░░░░░░░░░░░░░░  GOOD                                       │
│ PM2.5              1 µg/m³  ▌░░░░░░░░░░░░░░░░░░░░░░░  GOOD                                       │
│                                                                                                  │
│                                                                                                  │
│                                                                                                  │
│                                                                                                  │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│ ▲ battery 2.63 V   lightning 4 wk ago                                                            │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

The **24-hour wind chart overlays three series** — `min`, `mean`, `max` — from
`recorder/statistics_during_period` (decision D16), each in its own hue from a single family so
they read as one measurement rather than three unrelated things. This is the chart that
justified the statistics API, and it is the thing `home` exists for: the Lovelace version needs a
card and a legend; here it is six rows and reads at a glance.

Note the `INSIDE` bars share a colour (`ok` green) rather than each getting its own. Colour is
carrying *state*, not identity — per decision D34. When CO₂ crosses 1000 ppm that row turns
amber and nothing else changes, so the eye goes straight to it.

---

## Breakpoints

| Grid | Where it comes from | What survives |
|---|---|---|
| **100×30** | Pi panel, 8×16 font — the design target (D1) | everything |
| **80×24** | 10×20 font on the panel; a default terminal window | drops `OP`/`FLAGS`, narrower range bar |
| **64×20** | 12×24 font | drops `SPD`, `CPA`, `RANGE`; detail pane collapses |
| **50×15** | 16×32 font — large, readable across a room | drops `TYPE`; header text shortens |
| **< 44×12** | — | refusal screen |

```text
┌─ hatty · radar        AIRSPACE · 9 tracked ──────────────────────────────────┐
│ ●  all nominal   9 tracked · 0 flagged · last alert 6 d ago                  │
├──────────────────────────────────────────────────────────────────────────────┤
│   FLIGHT   TYPE    DIST  RANGE         ALT    SPD  BRG                       │
│   UAL1234  B738     9.1  █░░░░░░░   36,000 →   446  284                      │
│   RCH451   C17     12.6  █▌░░░░░░   24,000 ↓   402  195                      │
│   N77148   C172    18.3  ██▌░░░░░    4,600 ↑   178  270                      │
│   SWA2584  B737    24.7  ███░░░░░   34,000 →   455  312                      │
│   AAL691   A321    31.2  ████░░░░   20,875 ↓   432  101                      │
│   VTM300   C560    36.8  ████▌░░░   29,150 →   408   38                      │
│   DAL1508  A320    41.4  █████░░░   27,825 ↓   450  263                      │
│   JBU607   A320    46.6  █████▌░░   36,025 →   434  280                      │
│   N556GA   GLF6    63.1  ████████   41,000 ↓   488  340                      │
├─ UAL1234 ────────────────────────────────────────────────────────────────────┤
│ Type      B738                Squawk    4231                Vertical  -1,200 │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│ 1090 ●  978 ●  RID ●   ↑↓ select  ⏎ pin  / search                            │
└──────────────────────────────────────────────────────────────────────────────┘
```

```text
┌─ hatty · radar ──────────────────────────────────────────────┐
│ ●  all nominal                                               │
├──────────────────────────────────────────────────────────────┤
│   FLIGHT   TYPE    DIST      ALT   BRG                       │
│   UAL1234  B738     9.1   36,000 →  284                      │
│   RCH451   C17     12.6   24,000 ↓  195                      │
│   N77148   C172    18.3    4,600 ↑  270                      │
│   SWA2584  B737    24.7   34,000 →  312                      │
│   AAL691   A321    31.2   20,875 ↓  101                      │
│   VTM300   C560    36.8   29,150 →   38                      │
│   DAL1508  A320    41.4   27,825 ↓  263                      │
│   JBU607   A320    46.6   36,025 →  280                      │
│   N556GA   GLF6    63.1   41,000 ↓  340                      │
│                                                              │
│                                                              │
│                                                              │
│                                                              │
├──────────────────────────────────────────────────────────────┤
│ 1090 ●  978 ●  RID ●   ↑↓  ⏎  /                              │
└──────────────────────────────────────────────────────────────┘
```

```text
┌─ hatty · radar ────────────────────────────────┐
│ ●  all nominal                                 │
├────────────────────────────────────────────────┤
│   FLIGHT     DIST      ALT   BRG               │
│   UAL1234     9.1   36,000 →  284              │
│   RCH451     12.6   24,000 ↓  195              │
│   N77148     18.3    4,600 ↑  270              │
│   SWA2584    24.7   34,000 →  312              │
│   AAL691     31.2   20,875 ↓  101              │
│   VTM300     36.8   29,150 →   38              │
│   DAL1508    41.4   27,825 ↓  263              │
│                                                │
├────────────────────────────────────────────────┤
│ 1090 ●  978 ●  RID ●   ↑↓  ⏎  /                │
└────────────────────────────────────────────────┘
```

At 50×15 the table still answers the question the screen exists for — what is up there, how far,
how high, which way. That is the test a responsive rule has to pass: not "does it fit" but "does
it still say the thing."

---

## The rules

### Column drop order

Declared per column as a minimum width, so the engine never needs a special case per breakpoint:

| Column | Min width | Why it survives or goes |
|---|---|---|
| `FLIGHT` | always | identity |
| `DIST` | always | the primary question |
| `ALT` + vertical arrow | always | the second question |
| `BRG` | always | the third |
| `TYPE` | 56 | useful, not essential |
| `RANGE` bar | 68 | redundant with `DIST` — it is a scanning aid, first to go |
| `SPD` | 80 | rarely acted on |
| `CPA` | 88 | derived, and `DIST` approximates it |
| `OP`, `FLAGS` | 96 | flags matter, but the alert strip already carries urgency |

`FLAGS` going first looks wrong until you notice the **alert strip never drops** — anything
genuinely urgent is on line 2 regardless of width.

### Panel drop order

1. **Trend chart** — the first thing to go on `home`
2. **Detail pane** — collapses below 24 rows on `radar`
3. **Status-bar hints** — abbreviate before they vanish (`↑↓ select  ⏎ pin  / search` → `↑↓ ⏎ /`)
4. **Table rows** — scroll rather than drop, since the table is the content

**The alert strip never collapses.** It reserves its line at every size. That is decision D34's
"reserve, do not reflow" carried into the responsive rules — a dashboard whose alert disappears
when the window shrinks is worse than one that never had an alert.

### Refusal

```text
┌─ hatty ──────────────────────────────┐
│                                      │
│   terminal too small                 │
│   need 44×12, have 40×10             │
│                                      │
│   resize, or use a smaller font      │
│                                      │
│                                      │
│                                      │
└──────────────────────────────────────┘
```

Below **44×12** the screen refuses and says what it needs. Drawing a corrupted layout is worse
than declining to draw — plan requirement F6, and it is why the minimum is stated rather than
discovered.

---

## Two findings

**1. Leftover rows are currently trapped.** At 100×30 the `INSIDE` panel leaves four blank rows,
and at 64×20 the table leaves four. That is honest — there are only nine aircraft and six Awair
metrics — but it looks unfinished.

The rule that fixes it: **one panel per screen is designated elastic, and absorbs leftover
rows.** On `radar` that is the table (which can always show more, or scroll); on `home` it is the
trend chart (which can always be taller). Everything else takes its natural height. Not
implemented in these mockups — recorded as the layout-engine rule it implies.

**2. The hero number costs more than it earns below 22 rows.** `home` drops it for a single
summary line, and the compact version is arguably better — five rows for one temperature is
generous at any size. Worth asking in review whether the hero survives at 100×30 either.

---

## Phase 5 is now complete

`radar` and `home` are both specified: layout, table treatment, colour semantics, alert states,
drill-down, breakpoints, drop order, and a refusal floor.

Remaining before implementation is engineering, not design — plan §6 (module specs, widget
contracts, config schema) and §7 (adversarial review). The one open design dependency is
**spike S2**, which after decision D33 gates only the Braille trend charts, not the tables.
