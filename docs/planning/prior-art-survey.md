# Prior-Art Survey

Answers open question **O5**, a Phase 2 entry condition: *do adequate Home Assistant TUIs
already exist?* The brief's Phase 2 asks whether a custom application is justified at all,
and that question cannot be answered honestly without this.

Surveyed 2026-08-27. Method and limits recorded at the end.

**Conclusion: no adequate prior art exists.** Two purpose-built Home Assistant TUIs were
found — one archived with 4 stars, one a three-commit stub. Neither is a dashboard. The
strongest generic alternative is polling-only, which is the exact architecture this project
rejects. But the survey did surface **one genuine architectural rival** that Phase 2 should
weigh seriously, and it is not a TUI.

---

## 1. Purpose-built Home Assistant TUIs

### bhdr — the only real attempt

[bmedicke/bhdr](https://github.com/bmedicke/bhdr) — "beautiful home assistant TUI"

| | |
|---|---|
| Language / stack | Go |
| HA integration | **WebSocket API** — explicitly chosen "for the fastest possible response time" |
| Interface | Tree-based expandable/collapsible entity nodes; a switches view and a WebSocket log view |
| Configuration | `.bhdr.json` — connection, token, custom entity mappings |
| Control | Yes — toggles lights, switches, input booleans |
| Keybindings | VI-style |
| Status | **Archived 2024-11-05, read-only.** 4 stars, 3 forks, 84 commits |

This is the closest existing work, and its architectural instincts match this project's:
WebSocket over polling, JSON configuration, keyboard-driven. That convergence is mild
validation of §5 and §7.

**Why it does not satisfy the requirement.** It is an entity *browser and controller*, not a
dashboard. A collapsible tree of entities plus a log pane is the k9s half of the brief's
references with none of the btop half — no panels, gauges, bars, sparklines, trends, or data
tables, and no notion of a composed screen where information sits in a fixed place to be read
at a glance. The brief's core requirement is a glanceable status surface; bhdr is a navigator.
It is also archived, so adopting it means adopting an unmaintained dependency.

**Worth reading anyway.** 84 commits of working Go WebSocket-to-HA code is a useful reference
for the client layer, whatever framework Phase 4 selects.

### homeassistant-tui — a stub

[K0HAX/homeassistant-tui](https://github.com/K0HAX/homeassistant-tui) — "Terminal Interface
to Home Assistant, written in Golang."

2 stars, 1 fork, **3 total commits**, one source file, no README, one open issue. Framework
and API approach are undocumented and undeterminable from the repository page. This is an
abandoned start, not a usable tool. Recorded for completeness.

---

## 2. Line-oriented CLIs — wrong medium

[home-assistant-ecosystem/home-assistant-cli](https://github.com/home-assistant-ecosystem/home-assistant-cli)
(`hass-cli`) is the mature, semi-official option — Python, installable via
`pip install homeassistant-cli`, configured with `HASS_SERVER` and `HASS_TOKEN`. It manages
entities, devices, areas, integrations and services, with table, YAML and JSON output.
Version 1.0.0 was released in 2026.

It is genuinely good at what it does, and it is **not a candidate**. It is precisely the
"conventional command-line program that continually appends lines of output" the brief
contrasts itself against in its Core Concept section. `hass-cli state list` is a query tool
for scripting, not a persistent display. No amount of `watch(1)` turns it into one —
full-screen repaint on a 24-hour uptime is the thing the brief's performance philosophy
exists to avoid.

Useful to this project in one respect: as a **scriptable oracle during development and Phase
8 testing**, for asserting HA-side state without going through the app.

---

## 3. Generic TUI dashboard builders — the strongest "buy" candidate

### dashbrew

[rasjonell/dashbrew](https://github.com/rasjonell/dashbrew) — "TUI dashboard builder that lets
you visualize data from scripts and APIs right in your console."

| | |
|---|---|
| Language / stack | **Go + Bubble Tea** |
| Configuration | JSON — containers, components, styling, bindings |
| Layout | **Responsive flex containers**, row/column direction with flex values |
| Widgets | Text, todo lists, charts (numeric), histograms, tables |
| Data sources | Shell scripts, HTTP APIs, files |
| Update model | **Interval polling only** — `refresh_interval` per component. No streaming or WebSocket; long-running streams explicitly unsupported |
| Status | 270 stars, 72 commits, v1.1.0, MIT, actively maintained |

This is the most serious alternative found, and it deserves a straight answer rather than a
dismissal.

**Why it does not fit.** Its update model is the disqualifier. The brief's integration
requirement is event-driven — subscribe, maintain state, redraw affected regions — and
explicitly rejects "repeatedly polling the entire Home Assistant state just to refresh the
display." Dashbrew polls per component on an interval and cannot stream. Pointing it at HA
means either polling REST endpoints on a timer (the rejected architecture) or bolting a
WebSocket bridge onto someone else's app. And its widget vocabulary — text, todos, charts,
histograms, tables — is missing the gauges, threshold bars, sparklines and status indicators
that the target dashboard is largely composed of.

The honest cost comparison: adapting dashbrew means adding a streaming data source, a state
cache, and roughly half the widget set to an upstream project, then maintaining a fork or
negotiating the changes. That is comparable to building, with less control and a dependency
on someone else's roadmap.

**But it is valuable prior art.** Dashbrew is Go + Bubble Tea + JSON config + a flex layout
engine — the *exact* shape proposed in §5 and §6 of the plan. Its layout engine should be read
before writing one. Two secondary points: an actively maintained 270-star terminal dashboard
builder independently chose Bubble Tea, which is weak but non-zero evidence for the §6
leaning; and its config schema is a free set of lessons about what a declarative dashboard
model needs, which is directly relevant given the brief's instruction not to design the schema
prematurely.

### Grafana/Prometheus TUIs

[lovromazgon/grafana-tui](https://github.com/lovromazgon/grafana-tui) and
[fedexist/grafatui](https://github.com/fedexist/grafatui) render Grafana dashboards and
Prometheus series in the terminal. Both are the right *shape* and the wrong *data source* —
they assume a Grafana/Prometheus backend, not Home Assistant. Relevant only if HA data were
first shipped into Prometheus, which adds an entire data pipeline to avoid writing a WebSocket
client. Not pursued.

---

## 4. The genuine rival — server-side rendering to a dumb client

This category is not a TUI and is the most serious challenge to the project's premise. Phase 2
should press on it.

### homeassistant-dashboard-server

[petterhj/homeassistant-dashboard-server](https://github.com/petterhj/homeassistant-dashboard-server)
renders a customisable HA dashboard server-side and captures screenshots at configured
intervals, as a backend for wall-mounted displays such as e-paper panels.

**Why this is a real rival.** It shares this project's central insight — move rendering to a
capable server, leave the display device dumb — and then reuses the *entire existing Lovelace
widget ecosystem* for free. Every gauge, chart and card already exists. The Pi would display an
image rather than a character grid, which is arguably an even thinner client.

**The counter-case, which Phase 2 should test rather than accept from this document:**

- A browser is still rendering, just elsewhere. That is fine on the Proxmox host, but it keeps
  a Chromium dependency in the stack, which the brief lists as a non-goal.
- Whole-frame image refresh is the opposite of the brief's "redraw only the affected UI
  elements." At a desk-viewed dashboard with a 2.6 V battery warning and live aircraft, an
  interval-refreshed screenshot is a strictly worse fit than an event-driven one.
- The Pi still needs an image viewer, and probably X, which is the 536 MiB this project is
  trying to reclaim. Framebuffer viewers exist, but so does the complexity.
- No interactivity — the brief wants the architecture not to preclude control.
- Latency between a state change and it appearing is bounded below by the capture interval.

**Verdict: not adequate for this use case, but a legitimate design point.** If the requirement
were only "show me a wall display of static-ish sensor readings," this would very likely be the
correct answer and the project would be unnecessary. It is the strongest form of the "is a
custom application justified?" challenge, and it should be argued in Phase 2 on the merits, not
waved away.

### Kiosk browsers — the status quo this project rejects

[TouchKio](https://github.com/leukipp/touchkio) is a Node.js/**Electron** kiosk for HA
targeting exactly this hardware — a Raspberry Pi with a DSI touch display.
[DashD](https://github.com/EliasStar/DashD) is a lighter Chromium kiosk daemon. The common
guidance is Chromium kiosk mode plus LightDM to avoid a full desktop.

These are the measured problem, not a solution to it. Electron on a Pi 3B with 369 MiB
available against a 981-entity instance is the situation the brief opens by describing. LightDM
plus Chromium is lighter than a desktop and still requires a browser engine, a DOM and a
compositor on the display device. Recorded because Phase 2 must confirm the premise rather than
inherit it — and because a *measurement* of the current kiosk approach on this exact hardware
would make the argument empirical instead of rhetorical.

**Suggested addition to the spike list.** See S9 below.

---

## 5. What the absence means

No btop-, k9s- or lazygit-class Home Assistant TUI exists. The only serious attempt is archived
at 4 stars. Community searches surface Lovelace cards, kiosk setups and Docker images, not
terminal dashboards.

Two readings, and both are worth holding:

**The opportunity reading.** There is a genuine unmet niche, which strengthens decision D2's
open-source optionality. A polished terminal HA dashboard would be the only one of its kind.

**The warning reading, which Phase 2 should take seriously.** Absence of prior art is sometimes
absence of demand. Home Assistant's users overwhelmingly want a touch-friendly graphical
dashboard, and the population wanting a terminal one may be very small. bhdr was archived rather
than finished, which is weak evidence that its author's need was satisfied another way or
evaporated.

This does not undermine the *personal* case — the brief's user has specific hardware, a measured
constraint, and an existing dashboard they want to keep using. It does bear on the open-source
ambition, and it argues for building the personal tool first (decision D10) and deciding about
publication only once it has proved itself in daily use.

---

## 6. Effect on the plan

| Item | Effect |
|---|---|
| **O5** | **Closed.** No adequate prior art. Phase 2 entry condition satisfied. |
| §1 "Why a custom application is justified" | Strengthened on the TUI axis; the screenshot-server rival must be argued explicitly. |
| §6 framework leaning | Weakly reinforced — dashbrew independently chose Go + Bubble Tea. |
| §5 layout engine | Read dashbrew's flex container implementation before writing one. |
| §7 client layer | Read bhdr's WebSocket code as a reference implementation. |
| Phase 8 testing | `hass-cli` is a useful scriptable oracle for asserting HA-side state. |
| New spike | **S9** — measure the current kiosk approach on this Pi (see below). |

### Proposed spike S9

**Question:** what does the existing browser approach actually cost on this hardware?

**Method:** on the Pi 3B, load the primary Lovelace dashboard in Chromium kiosk mode and record
time-to-first-paint, time-to-interactive, steady-state RSS, and CPU during a live ADS-B update
burst.

**Why it is worth an hour.** Every justification in this plan rests on "the browser is too
heavy," which is currently an inference from 905 MiB total / 369 MiB free rather than a
measurement. If the numbers are damning, Phase 2's central question is settled empirically. If
they are merely mediocre, the project's scope should probably shrink. Either outcome is more
useful than the assumption.

---

## 7. Method and limits

**Method.** Web search across GitHub, the Home Assistant community forum, and general sources;
direct inspection of each candidate repository's README, activity and stated architecture.

**Limits, stated so the survey can be challenged rather than trusted:**

- Search-based and English-language only. A project with no SEO presence would be missed.
- HACS was not enumerated directly. HACS distributes frontend cards and integrations rather than
  standalone terminal applications, so the expected yield is low — but it was not checked.
- Repository claims about architecture were taken from READMEs and repository metadata, not from
  reading source. bhdr's WebSocket claim and dashbrew's polling-only claim are both from
  documentation.
- No search of private or self-hosted forges.
- A negative result is inherently weaker than a positive one. "No adequate tool was found" is a
  statement about this search, not a proof of non-existence.
