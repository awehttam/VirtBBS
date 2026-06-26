package ansi

import (
	"strings"
	"testing"
)

func TestBanner_allGlyphsMapToCP437(t *testing.T) {
	b := Banner("LarrysFarm")
	for _, r := range b {
		if r < 128 || r == '\r' || r == '\n' {
			continue
		}
		if _, ok := cp437Map[r]; !ok {
			t.Errorf("banner rune U+%04X %q not in cp437Map", r, string(r))
		}
	}
	cp := ToCP437(b)
	if strings.Count(cp, "?") > 0 {
		t.Errorf("ToCP437(Banner) contains %d question marks", strings.Count(cp, "?"))
	}
}
