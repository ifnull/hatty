package render

import (
	"regexp"
	"strings"
	"testing"
)

func TestMain(m *testing.M) { Strict = true; m.Run() }

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func visible(s string) string { return ansi.ReplaceAllString(s, "") }

// Every glyph verified by spike S2 must measure as exactly one cell. If
// go-runewidth's ambiguous-width setting ever changes, this fails first.
func TestDesignGlyphsAreSingleWidth(t *testing.T) {
	groups := map[string]string{
		"box drawing":    "─│┌┐└┘├┤┬┴┼",
		"block elements": "▁▂▃▄▅▆▇█",
		"shading":        "░▒▓█",
		"half blocks":    "▌▐▀▄",
		"arrows":         "↑↓→←",
		"status":         "▸▲●⚠",
		"units":          "°%µ²³",
		"punctuation":    "·—×",
		"braille":        "⠁⠂⠄⡀⢀⣀⣤⣶⣿⠿⡇⢸",
	}
	for name, gs := range groups {
		for _, r := range gs {
			if got := Width(string(r)); got != 1 {
				t.Errorf("%s: %q (U+%04X) measured %d cells, want 1", name, r, r, got)
			}
		}
	}
}

func TestPadExactWidth(t *testing.T) {
	for _, s := range []string{"", "a", "abc", "µg/m³", "▁▂▃▄▅", "⣿⡇⢶", "12.6"} {
		for _, w := range []int{1, 3, 5, 12} {
			if got := Width(Pad(s, w)); got != w {
				t.Errorf("Pad(%q,%d) width %d, want %d", s, w, got, w)
			}
			if got := Width(PadLeft(s, w)); got != w {
				t.Errorf("PadLeft(%q,%d) width %d, want %d", s, w, got, w)
			}
		}
	}
}

func TestTruncateNeverSplitsARune(t *testing.T) {
	s := "⣿⡇⢶⠿"
	for w := 0; w <= 6; w++ {
		got := Truncate(s, w)
		if Width(got) > w {
			t.Errorf("Truncate(%q,%d) = %q, width %d > %d", s, w, got, Width(got), w)
		}
		if !strings.HasPrefix(s, got) {
			t.Errorf("Truncate(%q,%d) = %q is not a prefix", s, w, got)
		}
	}
}

// Styling must not change measured width: escapes are added after measurement.
func TestStyleDoesNotAffectWidth(t *testing.T) {
	r := NewRow(20).
		Cell("UAL1234", 10, Left, Style{FG: 252, Bold: true}).
		Cell("9.1", 10, Right, Style{FG: 71})
	if got := Width(visible(r.String())); got != 20 {
		t.Fatalf("styled row visible width %d, want 20", got)
	}
	if !strings.Contains(r.String(), "\x1b[") {
		t.Fatal("expected escape sequences in styled output")
	}
}

// A row is exactly its declared width regardless of what the caller supplies.
func TestRowIsAlwaysExact(t *testing.T) {
	cases := []struct {
		name  string
		build func() *Row
	}{
		{"exact", func() *Row { return NewRow(10).Cell("abcde", 5, Left, Style{}).Cell("fghij", 5, Left, Style{}) }},
		{"overlong cell truncated", func() *Row { return NewRow(10).Cell("abcdefghijklmno", 10, Left, Style{}) }},
		{"caller overflows row", func() *Row { return NewRow(10).Cell("x", 8, Left, Style{}).Cell("yyyyyy", 6, Left, Style{}) }},
		{"wide runes", func() *Row { return NewRow(12).Cell("⣿⡇⢶µ³", 12, Left, Style{}) }},
		{"fill", func() *Row { return NewRow(30).Cell("─ TITLE ", 8, Left, Style{}).Fill("─", Style{}) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Width(visible(c.build().String())); got != c.build().want {
				t.Errorf("row width %d, want %d", got, c.build().want)
			}
		})
	}
}

// Strict mode must catch an under-filled row, which is how a layout bug
// surfaces in CI rather than as a corrupted frame on the panel.
func TestStrictModePanicsOnShortRow(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for a short row in Strict mode")
		}
	}()
	_ = NewRow(20).Cell("abc", 3, Left, Style{}).String()
}
