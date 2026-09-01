# 7-day soak — running

Started **2026-08-31 19:57 CDT** on `control-panel-mini` (Raspberry Pi 3B),
against the live Home Assistant instance. Covers Phase 8 criteria **P6** (RSS
≤ 64 MB, growth < 5 % over the final five days) and **R5** (no crash, no
unbounded growth, no stuck `Stale`), and contributes to **R3** (session churn).

> The daemon belongs on the Proxmox host beside Home Assistant. It is soaking on
> the Pi because that is the machine available — and it is the harsher
> environment: 905 MiB total against a server's, and 2.4 GHz Wi-Fi rather than a
> virtual bridge. A leak that does not appear here will not appear there.

## What is running

| Unit | Purpose |
|---|---|
| `hatty.service` | the daemon, `home` dashboard, 24 h chart window |
| `hatty-soak.service` | samples RSS, threads, fds, restarts every 60 s |
| `hatty-soak-client.service` | attaches a client 2 minutes in every 10 |

The client matters. A soak against an *idle* daemon proves almost nothing: the
session path, the frame sink and the per-session goroutines are where A1's leak
lived, and they only run when something is attached. Roughly 1,000 session
lifecycles over the week.

Installed as system services with `Restart=` and `WantedBy=multi-user.target`,
so the soak survives a reboot rather than quietly ending at one.

## Reviewing it

```bash
ssh control-panel-mini
sudo tail -20 /var/lib/hatty-soak/samples.csv
sudo journalctl -u hatty --since "-24h" | grep -Ei "error|reconnect|panic"
systemctl show hatty -p NRestarts --value      # must stay 0
```

`samples.csv` columns: `ts,rss_kb,threads,fds,restarts,sessions,ha_reconnects`.

## Pass criteria

| | Criterion | Threshold |
|---|---|---|
| **P6** | daemon RSS | ≤ 64 MB, and < 5 % growth across the final five days |
| **R5** | crashes | `NRestarts` = 0 |
| **R5** | goroutine/fd growth | threads and fds flat, not trending |
| **R3** | session churn | ~1,000 lifecycles with no accumulation |
| — | reconnect handling | `ha_reconnects` may be non-zero; each must be followed by a resubscribe |

## Baseline

```
2026-08-31T19:57:01  rss=9,132 kB  threads=9  fds=7  restarts=0
```

**9 MB against a 64 MB budget** — seven times under before the week even
starts. The interesting number is not the level but the slope.

## Already verified in passing

Setting the soak up exercised two Phase 8 criteria directly:

- **E9 / SIGHUP reload.** A key was added to `authorized_keys` and picked up
  with `systemctl reload hatty`; `NRestarts` stayed at 0, so no live session was
  dropped. That is the behaviour the decision claimed, in its production form
  rather than a unit test.
- **Restart recovery.** `systemctl restart hatty` came back, resubscribed and
  re-ran the statistics backfill unattended — one of R1's twenty restarts, and
  the mechanism the other nineteen will exercise.
