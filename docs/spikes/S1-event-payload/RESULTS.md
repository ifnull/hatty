# S1 — Results

Run 2026-08-31 from the Pi against `ha.home.arpa:8123`. Two measurements, 180 s and 90 s.

## Headline

| | Full bound set (28 entities) | MVP set, no `*_map` (26 entities) |
|---|---|---|
| Duration | 178.5 s | 89.8 s |
| Messages | 996 (**5.58/s**) | 357 (**3.98/s**) |
| Bytes | 1,337,910 (**7,494 B/s**) | 648,206 (**7,221 B/s**) |
| Message size | p50 208, p95 3,741, max 15,307 | p50 463, p95 3,748, max 14,829 |

**≈ 7.2 KB/s sustained**, and dropping the `*_map` entities saved only 3.6 % — they were not the
cost.

## D21 and Phase 7 finding D1 are confirmed

D21 claimed `ha-airspace` throttles to ~1/s, on the strength of its README. Phase 7 finding D1
objected that this was never measured. It is now, and the README was right:

| Entity | Updates | Rate |
|---|---|---|
| `sensor.airspace_flag_military` | 164 / 178.5 s | 0.92/s |
| `sensor.airspace_flag_interesting` | 164 | 0.92/s |
| `sensor.airspace_nearest_aircraft` | 164 | 0.92/s |
| `sensor.airspace_receiver_rx_1090_message_rate` | 162 | 0.91/s |
| `sensor.st_00128663_wind_speed` | 64 | 0.36/s |

No entity exceeds ~1/s. **Finding D1 is resolved.** But note the aggregate: the throttle is
*per entity*, and binding six chatty entities yields ~5.6 msg/s. That is the number that
matters, and it is not what D21 implied.

## Where the bytes actually go

Probing payloads directly:

| Entity | Payload | Shape |
|---|---|---|
| `sensor.airspace_flag_interesting` | **4,128 B** | `aircraft: LIST[10]`, ~413 B per item |
| `sensor.airspace_flag_military` | **3,847 B** | `aircraft: LIST[10]`, ~381 B per item |
| `sensor.airspace_nearest_aircraft` | 1,306 B | nested dicts, one aircraft |
| `sensor.airspace_aircraft_map` | **421 B** | *no list* — map marker only |
| `sensor.airspace_aircraft_count` | 395 B | scalar |

Two entities at ~4 KB, each re-sent ~0.9 times a second, account for essentially the entire
7.2 KB/s. **Risk R15 is confirmed precisely**: the cost is attribute-list rewrites, and
`subscribe_entities`' field-level diffing cannot reach inside an attribute value.

## Three assumptions this destroyed

**1. `sensor.airspace_aircraft_count` is 94, not 9.** Every mockup shows nine aircraft. The real
instance tracks **94**. A 94-row table does not fit a 32-row screen under any design.

**2. The flag lists are capped at 10.** `sensor.airspace_flag_interesting` has `state = '11'`
but `aircraft: LIST[10]` — the count and the list disagree because the list is truncated. Any
table bound to it is showing a subset and must say so.

**3. There is no full-aircraft-list entity.** `sensor.airspace_aircraft_map` is 421 B with no
list attribute — it is a map-marker entity for HA's map card, not the aircraft table. The only
collections are the flag sensors, each capped at 10.

See decision **D41**; the radar table's data source does not exist in the shape the design
assumed.

## What S1 did *not* measure

**This is the HA → daemon link, not the daemon → Pi link.** Those are different numbers with
different constraints:

- **HA → daemon: 7.2 KB/s.** Runs over a virtual bridge on one Proxmox host (D13). Entirely
  acceptable; no action needed.
- **daemon → Pi over SSH:** *unmeasured*, and it is the one requirement N3 actually constrains.
  Phase 7 finding A2 predicts near-continuous full-frame repaints from table re-sorting — about
  6 KB per frame at 4 Hz — which would be an order of magnitude worse than the HA link.

A2 therefore stands unresolved. It cannot be measured until the renderer exists; the mitigation
(sort hysteresis) should be built in from the start rather than retrofitted.
