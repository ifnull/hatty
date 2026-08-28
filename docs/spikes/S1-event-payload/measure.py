#!/usr/bin/env python3
"""
S1 — measure what Home Assistant state updates actually cost.

Answers two questions the Phase 1 plan could only guess at:

  1. How many bytes per second does the Radar panel's data actually move?
     (Risk R15 — since ha-airspace carries aircraft lists inside attributes,
     payload size is the cost driver, not event frequency.)

  2. How much does subscribe_entities save over subscribe_events?
     (Decision D6 — settled from source, but the magnitude is unmeasured.)

Run it twice, once per mode, and compare:

    export HATTY_HA_TOKEN=...                      # never pass as an argument (D15)
    ./measure.py --mode entities --seconds 300 --out entities.json
    ./measure.py --mode events   --seconds 300 --out events.json

Requires:  pip install websockets
"""

import argparse
import asyncio
import json
import os
import statistics
import sys
import time
from collections import Counter

try:
    import websockets
except ImportError:
    sys.exit("missing dependency: pip install websockets")


# The real ha-airspace entity set (see docs/planning/airspace-entity-model.md).
# The flag_* sensors carry an "aircraft" attribute that is a LIST of dicts, so
# any change rewrites the whole list -- risk R15, the reason this spike exists.
# The *_map entities are likely the largest payloads in the instance; they are
# included deliberately so S1 can tell us whether to bind them at all.
DEFAULT_ENTITIES = [
    # ha-airspace -- the payload-size question
    "sensor.airspace_nearest_aircraft",
    "sensor.airspace_nearest_drone",
    "sensor.airspace_aircraft_count",
    "sensor.airspace_drone_count",
    "sensor.airspace_flag_military",
    "sensor.airspace_flag_interesting",
    "sensor.airspace_flag_emergency",
    "sensor.airspace_flag_spoof_suspect",
    "sensor.airspace_aircraft_map",
    "sensor.airspace_drone_map",
    "sensor.airspace_drone_operator",
    "binary_sensor.airspace_alert_military_close",
    # receiver health -- candidate Infrastructure panel
    "binary_sensor.airspace_receiver_rx_1090_status",
    "sensor.airspace_receiver_rx_1090_message_rate",
    "binary_sensor.airspace_receiver_rx_978_status",
    "sensor.airspace_receiver_rx_978_message_rate",
    "binary_sensor.airspace_remote_id_dump3411_status",
    "sensor.airspace_remote_id_dump3411_message_rate",
    # WeatherFlow Tempest
    "sensor.st_00128663_feels_like",
    "sensor.st_00128663_humidity",
    "sensor.st_00128663_wind_speed",
    "sensor.st_00128663_wind_direction",
    "sensor.st_00128663_battery_voltage",
    "sensor.st_00128663_lightning_count",
    "event.st_00128663_lightning_strike",
    # Awair Element
    "sensor.awair_element_128797_score",
    "sensor.awair_element_128797_temperature",
    "sensor.awair_element_128797_carbon_dioxide",
    "sensor.awair_element_128797_pm2_5",
]


class Recorder:
    """Accumulates per-message stats without holding the payloads."""

    def __init__(self, watched):
        self.watched = set(watched)
        self.sizes = []           # bytes of every message received
        self.relevant_sizes = []  # bytes of messages touching a watched entity
        self.by_entity = Counter()
        self.first = None
        self.last = None

    def record(self, raw, entities_touched):
        now = time.monotonic()
        if self.first is None:
            self.first = now
        self.last = now

        n = len(raw)
        self.sizes.append(n)
        hits = self.watched & entities_touched
        if hits:
            self.relevant_sizes.append(n)
            for e in hits:
                self.by_entity[e] += 1

    def summary(self, mode, seconds):
        total = sum(self.sizes)
        relevant = sum(self.relevant_sizes)
        elapsed = (self.last - self.first) if self.first is not None else 0.0
        elapsed = max(elapsed, 1e-9)

        def pct(part, whole):
            return (100.0 * part / whole) if whole else 0.0

        s = {
            "mode": mode,
            "requested_seconds": seconds,
            "observed_seconds": round(elapsed, 2),
            "messages_total": len(self.sizes),
            "messages_relevant": len(self.relevant_sizes),
            "bytes_total": total,
            "bytes_relevant": relevant,
            "bytes_per_second": round(total / elapsed, 1),
            "relevant_bytes_per_second": round(relevant / elapsed, 1),
            "messages_per_second": round(len(self.sizes) / elapsed, 2),
            "percent_messages_relevant": round(pct(len(self.relevant_sizes), len(self.sizes)), 2),
            "percent_bytes_relevant": round(pct(relevant, total), 2),
            "by_entity": dict(self.by_entity.most_common()),
        }
        if self.sizes:
            ordered = sorted(self.sizes)
            s["message_size"] = {
                "min": ordered[0],
                "p50": int(statistics.median(ordered)),
                "p95": ordered[min(int(len(ordered) * 0.95), len(ordered) - 1)],
                "max": ordered[-1],
                "mean": int(statistics.fmean(ordered)),
            }
        return s


def entities_from_message(msg, mode):
    """Which entity ids does this message touch?"""
    ev = msg.get("event")
    if not isinstance(ev, dict):
        return set()

    if mode == "entities":
        # subscribe_entities: {"a": {...}} initial add, {"c": {...}} diffs,
        # {"r": [...]} removals. Keys of a/c are entity ids.
        found = set()
        for key in ("a", "c"):
            block = ev.get(key)
            if isinstance(block, dict):
                found |= set(block.keys())
        removed = ev.get("r")
        if isinstance(removed, list):
            found |= set(removed)
        return found

    # subscribe_events / state_changed
    data = ev.get("data")
    if isinstance(data, dict) and isinstance(data.get("entity_id"), str):
        return {data["entity_id"]}
    return set()


async def run(args):
    token = os.environ.get("HATTY_HA_TOKEN")
    if not token:
        sys.exit("HATTY_HA_TOKEN is not set. Export it; never pass a token as an argument (D15).")

    entities = args.entities or DEFAULT_ENTITIES
    rec = Recorder(entities)

    # D17: ids must be STRICTLY increasing, and reset per connection.
    next_id = 1

    print(f"connecting to {args.url}", file=sys.stderr)
    async with websockets.connect(args.url, max_size=None) as ws:
        hello = json.loads(await ws.recv())
        if hello.get("type") != "auth_required":
            sys.exit(f"unexpected greeting: {hello!r}")

        await ws.send(json.dumps({"type": "auth", "access_token": token}))
        ack = json.loads(await ws.recv())
        if ack.get("type") != "auth_ok":
            sys.exit(f"authentication failed: {ack!r}")
        print(f"authenticated (HA {ack.get('ha_version')})", file=sys.stderr)

        if args.mode == "entities":
            sub = {"id": next_id, "type": "subscribe_entities", "entity_ids": entities}
        else:
            # No server-side entity filter exists for this path -- that is the point.
            sub = {"id": next_id, "type": "subscribe_events", "event_type": "state_changed"}
        next_id += 1

        await ws.send(json.dumps(sub))
        result = json.loads(await ws.recv())
        if not result.get("success", False):
            sys.exit(f"subscription rejected: {result!r}")

        print(f"mode={args.mode}  watching {len(entities)} entities  "
              f"for {args.seconds}s ...", file=sys.stderr)

        deadline = time.monotonic() + args.seconds
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                break
            try:
                raw = await asyncio.wait_for(ws.recv(), timeout=remaining)
            except asyncio.TimeoutError:
                break
            try:
                msg = json.loads(raw)
            except json.JSONDecodeError:
                continue
            if msg.get("type") != "event":
                continue
            rec.record(raw, entities_from_message(msg, args.mode))

    return rec.summary(args.mode, args.seconds)


def report(s):
    print()
    print(f"  mode                 {s['mode']}")
    print(f"  observed             {s['observed_seconds']}s")
    print(f"  messages             {s['messages_total']:,}  "
          f"({s['messages_per_second']}/s)")
    print(f"  bytes                {s['bytes_total']:,}  "
          f"({s['bytes_per_second']:,} B/s)")
    if "message_size" in s:
        m = s["message_size"]
        print(f"  message size         min={m['min']:,}  p50={m['p50']:,}  "
              f"p95={m['p95']:,}  max={m['max']:,}")
    print(f"  relevant messages    {s['messages_relevant']:,} "
          f"({s['percent_messages_relevant']}%)")
    print(f"  relevant bytes       {s['bytes_relevant']:,} "
          f"({s['percent_bytes_relevant']}%)  "
          f"→ {s['relevant_bytes_per_second']:,} B/s")
    if s["by_entity"]:
        print("  updates by entity:")
        for name, count in s["by_entity"].items():
            print(f"      {count:6,}  {name}")
    print()
    if s["mode"] == "events":
        print("  Note: in events mode everything above the 'relevant' lines is data the")
        print("  dashboard receives, parses and discards. Compare against entities mode.")
        print()


def main():
    ap = argparse.ArgumentParser(description="S1 — measure HA state update cost")
    ap.add_argument("--url", default="ws://homeassistant.local:8123/api/websocket")
    ap.add_argument("--mode", choices=["entities", "events"], default="entities")
    ap.add_argument("--seconds", type=int, default=300)
    ap.add_argument("--entities", type=lambda v: [e.strip() for e in v.split(",") if e.strip()])
    ap.add_argument("--out", help="write raw results as JSON (spikes record raw results)")
    args = ap.parse_args()

    summary = asyncio.run(run(args))
    report(summary)

    if args.out:
        with open(args.out, "w", encoding="utf-8") as fh:
            json.dump(summary, fh, indent=2)
        print(f"  raw results → {args.out}\n")


if __name__ == "__main__":
    main()
