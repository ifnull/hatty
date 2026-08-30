#!/usr/bin/env bash
# S2 -- glyph coverage and width check for the hatty design.
#
# Run this ON THE PI, on the actual 800x480 panel, in the terminal you
# intend to use. Run it twice if you have not settled X vs bare console:
# once in a terminal emulator under X, once on a tty (Ctrl+Alt+F1).
#
#   bash check.sh
#
# TWO things are being tested, and the second matters more:
#
#   1. Coverage  -- does every glyph render, or do some show as tofu?
#   2. WIDTH     -- does every glyph occupy exactly ONE cell?
#
# A missing glyph is obvious and survivable. A double-width glyph is
# invisible until it corrupts an entire frame, which is risk R3. That is
# why every row below ends with a '|' that MUST line up with the ruler.

set -u
echo
echo "  terminal: ${TERM:-unset}   size: $(stty size 2>/dev/null || echo unknown)   colours: $(tput colors 2>/dev/null || echo unknown)"
if [ -n "${SSH_CONNECTION:-}" ]; then
  echo "  session:  SSH -- WARNING: the size above is your client, not the panel"
else
  echo "  session:  LOCAL -- the size above is this display"
fi
echo

# ---- ruler: every row below must end flush with the | at column 60 ----
printf '  '; for i in $(seq 1 30); do
  if   [ $((i % 10)) -eq 0 ]; then printf '%d' $(( (i/10) % 10 ))
  elif [ $((i % 5))  -eq 0 ]; then printf '+'
  else printf '.'; fi
done; printf '|\n\n'

# $1 glyphs, $2 how many CELLS they should occupy, $3 description.
# printf %-60s pads by BYTES, not display cells, so it cannot be used here --
# multi-byte UTF-8 would shift the marker even when the glyph is single-width.
row () { printf '  %s' "$1"; printf '%*s' "$((30 - $2))" ''; printf '|  %s\n' "$3"; }

echo "  REQUIRED -- the design does not work without these"
row '─│┌┐└┘├┤┬┴┼' 11 'box drawing'
row '▁▂▃▄▅▆▇█' 8 'block elements'
row '░▒▓█' 4 'shading'
row '▌▐▀▄' 4 'half blocks'
row '↑↓→←' 4 'arrows'
row '▸▲●⚠' 4 'status glyphs'
row '°%' 2 'degree, percent'

echo
echo "  WANTED -- trend charts degrade to block elements without these"
row '⠁⠂⠄⡀⢀⣀⣤⣶⣿⠿⡇⢸' 12 'braille'

echo
echo "  CHECK CAREFULLY -- known width hazards"
row 'µ ² ³' 5 'micro, superscripts'
row '· — ×' 5 'dot, em dash, times'

echo
echo "  colour (decision D34 needs 256):"
printf '  '
for n in 238 245 252 240 71 179 167 96 60 67 74 81; do
  printf '\033[38;5;%sm████\033[0m' "$n"
done
printf '\n  chrome label value muted / ok warn alert stale / alt0-3\n'

echo
cat <<'NOTE'
  HOW TO READ IT

  Every '|' above must sit directly under the '|' in the ruler.

    lines up          -> single width, safe
    pushed right      -> DOUBLE width, must not be used (risk R3)
    tofu / blank box  -> not in the font

  Coverage failures by section:
    REQUIRED missing  -> this font or terminal cannot run hatty. Try
                         DejaVu Sans Mono (decision D23), or move from the
                         bare console to X (decision D22).
    braille missing   -> expected on a bare framebuffer console, which is
                         capped at 256/512 glyphs. Trend charts fall back
                         to block elements; tables are unaffected (D33).
    µ / ³ wrong width -> substitute "ug/m3" on the Awair panel.

  Record the result in RESULTS.md next to this script -- raw output, not
  just a verdict.
NOTE
