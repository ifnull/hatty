#!/usr/bin/env python3
"""
Phase 5, round three -- the table IS the design.

The polar scope is descoped (decision D33). That is a simplification, not a
loss: a terminal is genuinely excellent at dense tabular data, and the
airspace feed is tabular. This round treats "a great looking table" as the
actual design problem it is, with gh-dash, k9s and lazygit as references --
all three named in the brief.

What makes a terminal table good, and what is applied here:
  - numerics right-aligned with consistent precision; units in the header
  - minimal chrome: whitespace separates columns, not box-drawing
  - one sort column, marked
  - a selection cursor that reads without colour
  - inline micro-visualisation where it earns its width
  - severity carried by a narrow glyph column, not by shouting

Braille is used only for the trend chart, never for the table, so the table
survives spike S2 failing.
"""

WIDTH = 100
INNER = WIDTH - 2
BLOCKS = " ▁▂▃▄▅▆▇█"


def _dot(dx, dy):
    return 0x40 << dx if dy == 3 else (0x01 << dy) << (dx * 3)


class Braille:
    def __init__(self, w, h):
        self.w, self.h = w, h
        self.dots = [[0] * (w * 2) for _ in range(h * 4)]

    def set(self, x, y):
        if 0 <= x < self.w * 2 and 0 <= y < self.h * 4:
            self.dots[y][x] = 1

    def line(self, x0, y0, x1, y1):
        n = int(max(abs(x1 - x0), abs(y1 - y0))) + 1
        for i in range(n + 1):
            t = i / n
            self.set(int(round(x0 + (x1 - x0) * t)), int(round(y0 + (y1 - y0) * t)))

    def rows(self):
        out = []
        for cy in range(self.h):
            r = ""
            for cx in range(self.w):
                b = 0
                for dx in range(2):
                    for dy in range(4):
                        if self.dots[cy * 4 + dy][cx * 2 + dx]:
                            b |= _dot(dx, dy)
                r += chr(0x2800 + b) if b else " "
            out.append(r)
        return out


def runchart(series, w, h):
    b = Braille(w, h)
    lo, hi = min(series), max(series)
    span = (hi - lo) or 1.0
    pts = [(int(i * (w * 2 - 1) / max(1, len(series) - 1)),
            int((1 - (v - lo) / span) * (h * 4 - 1))) for i, v in enumerate(series)]
    for a, c in zip(pts, pts[1:]):
        b.line(*a, *c)
    return b.rows()


def minibar(v, lo, hi, w):
    """Half-block resolution horizontal bar -- 2x the precision for free."""
    f = 0.0 if hi == lo else max(0.0, min(1.0, (v - lo) / (hi - lo)))
    halves = int(round(f * w * 2))
    full, rem = divmod(halves, 2)
    return ("█" * full + ("▌" if rem else "")).ljust(w, "░")


def full(t):
    return "│" + (" " + t)[:INNER].ljust(INNER) + "│"


def rule(title=""):
    return "├" + (f"─ {title} " + "─" * INNER)[:INNER] + "┤" if title else "├" + "─" * INNER + "┤"


# flight, type, dist_nm, brg, alt, spd, cpa, op, vs, flags
AIRCRAFT = [
    ("UAL1234", "B738", 9.1, 284, 36000, 446, 8.2, "US", "→", ""),
    ("RCH451",  "C17",  12.6, 195, 24000, 402, 11.8, "US", "↓", "MIL"),
    ("N77148",  "C172", 18.3, 270, 4600, 178, 16.5, "US", "↑", ""),
    ("SWA2584", "B737", 24.7, 312, 34000, 455, 22.2, "US", "→", ""),
    ("AAL691",  "A321", 31.2, 101, 20875, 432, 28.1, "US", "↓", ""),
    ("VTM300",  "C560", 36.8, 38, 29150, 408, 33.1, "US", "→", "LADD"),
    ("DAL1508", "A320", 41.4, 263, 27825, 450, 37.3, "US", "↓", ""),
    ("JBU607",  "A320", 46.6, 280, 36025, 434, 41.9, "US", "→", ""),
    ("N821HP",  "PC12", 48.9, 207, 31400, 411, 44.0, "US", "→", ""),
    ("N4412T",  "SR22", 52.3, 155, 8200, 165, 50.1, "US", "↑", ""),
    ("FDX1290", "B763", 58.7, 91, 38000, 470, 55.2, "US", "→", ""),
    ("N556GA",  "GLF6", 63.1, 340, 41000, 488, 60.4, "US", "↓", "PIA"),
]
DIST = [22.4, 20.1, 18.7, 17.2, 15.9, 14.1, 12.8, 11.4, 10.2, 9.6, 9.1,
        9.4, 10.8, 12.6, 14.9, 17.2, 19.8, 22.1, 24.6, 21.9, 18.4, 15.1, 12.2, 9.1]


def header():
    return ("  FLIGHT     TYPE     DIST↑  RANGE            ALT    SPD   BRG    CPA  OP  "
            "FLAGS").ljust(INNER - 1)


def line(a, sel=False):
    mark = "▸" if sel else " "
    return (f"{mark} {a[0]:<10} {a[1]:<6} {a[2]:>6.1f}  {minibar(a[2], 0, 60, 9)}  "
            f"{a[4]:>7,} {a[8]}  {a[5]:>4}   {a[3]:>3}  {a[6]:>5.1f}  {a[7]:<3} {a[9]:<5}")


def table_1():
    """DENSE -- gh-dash language. Maximum rows, minimum chrome."""
    out = ["┌" + ("─ AIRSPACE  ·  12 tracked  ·  0 drones " + "─" * INNER)[:INNER] + "┐"]
    out.append(full("NEAREST  UAL1234 · B738 · United · 9.1 nm brg 284° · 36,000 ft · 446 kt"))
    out.append(rule())
    out.append(full(header()))
    for i, a in enumerate(AIRCRAFT):
        out.append(full(line(a, sel=(i == 0))))
    out.append(rule())
    out.append(full("1090 ● 412/s   978 ● 37/s   RID ● 0        "
                    "⚠ tempest battery 2.63 V        14:23:07  ● HA 3ms"))
    out.append("└" + "─" * INNER + "┘")
    return out


def table_2():
    """MASTER-DETAIL -- k9s language. Table above, selected record below."""
    out = ["┌" + ("─ AIRSPACE  ·  12 tracked " + "─" * INNER)[:INNER] + "┐"]
    out.append(full(header()))
    for i, a in enumerate(AIRCRAFT[:9]):
        out.append(full(line(a, sel=(i == 1))))
    out.append(rule("RCH451"))
    detail = [
        ("Type", "C17 Globemaster III"), ("Operator", "US Air Force"),
        ("Registration", "07-7186"), ("Squawk", "4231"),
        ("Altitude", "24,000 ft  ↓ -1,200 ft/min"), ("Speed", "402 kt"),
        ("Distance", "12.6 nm  ·  bearing 195°"), ("Closest approach", "11.8 nm in 4 min"),
    ]
    for i in range(0, len(detail), 2):
        left = f"{detail[i][0]:<18} {detail[i][1]:<28}"
        right = f"{detail[i+1][0]:<18} {detail[i+1][1]}" if i + 1 < len(detail) else ""
        out.append(full(left + "  " + right))
    out.append(rule())
    out.append(full("↑↓ select   ⏎ pin   f filter   s sort   / search        "
                    "1090 ●  978 ●  RID ●   14:23:07"))
    out.append("└" + "─" * INNER + "┘")
    return out


def table_3():
    """TABLE + TREND -- the one addition a table cannot make on its own."""
    out = ["┌" + ("─ AIRSPACE  ·  12 tracked " + "─" * INNER)[:INNER] + "┐"]
    out.append(full(header()))
    for i, a in enumerate(AIRCRAFT[:11]):
        out.append(full(line(a, sel=(i == 0))))
    out.append(rule("DISTANCE TO NEAREST  ·  60 min"))
    for r in runchart(DIST, INNER - 2, 6):
        out.append(full(r))
    out.append(full(f"cur {DIST[-1]:>5.1f}   min {min(DIST):>5.1f}   max {max(DIST):>5.1f}   "
                    f"avg {sum(DIST)/len(DIST):>5.1f} nm       "
                    "1090 ●  978 ●  RID ●   ⚠ battery 2.63 V"))
    out.append("└" + "─" * INNER + "┘")
    return out


def check(name, lines):
    bad = [(i, len(l)) for i, l in enumerate(lines) if len(l) != WIDTH]
    print(f"# {name}: {len(lines)} rows x {WIDTH} -- {'OK' if not bad else f'ERRORS {bad[:3]}'}")
    print("\n".join(lines))
    print()


if __name__ == "__main__":
    check("TABLE 1 -- dense", table_1())
    check("TABLE 2 -- master/detail", table_2())
    check("TABLE 3 -- table + trend", table_3())
