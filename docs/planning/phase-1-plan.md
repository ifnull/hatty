# Phase 1 — Initial Planning Package

Status: **draft for review.** Produced from `BRIEF.md` plus requirements gathering
recorded in `docs/environment.md`.

Per the brief, this document exists to make the project *well-defined enough to
criticize intelligently*. It is not an architecture decision record and it is not
implementation-ready. Claims that have not been verified against primary documentation
are marked **[VERIFY]**. Claims that are guesses are marked **[ASSUMPTION]**.
Nothing here should be treated as settled because it is written down.

> **Verification pass, 2026-08-27.** Every Home Assistant protocol claim in §7 has been
> checked against the developer documentation and the `home-assistant/core` source, and
> §7 now carries inline citations. Two claims came back better than assumed and one was
> corrected; the consequences are folded through §2, §4, §10 and the decision log. The
> remaining **[VERIFY]** markers are all in §6 and §8 and concern *framework and library
> behaviour*, not the HA protocol — they resolve through spikes S4 and S8, not through
> reading documentation.

Contents follow the twelve deliverables named in the brief.

---

## 1. Product definition

**hatty is a full-screen terminal dashboard for Home Assistant, designed to be
viewed over SSH on hardware too weak to run a browser.**

It runs on a capable server, holds a live event-driven mirror of Home Assistant state, and
renders a dense, readable, keyboard-navigable dashboard into any terminal that connects to
it. The client does nothing but transport keystrokes and paint cells.

The immediate user is one person with a desk-mounted 7" Raspberry Pi panel, a 981-entity
Home Assistant instance, and a specific set of sensors (WeatherFlow, Awair, ADS-B) that no
one else will have. The design accommodates that without being trapped by it: dashboards are
configuration, not code, so a personal dashboard and a shippable generic one are the same
mechanism with different inputs.

**What it is not:** a Lovelace replacement, a general HA administration tool, or a
browser-avoidance stunt. It is a glanceable status surface for information that benefits
from being permanently visible.

### Why a custom application is justified

This question belongs to Phase 2 and should be pressed hard there. The Phase 1 position:

The obvious alternative is to optimise the browser path — a lighter browser, a kiosk-mode
Lovelace, a trimmed dashboard. That fails on the measured hardware for a structural reason
rather than a tuning reason: a Pi 3B has 905 MiB of RAM with 369 MiB free, and rendering a
981-entity HA frontend requires a full JS runtime, a DOM, and a compositor on the *display
device*. Terminal rendering moves essentially all of that to the server and leaves the Pi
with a workload — decode a byte stream, paint a character grid — that is a rounding error on
four A53 cores.

The weaker part of the argument, which Phase 2 should attack: a static generated image
served to a lightweight viewer, or an existing TUI, might also clear the bar without a
bespoke application.

**The survey has now been done** — see [`prior-art-survey.md`](prior-art-survey.md). No
adequate TUI exists: the only serious attempt (`bhdr`) is archived at 4 stars and is an
entity browser rather than a dashboard, and the strongest generic builder (`dashbrew`) is
polling-only, which is the architecture this project rejects. That closes the TUI half of
the question.

The *image* half remains open and is the stronger challenge.
[`homeassistant-dashboard-server`](https://github.com/petterhj/homeassistant-dashboard-server)
renders Lovelace server-side and ships screenshots to a dumb display, reusing the entire
existing widget ecosystem for free. Phase 2 should argue this on the merits rather than
inherit the brief's preference for a terminal. The survey states the counter-case; it does
not consider it settled.

**Premise refined 2026-08-27, from the user's direct experience.** The earlier framing —
"a browser cannot render this" — was wrong, and the correction matters. Chromium kiosk mode
against the HA kiosk view *has* been tried on this Pi, and once loaded it performs
acceptably. The pain is **startup: ten seconds or more to launch**, plus the general weight
of keeping a browser resident.

That reframes the value proposition more honestly, and more usefully:

> hatty's advantage is not that a terminal renders faster than a browser. It is that a
> terminal session is *already open*. There is no launch.

This is a better argument than the one it replaces, and it is decision-relevant rather than
rhetorical:

- It **strengthens §8 option C**. A daemon holding warm state means attaching costs nothing
  — the dashboard is on screen the instant the Pi reconnects. That is the ten seconds,
  eliminated by architecture.
- It **weakens §8 option A**, which pays a fresh connect-and-load cost on every invocation
  — the same failure mode as the browser, just cheaper.
- It leaves the screenshot-server rival (O10) standing on its own merits, since that
  approach is also instant-on. The discriminators there remain refresh latency,
  interactivity, and keeping Chromium in the stack — not startup.

Spike S9 is **withdrawn**. It was going to measure a premise the user has already lived, and
it could not discriminate between the two remaining architectures anyway.

---

## 2. MVP proposal

**MVP: a read-only, single-dashboard, event-driven terminal renderer for the personal
Radar/Weather/Interior dashboard at 100×30, that survives an HA restart and an SSH
disconnect without human intervention.**

Rationale for that boundary: the value is *daily glanceable use of the panel that already
sits on the desk*. Anything that does not serve a working dashboard on that panel is a
distraction from proving the premise. Control actions, multiple dashboards, and theming are
all cheap to add once the state pipeline and render loop are proven, and all expensive to
design around a pipeline that turns out not to work.

**In the MVP**

- One HA WebSocket connection with authentication, initial state load, and live subscription.
- In-memory state cache for a configured subset of entities.
- One dashboard, defined in configuration, rendering the three-column layout at 100×30.
- Widgets required by that dashboard only: labeled value, threshold bar, sparkline,
  key/value detail table, multi-column data table, relative-time, status indicator.
- Responsive degradation to a defined floor, and a legible refusal below it.
- Reconnect after HA restart and after client disconnect, without restarting the process.
- Token supplied from outside the repository.
- Config loader capable of *N* dashboards, shipping with 1.
- **History backfill** for sparklines and the 24 h wind chart, via
  `recorder/statistics_during_period`. Moved into the MVP by the §7 verification pass —
  it is directly supported and cheap, so shipping without it would be a self-inflicted
  degradation.

**Explicitly deferred past MVP**

- Any state-changing operation (service calls, toggles, scene activation).
- The generic default dashboard, and the second and third personal dashboards.
- Polar radar panel; touch interaction; theming; alert acknowledgement.
- Packaging for other people.

**Deliberate MVP tension, flagged for Phase 2:** building the personal dashboard first
maximises immediate value but tests the configuration model least — a config layer with one
consumer tends to grow assumptions. The counter-proposal Phase 2 should consider is
building the *generic* dashboard first. The Phase 1 recommendation is still the personal
one, because a tool the author does not use daily will not get the scrutiny that finds
the interesting bugs.

---

## 3. Requirements and non-requirements

### Functional requirements (MVP)

| ID | Requirement |
|---|---|
| F1 | Authenticate to Home Assistant over the WebSocket API using a token supplied outside the repository. |
| F2 | Load initial state for configured entities and subscribe to subsequent changes. |
| F3 | Maintain an in-memory state model; never poll full state on a timer to refresh the display. |
| F4 | Render a configured dashboard full-screen, repositioning rather than scrolling. |
| F5 | Redraw only changed regions where the framework permits. |
| F6 | Handle terminal resize, reflowing or degrading to a defined floor. |
| F7 | Reconnect automatically after HA restart or connection loss, with backoff. |
| F8 | Represent unavailable, unknown, and stale entities distinctly from valid values. |
| F9 | Select a dashboard by CLI argument; default when none is given. |
| F10 | Define dashboards declaratively — entity IDs, thresholds, units, labels, layout — with no source edits. |
| F11 | Exit cleanly, restoring terminal state, on signal or keystroke. |

### Non-functional requirements

| ID | Requirement | Target |
|---|---|---|
| N1 | Client-side CPU | Pi does terminal emulation only; no application logic. |
| N2 | Steady-state server CPU | Modest and bounded under ADS-B update load. Number to be set from spike S1. |
| N3 | Terminal bandwidth | Bounded per second under worst-case update rate. Number from spike S5. |
| N4 | Memory | Bounded regardless of uptime; no unbounded history accumulation. |
| N5 | Startup to first paint | Fast enough not to feel like a page load. Target to be set in Phase 3. |
| N6 | Continuous operation | Runs for weeks without restart or degradation. |
| N7 | Failure mode | Degrades to a legible status message; never a frozen stale dashboard presented as live. |

N2, N3 and N5 are deliberately unquantified. Inventing numbers before spikes S1 and S5 would
be exactly the kind of unsupported claim the brief warns about.

### Explicit non-requirements

Carried from the brief and confirmed: replacing the HA web UI; supporting every entity or
card type; rendering images or video; recreating Lovelace; remote administration of HA;
a plugin ecosystem; supporting every terminal emulator; running the application itself on
the Pi.

Added during Phase 1:

- **No geographic map rendering.** The primary dashboard contains one; it will not be
  reproduced. See the polar radar idea in §10.
- **No mouse support** in MVP. Touch is a nice-to-have on the todo list, not a requirement.
- **No multi-user access control.** Single operator.

---

## 4. Assumptions and open questions

### Assumptions

| ID | Assumption | If wrong |
|---|---|---|
| A1 | Terminal cell grid is the only display dimension the app can rely on; font and pixel size are client-side. | Simplifies nothing — this is near-certain. `ws_xpixel` may add an opportunistic hint. |
| A2 | 100×30 is the primary design target. | Derived from layout need + confirmed arm's-length viewing; robust. |
| A3 | The bare Linux console cannot render Braille. | Braille sparklines become available on the console; strictly more options. Spike S2. |
| A4 | HA and the app on one Proxmox host means restarts, not partitions, dominate reconnect design. | Network-partition handling becomes more important than planned. |
| A5 | A long-lived access token is an acceptable credential for a personal deployment. | Phase 4 security review may require something else. |
| A6 | 981 entities is small enough to mirror in memory if attribute payloads are bounded. | State cache needs selective subscription rather than whole-instance mirroring. |
| A7 | The user's three dashboards all target the same display profile. | Inline profiles become genuinely redundant; named profiles get introduced. |

### Open questions

**For the user**

1. **Real `stty size` on the panel.** The 116×32 reading is not trustworthy (see
   `docs/environment.md`). Not blocking; wanted before Phase 5.
2. **Is a "dashboard" one screen or a set of pages?** The brief describes navigating
   between dashboard *pages*; the user describes three top-level *dashboards*. These may be
   one mechanism or two. Affects the configuration model and the keybinding scheme.
   Needed before Phase 6.
3. **What are dashboards two and three?** Only the primary is specified. The generic default
   is understood in outline; the third is unknown.
4. **VM or LXC** for the dashboard instance on Proxmox.
5. **Does the Pi keep X, or move to a bare console?** Not merely an efficiency question —
   it determines the available glyph vocabulary (A3) and whether touch is reachable at all.

**For the project**

6. ~~**Do adequate HA TUIs already exist?**~~ **Closed 2026-08-27** by
   [`prior-art-survey.md`](prior-art-survey.md). They do not. A successor question is now
   open and is more interesting: **is a server-rendered screenshot approach a better fit
   than a TUI?** See §1 and the survey's §4.
7. ~~**Is history backfill required for sparklines at MVP?**~~ **Closed 2026-08-27** by the
   §7 verification pass. `recorder/statistics_during_period` supports `min`/`mean`/`max`
   directly and cheaply, so backfill is in the MVP. The residual question is an *instance*
   question, not a protocol one — which of the WeatherFlow and Awair entities actually have
   long-term statistics recorded. S6 answers it.
8. **Does the app own its SSH transport, or sit behind a login shell?** See §8. The single
   largest architectural fork in this document.
9. **What is the render-throttle policy** under ADS-B update load — coalesce by interval,
   by region, or by entity priority? Cannot be answered before spike S1.

---

## 5. Candidate system architecture

A layered pipeline, one direction, with a single owner for live state.

```text
┌──────────────────────────────────────────────────────────────┐
│ Home Assistant  (Proxmox VM)                                 │
└───────────────┬──────────────────────────────────────────────┘
                │ WebSocket: auth, initial state, subscriptions
                │ REST/WS: history + statistics backfill
                ▼
┌──────────────────────────────────────────────────────────────┐
│ HA CLIENT          connection lifecycle, auth, reconnect,    │
│                    backoff, message IDs, protocol decode     │
└───────────────┬──────────────────────────────────────────────┘
                │ normalised state events
                ▼
┌──────────────────────────────────────────────────────────────┐
│ STATE STORE        in-memory entity mirror, bounded ring      │
│                    buffers for sparkline series, staleness    │
│                    tracking, unavailable/unknown handling     │
└───────────────┬──────────────────────────────────────────────┘
                │ change notifications, scoped to subscribers
                ▼
┌──────────────────────────────────────────────────────────────┐
│ DASHBOARD MODEL    resolved config: panels, widgets, entity   │
│                    bindings, thresholds, units, layout rules  │
└───────────────┬──────────────────────────────────────────────┘
                │ widget view-models
                ▼
┌──────────────────────────────────────────────────────────────┐
│ LAYOUT ENGINE      cell-grid solver; breakpoint selection;    │
│                    per-size overrides; too-small refusal      │
└───────────────┬──────────────────────────────────────────────┘
                │ cell buffer
                ▼
┌──────────────────────────────────────────────────────────────┐
│ RENDERER           diff against previous frame; emit minimal  │
│                    escape sequences; glyph-tier fallback      │
└───────────────┬──────────────────────────────────────────────┘
                │ bytes
                ▼
┌──────────────────────────────────────────────────────────────┐
│ TRANSPORT          SSH pty  (see §8 — three candidate shapes) │
└───────────────┬──────────────────────────────────────────────┘
                ▼
        Raspberry Pi 3B — terminal emulation only
```

Cross-cutting, deliberately not in the flow: **configuration loading** (startup, plus
possible reload), **logging/diagnostics** (must not write to the rendered terminal), and
**credential acquisition** (never from the repository).

### Design commitments implied

- **One owner of live state.** The HA connection and state store are a single unit. Multiple
  rendering clients, if supported, attach to it rather than each opening their own connection.
  This is what makes §8 option C attractive and is the single most consequential shape decision.
- **Widgets are pure over view-models.** A widget receives resolved values and emits cells;
  it does not reach into the state store. Keeps widgets testable without a live HA.
- **Layout is data, not code.** The engine consumes declarative constraints. A new dashboard
  must never require a new code path.
- **Bindings address attribute paths, not just entity states.** Confirmed against the real
  dashboard YAML: `sensor.ads_b_aircraft_1090` and `..._978` each carry an `aircraft`
  attribute that is a *list of plane dicts*. The Radar table iterates a collection nested
  inside one entity's attributes, so the config model needs attribute paths and a table
  widget that iterates a bound collection — not one value per widget.
- **hatty maps values to appearance; Home Assistant derives values.** The line that keeps
  the config model from becoming a programming language. The existing dashboard does list
  merging, filtering, distance computation, sorting and top-N in sixty lines of embedded
  Jinja — that work belongs in HA template sensors, not in hatty config (decision D25).
  hatty owns the bounded, closed set: value→colour band, value→glyph, number formatting,
  relative time. See [`dashboard-source-analysis.md`](dashboard-source-analysis.md).
- **Panels can be conditionally visible.** The real dashboard hides its lightning cards
  unless strikes have occurred. Lovelace reflows; a fixed character grid cannot do so for
  free. Panels must declare whether they *collapse* or *reserve* their space (decision
  D26) — a Phase 5 design call, recorded here so the layout engine is not designed without
  it.
- **Glyph tier is a render-time input.** The same dashboard renders with Braille sparklines
  under X and block sparklines on the console. Widgets declare what they need; the renderer
  substitutes.

### Configuration shape (concepts, not schema)

The brief forbids designing the schema before the UI model exists, so this is deliberately
loose. A dashboard carries:

- **Layout constraints in cells** — `min_cols`/`min_rows` as hard requirements, plus the
  authoring target. Consumed by the layout engine; checkable every session.
- **Optional per-size overrides** — explicit placement at given dimensions, defaulting to
  automatic reflow. Media queries for a character grid.
- **An inlined display profile** — pixel resolution, font, physical context, glyph tier,
  colour depth. Mostly deployment metadata; the glyph tier and colour depth are functional.

Decision: profiles are **inlined per dashboard**, not named and shared. At three dashboards
the redundancy is roughly a dozen lines, and inlining keeps each dashboard file
self-contained and portable. Named profiles remain available as a purely additive change
if redundancy becomes real. Recorded in the decision log.

**Note:** the app cannot set a font. A dashboard declaring `8x16` is describing an intended
deployment, and the only honest response to a mismatch is a warning. Font-per-dashboard is
achievable, but at the launcher layer — see §8.

---

## 6. TUI framework comparison

**No selection is made here.** The brief assigns the decision to Phase 4, on measurable
criteria. This section frames the comparison and identifies what must be measured.

### Python + Textual

**Strengths.** A genuinely high-level widget, layout and event model — the only candidate
where responsive layout is substantially framework-provided rather than hand-computed.
CSS-like styling (TCSS) keeps presentation out of logic. Async-native, which suits a
WebSocket-driven application. Python is Home Assistant's own language, so protocol
examples, JSON handling and datetime/unit manipulation are all well-trodden. Testing
support is a real strength: Textual ships a `Pilot` driver for scripted interaction, plus
snapshot testing — directly useful for the terminal-dimension matrix in Phase 8.

**Weaknesses.** A Python runtime carries tens of megabytes of RSS and a slower cold start;
both are server-side costs here, so they matter less than they would on the Pi, but they
are not free. Deployment means a virtualenv or a bundler rather than a file copy. Textual's
API has moved quickly, which is real dependency risk for something intended to run
untouched for months. Per-frame CPU under a high update rate is a genuine unknown and must
be measured, not assumed.

**Critical uncertainty.** Serving a Textual app over a *programmatically owned* SSH
connection (§8 option C) has no documented, supported path known to this plan. `textual
serve` targets the web. Building on `asyncssh` with a custom driver is plausible but
unproven here. **[VERIFY / spike S4]** — if it fails, Textual is effectively limited to
options A and B.

### Go + Bubble Tea

**Strengths.** A single static binary; deployment is copying one file, with no runtime on
the server. Low memory and near-instant start. The Elm-style Model/Update/View split makes
state transitions pure functions, which is unusually easy to unit-test — attractive given
how much of Phase 7's risk list is state-transition bugs. The renderer diffs line-by-line
and skips unchanged lines, which speaks directly to the SSH-bandwidth requirement **[VERIFY]**.

**The decisive strength for this project** is `charmbracelet/wish`: an SSH server library
that serves Bubble Tea programs directly, handling the pty, window resize and public-key
authentication **[VERIFY]**. That makes §8 option C a first-class, documented path rather
than a research project — and option C is what removes the tmux dependency entirely.

**Weaknesses.** Layout is substantially manual. Lip Gloss composes styled strings and the
developer computes widths; there is no CSS-like layout solver, so the responsive engine in
§5 is more of a build. Unicode width handling is explicit (`go-runewidth`) rather than
automatic — an advantage for correctness, a cost in vigilance. No mature Go client for
Home Assistant is known, so the protocol layer is hand-written; the protocol is small
enough that this is perhaps 300 lines, not a project. Graph and sparkline primitives are
less developed than Python's.

### Lower-level libraries

`tcell`, `notcurses`, `blessed` and similar are not recommended. Neither the layout model
nor the event loop is the interesting part of this project, and building both by hand adds
risk without addressing any measured need. Revisit only if both candidates fail a
measured criterion.

### Decision matrix — to be filled by measurement, not argument

| Criterion | Source of the answer |
|---|---|
| SSH compatibility | S8 (Wish) and S4 (Textual-over-SSH) |
| Server CPU under ADS-B load | S1 feeding a load harness in both |
| Terminal bandwidth per update | S5, measured on both |
| Redraw efficiency | S5 |
| HA WebSocket support | Protocol is simple in both; Python has more prior art |
| Reconnect behaviour | Application-level in both; not a differentiator |
| Responsive layout | Framework-provided (Textual) vs. hand-built (Bubble Tea) |
| Testing support | Pilot + snapshots vs. pure-function unit tests. Both strong, differently |
| Packaging | Static binary vs. venv/bundle |
| Maintainability | Judgement; weigh API churn against hand-built layout |
| Widget primitives | Textual ahead today |
| Unicode/terminal compatibility | S7, both |

### Phase 1 leaning, stated so it can be attacked

**Go + Bubble Tea + Wish**, on the strength of option C being a supported path and of
deployment being a file copy. The reasoning that would overturn it: if spike S4 shows
Textual can be served over SSH cleanly, Textual's layout and testing advantages are
substantial and the responsive engine is the largest single piece of work in §5.

This is a leaning, not a decision. Phase 4 decides.

---

## 7. Home Assistant integration approach

**Verified against primary sources 2026-08-27.** Every protocol claim in this section was
checked against the Home Assistant developer documentation and the `home-assistant/core`
source on the `dev` branch. Sources are cited inline. Claims previously marked **[VERIFY]**
here are now either confirmed or corrected; two came back materially better than assumed.

### Connection and authentication — CONFIRMED

Connect to `/api/websocket`. The handshake is exactly:

```jsonc
// server →
{"type": "auth_required", "ha_version": "2021.5.3"}
// client →
{"type": "auth", "access_token": "ABCDEFGH"}
// server →
{"type": "auth_ok", "ha_version": "2021.5.3"}
// or
{"type": "auth_invalid", "message": "Invalid password"}
```

**Message IDs must be strictly increasing — corrected and now stronger than stated.** The
developer documentation says only that `id` is "an integer that the caller can use to
correlate messages to responses," which would permit any unique value. The source is
stricter. `connection.py` rejects any id less than *or equal to* the highest seen:

```python
if cur_id <= self.last_id:
    self.send_message(
        messages.error_message(
            cur_id, const.ERR_ID_REUSE, "Identifier values have to increase."
        )
    )
    return
```

This is a real constraint on the client design, not a convention: a monotonic counter is
mandatory, and it must reset on reconnect because the server's `last_id` is per-connection.
An implementation that reuses or lowers an id gets `ERR_ID_REUSE` rather than a silent
failure — which is at least a loud failure mode.

*Sources: [WebSocket API docs](https://developers.home-assistant.io/docs/api/websocket/);
`homeassistant/components/websocket_api/connection.py`.*

**Credentials.** A long-lived access token, created in the HA user profile. Two decisions
belong to the Phase 4 security review, not here:

- Whether the token belongs to a **dedicated non-admin HA user**. This reduces blast radius
  if the server or Pi is compromised. HA's non-admin permissions are coarse, so the actual
  reduction must be checked against the entity list rather than assumed. Note that
  `subscribe_entities` performs a per-entity read-permission check, so a non-admin user
  silently receives fewer entities rather than an error — a misconfiguration would look
  like missing data, not a failure.
- Where the token lives. Candidates: an environment variable injected by a systemd unit, a
  `0600` file outside the repository, or a systemd credential. `.gitignore` already excludes
  the obvious filenames, which is a backstop and not a strategy.

The token must never be a CLI argument — it would appear in `ps` output and shell history.

### Subscription strategy — CONFIRMED, and better than assumed

`subscribe_entities` **exists, accepts an entity filter, and sends compressed diffs.** It is
undocumented in the developer documentation — which lists only `subscribe_events` — but it is
defined in `websocket_api/commands.py` with this schema:

```python
{
    vol.Required("type"): "subscribe_entities",
    vol.Optional("entity_ids"): cv.entity_ids,
    **INCLUDE_EXCLUDE_BASE_FILTER_SCHEMA.schema,
}
```

So filtering is available two ways: an explicit `entity_ids` list, and include/exclude
patterns from the shared base filter schema.

**The wire format, from `websocket_api/messages.py` and `homeassistant/const.py`:**

Compressed state keys — `"s"` state, `"a"` attributes, `"c"` context, `"lc"` last-changed,
`"lu"` last-updated.

Event keys — `"a"` add, `"c"` change, `"r"` remove. Within a change, `"+"` carries additions
and modifications, `"-"` carries removals.

The initial message is an add containing the full compressed state for every subscribed
entity:

```jsonc
{"id": 1, "type": "event", "event": {"a": {"<entity_id>": {"s": "...", "a": {...}, "lc": ..., "lu": ...}}}}
```

Subsequent messages are diffs:

```jsonc
{"id": 1, "type": "event", "event": {
  "c": {"<entity_id>": {
    "+": {"s": "72.4", "lu": 1756... },
    "-": {"a": ["removed_attribute_name"]}
  }}}}
```

**This is materially better than the `state_changed` path for this application.** A
`state_changed` event carries complete old *and* new state objects; a `subscribe_entities`
diff carries only the fields that actually changed. For an ADS-B sensor updating one
altitude attribute every few seconds, the difference is a full attribute payload twice over
versus a handful of bytes. Combined with server-side entity filtering, this addresses risk
R1 at the source rather than by throttling downstream.

**Decision D6 is upgraded from provisional to settled:** subscribe to the configured entity
set via `subscribe_entities`, not to the whole instance via `subscribe_events`. Spike S3 no
longer needs to establish *whether* this works — it should now measure *how much* it saves
under real ADS-B load, which folds neatly into S1.

Two implementation notes the source makes visible. Diffs are per-entity and additive, so the
client must merge into its own cached state rather than replace — a client that treats a
`"c"` message as a whole state will silently lose every attribute not in this update. And the
`"-"` removal form means attributes can *disappear*, which a naive merge will never notice.

*Sources: `homeassistant/components/websocket_api/commands.py`,
`.../messages.py`, `homeassistant/const.py`.*

### Initial state — largely unnecessary

`get_states` exists and returns all states in one response. But since `subscribe_entities`
opens with a full compressed snapshot of exactly the subscribed set, **a separate
`get_states` call is redundant** for this design and should be omitted. That removes the
one-to-a-few-megabyte startup transfer for a 981-entity instance entirely: the app never
asks for the 940 entities it does not render.

### History and statistics — CONFIRMED, both paths exist

Both candidate WebSocket paths are real. The 24-hour wind min/mean/max chart is directly
supported.

**`recorder/statistics_during_period`** — pre-aggregated buckets, which is what the wind
chart wants:

```python
{
    vol.Required("type"): "recorder/statistics_during_period",
    vol.Required("start_time"): str,
    vol.Optional("end_time"): str,
    vol.Required("statistic_ids"): vol.All([str], vol.Length(min=1)),
    vol.Required("period"): vol.Any("5minute", "hour", "day", "week", "month", "year"),
    vol.Optional("units"): UNIT_SCHEMA,
    vol.Optional("types"): vol.All(
        [vol.Any("change", "last_reset", "max", "mean", "min", "state", "sum")],
        vol.Coerce(set),
    ),
}
```

`types` accepts exactly `min`, `mean` and `max` — precisely the three series in the Lovelace
wind chart. At `period: "5minute"` a 24-hour window is 288 buckets; at `"hour"`, 24. Either
is trivial to transfer and already aggregated server-side, so the client does no
aggregation at all.

Companion commands: `recorder/list_statistic_ids` (optionally filtered by
`statistic_type: "sum" | "mean"`) and `recorder/get_statistics_metadata` — both useful for
validating at startup that a configured statistic actually exists, rather than rendering an
empty chart.

**`history/stream`** — raw state history that backfills then continues live:

```python
{
    vol.Required("type"): "history/stream",
    vol.Required("start_time"): str,
    vol.Optional("end_time"): str,
    vol.Required("entity_ids"): [str],
    vol.Optional("include_start_time_state", default=True): bool,
    vol.Optional("significant_changes_only", default=True): bool,
    vol.Optional("minimal_response", default=False): bool,
    vol.Optional("no_attributes", default=False): bool,
}
```

It sends historical states first, then transitions to streaming live updates on the same
subscription. `history/history_during_period` is the one-shot equivalent with the same
schema.

**Recommendation.** Use `recorder/statistics_during_period` for the 24-hour wind chart and
any other aggregate series — it is cheaper and exact. Use `history/stream` only where raw
per-change history is genuinely needed. Note that `no_attributes` and `minimal_response`
exist specifically to shrink these payloads and should be set wherever the dashboard does
not bind attributes.

**This removes the MVP degradation proposed earlier.** The Phase 1 draft said sparklines
might have to ship live-accumulating with an empty leading edge if backfill proved
expensive. Backfill is cheap and directly supported, so that fallback is withdrawn and
open question O6 is closed: history backfill is in the MVP.

One caveat that is *not* resolved: statistics only exist for entities the recorder is
configured to keep, and only for sensors with a `state_class`. Whether the WeatherFlow wind
sensors actually have long-term statistics in this instance is an instance question, not a
protocol question — `recorder/list_statistic_ids` answers it directly. Folded into S6, which
shrinks from "does this work" to "which of my entities have statistics."

*Sources: `homeassistant/components/recorder/websocket_api.py`,
`homeassistant/components/history/websocket_api.py`.*

### Service calls — deferred, but shaped now

Out of MVP scope. The verified shape, for the record:

```jsonc
{"id": 24, "type": "call_service", "domain": "light", "service": "turn_on",
 "service_data": {"color_name": "beige"}, "target": {"entity_id": "light.kitchen"},
 "return_response": true}
```

Two constraints recorded so the architecture does not preclude them: state-changing
operations must be visually distinguishable from navigation, and must require a deliberate
confirming action. The brief's "avoid accidental activation" is a design requirement, not a
nicety — a dashboard on a *touchscreen* that also toggles switches has an obvious accident
mode.

### Also available, noted for later

`ping` / `pong` — a documented liveness check, directly useful for detecting a half-open
connection before the state cache goes stale (risk R6). `subscribe_trigger` — server-side
trigger evaluation, which could push alert conditions into HA rather than computing them in
the dashboard. Neither is MVP; both are worth remembering.

### State model

Per bound entity: current state, previous state, last-changed and last-updated timestamps,
the subset of attributes the dashboard binds, and a **bounded ring buffer** for series data.
Bounded is load-bearing — an unbounded series on an ADS-B sensor is the memory-growth failure
in R8.

Three value conditions must be distinct throughout, never collapsed into one "no data" case:
`unavailable` (HA says the entity is not reachable), `unknown` (HA has no value), and
**stale** (the app's own judgement that an update is overdue). The third is the app's
invention and matters most: a frozen dashboard presented as live is the failure mode N7
exists to prevent.

Type discipline: HA states are strings, and four shapes must be handled, not one:
scalar `sensor` states; **`binary_sensor`** on/off with context attributes (how alerts
arrive); **`event` entities**, whose state is a timestamp (this is where "4 weeks ago" on
the lightning panel comes from); and **list-valued attributes**, which is how the aircraft
table arrives. Attribute paths must also address **nested dicts** — `ha-airspace` keys
`bearing_to` and `distance_to` by watchpoint name, so bindings look like `bearing_to.home`. Template sensors in this instance are numerous and
may return non-numeric values into numeric widgets. Every numeric binding needs a declared
expected type and a defined behaviour when the value does not parse — displaying a fault
indicator, never crashing and never silently rendering zero.

### Service calls — deferred, but shaped now

Out of MVP scope. Two constraints recorded so the architecture does not preclude them:
state-changing operations must be visually distinguishable from navigation, and must require
a deliberate confirming action. The brief's "avoid accidental activation" is a design
requirement, not a nicety — a dashboard on a *touchscreen* that also toggles switches has an
obvious accident mode.

---

## 8. SSH and session architecture options

The largest architectural fork in this document.

### Option A — process per invocation

`ssh dashboard-server hatty`. Each connection starts a process, opens its own HA
connection, loads state, renders.

*For:* simplest possible model; no daemon, no session manager; a crash affects one session.
*Against:* full startup cost on every connect, including a multi-megabyte state load;
nothing survives disconnect; two clients mean two HA connections and two mirrors.

### Option B — persistent process inside tmux

One long-running process in a tmux session; the Pi attaches and reattaches.

*For:* survives disconnect; reattach is instant; well-understood operationally; tmux handles
detach/reattach and resize.
*Against:* introduces a dependency whose necessity the brief explicitly asks us to test.
Resize semantics across detach/reattach with differently-sized clients are a known source of
edge cases, and the user intends *deliberate font changes between dashboards*, which makes
resize handling load-bearing rather than incidental.

### Option C — daemon owning state, SSH front-end owning rendering

One process holds the HA connection and state store. Each SSH connection attaches a renderer
view onto that shared state. With `charmbracelet/wish` the process *is* the SSH server, and
no login shell is involved **[VERIFY, spike S8]**.

*For:* this is the shape the architecture already wants. Because state lives in the daemon,
**a disconnect drops a renderer, not the state** — reconnecting re-renders from a warm cache
immediately, with no HA round-trip. That is the benefit people reach for tmux to get, without
tmux. Multiple simultaneous clients at *different terminal sizes* fall out naturally, which
matters given the Pi panel and a desktop SSH session want different layouts. Public-key
authentication is enforced by the app, and a restricted user is achieved by construction
rather than by shell configuration.

*Against:* the app becomes a network service and inherits that responsibility — the security
review in Phase 4 must take it seriously. It constrains the framework choice (see S4). And
it does not survive a *daemon* restart, so a supervisor (systemd) is still required.

### Recommendation

**Option C, contingent on S8 and S4.** It resolves the brief's tmux question directly:
*tmux is operational convenience, not an architectural dependency* — but only if the daemon
owns the state. Under option A, tmux would be the only thing providing session persistence,
and the question would answer the other way.

Fallback ordering if C proves unworkable: **B**, then **A**.

### The launcher, and font-per-dashboard

Whatever transport wins, a thin client-side wrapper on the Pi is worth having. It is the only
layer that can act on a dashboard's declared font, because font is a client-side property and
the app runs on the server:

```bash
hatty-connect radar     # setfont 8x16  → 100×30, then connect, dashboard=radar
hatty-connect glance    # setfont 16x32 → 50×15,  then connect, dashboard=glance
```

Two programs, deliberately named apart: **`hatty`** is the server-side application, and
**`hatty-connect`** is the Pi-side wrapper that sets the console font and opens the session.
They run on different machines and do unrelated jobs; sharing a name would invite confusion
in exactly the situation where it costs most — reading a log or a systemd unit at 2 a.m.

This places font-per-dashboard in the session layer rather than the rendering layer, and it
gives the appliance model a concrete job beyond auto-launch on login. It also creates an edge
case Phase 7 should hunt: a font change between attachments means the terminal resizes across
a reconnect, and the layout engine must handle a *different* grid on reattach, not the one it
last drew.

### Boot and supervision

Recorded from `docs/environment.md`: a Proxmox host reboot takes down HA and the dashboard
simultaneously. The daemon must therefore start cleanly against an HA that is not yet
listening, retry with backoff, and present a legible waiting state rather than exiting. A
systemd unit with restart-on-failure is required regardless of which option wins.

---

## 9. Major technical risks

| ID | Risk | Why it is real here | Mitigation / resolves via |
|---|---|---|---|
| **R1** | ~~ADS-B event storm~~ **Downgraded 2026-08-27 — low.** Update volume is bounded and self-controlled | ADS-B comes from the user's own integration, [`ifnull/ha-airspace`](https://github.com/ifnull/ha-airspace), which deliberately creates **aggregate sensors carrying aircraft lists in attributes** rather than per-aircraft entities, and throttles publishes to a **default 1/s**. The Radar panel therefore binds a handful of entities at ≤1 Hz, not 45+ attributes at an uncontrolled rate. The rate is a configuration value we own | S1 rescoped to reading our own config and confirming the throttle. Residual concern moves to R15 |
| **R15** | **Attribute payload size, not event count** | Because aircraft data lives *inside* an attribute, any change rewrites the whole list. Field-level `subscribe_entities` diffing does not reach inside an attribute value, so each update carries the full aircraft JSON — roughly 1–2 KB at 1 Hz. Comfortable, but it means payload size is the cost driver, not message frequency | Confirm actual payload size in S1; bind only the attributes the dashboard renders |
| **R2** | ~~Console glyph budget~~ **Resolved 2026-08-27 by decision D22.** | The 512-glyph cap is a property of the *framebuffer console*, not of font availability — no installable font escapes it. Resolved by choosing the terminal stack rather than the font: X with no desktop environment gives full Unicode including Braille. The glyph-tier mechanism in the display profile is retained, since a desktop SSH client may still differ | S2 rescoped to confirming coverage under the chosen stack |
| **R3** | **Unicode width errors** — ambiguous-width characters break column alignment | Already present in the intended design: `µg/m³`, `⚠`, arrows, box-drawing. A width error is a corrupted frame, not a cosmetic flaw | S7; explicit width measurement; a width-audit test over the whole glyph set |
| **R4** | **Framework cannot serve option C** | Textual-over-SSH is unproven; the recommended architecture depends on it | S4 and S8 before Phase 4 decides |
| **R5** | **Secret exposure** — token in the repo, in `ps`, in logs, or in a crash dump | Multi-model, multi-reviewer repository; easy to leak by accident | `.gitignore` in place; no token via CLI argument; Phase 4 security review; log redaction |
| **R6** | **Reconnect and stale state** — reconnect appears to succeed while the cache is silently stale | The subtlest failure: a dashboard that looks live and is not | Explicit staleness tracking as a first-class value condition; visible connection indicator; disconnect/restart tests in Phase 8 |
| **R7** | **Resize and detach/reattach bugs** | Compounded by deliberate font changes between dashboards | S8; resize matrix testing; layout floor with legible refusal |
| **R8** | **Unbounded memory growth** | Series buffers on high-frequency sensors over weeks of uptime | Bounded ring buffers by construction; long-running soak test in Phase 8 |
| **R9** | **Type assumptions about HA state** | 46 helpers, mostly template sensors, several with structured attribute payloads | Declared expected types per binding; defined non-parsing behaviour; fault indicator not zero |
| **R10** | **Framework API churn** — **confirmed 2026-08-27, and on the Go side** | Building the S8 harness hit a live module rename: `github.com/charmbracelet/ssh` became `charm.land/ssh` at v0.4.3, while `wish v1.4.7` still depends on the old path, so a clean `go mod tidy` fails outright. The plan had flagged churn as a *Textual* concern; the first instance landed on the Go candidate. Phase 4 must weigh this against both, not one | Pin versions explicitly; treat a clean-checkout build as a CI gate; see `docs/spikes/README.md` |
| **R11** | **Single-host failure** | HA and dashboard share a Proxmox host by design | Accepted for a personal utility; recorded as a decision, not an oversight |
| **R12** | **History backfill unavailable or expensive** | Sparklines and the 24 h wind chart depend on it | S6; documented degradation to live-accumulation only |
| **R13** | **Accidental state changes** | Post-MVP, but the display is a *touchscreen* | Confirmation step; visual distinction; deferred entirely from MVP |
| **R14** | **Config model overfits one dashboard** | MVP ships a single, highly personal dashboard | Build the loader for *N*; write the generic dashboard early in Phase 2/3 as a design check |

---

## 10. Proposed prototypes and spikes

Each is small, measurement-only, and answers a question this plan could otherwise only guess
at. None is an implementation start. Ordered by how much downstream work they unblock.

**Harnesses are written and live in [`docs/spikes/`](../spikes/)** — S1, S4 and S8 are ready
to run, each with a README stating what to check and what to record. Note which machine each
runs on: S1 and S4 do not involve the Pi at all, and neither does S8's server half. The Pi is
needed for S2 (glyph coverage) and for S8's interactive matrix, where reconnecting at a
different grid is the test that matters.

| ID | Question | Method | Unblocks |
|---|---|---|---|
| **S1** | *Rescoped 2026-08-27.* Rate is bounded by our own `ha-airspace` publish throttle (default 1/s). The open question is now **payload bytes per update**, not event frequency | Read the deployed `ha-airspace` config for the actual throttle; subscribe and log bytes per update for the airspace and weather entities over one busy period | R1, R15, N2, N3 |
| **S2** | *Rescoped 2026-08-27.* D22 selects X-without-desktop, so the question is now **does the chosen font cover the full glyph set at 8×16 on the panel** | Run the glyph test from `docs/environment.md` under the chosen terminal + DejaVu Sans Mono; confirm Braille, box-drawing, block elements, arrows, `µ`, `²³`; check rendered width. **Add country-flag emoji** (`ha-airspace` supplies `country_flag`) — regional indicator pairs are double-width and render inconsistently | R2, R3, sparkline technique |
| **S3** | *Rescoped 2026-08-27.* Behaviour is confirmed in source (§7); the open question is now **how much** `subscribe_entities` saves over `state_changed` under real load | Connect both ways against the live instance; compare bytes and message counts for the same entity set. Merge into S1's capture window | R1, N3, N4 |
| **S4** | Can a Textual app be served over a programmatically owned SSH connection? | Attempt `asyncssh` + Textual with a custom driver; timebox hard; go/no-go | R4, §8 option C, the framework decision |
| **S5** | What does a frame actually cost over SSH at 100×30? | Instrument bytes written per update for full vs. diff redraw, under S1's measured load | N3, R1, framework decision |
| **S6** | *Largely closed 2026-08-27.* The dashboard's `statistics-graph` card already renders `mean`/`min`/`max` at `period: 5minute` over 1 day for `sensor.st_00128663_wind_speed`, so statistics demonstrably exist for the entity that needed them. Residual: confirm the same for any *other* series a dashboard wants | `recorder/list_statistic_ids` to enumerate what else is available; measure latency and size | R12, sparkline data sources |
| **S7** | Do the intended glyphs render at the expected width? | Width-audit the full glyph set in the Pi console, the Pi emulator, and a desktop terminal | R3 |
| **S8** | ✅ *Partially answered 2026-08-27.* Wish **does** serve a Bubble Tea app over SSH — the harness builds, listens, accepts a session with no login shell, and renders. Outstanding: real dimensions, live resize, **reconnect at a different grid**, and multiple clients at different sizes | Harness in [`docs/spikes/S8-wish-bubbletea/`](../spikes/S8-wish-bubbletea/); run the interactive matrix in its README from the Pi | R4, R7, R10, §8 option C |
| ~~S9~~ | **Withdrawn 2026-08-27.** Chromium kiosk has already been tried on this Pi: ~10 s to launch, acceptable once loaded. The premise is confirmed by experience, and the measurement could not discriminate between the TUI and screenshot-server options anyway. See §1 | — | — |

**Sequencing note.** S1, S2 and S3 are cheap, measurement-only, and change *scope*
conversations rather than implementation ones. The brief places prototype specification in
Phase 4, but running these three in parallel with Phases 2–3 costs little and prevents the
product and planning reviews from arguing over unmeasured quantities. This is the one
deviation from the brief's sequence that this plan proposes; see §12.

S4 and S8 must both complete before Phase 4 selects a framework.

---

## 11. Proposed repository structure for the planning phase

```text
hatty/
├── BRIEF.md                          # original handoff — never edited
├── README.md                         # what this is, current phase, how to navigate
├── .gitignore                        # secrets backstop (in place)
├── docs/
│   ├── environment.md                # measured facts, with method and date
│   ├── reference/                    # HA screenshots; primary dashboard source of truth
│   ├── planning/
│   │   ├── phase-1-plan.md           # this document
│   │   ├── phase-2-product-review.md # added by each phase, never overwriting the last
│   │   ├── phase-3-tpm-review.md
│   │   └── …
│   ├── decisions/
│   │   └── decision-log.md           # running log: what, why, what would overturn it
│   ├── spikes/
│   │   └── SNN-<slug>.md             # one per spike: question, method, raw result, conclusion
│   └── design/                       # Phase 5 onward: ASCII/Unicode mockups
└── (no source tree yet — deliberately)
```

Three conventions worth adopting now:

**Phases append, they do not overwrite.** Each review produces its own document. The brief
warns against treating prior AI output as authoritative; that is far easier to honour when
the earlier reasoning is still on disk to be argued with rather than silently replaced.

**Spikes record raw results, not just conclusions.** A spike whose numbers are gone cannot be
re-examined when a later phase disputes the conclusion drawn from them.

**The decision log records what would overturn each decision.** A decision with no falsifying
condition is a preference wearing a decision's clothes.

No source tree is proposed. Creating one before Phase 4 selects a framework would prejudge
the selection, and an empty `src/` is an invitation to start.

**Not done, recommended:** this is not yet a git repository. `git init` would give the review
process a history to read, which matters when several models and a human are all editing the
same documents. Left to the user rather than done unasked.

---

## 12. Recommended sequence for the review stages

The brief's eight phases are sound and the ordering is kept. Three refinements are proposed.

| Phase | Stage | Entry condition | Exit condition |
|---|---|---|---|
| 1 | Initial planning | — | **this document** |
| — | **Prior-art survey** | Phase 1 drafted | ✅ **Done 2026-08-27** — [`prior-art-survey.md`](prior-art-survey.md) |
| — | **Spikes S1–S3** *(proposed addition)* | Phase 1 drafted | Payload sizes, glyph coverage under the chosen stack, subscription saving |
| 2 | CEO / product review | Phase 1 + survey + S1–S3 | Revised scope; MVP confirmed or replaced |
| 3 | Technical PM review | Phase 2 | Work breakdown, milestones, acceptance criteria, risk register |
| — | **Spikes S4–S8** *(brief places these in Phase 4)* | Phase 3 | Framework go/no-go evidence |
| 4 | Engineering / architecture review | Phase 3 + all spikes | **Framework selected**; §8 option chosen; auth model decided |
| 5 | Design review | Phase 4 | Competing ASCII mockups; one selected; responsive breakpoints fixed |
| 6 | Detailed engineering review | Phase 5 | Implementation-ready specs |
| 7 | Adversarial review | Phase 6 | Documented attempts to break it, with outcomes |
| 8 | QA / test review | Phase 7 | Validation strategy; measurable MVP acceptance criteria |

### The three refinements

**1. Pull S1–S3 ahead of Phase 2.** Product review will otherwise argue about update rates
and glyph availability without data. Three cheap measurements make that conversation
factual. This is the one real deviation from the brief's sequence.

**2. ~~Add a prior-art survey as a Phase 2 entry condition.~~ Done 2026-08-27.** See
[`prior-art-survey.md`](prior-art-survey.md). It closed the TUI question and opened a better
one — whether a server-rendered *image* beats a terminal — and it added spike S9, which
measures the premise the project rests on.

**3. Fix the real terminal dimensions before Phase 5.** Not before Phase 4 — the target
of 100×30 is derived from layout requirements and does not depend on the measurement. But
responsive breakpoints do, and Phase 5 is where they get fixed.

### What Phase 2 should attack hardest

Offered because a review that only receives the author's framing tends to ratify it:

- **The screenshot-server alternative.** The prior-art survey found no adequate TUI but did
  find a credible rival that reuses all of Lovelace's widgets for free. This is now the
  strongest form of "is a custom application justified?" and the plan should have to defend
  against it explicitly.
- **Whether "no launch time" is enough.** With the premise refined (§1), the case now rests
  on instant-on and always-resident rather than on rendering speed. Phase 2 should ask
  whether that alone justifies a bespoke application, given the screenshot-server option
  delivers instant-on too.
- **The MVP choice.** §2 proposes building the *personal* dashboard first and admits this
  tests the configuration model least. The counter-case is real.
- **Whether option C is over-engineering.** A daemon with an SSH front-end is a network
  service for a single user with one panel on a desk. Option A plus tmux is materially
  simpler, and the plan should have to defend the difference.
- **Whether the 24 h chart and history backfill belong in the MVP at all**, given they may
  drag the recorder/statistics API onto the critical path for one widget.

### What this plan is least confident about

Stated plainly, since the brief asks for uncertainty to be surfaced rather than resolved by
guesswork:

1. ~~Every `subscribe_entities`, history and statistics claim in §7 is from working
   knowledge.~~ **Resolved 2026-08-27** — §7 is verified against primary sources and cited.
   Residual protocol risk is low. Note that `subscribe_entities` is *undocumented* in the
   developer docs and was confirmed only from source, so it carries a stability risk that a
   documented API would not: it could change without a documentation deprecation. Worth
   Phase 4 weighing, and worth an integration test that fails loudly if the wire format
   shifts.
2. The Textual-over-SSH assessment (§6, R4) is an absence of knowledge, not evidence of
   impossibility. S4 may well succeed.
3. No numbers are attached to N2, N3 or N5. That is deliberate, but it means the
   non-functional requirements are not yet testable.
4. The framework leaning in §6 rests substantially on one library (`wish`) whose behaviour
   has not been verified here.
