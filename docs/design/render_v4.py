#!/usr/bin/env python3
"""
Phase 5, round four -- colour semantics and alert treatment.

Prints REAL ANSI 256-colour output. Describing colour in markdown is
useless; run this in the terminal you will actually use:

    python3 render_v4.py            # all three states
    python3 render_v4.py alert      # just the alert state

Three states are rendered, because a dashboard's colour design is only
testable across the states it must distinguish:

    nominal   nothing wrong
    warning   tempest battery low, 978 receiver degraded
    alert     military contact inside the alert radius

Design rules applied, in order of importance:

 1. HUES ARE RESERVED. Red / amber / green mean STATE and nothing else.
    Altitude is a continuous category, not a severity, so it gets a
    sequential blue-cyan ramp. The existing Lovelace dashboard colours low
    altitude red, which collides with "something is wrong" -- on a screen
    that is glanced at rather than read, that collision is expensive.

 2. COLOUR NEVER CARRIES MEANING ALONE. Every coloured state also has a
    glyph or a word. Required for 8-colour terminals, for colour-blind
    readability, and because the client's colour depth is not guaranteed.

 3. MUTED, NOT SATURATED. This runs 24/7 at arm's length on a desk.
    Full-intensity red glares; desaturated red still reads as alarming.

 4. ALERTS RESERVE SPACE, THEY DO NOT REFLOW. Decision D26 gives panels a
    collapse-or-reserve choice; the alert strip reserves. A dashboard whose
    geometry jumps when something goes wrong is the "visually unstable"
    failure the brief warns about.
"""

import re
import sys

WIDTH = 100
INNER = WIDTH - 2


def fg(n):
    return f"\x1b[38;5;{n}m"


def bg(n):
    return f"\x1b[48;5;{n}m"


BOLD, DIM, RESET = "\x1b[1m", "\x1b[2m", "\x1b[0m"

# ---------------------------------------------------------------- palette
C = {
    # structure
    "chrome":   fg(238),   # borders, rules
    "label":    fg(245),   # column headers, field names
    "value":    fg(252),   # primary readings
    "title":    fg(255) + BOLD,
    "muted":    fg(240),   # de-emphasised / not applicable
    # state -- RESERVED HUES
    "ok":       fg(71),    # nominal
    "warn":     fg(179),   # attention wanted, not urgent
    "alert":    fg(167),   # urgent
    "alertbg":  bg(52) + fg(224) + BOLD,
    # value conditions (plan SS7)
    "stale":    fg(96),    # our judgement: update overdue
    "unavail":  fg(240),   # HA says unreachable
    # selection
    "sel":      bg(236),
    # altitude -- SEQUENTIAL, deliberately not a traffic light
    "alt0":     fg(60),    # < 10k
    "alt1":     fg(67),    # 10-20k
    "alt2":     fg(74),    # 20-30k
    "alt3":     fg(81),    # >= 30k
}


def alt_colour(ft):
    return C["alt0"] if ft < 10000 else C["alt1"] if ft < 20000 else \
        C["alt2"] if ft < 30000 else C["alt3"]


ANSI = re.compile(r"\x1b\[[0-9;]*m")


def vis(s):
    return len(ANSI.sub("", s))


def pad(s, w):
    return s + " " * max(0, w - vis(s))


def full(t, rowbg=""):
    body = pad(" " + t, INNER)
    if rowbg:
        # every RESET inside the row would drop the highlight; re-arm it
        body = body.replace(RESET, RESET + rowbg)
    return C["chrome"] + "│" + RESET + rowbg + body + RESET + C["chrome"] + "│" + RESET


def rule(title="", colour=None):
    c = colour or C["chrome"]
    if not title:
        return C["chrome"] + "├" + "─" * INNER + "┤" + RESET
    lab = f"─ {title} "
    return (C["chrome"] + "├" + RESET + c + lab + RESET + C["chrome"]
            + "─" * (INNER - vis(lab)) + "┤" + RESET)


def top(title):
    lab = f"─ {title} "
    return (C["chrome"] + "┌" + RESET + C["title"] + lab + RESET
            + C["chrome"] + "─" * (INNER - vis(lab)) + "┐" + RESET)


def bottom():
    return C["chrome"] + "└" + "─" * INNER + "┘" + RESET


def minibar(v, lo, hi, w, colour):
    f = 0.0 if hi == lo else max(0.0, min(1.0, (v - lo) / (hi - lo)))
    halves = int(round(f * w * 2))
    fl, rem = divmod(halves, 2)
    filled = "█" * fl + ("▌" if rem else "")
    return colour + filled + RESET + C["muted"] + "░" * (w - len(filled)) + RESET


# flight, type, dist, brg, alt, spd, cpa, op, vs, flag, condition
AIRCRAFT = [
    ("UAL1234", "B738", 9.1, 284, 36000, 446, 8.2, "US", "→", "", "ok"),
    ("RCH451",  "C17", 12.6, 195, 24000, 402, 11.8, "US", "↓", "MIL", "mil"),
    ("N77148",  "C172", 18.3, 270, 4600, 178, 16.5, "US", "↑", "", "ok"),
    ("SWA2584", "B737", 24.7, 312, 34000, 455, 22.2, "US", "→", "", "ok"),
    ("AAL691",  "A321", 31.2, 101, 20875, 432, 28.1, "US", "↓", "", "ok"),
    ("VTM300",  "C560", 36.8, 38, 29150, 408, 33.1, "US", "→", "LADD", "info"),
    ("DAL1508", "A320", 41.4, 263, 27825, 450, 37.3, "US", "↓", "", "stale"),
    ("JBU607",  "A320", 46.6, 280, 36025, 434, 41.9, "US", "→", "", "ok"),
    ("N556GA",  "GLF6", 63.1, 340, 41000, 488, 60.4, "US", "↓", "PIA", "info"),
]


def header():
    h = ("  FLIGHT     TYPE     DIST    RANGE            ALT    SPD   BRG    CPA  OP  FLAGS")
    return C["label"] + h + RESET


def row(a, sel=False, alerting=False):
    rowbg = C["sel"] if sel else ""
    mark = (C["alert"] + "▸" if alerting else C["value"] + "▸") + RESET if sel else " "

    cond = a[10]
    if cond == "stale":
        name = C["stale"] + f"{a[0]:<10}" + RESET
        suffix = C["stale"] + " stale 47s" + RESET
    elif cond == "mil":
        name = C["alert"] + BOLD + f"{a[0]:<10}" + RESET
        suffix = C["alert"] + f"{a[9]:<5}" + RESET
    elif cond == "info":
        name = C["value"] + f"{a[0]:<10}" + RESET
        suffix = C["muted"] + f"{a[9]:<5}" + RESET
    else:
        name = C["value"] + f"{a[0]:<10}" + RESET
        suffix = ""

    barc = C["alert"] if cond == "mil" else C["ok"] if a[2] < 25 else C["label"]
    s = (f"{mark} {name} {C['label']}{a[1]:<6}{RESET} "
         f"{C['value']}{a[2]:>6.1f}{RESET}  {minibar(a[2], 0, 65, 9, barc)}  "
         f"{alt_colour(a[4])}{a[4]:>7,}{RESET} {C['label']}{a[8]}{RESET}  "
         f"{C['value']}{a[5]:>4}{RESET}   {C['label']}{a[3]:>3}{RESET}  "
         f"{C['value']}{a[6]:>5.1f}{RESET}  {C['muted']}{a[7]:<3}{RESET} {suffix}")
    return full(s, rowbg)


def strip(state):
    """The reserved alert line. Always present -- content changes, geometry never."""
    if state == "alert":
        body = pad("  ⚠  MILITARY CONTACT   RCH451 · C17 · 12.6 nm · brg 195°"
                   " · descending · CPA 11.8 nm", INNER)
        return C["chrome"] + "│" + RESET + C["alertbg"] + body + RESET + C["chrome"] + "│" + RESET
    if state == "warning":
        return full(C["warn"] + "▲  2 warnings" + RESET + C["label"] +
                    "   tempest battery 2.63 V · 978 receiver degraded (3/s)" + RESET)
    return full(C["ok"] + "●  all nominal" + RESET + C["label"] +
                "   9 tracked · 0 drones · 0 flagged · last alert 6 d ago" + RESET)


def status(state):
    if state == "warning":
        rx = (C["ok"] + "1090 ●" + RESET + C["label"] + " 412/s  " + RESET
              + C["warn"] + "978 ▲" + RESET + C["label"] + " 3/s  " + RESET
              + C["ok"] + "RID ●" + RESET)
        extra = C["warn"] + " ▲ battery 2.63 V" + RESET
    elif state == "alert":
        rx = (C["ok"] + "1090 ●" + RESET + C["label"] + " 412/s   978 ● 37/s   RID ● 0" + RESET)
        extra = C["alert"] + "  ⚠ 1 alert" + RESET
    else:
        rx = C["ok"] + "1090 ●  978 ●  RID ●" + RESET + C["label"] + "   412/s · 37/s" + RESET
        extra = ""
    keys = C["muted"] + "↑↓ select  ⏎ pin  f filter  / search" + RESET
    return full(rx + extra + "    " + keys + C["label"] + "  14:23:07" + RESET)


def screen(state):
    sel = 1 if state == "alert" else 0
    out = [top(f"hatty · radar        AIRSPACE · 9 tracked")]
    out.append(strip(state))
    out.append(rule())
    out.append(full(header()))
    for i, a in enumerate(AIRCRAFT):
        out.append(row(a, sel=(i == sel), alerting=(state == "alert" and i == sel)))
    out.append(rule("RCH451" if state == "alert" else "UAL1234",
                    C["alert"] if state == "alert" else None))
    if state == "alert":
        det = [("Type", "C17 Globemaster III"), ("Operator", "US Air Force"),
               ("Squawk", "4231"), ("Vertical", "-1,200 ft/min")]
    else:
        det = [("Type", "B738"), ("Operator", "United Airlines"),
               ("Squawk", "1200"), ("Vertical", "level")]
    for i in range(0, len(det), 2):
        l = f"{C['label']}{det[i][0]:<14}{RESET}{C['value']}{det[i][1]:<32}{RESET}"
        r = f"{C['label']}{det[i+1][0]:<14}{RESET}{C['value']}{det[i+1][1]}{RESET}"
        out.append(full(l + "  " + r))
    out.append(rule())
    out.append(status(state))
    out.append(bottom())
    return out


def swatches():
    rows = [C["title"] + "  PALETTE" + RESET, ""]
    groups = [
        ("structure", ["chrome", "label", "value", "muted"]),
        ("state (reserved hues)", ["ok", "warn", "alert"]),
        ("value conditions", ["stale", "unavail"]),
        ("altitude ramp (sequential, not severity)", ["alt0", "alt1", "alt2", "alt3"]),
    ]
    for gname, keys in groups:
        line = f"  {C['label']}{gname:<42}{RESET}"
        for k in keys:
            line += C[k] + "████ " + RESET + C["muted"] + f"{k:<9}" + RESET
        rows.append(line)
    return rows


if __name__ == "__main__":
    want = sys.argv[1] if len(sys.argv) > 1 else None
    print()
    for line in swatches():
        print(line)
    print()
    for st in (["nominal", "warning", "alert"] if not want else [want]):
        bad = [(i, vis(l)) for i, l in enumerate(screen(st)) if vis(l) != WIDTH]
        print(f"  {C['label']}state: {st}{RESET}"
              + ("" if not bad else f"  WIDTH ERRORS {bad[:3]}"))
        print("\n".join(screen(st)))
        print()
