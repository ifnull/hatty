#!/usr/bin/env python3
"""
Phase 5, round two -- exploring visual direction rather than data selection.

Round one (render_mockups.py) got the data right and the LOOK wrong: flat
hierarchy, label:value pairs everywhere, tiny bars. It read like a
spreadsheet. What sampler and btop actually do is use SCALE to create
hierarchy -- big numbers for what matters, charts that fill their panel,
colour for state, fewer and larger elements.

This round adds the primitives that make that possible:
  - a Braille canvas (2x4 sub-cell) for smooth curves and a real radar scope
  - a block-digit font for hero numbers
  - runcharts with axes and cur/min/max/avg legends, sampler-style

BRAILLE IS ASSUMED HERE. Spike S2 has not run. If it fails, these degrade
to the block-element versions in round one -- which is why round one was
drawn first. See decisions D22/D23.
"""

import math

WIDTH = 100
BLOCKS = " ▁▂▃▄▅▆▇█"

# Braille dot bit for (dx in 0..1, dy in 0..3), base U+2800.
def _dot(dx, dy):
    return 0x40 << dx if dy == 3 else (0x01 << dy) << (dx * 3)


class Braille:
    """Character grid of w x h cells, each addressable as 2x4 dots."""

    def __init__(self, w, h):
        self.w, self.h = w, h
        self.dots = [[0] * (w * 2) for _ in range(h * 4)]

    def set(self, x, y):
        if 0 <= x < self.w * 2 and 0 <= y < self.h * 4:
            self.dots[y][x] = 1

    def line(self, x0, y0, x1, y1):
        steps = int(max(abs(x1 - x0), abs(y1 - y0))) + 1
        for i in range(steps + 1):
            t = i / steps
            self.set(int(round(x0 + (x1 - x0) * t)), int(round(y0 + (y1 - y0) * t)))

    def circle(self, cx, cy, r):
        steps = max(96, int(r * 24))
        for i in range(steps):
            a = 2 * math.pi * i / steps
            self.set(int(round(cx + r * 2 * math.sin(a))), int(round(cy - r * math.cos(a))))

    def rows(self):
        out = []
        for cy in range(self.h):
            row = ""
            for cx in range(self.w):
                bits = 0
                for dx in range(2):
                    for dy in range(4):
                        if self.dots[cy * 4 + dy][cx * 2 + dx]:
                            bits |= _dot(dx, dy)
                row += chr(0x2800 + bits) if bits else " "
            out.append(row)
        return out


DIGITS = {
    "0": ["███", "█ █", "█ █", "█ █", "███"],
    "1": ["  █", "  █", "  █", "  █", "  █"],
    "2": ["███", "  █", "███", "█  ", "███"],
    "3": ["███", "  █", "███", "  █", "███"],
    "4": ["█ █", "█ █", "███", "  █", "  █"],
    "5": ["███", "█  ", "███", "  █", "███"],
    "6": ["███", "█  ", "███", "█ █", "███"],
    "7": ["███", "  █", "  █", "  █", "  █"],
    "8": ["███", "█ █", "███", "█ █", "███"],
    "9": ["███", "█ █", "███", "  █", "███"],
    ".": ["   ", "   ", "   ", "   ", "  █"],
    " ": ["   ", "   ", "   ", "   ", "   "],
}


def bignum(text):
    rows = ["", "", "", "", ""]
    for ch in text:
        g = DIGITS.get(ch, DIGITS[" "])
        for i in range(5):
            rows[i] += g[i] + " "
    return rows


def runchart(series, w, h, lo=None, hi=None):
    """Braille line chart. series: list of value-lists."""
    b = Braille(w, h)
    allv = [v for s in series for v in s]
    lo = min(allv) if lo is None else lo
    hi = max(allv) if hi is None else hi
    span = (hi - lo) or 1.0
    for s in series:
        pts = []
        for i, v in enumerate(s):
            x = int(i * (w * 2 - 1) / max(1, len(s) - 1))
            y = int((1 - (v - lo) / span) * (h * 4 - 1))
            pts.append((x, y))
        for (x0, y0), (x1, y1) in zip(pts, pts[1:]):
            b.line(x0, y0, x1, y1)
    return b.rows()


def scope(aircraft, w, h, max_nm=50, rings=3):
    """Braille radar scope. aircraft: (bearing_deg, dist_nm) pairs."""
    b = Braille(w, h)
    cx, cy = w, h * 2
    ry = min(cy, w) * 0.92
    for r in range(1, rings + 1):
        b.circle(cx, cy, ry * r / rings)
    rows = b.rows()
    grid = [list(r) for r in rows]
    # cardinal crosshair + home, drawn as characters over the braille
    for y in range(h):
        if grid[y][w // 2] == " ":
            grid[y][w // 2] = "│"
    for x in range(w):
        if grid[h // 2][x] == " ":
            grid[h // 2][x] = "─"
    grid[h // 2][w // 2] = "⌂"
    for brg, dist in aircraft:
        if dist > max_nm:
            continue
        a = math.radians(brg)
        rad = (ry * dist / max_nm) / 4.0
        x = int(round(w / 2 + rad * 2 * math.sin(a)))
        y = int(round(h / 2 - rad * math.cos(a)))
        if 0 <= x < w and 0 <= y < h:
            grid[y][x] = "▲"
    return ["".join(r) for r in grid]


def bar(v, lo, hi, w, fill="█", empty="░"):
    f = 0.0 if hi == lo else max(0.0, min(1.0, (v - lo) / (hi - lo)))
    n = int(round(f * w))
    return fill * n + empty * (w - n)


def kv(label, value, w):
    return label + " " * max(1, w - len(label) - len(value)) + value


DIST = [22.4, 20.1, 18.7, 17.2, 15.9, 14.1, 12.8, 11.4, 10.2, 9.6, 9.1,
        9.4, 10.8, 12.6, 14.9, 17.2, 19.8, 22.1, 24.6, 21.9, 18.4, 15.1, 12.2, 9.1]
COUNT = [4, 5, 5, 6, 7, 7, 6, 8, 9, 8, 8, 7, 6, 6, 7, 8, 9, 9, 8, 8, 7, 8, 8, 8]
FEELS = [88, 88.4, 89, 89.6, 90.2, 90.8, 91.1, 90.9, 90.4, 90.1, 89.7, 89.9,
         90.3, 90.7, 91.0, 90.6, 90.2, 89.8, 89.4, 89.9, 90.1, 90.4, 90.0, 89.6]
WIND = [2.1, 3.4, 5.2, 4.8, 6.1, 5.5, 4.2, 3.8, 5.9, 6.3, 5.1, 4.4,
        3.2, 4.9, 6.0, 5.3, 4.1, 3.6, 5.0, 5.8, 5.3, 4.7, 5.28, 5.1]

AIRCRAFT = [
    ("UAL1234", "B738", 9.1, 284, 36000, 446, "US"),
    ("N77148", "C172", 18.3, 270, 4600, 178, "US"),
    ("SWA2584", "B737", 24.7, 312, 34000, 455, "US"),
    ("AAL691", "A321", 31.2, 101, 20875, 432, "US"),
    ("VTM300", "C560", 36.8, 38, 29150, 408, "US"),
    ("DAL1508", "A320", 41.4, 263, 27825, 450, "US"),
    ("JBU607", "A320", 46.6, 280, 36025, 434, "US"),
    ("N821HP", "PC12", 48.9, 207, 31400, 411, "US"),
]
SCOPE = [(a[3], a[2]) for a in AIRCRAFT]


def hline(l, mid, r, w, title=""):
    lab = f"{mid} {title} " if title else mid * 2
    return l + (lab + mid * w)[:w] + r


def row(cells, widths):
    out = "│"
    for c, w in zip(cells, widths):
        out += (" " + c)[:w].ljust(w) + "│"
    return out


def full(text):
    return "│" + (" " + text)[:WIDTH - 2].ljust(WIDTH - 2) + "│"


def stats(name, s, unit):
    return f"{name:<9} cur {s[-1]:>6.1f}   min {min(s):>6.1f}   max {max(s):>6.1f}   avg {sum(s)/len(s):>6.1f} {unit}"


def direction_1():
    """INSTRUMENT PANEL -- sampler's language. Hero number, big runcharts, no scope."""
    out = [hline("┌", "─", "┐", WIDTH - 2, "NEAREST CONTACT")]
    big = bignum("9.1")
    meta = ["", "UAL1234  ·  B738", "United Airlines", "36,000 ft   446 kt", "brg 284°   CPA 8.4 nm"]
    for i in range(5):
        out.append(full(f"  {big[i]:<26}  {meta[i]}"))
    out.append(hline("├", "─", "┤", WIDTH - 2, "DISTANCE TO NEAREST  ·  60 min"))
    for r in runchart([DIST], WIDTH - 4, 8):
        out.append(full(r))
    out.append(full(stats("nm", DIST, "nm")))
    out.append(hline("├", "─", "┤", WIDTH - 2, "TRAFFIC  ·  60 min"))
    for r in runchart([COUNT], WIDTH - 4, 7):
        out.append(full(r))
    out.append(full(stats("aircraft", COUNT, "  ")))
    out.append(hline("├", "─", "┤", WIDTH - 2, ""))
    out.append(full("1090 ● 412/s    978 ● 37/s    RID ● 0    8 tracked    ⚠ battery 2.63 V"))
    out.append("└" + "─" * (WIDTH - 2) + "┘")
    return out


def direction_2():
    """SCOPE-DOMINANT -- the radar is the product; everything else is chrome."""
    sw, iw = 63, 34
    sc = scope(SCOPE, sw - 1, 19)
    instr = [
        "NEAREST", "", "  UAL1234   B738", "  9.1 nm    brg 284°",
        "  36,000 ft  446 kt", "", "─" * (iw - 2), "",
        kv("Tracked", "8", iw - 2),
        kv("Drones", "0", iw - 2),
        kv("Military", "0", iw - 2),
        kv("Emergency", "0", iw - 2),
        "", "─" * (iw - 2), "",
        kv("Feels like", "90.1 °F", iw - 2),
        kv("Wind", "5.3 mph ESE", iw - 2),
        kv("Awair", "100", iw - 2),
        kv("Battery", "2.63 V ⚠", iw - 2),
    ]
    left_t = ("─ AIRSPACE  ·  50 nm  ·  rings 17 nm " + "─" * sw)[:sw]
    right_t = ("─ STATUS " + "─" * iw)[:iw]
    out = ["┌" + left_t + "┬" + right_t + "┐"]
    for i in range(19):
        out.append(row([sc[i] if i < len(sc) else "", instr[i] if i < len(instr) else ""], [sw, iw]))
    out.append("├" + "─" * sw + "┴" + "─" * iw + "┤")
    out.append(full("FLIGHT     TYPE   DIST     ALT    SPD  BRG   CPA  OP"))
    for a in AIRCRAFT[:6]:
        out.append(full(f"{a[0]:<10} {a[1]:<6} {a[2]:>5.1f} {a[4]:>7,} {a[5]:>6} {a[3]:>4} {a[2]*0.9:>5.1f}  {a[6]}"))
    out.append("└" + "─" * (WIDTH - 2) + "┘")
    return out


def direction_3():
    """SPLIT -- scope and trend side by side, table below. Balances both."""
    sw, cw = 41, 56
    sc = scope(SCOPE, sw - 1, 18)
    ch = runchart([DIST], cw - 2, 9)
    right = ["NEAREST  UAL1234 · B738 · 36,000 ft · 446 kt", ""] + ch + [
        "", stats("distance", DIST, "nm"),
        "", "1090 ● 412/s   978 ● 37/s   RID ● 0   ·   8 tracked"]
    out = [("┌─ SCOPE " + "─" * sw)[:sw + 1] + "┬" + ("─ NEAREST · 60 MIN " + "─" * cw)[:cw] + "┐"]
    for i in range(18):
        out.append(row([sc[i] if i < len(sc) else "", right[i] if i < len(right) else ""], [sw, cw]))
    out.append("├" + "─" * sw + "┴" + "─" * cw + "┤")
    out.append(full("FLIGHT     TYPE   DIST     ALT    SPD  BRG   CPA  OP   ENV"))
    env = ["90.1 °F feels", "5.3 mph ESE", "46.7 % humidity", "Awair 100",
           "68.4 °F inside", "436 ppm CO₂", "⚠ battery 2.63 V", "4 wk since strike"]
    for a, e in zip(AIRCRAFT, env):
        out.append(full(f"{a[0]:<10} {a[1]:<6} {a[2]:>5.1f} {a[4]:>7,} {a[5]:>6} {a[3]:>4} {a[2]*0.9:>5.1f}  {a[6]}   {e}"))
    out.append("└" + "─" * (WIDTH - 2) + "┘")
    return out


def check(name, lines):
    bad = [(i, len(l)) for i, l in enumerate(lines) if len(l) != WIDTH]
    print(f"# {name}: {len(lines)} rows x {WIDTH} -- {'OK' if not bad else f'WIDTH ERRORS {bad[:3]}'}")
    print("\n".join(lines))
    print()


if __name__ == "__main__":
    check("DIRECTION 1 -- instrument panel", direction_1())
    check("DIRECTION 2 -- scope-dominant", direction_2())
    check("DIRECTION 3 -- split", direction_3())
