# R2 — reboot recovery

Phase 8 criterion: *survives Pi reboot, daemon-host reboot, and simultaneous
reboot (D13) unattended.*

Run 2026-08-31 on `control-panel-mini`.

## Coverage

The daemon and the client are the same machine in this deployment, so **one Pi
reboot exercises all three cases at once** — and the simultaneous case is the
hardest of them, since nothing is left standing to paper over a missing
dependency.

Not covered: a **Home Assistant host** reboot. Restarting the HA *service* is
R1, which is done; rebooting the machine it runs on needs access to that host,
which this session does not have. D13's real scenario — the Proxmox host taking
Home Assistant and the daemon down together — is therefore still untested in its
exact form, and should be checked once the daemon lives there.

## Result — PASS

```
Pi reachable after reboot            23 s
boot → serving a backfilled frame    24 s
human actions required               0
```

Every service returned on its own:

| Unit | After reboot |
|---|---|
| `hatty.service` | active |
| `hatty-soak.service` | active |
| `hatty-soak-client.service` | active |
| `NRestarts` | 0 |

Startup sequence, unattended:

```
hatty starting
ssh: listening
ha: connected
ha: statistics requested
ha: subscribed
ha: statistics backfilled
```

And a client attached to a real frame — **9 numeric readings, 0 unavailable
indicators** — so bindings were genuinely `Valid`, not merely present:

```
┌─ hatty · home ─────────────────────────────────────────────
│ ●  nothing to do · conditions nominal
├─ outside ──────────────────────────────────────────────────
│ Feels like       88.7        Humidity         47
│ Wind             1.2         Gusting          2.2
│ Rain today       0.00        Low tonight      86
│ High today       87          Tempest battery  99
```

## Why the frame check is trustworthy here, unlike in R1

R1's frame check was weak because it sampled ~10 s after Home Assistant answered
its API — while HA was still repopulating entities — so an indicator meant
"HA is not ready", not "hatty failed".

Here HA was **up throughout**. A frame with zero indicators therefore proves what
it claims: hatty reconnected, resubscribed, backfilled, and resolved every
binding to a real value, entirely on its own.

## Note for the soak record

This reboot resets the soak's RSS baseline, so the trend in `samples.csv` has a
discontinuity at 21:46. A `# REBOOT for R2` marker was written into the CSV at
that point, so a later reading of the series does not mistake the reset for a
memory drop.
