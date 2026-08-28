# S1 — what HA state updates actually cost

**Questions.**

1. How many bytes per second does the Radar panel's data actually move? This is **risk
   R15** — because `ha-airspace` carries aircraft lists inside *attributes*, a change
   rewrites the whole list, and `subscribe_entities` field-level diffing cannot reach inside
   an attribute value. Payload size, not event frequency, is the cost driver.
2. How much does `subscribe_entities` save over `subscribe_events`? Decision **D6** is
   settled from source, but the magnitude is unmeasured. This absorbs what was spike S3.

**Why it matters.** Non-functional requirements N2 and N3 are deliberately unquantified in
the plan — inventing numbers before measuring is what the brief warns against. This spike
supplies them.

## Run it

```bash
pip install websockets
export HATTY_HA_TOKEN=...          # never pass a token as an argument (decision D15)

cd docs/spikes/S1-event-payload
./measure.py --mode entities --seconds 300 --out entities.json
./measure.py --mode events   --seconds 300 --out events.json
```

Use `--url ws://<host>:8123/api/websocket` if HA is not at `homeassistant.local`.

Find your real entity ids first — the defaults cover `ha-airspace` but not the WeatherFlow
or Awair sensors:

```bash
hass-cli entity list | grep -E 'airspace|weatherflow|awair|tempest'
./measure.py --entities sensor.airspace_nearest_aircraft,sensor.foo,sensor.bar ...
```

Run both modes over comparable periods — ideally with aircraft actually overhead, since a
quiet sky measures nothing interesting. Consider a second pair of runs at a quiet hour to
get the range.

## What the output means

`entities` mode subscribes with a server-side `entity_ids` filter, so everything received is
relevant. `events` mode has no such filter — HA delivers every `state_changed` in the
instance and the client discards most of it. The report's `relevant bytes` line is the
comparison that matters:

```
  bytes                3,882,110  (12,940 B/s)
  relevant bytes       71,432 (1.8%)  →  238 B/s
```

A large gap quantifies decision D6. It is also the number to quote if anyone later proposes
polling.

## What to conclude

- **Bytes/sec in `entities` mode** → the real input to N3, and the basis for the render
  throttle policy (open question O8).
- **p95 and max message size** → whether attribute payloads are as chunky as R15 predicts.
- **Messages/sec** → confirms the `ha-airspace` publish throttle (default 1/s) in practice.
  If it is much higher, check the deployed config; the rate is ours to set.

## Record

Commit `entities.json` and `events.json` alongside a `RESULTS.md`. Raw results, not just
conclusions — a later phase may dispute what was concluded from them.
