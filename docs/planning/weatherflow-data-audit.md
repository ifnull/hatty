# WeatherFlow / Tempest — data audit

The user reported that the Home Assistant Tempest integration "doesn't work as I would expect
even in HA — rain accumulation data doesn't match what is shown in the Tempest iOS app. Similar
issues with lightning."

This audits HA's actual entity states against the vendor app as ground truth. It matters to
hatty because **a dashboard that confidently renders wrong numbers is worse than one that
renders none**, and because Phase 7 finding A4 already flagged the lightning entities as a
staleness problem — it turns out they are also a *correctness* problem.

Compared 2026-08-31. App screenshot at 11:09 local; HA states pulled at 16:18 UTC ≈ 11:18
local — about nine minutes apart, which explains small drift in the live values.

---

## Comparison

| Metric | Tempest app | Home Assistant | Verdict |
|---|---|---|---|
| Temperature | 85.8 °F | `temperature` 87.836 °F | drift over 9 min — OK |
| Feels like | 91 °F | `feels_like` 94.83 °F | consistent with the above |
| Humidity | 62 % | `humidity` 60.06 % | drift — OK |
| **Battery voltage** | **2.61 V** | `battery_voltage` **2.614 V** | **exact match** |
| Battery state | **GOOD** | `battery` 97.6 % | agree — see below |
| Rain today | 0.00 in | `precipitation` 0.0 in | **match** |
| Rain yesterday | 0.00 in | — | — |
| Pressure | 30.050 inHg | `air_pressure` 28.767 inHg | **not a bug** — see below |
| Wind | ENE **1.8 mph**, gusting 0–4 | `wind_speed` **12.48 mph**, `wind_gust` 6.44, `wind_speed_average` 3.76 | **see below** |
| Lightning last | **3 d ago** | `event…lightning_strike` 2026-08-29T01:21Z (≈2 d) | roughly agrees |
| **Lightning distance** | **23–25 mi** | `lightning_average_distance` **0.0 mi** | **WRONG** |
| Lightning last distance | 23–25 mi | `lightning_last_distance` **unavailable** | **MISSING** |
| Lightning last strike | 3 d | `lightning_last_strike` **unavailable** | **MISSING** |
| Lightning last 3 h | 0 | `lightning_count` 0 | match |

---

## 1. Lightning is genuinely broken — confirmed

Three lightning entities disagree with the vendor:

- `sensor.st_00128663_lightning_last_distance` → **`unavailable`**
- `sensor.st_00128663_lightning_last_strike` → **`unavailable`**
- `sensor.st_00128663_lightning_average_distance` → **`0.0 mi`**

while the app reports a strike three days ago at 23–25 miles.

**`0.0 mi` is the dangerous one.** `unavailable` is honest — the integration is telling us it has
no value. But `0.0` is a *number*, and a dashboard would render it as "average strike distance:
0.0 mi", which reads as *a strike directly overhead*. That is the worst possible failure mode for
this particular sensor.

The `event` entity is the one thing that works: `event.st_00128663_lightning_strike` holds
`2026-08-29T01:21:10Z`, which is a real timestamp roughly matching the app's "3d". This also
confirms the Phase 6 §3 requirement that the state model handle the `event` domain, whose state
is a timestamp rather than a scalar.

**Consequence for hatty:** never bind `lightning_average_distance`. Derive "last strike" from the
`event` entity, and treat the other two as unavailable-by-default. More generally — see §5.

## 2. Pressure is not a bug

28.767 inHg vs 30.050 inHg is a 1.28 inHg gap, which is **station pressure versus sea-level
pressure**, not an error. 1.28 inHg corresponds to roughly 1,200–1,300 ft of elevation, which is
consistent with Texas hill country.

The app displays the sea-level-adjusted figure because that is what people expect from a
barometer. HA's `air_pressure` is the raw station reading. If a dashboard wants the familiar
number it needs the sea-level entity, which is **not present** in this entity set — worth
checking whether the integration exposes it under another name before binding pressure at all.

## 3. Wind: two different measurements, easily confused

`wind_speed` = 12.48 mph while `wind_gust` = 6.44 mph looks impossible — a gust cannot be lower
than the wind. But the timestamps differ (`16:18:17` vs `16:18:04`), and WeatherFlow reports on
two cadences: a **3-second "rapid wind"** observation carrying `wind_speed` and `wind_direction`,
and a **1-minute observation** carrying gust, lull and average.

So `wind_speed` is an instantaneous 3-second sample and `wind_gust` is the previous minute's
maximum. They are not contradictory, they are different questions.

That said, 12.48 mph is well outside the app's "1.8 mph, gusting 0–4", so it is either a genuine
gust in the intervening nine minutes or something is wrong. Unresolved.

**Consequence for hatty:** the design should bind **`wind_speed_average`** (3.76 mph — close to
the app's reading), not `wind_speed`. A dashboard driven by a 3-second sample will flicker
constantly and disagree with every other display of the same data. This also reduces update
volume, which is relevant to R1/A2.

## 4. Rain could not be reproduced

Both HA and the app currently report 0.00 in. The reported discrepancy is not visible with no
rain falling, so it cannot be diagnosed from this snapshot.

One suggestive detail: `event.st_00128663_precipitation_start` fired at **2026-08-30T19:16:55Z**,
yet both HA and the app report 0.00 in for that day. Either a false trigger, or genuine drizzle
below the accumulation resolution. Worth watching during the next rain — and worth capturing
`precipitation` alongside the app reading at the time, since that is the only way to pin it down.

## 5. The battery warning in every mockup may be a false alarm

HA and the app agree exactly on 2.614 V / 2.61 V. But **the vendor calls that state `GOOD`**,
while the existing Lovelace gauge (whose thresholds every hatty mockup has inherited) marks
anything below 2.65 V as yellow.

Those thresholds are the *user's*, not the vendor's, and they are stricter. HA also exposes
`sensor.st_00128663_battery` at **97.6 %**, which is the integration's own health assessment and
agrees with the app.

**Consequence for hatty:** bind the battery *percentage* for state and show voltage as detail, or
raise the voltage thresholds to match the vendor. As drawn, `home` and `radar` both display a
standing warning about a battery the manufacturer considers fine — which is precisely the
cry-wolf failure A4 identified, arriving from a different direction.

---

## 6. What this means for the architecture

This is not a hatty bug and hatty cannot fix it. But it changes two things.

**First, it strengthens D25.** Computation belongs in Home Assistant — and so does *correction*.
If `lightning_average_distance` is wrong, the fix is a template sensor in HA that derives a
trustworthy value, not a special case in hatty's renderer.

**Second, it adds a requirement that Phase 6 does not have:** a binding must be able to declare
that a value is only trustworthy under a condition. Concretely, `lightning_average_distance` is
meaningful only when `lightning_last_distance` is not `unavailable`.

Without that, hatty renders `0.0 mi` — a plausible-looking number that means "lightning
overhead". The `Fault` condition already exists in the Phase 6 state model; what is missing is a
way for configuration to *declare* the guard.

Proposed, for the Phase 6 revision:

```toml
[[panel.field]]
label      = "Last strike"
bind       = "event.st_00128663_lightning_strike"
format     = "relative"

[[panel.field]]
label      = "Distance"
bind       = "sensor.st_00128663_lightning_average_distance"
valid_when = "sensor.st_00128663_lightning_last_distance is available"
```

`valid_when` is a guard, not an expression language — it tests availability of another binding
and nothing more. That keeps D25's line intact while making it impossible to render a known-bad
number as if it were real.

**Recommendation for MVP scope:** drop lightning distance from the `home` screen until the
integration is diagnosed. Keep "last strike", which works. A panel that is honestly absent beats
one that is confidently wrong.
