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
//   v0.0.4  2026-06-24  Phase 11: PCBoard display file renderer with @code@ substitution
// ============================================================================

// Package display renders PCBoard-style display files with @code@ substitution.
//
// PCBoard BBS used a directory of display files to present customisable
// screens to callers.  Each file could contain plain ASCII, ANSI escape
// sequences, or PCBoard @codes@ that were expanded at runtime.
//
// VirtBBS looks for display files in the configured display/ directory,
// trying the following extensions in order: .ANS  .ASC  (no extension).
// If no file is found the call returns ("", ErrNotFound) so the caller can
// fall back gracefully.
//
// Supported @codes@:
//
//	@BBSNAME@     BBS name
//	@SYSOP@       Sysop name
//	@NAME@        Caller full name
//	@FIRST@       Caller first name
//	@CITY@        Caller city
//	@SECURITY@    Caller security level
//	@NODE@        Node number
//	@TIME@        Current time (HH:MM)
//	@DATE@        Current date (MM-DD-YY)
//	@TIMELEFT@    Minutes remaining this call
//	@TIMEON@      Minutes on this call so far
//	@NUMCALLS@    Total times this user has called
package display

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/virtbbs/virtbbs/internal/ansi"
)

// ErrNotFound is returned when no matching display file is found.
var ErrNotFound = errors.New("display file not found")

// Vars holds the runtime values for @code@ substitution.
type Vars struct {
	BBSName   string
	SysopName string
	Name      string // caller full name
	City      string
	Security  int
	Node      int
	TimeLeft  int // minutes remaining
	TimeOn    int // minutes on this call
	NumCalls  int // times online
}

// FirstName returns the first word of Name.
func (v *Vars) FirstName() string {
	parts := strings.Fields(v.Name)
	if len(parts) == 0 {
		return v.Name
	}
	return parts[0]
}

// Render loads a display file from displayDir, expands @codes@, and returns
// the ready-to-send string.  name should be the base file name without
// extension (e.g. "WELCOME", "NEWUSER", "BULLETIN").
func Render(displayDir, name string, vars *Vars) (string, error) {
	content, err := readFile(displayDir, name)
	if err != nil {
		return "", err
	}
	return expand(content, vars), nil
}

// readFile searches for <displayDir>/<name>.<ext> trying .ANS then .ASC
// then no extension, case-insensitively.
func readFile(displayDir, name string) (string, error) {
	candidates := []string{
		filepath.Join(displayDir, strings.ToUpper(name)+".ANS"),
		filepath.Join(displayDir, strings.ToLower(name)+".ans"),
		filepath.Join(displayDir, strings.ToUpper(name)+".ASC"),
		filepath.Join(displayDir, strings.ToLower(name)+".asc"),
		filepath.Join(displayDir, strings.ToUpper(name)),
		filepath.Join(displayDir, strings.ToLower(name)),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data), nil
		}
	}
	return "", ErrNotFound
}

// expand replaces PCBoard @codes@ in content with their runtime values.
func expand(content string, vars *Vars) string {
	now := time.Now()
	replacements := []string{
		"@BBSNAME@", vars.BBSName,
		"@SYSOP@", vars.SysopName,
		"@NAME@", vars.Name,
		"@FIRST@", vars.FirstName(),
		"@CITY@", vars.City,
		"@SECURITY@", itoa(vars.Security),
		"@NODE@", itoa(vars.Node),
		"@TIME@", now.Format("15:04"),
		"@DATE@", now.Format("01-02-06"),
		"@TIMELEFT@", itoa(vars.TimeLeft),
		"@TIMEON@", itoa(vars.TimeOn),
		"@NUMCALLS@", itoa(vars.NumCalls),
		// PCB colour codes — convert to ANSI equivalents
		"@CLS@", "\033[2J\033[H",
		"@PAUSE@", "", // can't pause in a simple string render; strip it
	}
	r := strings.NewReplacer(replacements...)
	out := r.Replace(content)
	out = ansi.DecodeANSBytes(out)
	out = ansi.ExpandPCBAnsi(out)
	out = alignBoxLines(out)
	// Normalise line endings: bare \n → \r\n for terminal compatibility.
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "\n")
	out = strings.ReplaceAll(out, "\n", "\r\n")
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
