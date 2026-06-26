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
//   v0.0.7  2026-06-24  Initial implementation — editor dispatch, types, helpers
// ============================================================================

// Package editor provides BBS message editors.
//
// Two editors are available:
//
//	EditorSimple     — traditional line-at-a-time input (PCBoard style)
//	EditorFullScreen — full-screen ANSI editor with word-wrap (SlyEdit style)
//
// The full-screen editor requires a VT100/ANSI terminal and should only be
// used when user.ANSI is true.  Edit() falls back to EditorSimple automatically.
package editor

import (
	"io"
	"strings"

	"github.com/virtbbs/virtbbs/internal/ansi"
)

// Editor type identifiers stored in user preferences.
const (
	EditorSimple     = "simple" // Line-by-line (no ANSI required)
	EditorFullScreen = "full"   // Full-screen ANSI (requires ANSI terminal)
)

// Config holds parameters for the editing session.
type Config struct {
	// Type selects the editor: EditorSimple or EditorFullScreen.
	// If ANSI is false, always falls back to EditorSimple.
	Type string

	// Subject is shown in the editor title bar (full-screen editor).
	Subject string

	// InitBody pre-populates the editor text (e.g. quoted reply text).
	InitBody string

	// WrapCol is the right margin for word-wrap (default 78).
	WrapCol int

	// ANSI indicates the user's terminal supports ANSI/VT100 sequences.
	ANSI bool

	// MaxLines is the maximum number of body lines allowed (default 500).
	MaxLines int

	// BBSName is shown in the editor title bar.
	BBSName string

	// CP437Out is true for Telnet (SyncTerm etc.); false for SSH UTF-8 terminals.
	// When zero, defaults to CP437 for backward compatibility.
	CP437Out bool
}

// Result is the outcome of an editing session.
type Result struct {
	// Body is the composed text with CRLF line endings.
	// Empty if Aborted is true.
	Body string

	// Aborted is true when the user explicitly abandoned the message (Ctrl+A).
	Aborted bool

	// Lines is the number of lines in the composed message.
	Lines int
}

func editorEncode(cfg Config) func(string) string {
	return func(s string) string { return ansi.EncodeOutput(s, cfg.CP437Out) }
}

// Edit invokes the editor described by cfg and returns the result.
func Edit(rw io.ReadWriter, cfg Config) Result {
	if cfg.WrapCol <= 0 {
		cfg.WrapCol = 78
	}
	if cfg.MaxLines <= 0 {
		cfg.MaxLines = 500
	}
	if cfg.Type == "" {
		cfg.Type = EditorSimple
	}

	if cfg.Type == EditorFullScreen && cfg.ANSI {
		return runFullScreen(rw, cfg)
	}
	return runSimple(rw, cfg)
}

// ─── Shared helpers ──────────────────────────────────────────────────────────

// splitLines splits a body string into lines, normalising line endings.
// Always returns at least one element.
func splitLines(body string) []string {
	if body == "" {
		return []string{""}
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	// Remove trailing newline so we don't get a spurious empty final element.
	body = strings.TrimRight(body, "\n")
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// joinLines joins a []string buffer into a CRLF body string.
func joinLines(lines []string) string {
	// Trim trailing blank lines.
	end := len(lines)
	for end > 1 && lines[end-1] == "" {
		end--
	}
	return strings.Join(lines[:end], "\r\n")
}

// clamp returns v clamped to [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
