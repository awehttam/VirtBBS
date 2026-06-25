// ============================================================================
// VirtBBS — A modern BBS server inspired by PCBoard BBS
//           (Clark Development Company, 1987-1996)
//
// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// MIT License
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
// OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.
//
// Change History:
//   v0.0.6  2026-06-24  Initial implementation — dynamic ANSI block-letter banner
//   v0.0.8  2026-06-25  Add apostrophe (') glyph to the block-letter font
// ============================================================================

// Package ansi provides ANSI escape sequence helpers and the dynamic BBS
// banner generator.
//
// Banner() generates a coloured block-letter banner from a BBS name string
// using CP437 block-drawing characters rendered via ANSI colour codes.
// Characters not in the font table are rendered as a space.
package ansi

import (
	"strings"
)

// Raw ANSI escape sequences used within the banner.
// These are lowercase to avoid clashing with the function declarations in ansi.go.
const (
	bnReset  = "\x1b[0m"
	bnBold   = "\x1b[1m"
	bnCyan   = "\x1b[96m" // bright cyan
	bnBlue   = "\x1b[94m" // bright blue
	bnWhite  = "\x1b[97m" // bright white
)

// Banner generates a multi-line ANSI block-letter banner for name.
// Each character is 5 columns wide and 5 rows tall.
// The banner is surrounded by a decorative border.
// Returns a string with CRLF line endings suitable for sending to a terminal.
func Banner(name string) string {
	name = strings.ToUpper(name)
	if len(name) > 20 {
		name = name[:20]
	}

	rows := make([]strings.Builder, 5)
	for i, ch := range name {
		glyph, ok := font[ch]
		if !ok {
			glyph = fontSpace
		}
		_ = i
		for row := 0; row < 5; row++ {
			rows[row].WriteString(glyph[row])
			rows[row].WriteString(" ") // inter-character gap
		}
	}

	var sb strings.Builder

	width := 5*len(name) + len(name) + 2 // 5 cols + 1 gap per char + 2 border
	if width < 40 {
		width = 40
	}

	// Top border.
	sb.WriteString(bnCyan + bnBold)
	sb.WriteString("╔")
	sb.WriteString(strings.Repeat("═", width))
	sb.WriteString("╗\r\n")

	// Empty line.
	sb.WriteString("║" + strings.Repeat(" ", width) + "║\r\n")

	// Letter rows.
	for row := 0; row < 5; row++ {
		line := rows[row].String()
		// Pad to width.
		padded := line
		vis := visWidth(line)
		if vis < width {
			padded = line + strings.Repeat(" ", width-vis)
		} else if vis > width {
			padded = line
		}

		if row == 0 || row == 4 {
			sb.WriteString(bnBlue)
		} else {
			sb.WriteString(bnWhite)
		}
		sb.WriteString(bnCyan + bnBold + "║" + bnReset + bnBold)
		sb.WriteString(" " + padded)
		sb.WriteString(bnCyan + bnBold + "║\r\n")
	}

	// Empty line.
	sb.WriteString(bnCyan + bnBold + "║" + strings.Repeat(" ", width) + "║\r\n")

	// Bottom border.
	sb.WriteString("╚" + strings.Repeat("═", width) + "╝\r\n")
	sb.WriteString(bnReset)

	return sb.String()
}

// visWidth returns the visible character width (ignoring ANSI escape sequences).
func visWidth(s string) int {
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

// font maps uppercase letters and digits to 5-row block-letter glyphs.
// Each row is 5 visible characters wide using block chars and spaces.
// Block characters used: █ (U+2588 FULL BLOCK), ▀ (U+2580), ▄ (U+2584)

var fontSpace = [5]string{
	"     ",
	"     ",
	"     ",
	"     ",
	"     ",
}

var font = map[rune][5]string{
	'A': {
		" ▄█▄ ",
		"█   █",
		"█████",
		"█   █",
		"█   █",
	},
	'B': {
		"████ ",
		"█   █",
		"████ ",
		"█   █",
		"████ ",
	},
	'C': {
		" ████",
		"█    ",
		"█    ",
		"█    ",
		" ████",
	},
	'D': {
		"████ ",
		"█   █",
		"█   █",
		"█   █",
		"████ ",
	},
	'E': {
		"█████",
		"█    ",
		"████ ",
		"█    ",
		"█████",
	},
	'F': {
		"█████",
		"█    ",
		"████ ",
		"█    ",
		"█    ",
	},
	'G': {
		" ████",
		"█    ",
		"█  ██",
		"█   █",
		" ████",
	},
	'H': {
		"█   █",
		"█   █",
		"█████",
		"█   █",
		"█   █",
	},
	'I': {
		"█████",
		"  █  ",
		"  █  ",
		"  █  ",
		"█████",
	},
	'J': {
		"█████",
		"   █ ",
		"   █ ",
		"█  █ ",
		" ██  ",
	},
	'K': {
		"█   █",
		"█  █ ",
		"███  ",
		"█  █ ",
		"█   █",
	},
	'L': {
		"█    ",
		"█    ",
		"█    ",
		"█    ",
		"█████",
	},
	'M': {
		"█   █",
		"██ ██",
		"█ █ █",
		"█   █",
		"█   █",
	},
	'N': {
		"█   █",
		"██  █",
		"█ █ █",
		"█  ██",
		"█   █",
	},
	'O': {
		" ███ ",
		"█   █",
		"█   █",
		"█   █",
		" ███ ",
	},
	'P': {
		"████ ",
		"█   █",
		"████ ",
		"█    ",
		"█    ",
	},
	'Q': {
		" ███ ",
		"█   █",
		"█ █ █",
		"█  ██",
		" ████",
	},
	'R': {
		"████ ",
		"█   █",
		"████ ",
		"█  █ ",
		"█   █",
	},
	'S': {
		" ████",
		"█    ",
		" ███ ",
		"    █",
		"████ ",
	},
	'T': {
		"█████",
		"  █  ",
		"  █  ",
		"  █  ",
		"  █  ",
	},
	'U': {
		"█   █",
		"█   █",
		"█   █",
		"█   █",
		" ███ ",
	},
	'V': {
		"█   █",
		"█   █",
		"█   █",
		" █ █ ",
		"  █  ",
	},
	'W': {
		"█   █",
		"█   █",
		"█ █ █",
		"██ ██",
		"█   █",
	},
	'X': {
		"█   █",
		" █ █ ",
		"  █  ",
		" █ █ ",
		"█   █",
	},
	'Y': {
		"█   █",
		" █ █ ",
		"  █  ",
		"  █  ",
		"  █  ",
	},
	'Z': {
		"█████",
		"   █ ",
		"  █  ",
		" █   ",
		"█████",
	},
	'0': {
		" ███ ",
		"█  ██",
		"█ █ █",
		"██  █",
		" ███ ",
	},
	'1': {
		" ██  ",
		"  █  ",
		"  █  ",
		"  █  ",
		"█████",
	},
	'2': {
		" ███ ",
		"    █",
		" ███ ",
		"█    ",
		"█████",
	},
	'3': {
		"████ ",
		"    █",
		" ███ ",
		"    █",
		"████ ",
	},
	'4': {
		"█   █",
		"█   █",
		"█████",
		"    █",
		"    █",
	},
	'5': {
		"█████",
		"█    ",
		"████ ",
		"    █",
		"████ ",
	},
	'6': {
		" ███ ",
		"█    ",
		"████ ",
		"█   █",
		" ███ ",
	},
	'7': {
		"█████",
		"    █",
		"   █ ",
		"  █  ",
		" █   ",
	},
	'8': {
		" ███ ",
		"█   █",
		" ███ ",
		"█   █",
		" ███ ",
	},
	'9': {
		" ███ ",
		"█   █",
		" ████",
		"    █",
		" ███ ",
	},
	' ': fontSpace,
	'-': {
		"     ",
		"     ",
		"█████",
		"     ",
		"     ",
	},
	'!': {
		"  █  ",
		"  █  ",
		"  █  ",
		"     ",
		"  █  ",
	},
	'\'': {
		" █   ",
		" █   ",
		"     ",
		"     ",
		"     ",
	},
}
