# ha-airspace — Entity and Attribute Contract

Read from the Airspace dashboard's Lovelace YAML, 2026-08-27. **Supersedes the ADS-B section
of [`dashboard-source-analysis.md`](dashboard-source-analysis.md)**, which described the
pre-`ha-airspace` setup (`sensor.ads_b_aircraft_1090` / `_978`) that is scheduled for removal.

This is the data contract the Radar panel binds against. It matters more than the dashboard
layout, because the layout is a Phase 5 question and this is not.

> **Privacy.** The source YAML contains LAN addresses for the flight-map and drone web UIs.
> Not reproduced here — decision D27.

---

## 1. The headline: derivation is already done upstream

The previous dashboard computed distance, bearing, sorting and "closest" scoring in ~60 lines
of embedded Jinja. `ha-airspace` provides all of it as attributes:

| Provided | Was previously computed in Jinja |
|---|---|
| `distance_nm` | equirectangular approximation from hardcoded home lat/lon |
| `bearing_deg` | not computed at all |
| `predicted_closest_approach_nm` | not computed at all |
| flag classification (military / interesting / emergency / spoof) | not available |
| `db_metadata` enrichment (model, operator, PIA/LADD/mil) | not available |

**This confirms decision D25 empirically, and shows the line is already drawn in the right
place.** The remaining Jinja in the Airspace dashboard is almost entirely *presentational* —
`'%.1f'|format(...)`, `round(0)|int`, `|join(', ')`, and fallbacks to `—`. The only
conditional logic is a small precedence chain for the Notes column
(`PIA` → `LADD` → `mil` → `ownop`), which is exactly the bounded value→appearance mapping
that plan §5 assigns to hatty's widgets.

So the split hatty needs is not merely *possible*, it is already how this data arrives.

## 2. The gift for the polar radar

`distance_nm` **and** `bearing_deg` are supplied per aircraft. Polar coordinates, directly.

The terminal polar radar (decision D11, the replacement for the map) therefore requires **no
computation whatsoever** — plot each aircraft at its given bearing and distance. Combined
with the ring geometry recovered earlier (5 rings, 10-unit intervals) and Braille sub-cell
resolution (decision D23), this is now a fully specified Phase 5 panel rather than an idea.

One change: `ha-airspace` reports **nautical miles**, where the older setup used statute
miles. Ring intervals should be restated in nm.

---

## 3. Entity contract

### Nearest aircraft — `sensor.airspace_nearest_aircraft`

**State:** distance in nm (float). `unknown` / `unavailable` when nothing is in range —
handled explicitly by the existing templates, and a good model for hatty's own three value
conditions.

| Attribute | Type | Notes |
|---|---|---|
| `flight` | str | callsign; may be absent |
| `hex` | str | ICAO 24-bit address |
| `country_flag` | str | emoji flag — **see §5** |
| `aircraft_type` | str | |
| `alt_baro_ft` | int | |
| `ground_speed_kt` | float | |
| `squawk` | str | |
| `registration` | str | |
| `flags` | list[str] | classification labels |
| `predicted_closest_approach_nm` | float | genuinely derived; nothing else provides this |
| `bearing_to` | dict | **keyed by watchpoint** → `bearing_to.home` |
| `distance_to` | dict | **keyed by watchpoint** |
| `db_metadata` | dict | `model`, `ownop`, `pia`, `ladd`, `mil`, `make`, `status` |
| `photo` | dict | `thumbnail_url`, `photographer` — not renderable, see §5 |

Note `bearing_to` / `distance_to` are **nested dicts keyed by watchpoint name**. Binding needs
nested attribute paths (`bearing_to.home`), not just one level.

### Nearest drone — `sensor.airspace_nearest_drone`

State is distance in nm. Attributes: `operator_id`, `track_id`, `ua_type`, `id_type`,
`agl_ft`, `operator_lat` (presence indicates whether the operator is located),
`bearing_to`, `db_metadata` (`make`, `model`, `status`).

### Flag collections — `sensor.airspace_flag_<flag>`

Flags in use: `military`, `interesting`, `emergency`, `spoof_suspect`.

**State:** count. **Attribute `aircraft`:** list of dicts, each with `flight`, `hex`,
`country_flag`, `aircraft_type`, `alt_baro_ft`, `distance_nm`, `bearing_deg`, `squawk`,
`db_metadata`; drone entries carry `track_id` instead of `flight`.

This is the list-valued attribute that drives risk R15 and the iterating-table widget
requirement.

### Counts

`sensor.airspace_aircraft_count`, `sensor.airspace_drone_count`.

### Map entities

`sensor.airspace_aircraft_map`, `sensor.airspace_drone_map`, `sensor.airspace_drone_operator`
— intended for HA's map card. Likely the **largest payloads** in the instance, since they
carry positions for everything tracked. Prime candidates for *not* binding: hatty's polar
radar can be driven from the flag collections and nearest-aircraft instead. Measure in S1
before deciding.

### Alerts — `binary_sensor.airspace_alert_military_close`

A real alert entity: state `on`/`off`, with `distance_to`, `country_flag`, `flight`, `hex`,
`entity_picture`, `photographer`.

**`binary_sensor` is a third entity domain** for the state model, alongside `sensor` and
`event`. It is also the cleanest possible input for the alerts panel — a boolean with
context attributes, no client-side threshold logic.

### Receiver health

| Entity | |
|---|---|
| `binary_sensor.airspace_receiver_rx_1090_status` | up/down |
| `sensor.airspace_receiver_rx_1090_aircraft_count` | |
| `sensor.airspace_receiver_rx_1090_message_rate` | msg/s |
| …same for `rx_978` | |
| `binary_sensor.airspace_remote_id_dump3411_status` | Remote ID feed |
| `sensor.airspace_remote_id_dump3411_drone_count` | |
| `sensor.airspace_remote_id_dump3411_message_rate` | |

Nine entities of genuine infrastructure health. This is ready-made content for an
Infrastructure panel, which the brief listed as a candidate category and which had no
concrete source until now.

---

## 4. Interaction model, observed

The Airspace dashboard uses a **glance row → popup detail** pattern: compact counts across
the top, tapping one opens the full table or record. That is precisely the k9s drill-down the
brief names as a reference, and it maps cleanly onto a terminal: a summary row, then
select-to-expand.

Worth recording as an input to the Phase 5 navigation design — the drill-down model is
already the user's own mental model for this data, not something to invent.

It also uses `type: conditional` for the military-close alert, reinforcing decision D26
(conditional panel visibility).

---

## 5. What the terminal loses, and what to do instead

**Aircraft photos** (`photo.thumbnail_url`, `entity_picture`) cannot be rendered — image
output is an explicit non-goal. No mitigation needed; the textual record is the point.

**Country flag emoji** (`country_flag`) are a real hazard, not a cosmetic loss. Regional
indicator pairs are **double-width** and inconsistently rendered across terminals and fonts —
exactly risk R3, and the console font question from decision D22/D23. Options: substitute
the two-letter country code, or verify the flag renders at a stable width under the chosen
font. **Add to spike S2's glyph test.**

---

## 6. Effect on the plan

| Item | Effect |
|---|---|
| **D25** (compute in HA) | Strengthened — derivation is already upstream; remaining Jinja is presentational |
| **D11** / polar radar | Fully specified: `bearing_deg` + `distance_nm` supplied; no computation needed. Rings in **nm** |
| §7 state model | Add `binary_sensor`; add nested watchpoint-keyed dicts (`bearing_to.home`) |
| Alerts panel | Concrete source: `binary_sensor.airspace_alert_military_close` |
| Infrastructure panel | Concrete source: nine receiver-health entities |
| Phase 5 navigation | Glance→popup drill-down is the user's existing mental model |
| Spike S1 | Entity defaults corrected to the real `airspace_*` set; `_map` entities flagged as likely largest payloads |
| Spike S2 | Add country-flag emoji to the width test |
| Units | `ha-airspace` reports **nm**, not statute miles |
