# S2 — Results

Run 2026-08-30 on the Raspberry Pi 3B, on the 800×480 DSI panel.

## Environment

```
terminal: foot   size: 24 80   colours: 256
font:     DejaVu Sans Mono:size=9, fullscreen
compositor: labwc (Wayland)
```

## The font ladder — all measured `LOCAL` on the panel, foot fullscreen

| Grid | Cell (px) | Char width | Design fit |
|---|---|---|---|
| 133 × 36 | 6.0 × 13.3 | 1.17 mm | all 9 table columns |
| 114 × 34 | 7.0 × 14.1 | 1.36 mm | all 9 table columns |
| **100 × 32** | **8.0 × 15.0** | **1.55 mm** | **all 9 table columns — meets D1 with 2 rows spare** |
| 80 × 25 | 10.0 × 19.2 | 1.94 mm | drops `OP`, `FLAGS`, `CPA` (D36) |

For comparison, LXTerminal windowed on the desktop gave **80 × 22** — foot fullscreen at the
same apparent text size gives 80 × 25, and three more rows at no readability cost is the whole
argument for D22.

**The 100×30 target of decision D1 is confirmed achievable**, at roughly a size-8 font, with two
rows to spare for the elastic panel (D37). Which grid to *adopt* depends on readability at the
user's actual seating distance — a judgement no measurement settles.

> **Note on the `session: SSH` warning.** It is a false positive. `foot` was launched from
> inside an SSH shell and inherited `SSH_CONNECTION`, but the script runs inside
> `foot -e bash -c …`, so `stty size` reports *foot's* pty — which is the panel. `stty size`
> always reports the terminal being rendered into, which is the number that matters. The
> check has been softened accordingly.

## Verdict: PASS — everything renders, everything is single-width

Alignment verified arithmetically rather than by eye: the ruler's marker sits at character
index 32, and **every test row's marker also sits at index 32**.

| Row | Glyphs | Marker index | Result |
|---|---|---|---|
| box drawing `─│┌┐└┘├┤┬┴┼` | 11 | 32 | aligned |
| block elements `▁▂▃▄▅▆▇█` | 8 | 32 | aligned |
| shading `░▒▓█` | 4 | 32 | aligned |
| half blocks `▌▐▀▄` | 4 | 32 | aligned |
| arrows `↑↓→←` | 4 | 32 | aligned |
| status glyphs `▸▲●⚠` | 4 | 32 | aligned |
| degree, percent `°%` | 2 | 32 | aligned |
| **braille** `⠁⠂⠄⡀⢀⣀⣤⣶⣿⠿⡇⢸` | 12 | 32 | **aligned** |
| micro, superscripts `µ ² ³` | 5 | 32 | aligned |
| dot, em dash, times `· — ×` | 5 | 32 | aligned |

Colour: 256 confirmed, D34 palette swatches render as distinct hues.

## What this settles

- **Braille is available.** Trend charts on `home` and the optional runchart on `radar` render
  at full 2×4 sub-cell resolution. No fallback to block elements needed. This was the last open
  design dependency.
- **`µg/m³` is safe.** The planned substitution to `ug/m3` is not needed.
- **The full design glyph set is usable**, including the half-block bars that give the `RANGE`
  column double precision.

## The caveat that matters

Most of these glyphs have East Asian Width property **`Ambiguous`** — box drawing, block
elements, shading, half blocks, arrows, `▲ ● ° ² ³ · — ×`. Ambiguous means *the terminal
decides*: single-width in a Western locale, double-width in a CJK one, and it varies between
terminal emulators.

Braille (U+2800–28FF) is `Neutral`, and `µ` is `Narrow` — those two are safe anywhere.
Everything else is safe **in this combination and not provably elsewhere**.

So the terminal is now part of the contract, not just the font. See decision D39.
