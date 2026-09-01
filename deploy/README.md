# Deployment

Two machines. The daemon runs beside Home Assistant; the Pi is a terminal
appliance that renders what the daemon draws.

```
Proxmox host                          Raspberry Pi 3B
┌───────────────┐                     ┌──────────────────┐
│ Home Assistant│◄── WebSocket ───────│                  │
│               │    (7.2 KB/s)       │                  │
├───────────────┤                     │  cage            │
│ hatty daemon  │◄── SSH ─────────────│   └ foot         │
│ :2222         │    (582 B/s)        │      └ ssh       │
└───────────────┘                     └──────────────────┘
```

Both measured, not estimated: `docs/spikes/S1-event-payload/RESULTS.md` and
`docs/spikes/P2-bandwidth/RESULTS.md`.

## Daemon

```bash
GOOS=linux GOARCH=arm64 go build -o hatty ./cmd/hatty   # or amd64
sudo install -m 0755 hatty /usr/local/bin/hatty
sudo useradd --system --home /var/lib/hatty --shell /usr/sbin/nologin hatty

sudo install -d -m 0750 -o hatty -g hatty /etc/hatty /etc/hatty/dashboards
sudo install -m 0640 -o hatty -g hatty dashboards/*.toml /etc/hatty/dashboards/

# The token. Never a command-line argument (D15).
printf '%s' 'YOUR_LONG_LIVED_TOKEN' | sudo tee /etc/hatty/ha_token >/dev/null
sudo chmod 0400 /etc/hatty/ha_token && sudo chown hatty:hatty /etc/hatty/ha_token

# The allow-list. The daemon REFUSES TO START without it (C1): wish accepts any
# public key by default, and this service exposes home-relative aircraft
# bearings.
sudo install -m 0640 -o hatty -g hatty ~/.ssh/id_ed25519.pub /etc/hatty/authorized_keys

sudo install -m 0644 deploy/hatty.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now hatty
```

Adding or revoking a key later needs no restart (E9):

```bash
sudo systemctl reload hatty      # SIGHUP
```

## Pi

```bash
sudo apt install foot cage
sudo install -m 0755 deploy/hatty-connect /usr/local/bin/
sudo install -d -m 0755 /etc/hatty
sudo install -m 0644 deploy/connect.conf.example /etc/hatty/connect.conf
sudoedit /etc/hatty/connect.conf          # set HATTY_HOST

# Key for the daemon's allow-list.
ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519 -N ''
cat ~/.ssh/id_ed25519.pub                 # add to /etc/hatty/authorized_keys

sudo install -m 0644 deploy/hatty-kiosk.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now hatty-kiosk
```

To try it without committing to the kiosk:

```bash
hatty-connect radar
```

## Why it is shaped this way

**The Pi runs no application code.** It renders a terminal, which is a rounding
error on four Cortex-A53 cores — and the reason the project exists, since a
browser needs ten seconds to launch on this hardware and stays resident (D24).

**Reattaching is instant.** The daemon holds the Home Assistant connection and
the state, so a dropped SSH session loses a *renderer*, not the state (D7). That
is the browser's ten seconds eliminated by architecture rather than tuning.

**A stalled client is tolerated, a dead one is reaped.** Verified on this
hardware: a stalled client blocks the write for seven seconds with no error
while the store stays at ~300 µs, and recovers cleanly on resume
(`docs/spikes/E1-backpressure/RESULTS.md`). An earlier design would have blanked
the dashboard on every Wi-Fi hiccup.

**Font-per-dashboard is the launcher's job** (D5), because a font is a client
property and the daemon cannot set one. `HATTY_FONT_RADAR` and friends are how
the recorded 80×25 fallback would be applied, with no change to the daemon.
