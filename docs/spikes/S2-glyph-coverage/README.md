# S2 — glyph coverage and width

**Question.** Does the Pi's terminal render every glyph the design uses, and does each occupy
exactly one cell?

**Run it on the Pi**, on the 800×480 panel, in the terminal you intend to use:

```bash
bash check.sh
```

Run it twice if X-vs-console is still open (decision D22): once in a terminal emulator under X,
once on a bare tty (`Ctrl+Alt+F1`).

## Two things are tested, and the second matters more

**Coverage** — does the glyph render, or is it tofu? Obvious and survivable.

**Width** — does it occupy one cell? *Not* obvious, and it corrupts an entire frame rather than
one character. This is risk R3, and it is why every row ends with a `|` that must line up with
the ruler.

The script pads with explicit **cell counts**, not `printf %-60s`, because `printf` pads by bytes
and multi-byte UTF-8 would shift the marker even for a correctly single-width glyph. A shifted
marker therefore means a genuinely double-width glyph, not a measuring artefact.

## What the result changes

| Outcome | Consequence |
|---|---|
| REQUIRED section incomplete | This font/terminal cannot run hatty. Try DejaVu Sans Mono (D23), or move off the bare console to X (D22). |
| Braille missing | Expected on a bare framebuffer console — it is capped at 256/512 glyphs, so Braille cannot fit alongside everything else. Trend charts fall back to block elements; **tables are unaffected** (D33). |
| `µ` or `³` double-width | Substitute `ug/m3` on the Awair panel. |
| Fewer than 256 colours | The palette in D34 degrades; the glyph-plus-colour rule already covers this, but note it. |

Braille is the only *optional* section. Since decision D33 descoped the polar scope, Braille now
gates only the trend charts on `home` and the optional runchart on `radar` — never the tables,
which are the product.

## Record

Write `RESULTS.md` here with the raw output from both runs, plus the `stty size` and `tput colors`
the script reports. Raw results, not just a verdict — a later phase may dispute the conclusion.
