package display

import (
	"strings"
	"unicode/utf8"

	"github.com/virtbbs/virtbbs/internal/ansi"
)

// alignBoxLines pads or truncates ║-bordered content rows so their inner width
// matches the ═ count from the top border row. @code@ substitution often
// changes line lengths after the display file was authored.
func alignBoxLines(s string) string {
	lines := strings.Split(s, "\n")
	borderW := 0
	for _, line := range lines {
		if w := boxBorderInnerWidth(line); w > 0 {
			borderW = w
			break
		}
	}
	if borderW == 0 {
		return s
	}
	for i, line := range lines {
		if aligned, ok := alignBoxContentLine(line, borderW); ok {
			lines[i] = aligned
		}
	}
	return strings.Join(lines, "\n")
}

func boxBorderInnerWidth(line string) int {
	if !strings.ContainsRune(line, '╔') {
		return 0
	}
	n := 0
	for _, r := range line {
		if r == '═' {
			n++
		}
	}
	return n
}

func alignBoxContentLine(line string, width int) (string, bool) {
	if strings.ContainsRune(line, '╔') || strings.ContainsRune(line, '╚') {
		return line, false
	}
	first := strings.IndexRune(line, '║')
	last, lastSize := lastRuneIndex(line, '║')
	if first < 0 || last <= first {
		return line, false
	}
	_, firstSize := utf8.DecodeRuneInString(line[first:])
	prefix := line[:first+firstSize]
	suffix := line[last : last+lastSize]
	middle := line[first+firstSize : last]
	fitted := ansi.FitVisibleWidth(middle, width)
	return prefix + fitted + suffix, true
}

func lastRuneIndex(s string, target rune) (int, int) {
	last := -1
	lastSize := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == target {
			last = i
			lastSize = size
		}
		i += size
	}
	return last, lastSize
}