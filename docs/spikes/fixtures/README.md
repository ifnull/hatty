# Test fixtures

Recorded from the live Home Assistant instance. Phase 8's L3 tests replay these, so protocol,
diff-merge, reconnect and backfill tests run against **real traffic shapes** rather than a
hand-written mock that would only encode what the author already believed.

| File | Contents |
|---|---|
| `frames-busy.jsonl` | 120 `subscribe_entities` frames, ~167 KB, aircraft overhead |
| `record.py` | the capture tool |
| `sanitize.py` | the scrubber that makes a capture publishable |

## Why sanitisation is not optional

A raw capture contains, per aircraft, `distance_nm`, `bearing_deg`, `hex` and `flight`. Against
public ADS-B history those four **triangulate the watchpoint precisely** — the home address, in
effect. Decision D27 keeps location data out of this repository, and a raw trace violates it
just as surely as pasting coordinates would.

`sanitize.py` therefore:

- rotates every bearing by a fixed constant, and scales every distance by a fixed factor —
  so geometry is *self-consistent but displaced*;
- pseudonymises `hex`, `flight`, `registration`, `ownop` and `track_id` via a salted hash, so
  the same aircraft stays the same aircraft across frames;
- drops `lat`, `lon`, `operator_lat`, `operator_lon` and `seen_by` outright;
- rebases timestamps to a relative `dt`.

**What it deliberately preserves**, because the tests depend on it: frame count, inter-frame
timing, payload sizes, `+`/`-` diff structure, list lengths and truncation, value conditions
(`unavailable`/`unknown`), and every type shape including the nested watchpoint dicts and
list-valued attributes.

So the fixture is realistic in every dimension the code cares about and useless for locating
anyone.

## Regenerating

```bash
# on a host that can reach HA, with HATTY_HA_TOKEN exported
python3 record.py 300 /tmp/raw.jsonl          # 5 minutes
python3 sanitize.py /tmp/raw.jsonl frames-busy.jsonl 120
```

**Never commit the raw capture.** `.gitignore` excludes `*-raw.jsonl` and `hatty-fixtures/`, but
that is a backstop, not the rule.

## Still needed

`frames-quiet.jsonl` (empty sky), `frames-malformed.jsonl` (hand-derived: wrong types, `null`
where a number belongs, an attribute list that becomes a scalar), `stats-24h.json`,
`forecast-daily.json` with a sub-freezing `templow`, and `config-invalid/*.toml` — one per
validation rule. See `phase-8-qa.md` §2.
