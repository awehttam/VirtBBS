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
//   v0.1.0  2026-06-25  FidoNet message formatting: TZUTC kludge (netmail+echomail), standard
//                        tear line + Origin line replacing the non-standard \x01ORIGIN kludge,
//                        configurable echomail taglines, SEEN-BY/PATH construction and parsing
//                        with inbound merge-on-relay, MSGID-based toss dedupe, and a fix for the
//                        echomail resend-loop bug (messages are now marked exported and only
//                        written to a given uplink's PKT once)
//   v0.1.1  2026-06-25  Fix garbled box-drawing/special characters in SyncTerm and other CP437
//                        terminals by translating UTF-8 output to CP437 at every I/O funnel
//                        (session, editor, PPL); implement the FidoNet PING/PONG netmail test
//                        convention with automatic reply on toss and a sysop "Ping a node" menu
//                        option; add FidoNet Config.md documenting all [fido] settings
//   v0.2.0  2026-06-25  Implement AreaFix: responder for downlink subscribe/unsubscribe netmail
//                        requests (+TAG/-TAG/%LIST/%QUERY/%HELP) with automatic reply during toss,
//                        a request generator for subscribing to our own uplink's AreaFix, and
//                        scan-time fan-out of echomail to subscribed downlinks alongside the
//                        normal uplink packet; new [[fido.downlinks]]/areafix_password config and
//                        a sysop "AreaFix" submenu for managing downlinks and upstream requests
//   v0.3.0  2026-06-25  Automatic FidoNet poll/toss scheduler: every enabled network with a
//                        configured uplink is polled automatically (default every 6 hours,
//                        per-network override via poll_interval_mins, clamped to a 5-minute
//                        minimum), with a toss immediately following every poll — manual,
//                        API, and scheduled — via the new fido.PollAndToss. TossDir/TossFile,
//                        AutoRespondPing, and the AreaFix responder now take a *NetworkDef
//                        instead of *Config, so any configured network can be tossed, not
//                        just primary. No BinkP server exists (dial-out only) — documented
//                        as a known limitation in FidoNet Config.md §6.1.
//   v0.4.0  2026-06-25  BinkP server: VirtBBS now listens on every enabled network's
//                        binkp_port and answers inbound polls from configured uplinks/
//                        downlinks (handshake, password auth, file exchange tagged by
//                        destination address, auto-toss of what's received). TossAll
//                        tosses every enabled network in one pass (sysop/API/CLI toss
//                        commands are no longer primary-network-only). AreaFix downlinks
//                        are now genuinely per-network (Config.Downlinks vs each
//                        NetworkDef's own Downlinks). New FileFix (mirrors AreaFix for
//                        file areas via [fido.file_areas]/[[fido.downlinks]] — tracks
//                        subscriptions; no TIC distribution pipeline yet acts on them).
//                        New TRACE netmail utility (mirrors PING, replies with routing
//                        details). RequestAreaFix/SendPing now take *NetworkDef for full
//                        multi-network support. Documented DB auto-migration in
//                        Installation.md.
//   v0.5.0  2026-06-25  Automatic nodelist updates: per network, defaults to scanning
//                        https://www.darkrealms.ca/ for the current day's "Fidonet Daily
//                        Nodelist (Z1/ZIP) day NNN" link (the real download URL changes
//                        daily and isn't derivable from a fixed pattern), downloading and
//                        ZIP-sniffing the result (not trusting the URL's misleading
//                        extension), then importing via the existing ImportFile. New
//                        nodelist_url/nodelist_update_interval_hours config, a second
//                        per-network scheduler ticker, a sysop "[L]oad nodelist now" menu
//                        option, and a fido.nodelist.fetch API endpoint.
//   v0.5.1  2026-06-25  Fix a startup crash on pre-existing databases: schema.sql created
//                        idx_messages_fido_msgid before migrate()'s ALTER TABLE added the
//                        fido_msgid column, so any DB created before that column existed
//                        failed to open ("no such column: fido_msgid"). The index is now
//                        created in migrate(), after the column is guaranteed to exist.
//   v0.6.0  2026-06-26  Phase 0 of VirtAnd (Android point client) / VirtTerm (.NET
//                        graphical terminal client): new user_api_tokens table and
//                        users.Store.CreateAPIToken/AuthenticateToken/RevokeAPIToken/
//                        ListAPITokens, laying the groundwork for the new user-facing
//                        internal/userapi package these two new clients will use.
//   v0.7.0  2026-06-26  Phase 1 of VirtAnd/VirtTerm: real legacy QWK/REP binary
//                        packet support (new internal/qwk — BuildPacket/ParseRep/
//                        PostReplies) plus base64-in-JSON file content transfer
//                        and QWK download/upload endpoints in internal/userapi
//                        (qwk.download, qwk.upload, files.download, files.upload).
//   v0.8.0  2026-06-26  Phase 2 of VirtTerm: new internal/virtterm — a TLS
//                        terminal-transport listener (self-signed cert
//                        auto-generated like the SSH host key) that hands
//                        connections unmodified to the existing session.Run,
//                        wired into main.go alongside Telnet/SSH/userapi.
//   v0.9.0  2026-06-26  Sysop GUI gap-fill: tokens.list/tokens.revoke and
//                        fido.nodelist.version added to internal/api so the
//                        sysop GUI can administer VirtAnd/VirtTerm API tokens
//                        and see nodelist import status — both previously
//                        added server-side in Phases 0/2 but never surfaced
//                        in the GUI.
//   v0.9.1  2026-06-26  VirtTerm/VirtTermMac: new internal/userapi session.whoami
//                        endpoint so clients can show the logged-in user's name and
//                        the BBS name (e.g. in a window title bar) without scraping
//                        the terminal byte stream for it.
// ============================================================================

// Package version holds the VirtBBS version number.
// Increment rules:
//   - Patch: bump on every change/fix
//   - Minor: bump on significant feature additions or when directed
//   - Major: bump only on explicit request
package version

// Version is the current VirtBBS release version.
const Version = "0.9.1"
