#!/usr/bin/env python3
"""
Phase 5, round five -- the `home` screen, and responsive behaviour.

Everything is parameterised by (width, height) rather than drawn at one
size, because that is the only honest way to design responsive layout: the
rules have to produce the small versions, not be reverse-engineered from
them afterwards. This file is effectively a prose specification of the
layout engine described in plan SS5.

    python3 render_v5.py              # every screen at every breakpoint
    python3 render_v5.py home 80 24   # one screen at one size

Prints real ANSI. Colour roles follow decision D34.
"""

import re
import sys

BOLD, DIM, RESET = "\x1b[1m", "\x1b[2m", "\x1b[0m"
fg = lambda n: f"\x1b[38;5;{n}m"
bg = lambda n: f"\x1b[48;5;{n}m"

C = {
    "chrome": fg(238), "label": fg(245), "value": fg(252), "title": fg(255) + BOLD,
    "muted": fg(240), "ok": fg(71), "warn": fg(179), "alert": fg(167),
    "stale": fg(96), "sel": bg(236),
    "alt0": fg(60), "alt1": fg(67), "alt2": fg(74), "alt3": fg(81),
    "s_min": fg(66), "s_mean": fg(74), "s_max": fg(110),
}
ANSI = re.compile(r"\x1b\[[0-9;]*m")
vis = lambda s: len(ANSI.sub("", s))


def pad(s, w):
    v = vis(s)
    if v <= w:
        return s + " " * (w - v)
    out, n = "", 0                      # ANSI-aware truncation
    i = 0
    while i < len(s) and n < w:
        m = ANSI.match(s, i)
        if m:
            out += m.group(); i = m.end()
        else:
            out += s[i]; n += 1; i += 1
    return out + RESET


def _dot(dx, dy):
    return 0x40 << dx if dy == 3 else (0x01 << dy) << (dx * 3)


class Braille:
    def __init__(s, w, h):
        s.w, s.h = w, h
        s.d = [[0] * (w * 2) for _ in range(h * 4)]

    def set(s, x, y):
        if 0 <= x < s.w * 2 and 0 <= y < s.h * 4:
            s.d[y][x] = 1

    def line(s, x0, y0, x1, y1):
        n = int(max(abs(x1 - x0), abs(y1 - y0))) + 1
        for i in range(n + 1):
            t = i / n
            s.set(int(round(x0 + (x1 - x0) * t)), int(round(y0 + (y1 - y0) * t)))

    def rows(s):
        o = []
        for cy in range(s.h):
            r = ""
            for cx in range(s.w):
                b = 0
                for dx in range(2):
                    for dy in range(4):
                        if s.d[cy * 4 + dy][cx * 2 + dx]:
                            b |= _dot(dx, dy)
                r += chr(0x2800 + b) if b else " "
            o.append(r)
        return o


def plot(series, w, h, lo, hi):
    b = Braille(w, h)
    span = (hi - lo) or 1.0
    pts = [(int(i * (w * 2 - 1) / max(1, len(series) - 1)),
            int((1 - (v - lo) / span) * (h * 4 - 1))) for i, v in enumerate(series)]
    for a, c in zip(pts, pts[1:]):
        b.line(*a, *c)
    return b.rows()


def overlay(layers):
    """Merge coloured braille layers; later layers win a cell."""
    out = []
    for row in zip(*[l[0] for l in layers]):
        s = ""
        for x in range(len(row[0])):
            ch, col = " ", ""
            for i, cell in enumerate(row):
                if cell[x] != " ":
                    ch, col = cell[x], layers[i][1]
            s += (col + ch + RESET) if ch != " " else " "
        out.append(s)
    return out


def bar(v, lo, hi, w, colour):
    f = 0.0 if hi == lo else max(0.0, min(1.0, (v - lo) / (hi - lo)))
    hv = int(round(f * w * 2)); fl, rem = divmod(hv, 2)
    filled = "█" * fl + ("▌" if rem else "")
    return colour + filled + RESET + C["muted"] + "░" * (w - len(filled)) + RESET


DIGITS = {"0": ["███", "█ █", "█ █", "█ █", "███"], "1": ["  █", "  █", "  █", "  █", "  █"],
          "2": ["███", "  █", "███", "█  ", "███"], "3": ["███", "  █", "███", "  █", "███"],
          "4": ["█ █", "█ █", "███", "  █", "  █"], "5": ["███", "█  ", "███", "  █", "███"],
          "6": ["███", "█  ", "███", "█ █", "███"], "7": ["███", "  █", "  █", "  █", "  █"],
          "8": ["███", "█ █", "███", "█ █", "███"], "9": ["███", "█ █", "███", "  █", "███"],
          ".": ["   ", "   ", "   ", "   ", "  █"], "°": ["██ ", "██ ", "   ", "   ", "   "]}


def bignum(t):
    r = ["", "", "", "", ""]
    for ch in t:
        g = DIGITS.get(ch, ["   "] * 5)
        for i in range(5):
            r[i] += g[i] + " "
    return r


# ---------------------------------------------------------------- data
WMIN = [0.0, 0.2, 0.9, 0.4, 1.1, 0.8, 0.3, 0.1, 1.0, 1.4, 0.6, 0.2,
        0.0, 0.5, 1.2, 0.7, 0.3, 0.1, 0.6, 1.1, 0.8, 0.4, 0.9, 0.7]
WMEAN = [2.1, 3.4, 5.2, 4.8, 6.1, 5.5, 4.2, 3.8, 5.9, 6.3, 5.1, 4.4,
         3.2, 4.9, 6.0, 5.3, 4.1, 3.6, 5.0, 5.8, 5.3, 4.7, 5.3, 5.1]
WMAX = [4.8, 6.9, 9.1, 8.2, 10.4, 9.6, 7.7, 7.1, 9.9, 11.2, 8.8, 7.9,
        6.1, 8.4, 10.1, 9.2, 7.4, 6.8, 8.7, 10.0, 9.3, 8.1, 9.4, 8.9]
INSIDE = [("Awair score", 100, 0, 100, "ok", ""), ("Temperature", 68.4, 60, 80, "ok", "°F"),
          ("Humidity", 49.3, 0, 100, "ok", "%"), ("CO₂", 436, 400, 2000, "ok", "ppm"),
          ("VOC", 117, 0, 1500, "ok", "ppb"), ("PM2.5", 1, 0, 50, "ok", "µg/m³")]
AIR = [("UAL1234", "B738", 9.1, 284, 36000, 446, 8.2, "US", "→", "", "ok"),
       ("RCH451", "C17", 12.6, 195, 24000, 402, 11.8, "US", "↓", "MIL", "mil"),
       ("N77148", "C172", 18.3, 270, 4600, 178, 16.5, "US", "↑", "", "ok"),
       ("SWA2584", "B737", 24.7, 312, 34000, 455, 22.2, "US", "→", "", "ok"),
       ("AAL691", "A321", 31.2, 101, 20875, 432, 28.1, "US", "↓", "", "ok"),
       ("VTM300", "C560", 36.8, 38, 29150, 408, 33.1, "US", "→", "LADD", "info"),
       ("DAL1508", "A320", 41.4, 263, 27825, 450, 37.3, "US", "↓", "", "stale"),
       ("JBU607", "A320", 46.6, 280, 36025, 434, 41.9, "US", "→", "", "ok"),
       ("N556GA", "GLF6", 63.1, 340, 41000, 488, 60.4, "US", "↓", "PIA", "info")]

MIN_W, MIN_H = 44, 12


class Frame:
    def __init__(s, w, h, title):
        s.w, s.h, s.inner = w, h, w - 2
        lab = f"─ {title} "
        if vis(lab) > s.inner:
            lab = lab[:s.inner]
        s.lines = [C["chrome"] + "┌" + RESET + C["title"] + lab + RESET
                   + C["chrome"] + "─" * (s.inner - vis(lab)) + "┐" + RESET]

    def row(s, t, rowbg=""):
        body = pad(" " + t, s.inner)
        if rowbg:
            body = body.replace(RESET, RESET + rowbg)
        s.lines.append(C["chrome"] + "│" + RESET + rowbg + body + RESET
                       + C["chrome"] + "│" + RESET)

    def rule(s, title="", colour=None):
        if not title:
            s.lines.append(C["chrome"] + "├" + "─" * s.inner + "┤" + RESET)
        else:
            lab = f"─ {title} "
            s.lines.append(C["chrome"] + "├" + RESET + (colour or C["chrome"]) + lab + RESET
                           + C["chrome"] + "─" * (s.inner - vis(lab)) + "┤" + RESET)

    def fill(s, reserve=0):
        while len(s.lines) < s.h - 1 - reserve:
            s.row("")

    def close(s):
        s.fill()
        s.lines.append(C["chrome"] + "└" + "─" * s.inner + "┘" + RESET)
        return s.lines

    def left(s, reserve=0):
        return s.h - 1 - reserve - len(s.lines)


def too_small(w, h):
    f = Frame(max(w, 24), h, "hatty")
    f.row("")
    f.row(C["alert"] + "  terminal too small" + RESET)
    f.row(C["label"] + f"  need {MIN_W}×{MIN_H}, have {w}×{h}" + RESET)
    f.row("")
    f.row(C["muted"] + "  resize, or use a smaller font" + RESET)
    return f.close()


def strip(f, state, w):
    """The reserved alert line. NEVER collapses -- decision D34."""
    if state == "alert":
        txt = ("  ⚠  MILITARY   RCH451 · 12.6 nm · brg 195°" if w < 80
               else "  ⚠  MILITARY CONTACT   RCH451 · C17 · 12.6 nm · brg 195° · descending")
        body = pad(txt, f.inner)
        f.lines.append(C["chrome"] + "│" + RESET + bg(52) + fg(224) + BOLD + body
                       + RESET + C["chrome"] + "│" + RESET)
    elif state == "warning":
        f.row(C["warn"] + "▲  2 warnings" + RESET + C["label"]
              + ("" if w < 70 else "   battery 2.63 V · 978 receiver degraded") + RESET)
    else:
        f.row(C["ok"] + "●  all nominal" + RESET + C["label"]
              + ("" if w < 70 else "   9 tracked · 0 flagged · last alert 6 d ago") + RESET)


# --------------------------------------------------------- radar screen
COLS = [  # (min width to include, header, cell fn)
    (0,  "  FLIGHT   ", lambda a: f"  {C['value']}{a[0]:<9}{RESET}"),
    (56, "TYPE  ",      lambda a: f"{C['label']}{a[1]:<6}{RESET}"),
    (0,  "  DIST",      lambda a: f"{C['value']}{a[2]:>6.1f}{RESET}"),
    (68, "  RANGE   ",  lambda a: "  " + bar(a[2], 0, 65, 8, C['alert'] if a[10] == 'mil'
                                             else C['ok'] if a[2] < 25 else C['label'])),
    (0,  "      ALT",   lambda a: f"  {_altc(a[4])}{a[4]:>7,}{RESET}"),
    (0,  " ",           lambda a: f" {C['label']}{a[8]}{RESET}"),
    (80, "   SPD",      lambda a: f"  {C['value']}{a[5]:>4}{RESET}"),
    (0,  "  BRG",       lambda a: f"  {C['label']}{a[3]:>3}{RESET}"),
    (88, "    CPA",     lambda a: f"  {C['value']}{a[6]:>5.1f}{RESET}"),
    (96, "  OP  FLAGS", lambda a: f"  {C['muted']}{a[7]:<3}{RESET} " + _flag(a)),
]
_altc = lambda ft: C["alt0"] if ft < 10000 else C["alt1"] if ft < 20000 else \
    C["alt2"] if ft < 30000 else C["alt3"]


def _flag(a):
    if a[10] == "mil":
        return C["alert"] + f"{a[9]:<5}" + RESET
    if a[10] == "stale":
        return C["stale"] + "stale" + RESET
    return C["muted"] + f"{a[9]:<5}" + RESET if a[9] else ""


def radar(w, h, state="nominal"):
    if w < MIN_W or h < MIN_H:
        return too_small(w, h)
    active = [c for c in COLS if w >= c[0]]
    f = Frame(w, h, f"hatty · radar" + ("        AIRSPACE · 9 tracked" if w >= 70 else ""))
    strip(f, state, w)
    f.rule()
    f.row(C["label"] + "".join(c[1] for c in active) + RESET)
    want_detail = h >= 24
    budget = f.left(2) - (1 + (3 if want_detail else 0))
    sel = 1 if state == "alert" else 0
    for i, a in enumerate(AIR[:max(1, budget)]):
        f.row("".join(c[2](a) for c in active), C["sel"] if i == sel else "")
    if want_detail:
        who = AIR[sel]
        f.rule(who[0], C["alert"] if state == "alert" else None)
        d = [("Type", who[1]), ("Squawk", "4231"), ("Vertical", "-1,200 ft/min")]
        if w >= 80:
            f.row("".join(f"{C['label']}{k:<10}{RESET}{C['value']}{v:<20}{RESET}" for k, v in d))
        else:
            for k, v in d[:2]:
                f.row(f"{C['label']}{k:<10}{RESET}{C['value']}{v}{RESET}")
    f.fill(2)
    f.rule()
    keys = "↑↓ select  ⏎ pin  / search" if w >= 70 else "↑↓  ⏎  /"
    f.row(C["ok"] + "1090 ●  978 ●  RID ●" + RESET + C["muted"] + "   " + keys + RESET)
    return f.close()


# ---------------------------------------------------------- home screen
def home(w, h, state="warning"):
    if w < MIN_W or h < MIN_H:
        return too_small(w, h)
    f = Frame(w, h, "hatty · home" + ("        OUTSIDE 90.1 °F · INSIDE 68.4 °F" if w >= 70 else ""))
    strip(f, state, w)
    f.rule()

    hero = h >= 22 and w >= 60
    if hero:
        big = bignum("90.1")
        side = ["", C["label"] + "feels like" + RESET,
                C["value"] + "46.68 % humidity" + RESET,
                C["value"] + "5.28 mph  ESE" + RESET,
                C["label"] + "0.00 in rain today" + RESET]
        for i in range(5):
            f.row(f"  {C['value']}{big[i]}{RESET}   {side[i]}")
    else:
        f.row(f"{C['label']}Feels like{RESET}  {C['value']}90.1 °F{RESET}   "
              f"{C['label']}Wind{RESET} {C['value']}5.3 ESE{RESET}   "
              f"{C['label']}Hum{RESET} {C['value']}46.7 %{RESET}")

    chart_h = 6 if h >= 28 else 4 if h >= 22 else 0
    if chart_h and w >= 60:
        f.rule("WIND · 24 H · min / mean / max")
        cw = f.inner - 2
        lo, hi = 0, max(WMAX)
        f.lines.extend(
            C["chrome"] + "│" + RESET + pad(" " + r, f.inner) + C["chrome"] + "│" + RESET
            for r in overlay([(plot(WMAX, cw, chart_h, lo, hi), C["s_max"]),
                              (plot(WMEAN, cw, chart_h, lo, hi), C["s_mean"]),
                              (plot(WMIN, cw, chart_h, lo, hi), C["s_min"])]))
        f.row(C["s_max"] + "max 11.2" + RESET + "  " + C["s_mean"] + "mean 4.8" + RESET
              + "  " + C["s_min"] + "min 0.0" + RESET + C["label"] + " mph" + RESET)

    f.rule("INSIDE" if w >= 50 else "IN")
    barw = max(6, min(24, w - 46))
    for name, v, lo, hi, st, unit in INSIDE[: max(2, f.left(2))]:
        val = f"{v:g} {unit}".strip()
        if w >= 62:
            f.row(f"{C['label']}{name:<14}{RESET}{C['value']}{val:>12}{RESET}  "
                  + bar(v, lo, hi, barw, C["ok"]) + C["ok"] + "  GOOD" + RESET)
        else:
            f.row(f"{C['label']}{name:<14}{RESET}{C['value']}{val:>10}{RESET}")

    f.fill(2)
    f.rule()
    warn = C["warn"] + "▲ battery 2.63 V" + RESET
    f.row(warn + C["label"] + ("   lightning 4 wk ago" if w >= 60 else "") + RESET)
    return f.close()


SCREENS = {"radar": radar, "home": home}

if __name__ == "__main__":
    if len(sys.argv) == 4:
        print("\n".join(SCREENS[sys.argv[1]](int(sys.argv[2]), int(sys.argv[3]))))
        sys.exit()
    for name in ("home", "radar"):
        for w, hh in ((100, 30), (80, 24), (64, 20), (50, 15)):
            out = SCREENS[name](w, hh, "alert" if name == "radar" else "warning")
            bad = [(i, vis(l)) for i, l in enumerate(out) if vis(l) != w]
            print(f"\n  {C['label']}{name}  {w}×{hh}{RESET}"
                  + (f"  {C['alert']}WIDTH {bad[:2]}{RESET}" if bad else "")
                  + (f"  {C['alert']}HEIGHT {len(out)}{RESET}" if len(out) != hh else ""))
            print("\n".join(out))
    print(f"\n  {C['label']}below minimum  40×10{RESET}")
    print("\n".join(too_small(40, 10)))
