#!/usr/bin/env python3
"""
Phase 5 mockup generator.

Draws candidate hatty layouts at an exact character grid so the mockups are
honest about width. Hand-drawn ASCII lies -- it is far too easy to write a
column that does not actually fit. This renders to a fixed grid and asserts
every line is exactly WIDTH columns.

Deliberately block-element only (no Braille): spike S2 has not run, and a
design that works without Braille degrades to nothing if Braille turns out
to be available. See decisions D22/D23.

Output is pasted into README.md in this directory. Regenerate with:
    python3 render_mockups.py
"""

import math

WIDTH, HEIGHT = 100, 30
BLOCKS = " ▁▂▃▄▅▆▇█"


def spark(values, width, lo=None, hi=None):
    """Block-element sparkline. One cell per sample, resampled to width."""
    if not values:
        return " " * width
    lo = min(values) if lo is None else lo
    hi = max(values) if hi is None else hi
    span = (hi - lo) or 1.0
    out = []
    for i in range(width):
        v = values[int(i * len(values) / width)]
        idx = int((v - lo) / span * (len(BLOCKS) - 1))
        out.append(BLOCKS[max(0, min(len(BLOCKS) - 1, idx))])
    return "".join(out)


def bar(value, lo, hi, width, fill="█", empty="░"):
    frac = 0.0 if hi == lo else (value - lo) / (hi - lo)
    frac = max(0.0, min(1.0, frac))
    n = int(round(frac * width))
    return fill * n + empty * (width - n)


def kv(label, value, width):
    """Label left, value right-aligned, padded to width."""
    pad = width - len(label) - len(value)
    return label + " " * max(1, pad) + value


def polar(aircraft, w, h, max_nm=50, rings=5):
    """
    Radar scope. Character cells are ~2:1 tall, so x is scaled 2x to keep
    rings visually circular. aircraft: list of (bearing_deg, dist_nm, mark).
    """
    grid = [[" "] * w for _ in range(h)]
    cx, cy = (w - 1) / 2.0, (h - 1) / 2.0
    ry = min(cy, (w - 1) / 4.0)

    for r in range(1, rings + 1):
        rad = ry * r / rings
        steps = max(24, int(rad * 12))
        for i in range(steps):
            a = 2 * math.pi * i / steps
            x = int(round(cx + rad * 2 * math.sin(a)))
            y = int(round(cy - rad * math.cos(a)))
            if 0 <= x < w and 0 <= y < h and grid[y][x] == " ":
                grid[y][x] = "·"

    for y in range(h):
        if 0 <= int(cx) < w and grid[y][int(cx)] == " ":
            grid[y][int(cx)] = "│" if y != int(cy) else "┼"
    for x in range(w):
        if grid[int(cy)][x] == " ":
            grid[int(cy)][x] = "─"
    grid[int(cy)][int(cx)] = "⌂"

    for brg, dist, mark in aircraft:
        if dist > max_nm:
            continue
        a = math.radians(brg)
        rad = ry * dist / max_nm
        x = int(round(cx + rad * 2 * math.sin(a)))
        y = int(round(cy - rad * math.cos(a)))
        if 0 <= x < w and 0 <= y < h:
            grid[y][x] = mark
    return ["".join(row) for row in grid]


# --------------------------------------------------------------------------
# Sample data -- shapes and magnitudes from the real dashboards.
# --------------------------------------------------------------------------
FEELS = [88, 88.4, 89, 89.6, 90.2, 90.8, 91.1, 90.9, 90.4, 90.1, 89.7, 89.9,
         90.3, 90.7, 91.0, 90.6, 90.2, 89.8, 89.4, 89.9, 90.1, 90.4, 90.0, 89.6]
WIND = [2.1, 3.4, 5.2, 4.8, 6.1, 5.5, 4.2, 3.8, 5.9, 6.3, 5.1, 4.4,
        3.2, 4.9, 6.0, 5.3, 4.1, 3.6, 5.0, 5.8, 5.3, 4.7, 5.28, 5.1]
SCORE = [98, 99, 100, 100, 99, 100, 100, 98, 97, 99, 100, 100,
         99, 100, 100, 100, 99, 98, 100, 100, 100, 99, 100, 100]

AIRCRAFT = [
    ("UAL1234", "A5AC3B", 9.1, 284, 36000, 446, "B738", "US"),
    ("N77148", "AA6FC3", 18.3, 270, 4600, 178, "C172", "US"),
    ("SWA2584", "AC5A4A", 24.7, 312, 34000, 455, "B737", "US"),
    ("AAL691", "AD9EDF", 31.2, 101, 20875, 432, "A321", "US"),
    ("VTM300", "0D0DC4", 36.8, 38, 29150, 408, "C560", "US"),
    ("DAL1508", "A39630", 41.4, 263, 27825, 450, "A320", "US"),
    ("JBU607", "ADB723", 46.6, 280, 36025, 434, "A320", "US"),
    ("N821HP", "AB354F", 48.9, 207, 31400, 411, "PC12", "US"),
]
SCOPE = [(a[3], a[2], "✈") for a in AIRCRAFT]


def panel_weather(w):
    return [
        kv("Feels like", "90.1 °F", w),
        spark(FEELS, w),
        kv("Humidity", "46.68 %", w),
        kv("Wind", "5.28 mph ESE", w),
        kv("Rain today", "0.00 in", w),
        "",
        "WIND 24H   max · mean · min".ljust(w),
        spark([v * 1.4 for v in WIND], w, lo=0, hi=9),
        spark(WIND, w, lo=0, hi=9),
        spark([v * 0.5 for v in WIND], w, lo=0, hi=9),
        "",
        "LIGHTNING".ljust(w),
        kv("Last strike", "4 wk ago", w),
        kv("Strikes today", "0", w),
        "",
        kv("Tempest battery", "2.63 V", w),
        bar(2.63, 2.40, 2.80, w - 6) + "  WARN",
    ]


def panel_interior(w):
    return [
        kv("Awair score", "100", w),
        spark(SCORE, w, lo=90, hi=100),
        kv("Temperature", "68.4 °F", w),
        "",
        kv("CO₂", "436 ppm", w),
        bar(436, 400, 2000, w - 6) + "  GOOD",
        kv("VOC", "117 ppb", w),
        bar(117, 0, 1500, w - 6) + "  GOOD",
        kv("PM2.5", "1 µg/m³", w),
        bar(1, 0, 50, w - 6) + "  GOOD",
        kv("Humidity", "49.3 %", w),
        bar(49.3, 0, 100, w - 6) + "  GOOD",
    ]


def frame(cols, widths, titles, height):
    """Render side-by-side panels as an exact-width box."""
    top = "┌"
    for i, (wd, t) in enumerate(zip(widths, titles)):
        label = f"─ {t} " if t else "─" * 2
        top += (label + "─" * (wd - len(label)))[:wd]
        top += "┬" if i < len(widths) - 1 else "┐"
    lines = [top]
    for r in range(height):
        row = "│"
        for i, wd in enumerate(widths):
            cell = cols[i][r] if r < len(cols[i]) else ""
            row += (" " + cell)[:wd].ljust(wd) + "│"
        lines.append(row)
    return lines


def close(widths):
    out = "└"
    for i, wd in enumerate(widths):
        out += "─" * wd + ("┴" if i < len(widths) - 1 else "┘")
    return out


def statusbar(text_left, text_right, width):
    pad = width - len(text_left) - len(text_right) - 4
    return "│" + " " + text_left + " " * max(1, pad) + text_right + " " + "│"


def layout_a():
    """Faithful three-column -- mirrors the Lovelace structure."""
    w = 32
    widths = [w, w, w]
    radar = [
        "NEAREST  UAL1234  A5AC3B",
        "B738 · United Airlines",
        "",
        kv("Distance", "9.1 nm", w - 1),
        kv("Bearing", "284°", w - 1),
        kv("Altitude", "36,000 ft", w - 1),
        kv("Speed", "446 kt", w - 1),
        kv("Closest approach", "8.4 nm", w - 1),
        kv("Squawk", "1200", w - 1),
        "",
        "FLIGHT     DIST   ALT  BRG",
        "───────────────────────────",
    ] + [
        f"{a[0]:<9}{a[2]:>5.1f} {a[4]/1000:>5.1f}k {a[3]:>3}"
        for a in AIRCRAFT[:5]
    ] + [
        "",
        "8 tracked · 0 drones",
    ]
    cols = [panel_weather(w - 1), panel_interior(w - 1), radar]
    out = frame(cols, widths, ["WEATHER", "INTERIOR", "RADAR"], 26)
    out.append("├" + "─" * w + "┴" + "─" * w + "┴" + "─" * w + "┤")
    out.append(statusbar("hatty · home", "14:23:07  ● HA 3ms  ⚠ 1", WIDTH))
    out.append("└" + "─" * (WIDTH - 2) + "┘")
    return out


def layout_b():
    """Radar-forward -- the polar scope gets real estate."""
    lw, rw = 38, 59
    scope = polar(SCOPE, rw - 1, 15)
    right = ["NEAREST  UAL1234 · B738 · 9.1 nm · brg 284° · 36,000 ft"] + [""] + scope + [""]
    left = [
        kv("Feels like", "90.1 °F", lw - 1),
        spark(FEELS, lw - 1),
        kv("Humidity", "46.68 %", lw - 1),
        kv("Wind", "5.28 mph ESE", lw - 1),
        "",
        kv("Awair score", "100", lw - 1),
        spark(SCORE, lw - 1, lo=90, hi=100),
        kv("Temperature", "68.4 °F", lw - 1),
        kv("CO₂", "436 ppm", lw - 1),
        bar(436, 400, 2000, lw - 7) + "  GOOD",
        kv("PM2.5", "1 µg/m³", lw - 1),
        bar(1, 0, 50, lw - 7) + "  GOOD",
        "",
        kv("Tempest battery", "2.63 V", lw - 1),
        bar(2.63, 2.40, 2.80, lw - 7) + "  WARN",
        "",
        "RECEIVERS".ljust(lw - 1),
        kv("1090  ● up", "412 msg/s", lw - 1),
        kv("978   ● up", "37 msg/s", lw - 1),
        kv("RemoteID ● up", "0 drones", lw - 1),
    ]
    out = frame([left, right], [lw, rw], ["ENVIRONMENT", "AIRSPACE  ·  50 nm  ·  rings 10 nm"], 21)
    out.append("├" + "─" * lw + "┴" + "─" * rw + "┤")
    hdr = " FLIGHT     TYPE   DIST    ALT   SPD  BRG  SQK  OP"
    out.append("│" + hdr.ljust(WIDTH - 2) + "│")
    for a in AIRCRAFT[:4]:
        row = (f" {a[0]:<10}{a[6]:<6}{a[2]:>5.1f} {a[4]:>7,} {a[5]:>5} {a[3]:>4}"
               f" {'1200':>4}  {a[7]}")
        out.append("│" + row.ljust(WIDTH - 2) + "│")
    out.append(statusbar("hatty · radar", "8 tracked · 14:23:07  ● HA 3ms", WIDTH))
    out.append("└" + "─" * (WIDTH - 2) + "┘")
    return out


def layout_c():
    """Dense grid -- btop-style, maximum information per cell."""
    w1, w2, w3 = 24, 24, 48
    scope = polar(SCOPE, w3 - 1, 17)
    c1 = [
        kv("Feels", "90.1°F", w1 - 1),
        spark(FEELS, w1 - 1),
        kv("Humid", "46.7%", w1 - 1),
        kv("Wind", "5.3 ESE", w1 - 1),
        spark(WIND, w1 - 1),
        kv("Rain", "0.00\"", w1 - 1),
        "",
        kv("Lightning", "4wk", w1 - 1),
        kv("Battery", "2.63V", w1 - 1),
        bar(2.63, 2.40, 2.80, w1 - 1),
    ]
    c2 = [
        kv("Score", "100", w2 - 1),
        spark(SCORE, w2 - 1, lo=90, hi=100),
        kv("Temp", "68.4°F", w2 - 1),
        kv("Humid", "49.3%", w2 - 1),
        "",
        kv("CO₂", "436", w2 - 1),
        bar(436, 400, 2000, w2 - 1),
        kv("VOC", "117", w2 - 1),
        bar(117, 0, 1500, w2 - 1),
        kv("PM2.5", "1", w2 - 1),
        bar(1, 0, 50, w2 - 1),
    ]
    out = frame([c1, c2, scope], [w1, w2, w3],
                ["WEATHER", "INTERIOR", "AIRSPACE 50nm"], 17)
    out.append("├" + "─" * w1 + "┴" + "─" * w2 + "┴" + "─" * w3 + "┤")
    hdr = " FLIGHT     TYPE   DIST    ALT   SPD  BRG   CPA  OP   STATUS"
    out.append("│" + hdr.ljust(WIDTH - 2) + "│")
    for a in AIRCRAFT[:8]:
        row = (f" {a[0]:<10}{a[6]:<6}{a[2]:>5.1f} {a[4]:>7,} {a[5]:>5} {a[3]:>4}"
               f" {a[2]*0.9:>5.1f}  {a[7]:<4} {'level':<8}")
        out.append("│" + row.ljust(WIDTH - 2) + "│")
    out.append(statusbar("hatty · radar", "8 tracked · ● HA 3ms · 14:23:07", WIDTH))
    out.append("└" + "─" * (WIDTH - 2) + "┘")
    return out


def check(name, lines):
    bad = [(i, len(l)) for i, l in enumerate(lines) if len(l) != WIDTH]
    status = "OK" if not bad else f"WIDTH ERRORS {bad[:4]}"
    print(f"# {name}: {len(lines)} rows x {WIDTH} cols -- {status}")
    if len(lines) > HEIGHT:
        print(f"#   WARNING: {len(lines)} rows exceeds the {HEIGHT}-row budget")
    print("\n".join(lines))
    print()


if __name__ == "__main__":
    check("LAYOUT A -- faithful three-column", layout_a())
    check("LAYOUT B -- radar-forward", layout_b())
    check("LAYOUT C -- dense grid", layout_c())
