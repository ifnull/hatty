# Measured Environment

Requirements-gathering output. The brief asks that target display dimensions and
practical terminal size be captured rather than assumed; this file records what was
actually measured, with the date and method, so later phases can distinguish
measurement from assumption.

Measured: 2026-08-26 / 2026-08-27.

---

## Client — Raspberry Pi (thin terminal)

| Property | Value | Source |
|---|---|---|
| Board | Raspberry Pi 3 Model B Rev 1.2 | `/proc/device-tree/model` |
| Revision code | `a22082` | `/proc/cpuinfo` |
| SoC | BCM2837, 4× Cortex-A53 @ 1.2 GHz | board revision |
| Architecture | `aarch64` (64-bit userspace) | `uname -m` |
| RAM | 905 MiB total; 536 MiB used, 369 MiB available at measurement | `free -h` |
| OS | Debian GNU/Linux 13 (trixie) | `/etc/os-release` |
| Networking | 100 Mbit Ethernet; 2.4 GHz-only 802.11n | board revision |

### Display

| Property | Value | Source |
|---|---|---|
| Panel | 800×480 DSI (official 7" Raspberry Pi touchscreen) | `/sys/class/drm/card0/card0-DSI-1/modes` |
| HDMI | no modes reported (inactive) | `/sys/class/drm/card0/card0-HDMI-A-1/modes` |
| Framebuffer | `800,480` | `/sys/class/graphics/fb0/virtual_size` |
| Touch | yes (panel is a touchscreen) | panel model |
| Physical width | ≈ 155 mm → ≈ 0.19 mm/px | panel spec |
| Viewing distance | arm's length, desk-mounted | user, 2026-08-27 |

### Terminal

| Property | Value | Source |
|---|---|---|
| `TERM` | `xterm-256color` | measured |
| Colors | 256 | `tput colors` |
| Locale | `en_US.UTF-8` | `locale` |
| `stty size` | `32 116` | measured (**see caveat**) |

> ### ✅ RESOLVED 2026-08-29 — measured on the panel
>
> Spike S2 was run **locally on the panel** (the script confirms `session: LOCAL`), and reports:
>
> ```
> terminal: xterm-256color   size: 22 80   colours: 256
> ```
>
> **The panel's real terminal is 80 columns × 22 rows.** The earlier `116 × 32` reading was
> indeed from an SSH client, as suspected. Open question O9 is closed.
>
> **Why 80×22 and not 100×30:** the terminal is running *windowed* on the full Raspberry Pi OS
> desktop — taskbar, window title bar and a File/Edit/Tabs/Help menu bar together consume
> roughly 85 px of the 480 px height, and the font is about 10×18 rather than 8×16.
>
> | Configuration | Grid |
> |---|---|
> | windowed on the desktop, ~10×18 font — **what exists today** | **80 × 22** |
> | fullscreen, ~10×18 font | 80 × 26 |
> | fullscreen, 8×16 font — **decision D22 + D23** | **100 × 30** |
>
> So the 100×30 target of decision D1 is achievable, but requires *both* a fullscreen terminal
> and an 8×16 font — neither of which is in place yet. This is precisely the change D22 already
> calls for (X with no desktop environment, single fullscreen terminal). See decision D38.

### Font size → grid, at 800×480

| Font (px) | Grid | Char width | Note |
|---|---|---|---|
| 6×12 | 133×40 | 1.16 mm | dense; poor readability |
| 7×14 | 114×34 | 1.36 mm | |
| **8×16** | **100×30** | **1.55 mm** | **selected target** — Linux console default |
| 10×20 | 80×24 | 1.94 mm | classic 80×24 |
| 12×24 | 66×20 | 2.33 mm | |
| 16×32 | 50×15 | 3.10 mm | comfortable at distance; single-column only |

**Target: 100×30 at 8×16.** Selected because the intended primary dashboard has three
dense columns, and ~100 columns is close to the minimum that renders it without
shedding table columns. At arm's length (confirmed desk placement) an 8×16 cell gives
roughly 2.2 mm cap height, which is comfortable at 45–70 cm. Larger fonts are readable
from further away but force a paged, single-column redesign.

### Known client constraint: console glyph budget

A bare Linux framebuffer console uses a font limited to **256 or 512 glyphs**. Box-drawing
(U+2500–U+257F) and block elements (U+2580–U+259F) are typically present in the Uni-series
console fonts; Braille (U+2800–U+28FF) is 256 code points on its own and almost certainly
will not fit. Braille-based sub-cell plotting is therefore expected to be unavailable on
the bare console but available under X in a terminal emulator.

**Superseded 2026-08-27 by decision D22.** The cap is a property of the framebuffer console,
not of font availability — no installable font escapes it. Resolved by running X without a
desktop environment, which gives full Unicode including Braille. The glyph test below is still
worth running under the chosen terminal and font (**DejaVu Sans Mono**, decision D23) to
confirm coverage and rendered width at 8×16. See spike S2.

Test to run on the Pi, once in a terminal emulator and once on a bare tty (Ctrl+Alt+F1):

```bash
printf '─ │ ┌ ┴ | ▁▂▃▄▅▆▇█ | ⣿⡇⢶ | ↑→↓↗ | ⚠ µ ²³\n'
```

Any glyph rendering as tofu on the tty is unavailable to a console-based appliance.

---

## Server — Proxmox host

| Property | Value | Source |
|---|---|---|
| Platform | Proxmox | user |
| Home Assistant | migrating to a VM on this host | user |
| Dashboard app | separate instance on the **same physical host** | user |
| Link HA ↔ app | virtual bridge, same host | derived |

Consequences, carried into architecture:

- Latency between the app and HA is sub-millisecond with no physical network in the path.
  Reconnect design is dominated by **HA restarts**, not network partitions.
- A host reboot takes down HA and the dashboard **simultaneously**. The app must start
  cleanly against an HA that is not yet up, and retry rather than exit.
- Single point of failure for both. Acceptable for a personal utility; recorded so it is
  a decision rather than an oversight.
- VM vs. LXC for the dashboard instance is undecided (see open questions).

---

## Home Assistant instance

Measured from the UI screenshots in `docs/reference/`, 2026-08-26.

| Property | Value |
|---|---|
| Devices | 200 |
| Entities | **981** |
| Helpers | 46 |
| Install type | Supervised or HAOS (Home Assistant Supervisor present, 16 services) |

### Integrations present

Apple iCloud (9 devices), Apple TV, **Awair**, Backup, Belkin WeMo, Bluetooth,
Brother Printer, Browser mod, DLNA Digital Media Renderer/Server, **Frigate** (2 devices),
Google Cast (10), Google Translate TTS, HACS (15 services), HDHomeRun (3),
Home Assistant iOS, Home Assistant Supervisor (16), **iBeacon Tracker** (19 devices),
Internet Printing Protocol, LG ThinQ (2), Matter, **Met.no**, Mobile App (3), MQTT,
Radio Browser, Raspberry Pi, Raspberry Pi Power Supply Checker, RESTful, **Sense**,
Shopping List, SmartThings (bulk of devices).

Not visible in the captured screenshots but present per the primary dashboard:
**WeatherFlow** (Tempest station `ST-00128663`) and an **ADS-B** source feeding template sensors.

### Helpers of note

46 helpers, heavily template sensors:

- `sensor.airspace_aircraft_map`, `sensor.airspace_drone_map`, `sensor.airspace_drone_operator`
- `sensor.closest_aircraft`, `sensor.closest_aircraft_3d_distance_mi`, `sensor.closest_aircraft_message`
- `input_select.hdhomerun_channel`, `input_select.hdhomerun_favorite_chan…`
- `input_text.material_you_base_color`

Template sensors can emit `unknown`/`unavailable` and type-inconsistent values; several of
these appear to carry structured payloads in attributes rather than scalar states. The state
model must handle attribute-heavy entities, not just scalars.

### Live conditions at capture

Real content for an alerts panel existed on day one:

- Three entities in an error/unavailable state: `[TV] 85 inch TV`, `85 inch TV` /
  `TV channel` / `TV channel name` (SmartThings + DLNA), and both iBeacon entities.
- WeatherFlow Tempest **battery at 2.63 V**, rendering in the red zone in Lovelace.

---

## Primary dashboard (source of truth for design)

`docs/reference/Screenshot from 2026-08-26 23-51-49.png` is the Lovelace dashboard the
Pi's primary terminal dashboard should be modeled on. Three columns:

**Weather Station** (WeatherFlow Tempest `ST-00128663`)
: feels-like temp + sparkline, humidity, wind speed + dense sparkline, wind direction (110°),
  rain, a 24 h wind min/mean/max chart, battery voltage gauge.

**Lightning + Interior Climate** (Awair)
: last lightning strike as relative time ("4 weeks ago"), humidity / CO₂ / VOC / PM2.5
  radial gauges, Awair Score with sparkline, temperature.

**Radar** (ADS-B)
: closest-aircraft detail table (flight, tail, distance, altitude, speed, heading, vertical
  rate), a "tracking N aircraft" table of 9 rows × 7 columns, and a geographic map.

Translation notes:

- The aircraft tables are *more* natural in a terminal than in HTML.
- Radial gauges become labeled bars with threshold colouring — arc lost, semantics kept.
- The map cannot be rendered (explicit non-goal). Candidate replacement: a polar radar
  scope plotting aircraft by bearing and distance. See `docs/planning/phase-1-plan.md`.
- Relative-time formatting ("4 weeks ago") is a shared widget requirement.
- Wind direction and vertical speed map to arrow glyphs (↖↑↗→↘↓↙←, ↑→↓).

### Update rate — bounded, and ours to set

**Revised 2026-08-27.** ADS-B comes from the user's own integration,
[`ifnull/ha-airspace`](https://github.com/ifnull/ha-airspace), which reads a local receiver
(dump1090 / dump1090-fa / readsb / dump978, plus dump3411 for drone Remote ID) and publishes
over MQTT. It is local-first: no cloud API, no account.

Critically for this project, it **deliberately avoids per-aircraft entities** — "that would
overwhelm HA's registry" — and instead exposes aggregate sensors carrying distance-sorted
aircraft lists in attributes:

- `sensor.airspace_nearest_aircraft` — distance to closest aircraft, details in attributes
- `sensor.airspace_aircraft_count`, `sensor.airspace_drone_count`
- `sensor.airspace_nearest_drone` — operator location in attributes
- `sensor.airspace_flag_<flag>` — flag counts, aircraft list in attributes
- per-receiver status/stats entities

Publish rates are **throttled, default 1/s**. So the Radar panel binds a handful of entities
at ≤1 Hz, and the rate is a configuration value under the user's control — not an
uncontrolled firehose. Risk R1 is downgraded accordingly.

Two consequences carried into the architecture:

1. **The cost driver is payload size, not event count.** A change inside an attribute rewrites
   the whole aircraft list, and `subscribe_entities` field-level diffing does not reach inside
   an attribute value. Roughly 1–2 KB per update at 1 Hz — comfortable, but measured by S1.
2. **Bindings must address attribute paths and iterate collections.** The aircraft table reads
   a list nested inside one entity's attributes, not one value per widget.

The integration also binds entity availability to `airspace/status`, so entities go
`unavailable` when the service stops rather than showing stale values — the staleness
discipline this project wants, already handled upstream for this source.
