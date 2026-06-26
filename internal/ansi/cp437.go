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
//   v0.1.1  2026-06-25  Initial implementation — UTF-8 to CP437 output translation
// ============================================================================

package ansi

import (
	"strings"
	"unicode/utf8"
)

// cp437Map translates Unicode runes used in VirtBBS source/display files
// (box-drawing, block elements, and a handful of punctuation marks) into
// their single-byte CP437 codes. Classic BBS terminals such as SyncTerm,
// NetRunner, and mTelnet assume the CP437 codepage, not UTF-8 — sending
// the raw multi-byte UTF-8 sequence for e.g. '╔' causes each byte to be
// rendered as its own (wrong) CP437 glyph, producing visible garbage like
// "Γòö" instead of a box-drawing corner.
//
// Where CP437 has no equivalent glyph (em dash, ellipsis, multiplication
// sign) the rune is folded to a single-width ASCII approximation instead,
// to avoid disturbing fixed-width layouts.
var cp437Map = map[rune]byte{
	'§': 0x15, // section sign
	'±': 0xF1, // plus-minus sign
	'×': 'x',  // multiplication sign (no CP437 glyph) — ASCII fallback
	'—': '-',  // em dash (no CP437 glyph) — ASCII fallback
	'…': '.',  // ellipsis (no CP437 glyph) — single-width ASCII fallback
	'→': 0x1A, // rightwards arrow
	'─': 0xC4, // box drawings light horizontal
	'│': 0xB3, // box drawings light vertical
	'┌': 0xDA, // box drawings light down and right
	'┐': 0xBF, // box drawings light down and left
	'└': 0xC0, // box drawings light up and right
	'┘': 0xD9, // box drawings light up and left
	'├': 0xC3, // box drawings light vertical and right
	'┤': 0xB4, // box drawings light vertical and left
	'┬': 0xC2, // box drawings light down and horizontal
	'┴': 0xC1, // box drawings light up and horizontal
	'┼': 0xC5, // box drawings light vertical and horizontal
	'═': 0xCD, // box drawings double horizontal
	'║': 0xBA, // box drawings double vertical
	'╔': 0xC9, // box drawings double down and right
	'╗': 0xBB, // box drawings double down and left
	'╚': 0xC8, // box drawings double up and right
	'╝': 0xBC, // box drawings double up and left
	'╠': 0xCC, // box drawings double vertical and right
	'╣': 0xB9, // box drawings double vertical and left
	'╦': 0xCB, // box drawings double down and horizontal
	'╩': 0xCA, // box drawings double up and horizontal
	'╬': 0xCE, // box drawings double vertical and horizontal
	'►': '>',  // black right-pointing pointer — ASCII fallback (stats sections)
	'▀': 0xDF, // upper half block
	'▄': 0xDC, // lower half block
	'█': 0xDB, // full block
}

// cp437ToUnicode maps each CP437 byte to its Unicode glyph. PCBoard .ANS
// display files are often stored as raw CP437; when read into a Go string
// each byte 0x80-0xFF becomes rune U+00xx and must be re-decoded.
var cp437ToUnicode = buildCP437ToUnicode()

func buildCP437ToUnicode() [256]rune {
	var t [256]rune
	for i := 0; i < 0x20; i++ {
		t[i] = rune(i)
	}
	for i := 0x20; i <= 0x7E; i++ {
		t[i] = rune(i)
	}
	t[0x7F] = '⌂'

	writeRange := func(base byte, glyphs string) {
		i := int(base)
		for _, ch := range glyphs {
			t[i] = ch
			i++
		}
	}
	writeRange(0x80, "ÇüéâäàåçêëèïîìÄÅÉæÆôöòûùÿÖÜ¢£¥₧ƒ")
	writeRange(0xA0, "áíóúñÑªº¿⌐¬½¼¡«»░▒▓│┤╡╢╖╕╣║╗╝╜╛┐")
	writeRange(0xC0, "└┴┬├─┼╞╟╚╔╩╦╠═╬╧╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀")
	writeRange(0xE0, "αßΓπΣσµτΦΘΩδ∞φε∩≡±≥≤⌠⌡÷≈°∙·√ⁿ²■ ")
	return t
}

// RuneFromCP437Byte returns the Unicode character for a CP437 code-point byte.
func RuneFromCP437Byte(b byte) rune {
	return cp437ToUnicode[b]
}

// DecodeANSBytes converts a PCBoard .ANS file string to Unicode. Native
// PCBoard files store box-drawing as raw CP437 bytes; UTF-8 .ANS files pass
// through unchanged.
func DecodeANSBytes(s string) string {
	if utf8.ValidString(s) {
		for _, r := range s {
			if r > 0xFF {
				return s
			}
		}
	}

	raw := []byte(s)
	needsWork := false
	for _, b := range raw {
		if b >= 0x80 {
			needsWork = true
			break
		}
	}
	if !needsWork {
		return s
	}

	var sb strings.Builder
	sb.Grow(len(raw))
	for _, b := range raw {
		if b < 0x80 {
			sb.WriteByte(b)
			continue
		}
		sb.WriteRune(RuneFromCP437Byte(b))
	}
	return sb.String()
}

// ExpandPCBAnsi converts PCBoard display-file ANSI conventions to real ESC
// sequences. PCBoard stores ESC as '[' so files remain plain ASCII editable.
// Only sequences that start with a digit after '[' are converted (e.g. [1;36m,
// [0m) so menu text like [S]tats is not touched.
func ExpandPCBAnsi(s string) string {
	needsWork := false
	for i := 0; i < len(s); i++ {
		if s[i] == '[' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			needsWork = true
			break
		}
	}
	if !needsWork {
		return s
	}

	var sb strings.Builder
	sb.Grow(len(s) + 16)
	for i := 0; i < len(s); i++ {
		if s[i] == '[' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			j := i + 1
			for j < len(s) && s[j] >= '0' && s[j] <= '?' {
				j++
			}
			if j < len(s) && ((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				sb.WriteByte(0x1B)
				sb.WriteByte('[')
				sb.WriteString(s[i+1 : j+1])
				i = j
				continue
			}
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

// ToCP437 rewrites s, converting the Unicode characters above into their
// CP437 single-byte codes and leaving plain ASCII untouched. Any other
// non-ASCII rune (none expected in current VirtBBS source/display files)
// is folded to '?' as a safe fallback rather than corrupting the stream.
//
// Call this as the last step before writing text to a terminal connection
// — i.e. inside the I/O funnel functions, not earlier — so that internal
// string-building logic (width padding, etc.) continues to operate on
// ordinary UTF-8 strings where each of these characters is exactly one
// rune wide.
// EncodeOutput translates box-drawing and special characters for the connected
// terminal. Classic Telnet clients (SyncTerm, etc.) expect CP437; SSH and modern
// UTF-8 terminals should receive UTF-8 unchanged.
func EncodeOutput(s string, cp437 bool) string {
	if cp437 {
		return ToCP437(s)
	}
	return s
}

func ToCP437(s string) string {
	hasNonASCII := false
	for _, r := range s {
		if r > 127 {
			hasNonASCII = true
			break
		}
	}
	if !hasNonASCII {
		return s // fast path: nothing to translate
	}

	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		if r < 128 {
			sb.WriteByte(byte(r))
			continue
		}
		if b, ok := cp437Map[r]; ok {
			sb.WriteByte(b)
			continue
		}
		sb.WriteByte('?')
	}
	return sb.String()
}
