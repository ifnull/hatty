# Phase 7 (round 2) — Adversarial Review of r2

> **All ten findings are closed in [r3](phase-6-engineering-r3.md).**

Attacks [`phase-6-engineering-r2.md`](phase-6-engineering-r2.md). Round 1 attacked r1 and its
thirteen findings were fixed; **the fixes are new code paths, and new code paths are where bugs
live.** r2 said so itself in its §13.5. This is that review.

Same conflict of interest as round 1: one author for the spec and the attack. Treat as a first
pass.

**Ten findings.** Four are direct bugs *inside* the round-1 fixes — the repairs introduced
defects of their own. One is a security hole that the C2 fix does not actually close.

---

## Scope correction, applied first

The user's decision framing was *"to help frame the dashboard. If it requires adding an
additional service, we can remove it from our MVP and add it to the todos for a fast follow."*

Applying that to r2 §0:

| Decision | Data | MVP? |
|---|---|---|
| Freeze tonight → cover water fixtures | daily forecast `templow`, **Met.no, already in HA** | **yes** — no new service |
| Gusty now → take the flag down | `wind_gust`, live from Tempest | **yes** |
| Windy later → plan ahead | daily forecast `wind_speed` (average, no gust) | **yes, with the caveat stated on screen** |
| *Gusty later* → pre-emptively secure | needs `wind_gust_speed` — **no provider has it** | **no — fast-follow todo** |

Only the last needs a new service. The rest ship. The `decisions` panel stays in MVP; its
trigger set shrinks by one.

---

## Bugs inside the round-1 fixes

### E1. The reaper reaps healthy sessions — and watches the wrong signal

r2 §2 reaps a session whose `lastConsumed` is older than **3 × tick**. At 4 Hz that is
**750 ms**.

A Raspberry Pi 3B on 2.4 GHz Wi-Fi (`environment.md`) routinely stalls longer than 750 ms.
Every such hiccup now kills the SSH session, blanking the dashboard and forcing a reconnect —
which is *worse* than the stale frame it was protecting against. Under normal conditions this
turns a working dashboard into a reconnect loop.

The deeper error is the signal. **Slowness is not death.** The depth-1 replacing sink already
handles a slow consumer correctly, by dropping intermediate frames — that is the whole point of
it. Disconnecting is the wrong response to a condition the design already tolerates.

**Required:** do not reap on render latency. Reap only on *confirmed channel death* — SSH
keepalive failure or a write error. Keep the context cancellation from A1, which is what
prevents the goroutine leak; drop the latency threshold entirely.

### E2. Latest-wins silently discards alerts

The depth-1 replacing sink keeps only the newest frame. For a dashboard of current state that
is correct — an old frame is worthless.

**Alerts are not state, they are events.** If `binary_sensor.airspace_alert_military_close`
goes on and then off between two consumed frames, the operator never sees it. The one thing on
the screen that reserves a permanent line (D34) is the one thing the transport can drop.

**Required:** fix it in the state model, not the transport. The alert strip renders *"active,
or active within the last N seconds"*, so a transient alert persists across dropped frames and
decays visibly rather than vanishing. Latest-wins then stays correct for everything.

### E3. Idempotent `Put` discards statistics corrections and makes reconnect backfill a no-op

r2 §3 specifies `Put` as "idempotent; ignores an older sample for a filled slot." Two failures:

1. **HA revises statistics.** The recorder recomputes; a later fetch for the same bucket can
   legitimately carry a corrected value. First-write-wins discards the correction permanently.
2. **Reconnect re-fetches statistics** (r2 §4). Every bucket is already filled from before the
   disconnect, so **the entire refetch is discarded** — the backfill that exists to repair a
   gap does nothing.

The A3 fix solved ordering and introduced a correctness bug.

**Required:** buckets carry provenance. A statistics write beats a live write for the same
bucket; a newer statistics write beats an older statistics write; a live write only fills an
empty bucket. Then ordering *and* correction both work.

### E4. Bucket size is configured independently of the source that fills it

`Ring.bucket` is per-series configuration; statistics arrive at `period: "5minute"`. Nothing
checks they agree.

- Bucket smaller than the period → four of every five slots stay empty; the chart renders as a
  comb.
- Bucket larger → several statistics samples collide and all but one are discarded.

**Required:** derive the bucket from the backfill source rather than configuring it, or reject
a mismatch at load. A chart that silently renders as a comb is exactly the quiet wrongness this
project keeps trying to eliminate.

---

## Internal inconsistencies

### E5. r2 introduces a second, undocumented addressing grammar

§3 defines bindings as `entity_id[':' path]`. §5's decisions panel then writes:

```toml
when = "forecast.daily[0].templow < 32"
```

`forecast.daily[0].templow` is not an entity id and not a path on one. r2 has two incompatible
addressing schemes and documents only one — so the config loader specified in §5 cannot parse
the example given in §5.

**Required:** pick one. Either the forecast becomes a pseudo-entity addressable by the normal
grammar (`forecast.home:daily[0].templow`), or the grammar gains an explicit forecast namespace
with stated semantics. The former is less machinery.

### E6. `valid_when` can silently disable the thing it guards

```toml
valid_when = "sensor.st_00128663_lightning_last_distance is available"
```

If that referent is not itself in the bound set, it is never subscribed, so its condition is
permanently `Unknown`, so **the guard never passes and the guarded value never renders.**

The failure is silent and looks identical to "the sensor is legitimately unavailable" — which
is precisely the condition it was written to detect. A misconfiguration is indistinguishable
from the real fault.

**Required:** guard referents are auto-added to the subscription set at load, or validation
rejects a referent that is not bound.

### E7. Sort hysteresis buys stability with sort correctness, and r2 does not say so

Bucketing distance to 0.5 nm means two contacts in the same bucket are ordered by `hex` — so
the table can display **12.6 above 12.4** while its header says sorted by distance.

That is a real cost, not a free win. Any hysteresis has it: stability and true ordering are in
tension, and r2 §7 presents the mitigation as though it were only an improvement.

**Required:** state the trade. Then either display the bucketed value so the ordering matches
what is shown, or accept visible out-of-order rows and document it. Worth noting the pressure
to do this at all comes from an **unmeasured** concern (A2, SSH-link bandwidth) — measuring
first may show the plain sort is affordable.

---

## Security

### E8. `Secret` does not protect the path the token actually travels

r2 §12 adds `Secret` with redacting `String`, `GoString` and `MarshalJSON`. It protects `%+v`
dumps. It does not protect the real exposure:

The token is sent to Home Assistant inside a JSON auth frame. Either

- the field is typed `Secret`, `MarshalJSON` redacts it, and **authentication breaks**; or
- the code unwraps to `string` at the call site, and the marshalled bytes carry the live token —
  so any protocol trace of outbound frames logs it.

Protocol tracing is exactly what gets enabled when debugging reconnects (r2 §11 requires
connection-lifecycle logging). The C2 fix closes the tidy path and leaves the likely one open.

**Required:** one explicit unwrap at the single point of use, plus a frame redactor in the trace
path that scrubs `access_token` before any frame is logged. Test it: assert the token string
never appears in captured log output.

### E9. `authorized_keys` is read once at startup

C1 correctly fails closed, but the file is loaded at start. Adding or revoking a key requires a
daemon restart, which drops every session — and there is no revocation story at all for a
compromised key short of that restart.

Minor for a single-user deployment; worth a line in the spec rather than silence, since "fail
closed" implies a security posture that static keys only half deliver.

---

## Safety

### E10. Suppressing a stale-forecast decision is a silent failure

r2 §9: *"decisions depending on [a dropped forecast] are suppressed, not evaluated against
stale data."*

That inverts the project's own rule. If the forecast subscription drops at 18:00 and a freeze is
coming at 02:00, the user gets **nothing** — no warning, no indication that a warning was
withheld. The uncovered water fixtures freeze.

r2 §9's own through-line is *"degrade to a legible status, never to a lie."* Silence is not a
legible status. **A stale freeze warning is far more useful than no freeze warning.**

**Required:** safety-relevant decisions render with their staleness marked —
`freeze tonight · forecast 4 h old` — rather than being withheld. Suppression is acceptable only
for decisions with no safety consequence, and that distinction must be declared per decision,
not applied globally.

---

## What survived

- **The A1 goroutine fix.** The atomic-pointer sink with context cancellation is correct; only
  the reaper's trigger is wrong (E1).
- **B1's ranked elasticity, B2's width budget, D3's immutability invariant, D4's declared
  variables.** I could not break any of them.
- **The verified facts.** The dependency lane, the measured update rates, and the forecast
  subscription were all checked against reality rather than reasoned about, and none moved.
- **D41's flagged-subset table.** Honest about truncation, and the right product besides.

---

## Verdict

The architecture still holds. But **four of the ten findings are defects introduced by round 1's
own repairs** (E1, E2, E3, E4), which is the strongest possible argument for the brief's
insistence on iterated adversarial review — and a warning that a third round is not optional
just because the second one found less.

E3 and E8 are the ones I would fix first: E3 quietly corrupts historical charts and disables
reconnect repair, and E8 leaves the credential exposed on the debugging path most likely to be
used in anger.
