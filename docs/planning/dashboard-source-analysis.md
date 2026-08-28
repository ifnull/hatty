# Primary Dashboard — Source Analysis

The Lovelace YAML for the dashboard in
`docs/reference/Screenshot from 2026-08-26 23-51-49.png`, read as a requirements document.
This is the single most useful artefact so far: it replaces inference about the primary
dashboard with its actual definition.

Analysed 2026-08-27.

> **Privacy note.** The source YAML hardcodes home latitude and longitude, and a LAN address.
> Those are **deliberately not reproduced here**, and must never be committed. See decision
> D27.

---

## 1. Real entity IDs

Previously guessed; now known.

**WeatherFlow Tempest** — `sensor.st_00128663_*`
`precipitation`, `feels_like`, `humidity`, `wind_speed`, `wind_direction`,
`battery_voltage`, `lightning_average_distance`, `lightning_count`
plus `event.st_00128663_lightning_strike`

**Awair Element** — `sensor.awair_element_128797_*`
`humidity`, `carbon_dioxide`, `volatile_organic_compounds_parts`, `pm2_5`, `score`,
`temperature`

**ADS-B — ⚠️ SUPERSEDED.** `sensor.ads_b_aircraft_1090` / `_978` are the *pre-`ha-airspace`*
setup and are scheduled for removal by the user. See
[`airspace-entity-model.md`](airspace-entity-model.md) for the current contract. The §3
analysis below still stands as a record of what the old dashboard had to compute by hand —
and reads better in hindsight, since `ha-airspace` now supplies most of it.

`event.` is a **new entity domain** for this project. Event entities carry a timestamp state
rather than a scalar, which is where "4 weeks ago" comes from. Add to the type discipline in
§7: the state model must handle `sensor`, `event`, and list-valued attributes.

---

## 2. Statistics confirmed — S6's residual question closed

The wind chart is a `statistics-graph` card:

```yaml
chart_type: line
period: 5minute
type: statistics-graph
entities: [sensor.st_00128663_wind_speed]
stat_types: [mean, min, max]
days_to_show: 1
```

This is exactly `recorder/statistics_during_period` with `period: "5minute"`,
`types: [mean, min, max]`, over 24 hours — the API verified in plan §7 and adopted in
decision D16.

**Because this card renders, long-term statistics demonstrably exist for the wind sensor.**
That closes the residual half of spike S6 ("which of my entities have statistics") for the
one entity that actually needed it. S6 shrinks to confirming the same for any *other* series
a dashboard wants.

---

## 3. The big finding — where computation lives

The Radar panel is roughly sixty lines of Jinja embedded in two `markdown` cards. It:

- merges the 1090 and 978 aircraft lists;
- filters to planes with `lat`, `lon`, and an `alt_baro` that is not `none` or `'ground'`;
- computes distance from home with a crude equirectangular approximation
  (`((Δlat)² + (Δlon)²)^0.5 × 69`);
- computes a 3D "closest" score as `(dist² + alt_miles²)^0.5`;
- sorts by that score, takes the first for the detail card and the nearest 15 for the table;
- maps altitude to a colour band (<5 000 / <15 000 / <30 000 / above);
- maps `baro_rate` to a direction glyph at a ±300 ft/min threshold;
- formats links, thousands separators, and units.

**None of that is data binding. It is a program.**

> **Postscript, 2026-08-27.** `ha-airspace` supplies `distance_nm`, `bearing_deg`,
> `predicted_closest_approach_nm` and flag classification as attributes, so most of this
> Jinja no longer needs to exist. The analysis below is retained because it is *why* D25
> was made, and because it shows what the alternative looks like.

This is the most consequential thing the YAML reveals, because plan §5 assumed bindings were
"attribute path → widget." They are not. The real dashboard needs list merging, predicate
filtering, derived fields, sorting by a computed value, top-N, and threshold mapping.

Two ways to absorb it:

| | Compute in hatty | Compute in Home Assistant |
|---|---|---|
| Shape | A transform pipeline in hatty's config | Template sensors; hatty binds pre-computed values |
| hatty config | Becomes a small programming language | Stays declarative |
| Cost | Large, open-ended scope; every new dashboard risks new primitives | Each dashboard change needs an HA config change |
| Precedent | none | **Already in use** — the instance has `sensor.closest_aircraft`, `sensor.closest_aircraft_3d_distance_mi`, `sensor.closest_aircraft_message` doing precisely this |

**Recommendation: compute in Home Assistant.** See decision D25. The instance already
contains template sensors doing this exact job, HA already has a mature template engine, and
keeping hatty's binding layer declarative is what prevents the config model from turning
into a scripting language — which is the single most likely way this project's scope runs
away.

hatty still needs *presentational* mapping — value → colour band, value → glyph, number
formatting, relative time. That is a bounded, closed set and belongs in widgets. The line to
hold is: **hatty maps values to appearance; Home Assistant derives values.**

Curiously, the dashboard recomputes in Jinja what those template sensors already provide.
Worth asking why before assuming the sensors are adequate — they may be stale, or predate
the dual-band 1090/978 setup.

---

## 4. Conditional visibility — a layout requirement

Several cards carry `visibility` conditions:

```yaml
visibility:
  - condition: numeric_state
    entity: sensor.st_00128663_lightning_count
    above: 0
```

So the lightning cards appear only when there have been strikes. In Lovelace, a hidden card
simply reflows the grid.

**In a fixed character grid this is materially harder,** and plan §5 does not cover it. When
a panel disappears at 100×30, the options are: leave the space empty, collapse and reflow the
remaining panels, or substitute a placeholder. Each has a cost — reflow means the dashboard's
geometry changes under the reader, which is exactly the "visually unstable" failure the brief
warns against for a glanceable display.

Recorded as decision D26 and flagged for Phase 5. The likely answer is that panels declare
whether they *collapse* or *reserve* their space, but that is a design call, not a Phase 1 one.

---

## 5. Exact thresholds, ready to reuse

Directly transferable to hatty's threshold bars:

| Metric | Range | Bands |
|---|---|---|
| Battery voltage | 0 – 2.8 V | red < 2.55, yellow 2.55–2.65, green ≥ 2.65 |
| Interior humidity | 0 – 100 % | red < 30, green 30–60, red ≥ 60 |
| CO₂ | 400 – 2000 ppm | green < 1000, orange 1000–1500, red ≥ 1500 |
| VOC | 0 – 1500 ppb | green < 500, orange 500–1000, red ≥ 1000 |
| PM2.5 | 0 – 50 µg/m³ | green < 12, orange 12–35, red ≥ 35 |
| Altitude (aircraft) | — | < 5 000 / < 15 000 / < 30 000 / above |
| Vertical rate | — | climb > +300, level, descent < −300 ft/min |

**Do not copy the min/max verbatim.** The battery gauge spans 0–2.8 V for a sensor that
lives between roughly 2.4 and 2.8, so more than 85 % of the scale represents impossible
values and the needle barely moves. A terminal bar should span the *useful* range. The
Lovelace ranges encode what the card widget needed, not what the data means.

Correction to an earlier note in this repo: the observed 2.63 V sits in the **yellow**
band, not red. The gauge merely *looks* red because the arc is coloured across a 0-based
scale. The conclusion is unchanged — there is a genuine warning-level condition on day
one — but the severity was overstated.

---

## 6. The map, and a gift for the polar radar

The third card in the Radar stack is an `iframe` pointing at a local ADS-B web UI, with
these parameters:

```
ringCount=5&ringBaseDistance=10&ringInterval=10
```

**Five rings, first at 10 miles, 10 miles apart — a 50-mile polar scope.** So the existing
mental model for this data is *already* a ring-based radar display, not a slippy map.

That is a strong argument for the polar radar panel floated in Phase 1 §10 as the terminal
replacement for the map (decision D11), and it hands Phase 5 a concrete specification
instead of a blank page: plot aircraft by bearing and distance, five rings at 10-mile
intervals, 50-mile range. With Braille available (decision D23) that is a genuinely good
terminal visualisation, and unlike the raster map it is *better* in this medium than in a
browser.

---

## 7. Effect on the plan

| Item | Effect |
|---|---|
| Spike S6 | Residual question closed for wind speed — statistics demonstrably exist |
| Spike S1 | Harness defaults updated to the real entity IDs; now runnable as-is |
| §5 config model | **Computation moves to HA template sensors** (D25); hatty maps values to appearance only |
| §5 layout engine | **Conditional panel visibility** is a requirement (D26) |
| §7 state model | Add the `event` domain and list-valued attributes to type discipline |
| §10 / D11 | Polar radar gains a concrete spec: 5 rings, 10 mi apart, 50 mi range |
| Phase 5 | Threshold table above is ready-made input; Lovelace min/max are not |
| Security | Home coordinates and LAN addresses must never be committed (D27) |
