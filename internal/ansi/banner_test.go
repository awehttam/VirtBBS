package ansi

import (
	"strings"
	"testing"
)

func visibleWidth(s string) int {
	return VisibleWidth(s)
}

func TestBanner_lineWidthsConsistent(t *testing.T) {
	b := Banner("VirtBBS")
	lines := strings.Split(strings.TrimSuffix(b, "\r\n"), "\r\n")
	if len(lines) < 3 {
		t.Fatalf("expected multiple lines, got %d", len(lines))
	}
	topW := visibleWidth(lines[0])
	for i, line := range lines {
		if line == "" {
			continue
		}
		w := visibleWidth(line)
		if w != topW {
			t.Errorf("line %d width %d != top border width %d: %q", i, w, topW, line)
		}
	}
}

func TestBanner_closingBorderIsCyan(t *testing.T) {
	b := Banner("AB")
	// Letter rows should end with cyan bold before closing ║
	if !strings.Contains(b, bnCyan+bnBold+"║\r\n") {
		t.Errorf("expected letter rows to end with cyan bold ║")
	}
}