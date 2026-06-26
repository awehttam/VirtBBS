package ansi

import (
	"strings"
	"unicode/utf8"
)

// VisibleWidth returns the number of terminal columns occupied by s,
// ignoring ANSI escape sequences.
func VisibleWidth(s string) int {
	w := 0
	inEsc := false
	for _, c := range s {
		if inEsc {
			if c == 'm' {
				inEsc = false
			}
			continue
		}
		if c == '\x1b' {
			inEsc = true
			continue
		}
		w++
	}
	return w
}

// FitVisibleWidth pads or truncates s so its visible width equals width.
func FitVisibleWidth(s string, width int) string {
	vis := VisibleWidth(s)
	if vis == width {
		return s
	}
	if vis < width {
		return s + strings.Repeat(" ", width-vis)
	}
	return truncateVisible(s, width)
}

func truncateVisible(s string, width int) string {
	var sb strings.Builder
	sb.Grow(len(s))
	vis := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			j := i + 1
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++
			}
			sb.WriteString(s[i:j])
			i = j
			continue
		}
		if vis >= width {
			break
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		sb.WriteRune(r)
		vis++
		i += size
	}
	return sb.String()
}