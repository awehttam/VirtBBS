// ============================================================================
// VirtBBS — A modern BBS server inspired by PCBoard BBS
//           (Clark Development Company, 1987-1996)
//
// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// MIT License
// (see editor.go for full text)
//
// Change History:
//   v0.0.7  2026-06-24  Simple line-by-line editor (legacy PCBoard style)
// ============================================================================

package editor

import (
	"fmt"
	"io"
	"strings"
)

// runSimple implements the traditional PCBoard-style line-by-line editor.
// The user types one line at a time; a blank line ends the message.
// Commands on a line beginning with "/" are handled:
//
//	/S — save and send
//	/A — abort
//	/L — list the message so far
//	/H or /? — help
func runSimple(rw io.ReadWriter, cfg Config) Result {
	enc := editorEncode(cfg)
	writeStr(rw, enc, "\r\n")
	writeStr(rw, enc, "\x1b[1;36m┌─ Message Editor ─────────────────────────────────────────────────────┐\x1b[0m\r\n")
	writeStr(rw, enc, "\x1b[36m│\x1b[0m Enter text one line at a time.  Blank line to end.                  \x1b[36m│\x1b[0m\r\n")
	writeStr(rw, enc, "\x1b[36m│\x1b[0m Commands: \x1b[1;33m/S\x1b[0m=Save  \x1b[1;33m/A\x1b[0m=Abort  \x1b[1;33m/L\x1b[0m=List  \x1b[1;33m/?\x1b[0m=Help           \x1b[36m│\x1b[0m\r\n")
	writeStr(rw, enc, "\x1b[1;36m└──────────────────────────────────────────────────────────────────────┘\x1b[0m\r\n")

	// Pre-populate with InitBody (e.g. quoted text for replies).
	var lines []string
	if cfg.InitBody != "" {
		lines = splitLines(cfg.InitBody)
		for _, l := range lines {
			writeStr(rw, enc, "\x1b[90m > \x1b[0m"+l+"\r\n")
		}
		writeStr(rw, enc, "\r\n")
	}

	wrapCol := cfg.WrapCol
	if wrapCol <= 0 {
		wrapCol = 78
	}

	for {
		lineNum := len(lines) + 1
		writeStr(rw, enc, fmt.Sprintf("\x1b[32m%3d:\x1b[0m ", lineNum))
		line := readLine(rw, enc)

		// Slash commands (only at start of line).
		if strings.HasPrefix(line, "/") {
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch cmd {
			case "/S", "/SAVE":
				body := joinLines(lines)
				return Result{Body: body, Lines: len(lines)}
			case "/A", "/ABORT":
				writeStr(rw, enc, "\x1b[1;31mMessage aborted.\x1b[0m\r\n")
				return Result{Aborted: true}
			case "/L", "/LIST":
				writeStr(rw, enc, "\r\n\x1b[1;36m── Message so far ──────────────────────────────────\x1b[0m\r\n")
				for i, l := range lines {
					writeStr(rw, enc, fmt.Sprintf("\x1b[90m%3d│\x1b[0m %s\r\n", i+1, l))
				}
				writeStr(rw, enc, "\x1b[1;36m────────────────────────────────────────────────────\x1b[0m\r\n\r\n")
				continue
			case "/H", "/?", "/HELP":
				writeStr(rw, enc, "\r\n\x1b[1;33mSimple Editor Commands:\x1b[0m\r\n")
				writeStr(rw, enc, "  \x1b[33m/S\x1b[0m  or blank line after text — Save and send\r\n")
				writeStr(rw, enc, "  \x1b[33m/A\x1b[0m  — Abort and discard message\r\n")
				writeStr(rw, enc, "  \x1b[33m/L\x1b[0m  — List message so far\r\n")
				writeStr(rw, enc, "  \x1b[33m/?\x1b[0m  — This help screen\r\n\r\n")
				continue
			}
		}

		// Blank line = end of message.
		if line == "" {
			if len(lines) == 0 {
				writeStr(rw, enc, "\x1b[1;31mNo message entered.\x1b[0m\r\n")
				return Result{Aborted: true}
			}
			body := joinLines(lines)
			return Result{Body: body, Lines: len(lines)}
		}

		// Soft word-wrap: if line is too long, split at last space before wrapCol.
		for len(line) > wrapCol {
			split := wrapCol
			for i := wrapCol - 1; i > 0; i-- {
				if line[i] == ' ' {
					split = i
					break
				}
			}
			lines = append(lines, line[:split])
			line = strings.TrimLeft(line[split:], " ")
			if len(lines) >= cfg.MaxLines {
				writeStr(rw, enc, "\x1b[1;31mMaximum message length reached.\x1b[0m\r\n")
				body := joinLines(lines)
				return Result{Body: body, Lines: len(lines)}
			}
		}

		lines = append(lines, line)
		if len(lines) >= cfg.MaxLines {
			writeStr(rw, enc, "\x1b[1;31mMaximum message length reached.\x1b[0m\r\n")
			body := joinLines(lines)
			return Result{Body: body, Lines: len(lines)}
		}
	}
}

// readLine reads one line of input with basic backspace support.
func readLine(rw io.ReadWriter, enc func(string) string) string {
	var buf []byte
	b := make([]byte, 1)
	for {
		if _, err := rw.Read(b); err != nil {
			break
		}
		ch := b[0]
		switch {
		case ch == 0x0D || ch == 0x0A: // Enter
			writeStr(rw, enc, "\r\n")
			return string(buf)
		case ch == 0x08 || ch == 0x7F: // Backspace
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				writeStr(rw, enc, "\x08 \x08")
			}
		case ch == 0x03 || ch == 0x01: // Ctrl+C / Ctrl+A — signal abort via /A
			writeStr(rw, enc, "\r\n")
			return "/A"
		case ch >= 0x20: // printable
			buf = append(buf, ch)
			_, _ = rw.Write([]byte{ch})
		}
	}
	return string(buf)
}

func writeStr(w io.Writer, enc func(string) string, s string) {
	_, _ = io.WriteString(w, enc(s))
}