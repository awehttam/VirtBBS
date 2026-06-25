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
//   v0.0.2  2026-06-24  Phase 10: multi-node chat, broadcast, node kick
//   v0.0.3  2026-06-24  Phase 9: MSGS importer, PCBOARD.DAT CLI, FidoNet PKT toss/scan
//   v0.0.4  2026-06-24  Phase 11: display files, last-read tracking, profile menu, idle/time limits
//   v0.0.5  2026-06-24  Phase 12/13/14: door games (PTY), full Zmodem rewrite, rich callers log
//   v0.0.6  2026-06-24  FidoNet: nodelist importer, in-BBS browser, netmail compose (crash/routed),
//                        BinkP poll client, multi-uplink echomail bundling, multi-network support,
//                        zone-aware routing, point address handling, conference echo flag editor,
//                        dynamic ANSI block-letter banner (shown at login if user.ANSI=true)
//   v0.0.7  2026-06-24  Sophisticated message editors: full-screen ANSI editor (SlyEdit-inspired)
//                        with word-wrap, cursor movement, cut/paste, insert/overwrite, quoted replies;
//                        simple line editor with /S /A /L /? slash commands; user-selectable via
//                        profile menu [M]; EditorType field added to user record
//   v0.0.8  2026-06-25  Replaced the Fyne sysop GUI with a .NET/Avalonia UI console; ANSI banner
//                        font now supports the apostrophe (') character; bumped golang.org/x/crypto
//                        and Go toolchain to clear Dependabot vulnerabilities
// ============================================================================

// Package version holds the VirtBBS version number.
// Increment rules:
//   - Patch: bump on every change/fix
//   - Minor: bump on significant feature additions or when directed
//   - Major: bump only on explicit request
package version

// Version is the current VirtBBS release version.
const Version = "0.0.8"
