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
//   v0.0.1  2026-06-24  Initial implementation
// ============================================================================

package ppl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/virtbbs/virtbbs/internal/ansi"
)

// Run compiles and executes a PPL source file (.PPS) given an Environment.
// Returns any execution error.
func Run(ppsPath string, env *Environment) error {
	src, err := os.ReadFile(ppsPath)
	if err != nil {
		return fmt.Errorf("ppl: read %s: %w", ppsPath, err)
	}
	env.PPEPath = filepath.Dir(ppsPath)
	return RunSource(string(src), env)
}

// RunSource compiles and executes a PPL source string.
func RunSource(src string, env *Environment) error {
	lexer := NewLexer(src)
	tokens := lexer.Tokenize()
	parser := NewParser(tokens)
	prog, err := parser.Parse()
	if err != nil {
		return fmt.Errorf("ppl parse: %w", err)
	}
	interp := NewInterpreter(prog, env)
	return interp.Run()
}

// EnvFromSession creates a PPL Environment connected to a BBS session's I/O.
// inputLine reads one line of user input after an optional prompt; the session
// layer should supply a reader that treats both CR and LF as end-of-line (SSH
// PTYs typically send CR only).
func EnvFromSession(rw io.ReadWriter, userName, userCity string, userSec, timesOn, nodeNum int, bbsName, sysopName string, cp437Out bool, inputLine func(prompt string) string) *Environment {
	if inputLine == nil {
		inputLine = func(prompt string) string { return readLineRaw(rw, prompt, cp437Out) }
	}
	return &Environment{
		Print: func(s string) {
			_, _ = io.WriteString(rw, ansi.EncodeOutput(s, cp437Out))
		},
		Input: inputLine,
		ReadKey: func() byte {
			buf := make([]byte, 1)
			_, _ = rw.Read(buf)
			return buf[0]
		},
		DisplayFile: func(path string) {
			data, err := os.ReadFile(path)
			if err != nil {
				return
			}
			_, _ = rw.Write(data)
		},
		Hangup: func() {
			// The session layer handles the actual disconnect;
			// we just signal quit via the interpreter's signal mechanism.
		},
		UserName:    userName,
		UserCity:    userCity,
		UserSec:     userSec,
		UserTimesOn: timesOn,
		NodeNum:     nodeNum,
		BBSName:     bbsName,
		SysopName:   sysopName,
	}
}

// readLineRaw is a fallback line reader for tests/tools without a session.
func readLineRaw(rw io.ReadWriter, prompt string, cp437Out bool) string {
	if prompt != "" {
		_, _ = io.WriteString(rw, ansi.EncodeOutput(prompt, cp437Out))
	}
	var buf []byte
	single := make([]byte, 1)
	for {
		n, err := rw.Read(single)
		if n > 0 {
			b := single[0]
			if b == '\r' || b == '\n' {
				break
			}
			if b == 0x08 || b == 0x7F {
				if len(buf) > 0 {
					buf = buf[:len(buf)-1]
				}
				continue
			}
			if b < 0x20 {
				continue
			}
			buf = append(buf, b)
		}
		if err != nil {
			break
		}
	}
	return string(buf)
}
