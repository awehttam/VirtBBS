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
//   v0.10.0 2026-06-26  File catalog tools, stats, and display fixes: sysop file
//                        directory scan (registers disk files, flags missing
//                        entries, extracts FILE_ID.DIZ from ZIPs, counts scan
//                        adds as uploads); daily + on-change LOCALFIL.ZIP
//                        (SLDIR-style master listing with FILE_ID.DIZ, General
//                        file area); sysop [E]dit desc in the file menu; new
//                        [S]tats main-menu screen and PPL GETSTATS statement
//                        (session/account/system counters — see ppe/stats.md);
//                        sysop functions moved from 'S' to '!' on the main menu
//                        so 'S' is free for Stats; LOGON.ANS PCBoard ANSI
//                        expansion/CP437 decode/box-alignment fixes; stale
//                        node-row purge on list/startup; PPL input via session
//                        readline.
//   v0.11.0 2026-06-26  Real Zmodem file transfer support in VirtTerm and
//                        VirtTermMac: both clients now detect the server's
//                        download/upload Zmodem cues mid-stream, hand off to
//                        a new client-side Zmodem implementation (Terminal/
//                        Zmodem.cs, a C# port of this package), and prompt
//                        for a save folder / upload file via a native file
//                        picker. Found and fixed four real bugs in this
//                        package along the way, via a Go<->C# interop test
//                        harness: a deadlock in readHexFrame's trailing
//                        CR/LF/XON consumption; ZFILE/ZDATA's data subpacket
//                        never actually being read on the hex-header path
//                        (only the unused binary path read it); the
//                        subpacket CRC trailer bytes not being un-escaped
//                        like the payload; and ReceiveFile only reading one
//                        data subpacket per ZDATA header instead of the full
//                        run of subpackets a real transfer sends.
//   v0.11.1 2026-06-27  Fix the Stats screen's pager: the 23-line auto-pause
//                        landed wherever the count happened to fall, often
//                        mid-section. Added an explicit pause right before
//                        the first section header so the title/banner block
//                        always displays in full before the first pause.
//   v0.12.0 2026-06-27  Detect a vanished underlying volume (e.g. the USB/
//                        external drive VirtBBS runs from getting ejected)
//                        and exit gracefully instead of spinning forever.
//                        cmd/virtbbs/main.go now runs a watchVolume
//                        goroutine that os.Stat()s the database path every
//                        5s; after 3 consecutive failures it logs a clear
//                        diagnostic and exits, rather than leaving the
//                        process pegging the CPU and requiring a manual
//                        kill -9 (observed directly: ~200-250% CPU,
//                        unresponsive to plain SIGTERM).
//   v0.13.0 2026-06-27  VirtNet: this BBS can now be the authoritative hub
//                        of a FidoNet-compatible network (zone:net/node,
//                        no uplink configured), not just a leaf polling an
//                        uplink. New: self-service "apply to join" (profile
//                        menu) and sysop approval with net/node allocation
//                        (internal/fido/members.go, fido_join_requests/
//                        fido_members); a nodelist generator — the missing
//                        encoder half of nodelist.go, which only ever
//                        imported — producing a daily full
//                        (VirtNode.Z045) + diff (VirtNode.D045) nodelist
//                        from fido_members; a BinkleyTerm-style plain-text
//                        routing-table import/export alongside the
//                        DB-backed one; delegated sub-nets (a downstream
//                        member running its own VirtBBS as a sub-hub
//                        auto-announces nodes it registers to SysOp@hub via
//                        a new NODE ANNOUNCE netmail convention, mirroring
//                        AreaFix/Ping/Trace's existing dispatch patterns);
//                        auto-created "<Network> Nodelists" (echo) and
//                        "<Network> Sysops" conferences plus a "<Network>
//                        Nodelist Files" file area (first auto-creation of
//                        either anywhere in this codebase); downstream
//                        nodelist distribution reuses the existing
//                        downlink echomail fan-out unmodified, with
//                        auto-processing on arrival (writes the file,
//                        registers it, re-imports via the existing
//                        unmodified fido.ImportFile); a NodeChgs.txt change
//                        log and Graphviz-rendered network diagrams
//                        (full/hubs-only/per-hub — a Go rewrite of
//                        github.com/ftoledo's node2dot.py gist), all zipped
//                        with a FILE_ID.DIZ into the Nodelist Files area.
//   v0.13.1 2026-06-27  VirtNet: multiple addresses (AKAs) per network/
//                        downlink, the standard BinkleyTerm/FrontDoor
//                        convention (FTS-1026's M_ADR command always lists
//                        every address a system answers to, space-
//                        separated). A hub network now automatically also
//                        answers to zone:net/0 (e.g. 300:1/1 also answers
//                        to 300:1/0) without needing it configured
//                        explicitly, and any net's Host member gets the
//                        same zone:net/0 alias on their Downlink — "to
//                        make any host a hub, they need the additional /0
//                        address." New NetworkDef.AKAs/AllAddrs and
//                        Downlink.AKAs/MatchesAddr; binkp.go's two M_ADR
//                        sends now advertise every address, not just the
//                        primary one (incoming M_ADR parsing already
//                        split on whitespace into a list — only the
//                        sending/matching side was single-address).
//   v0.14.0 2026-06-27  VirtNet: ROUTES.BBS-style static routing table —
//                        wildcard address patterns (most specific wins:
//                        exact node > zone:net/* > zone:* > *) mapped to a
//                        "route via this address instead" next-hop, used
//                        when a destination isn't a direct Downlink — e.g.
//                        a node behind a delegated sub-hub gets physically
//                        handed to that sub-hub. Default net->Host (/0)
//                        routes auto-seed the moment a net gets a Host
//                        (ApproveJoinRequest/ApplyNodeAnnounceInfo).
//                        Exportable/importable as a literal ROUTES.BBS
//                        file. Wired into real outbound routing:
//                        RouteAddr now consults this table before falling
//                        back to the uplink (unchanged for direct
//                        Downlinks and the no-route-found case), and
//                        OutboundDir now uses a per-next-hop subdirectory
//                        whenever the resolved hop isn't the uplink, not
//                        only for crash netmail — confirmed by inspection
//                        that binkp.go's existing per-peer crash-
//                        subdirectory lookup needs no changes to pick this
//                        up automatically.
//   v0.15.0 2026-06-27  FidoNet orphan-mail holding: unknown echomail areas,
//                        netmail not addressed to this node, and failed DB
//                        inserts are saved as one-message .PKT files under
//                        <inbound>/.holding/ (or holding_dir) with ORPHANS.log
//                        and a sysop NetMail summary after each toss. Configurable
//                        primary network name (fido.name), tic_password,
//                        holding_dir; per-network default mail directories
//                        (fido/<Name>_inbound etc.) auto-created on config.save;
//                        fido.network.rename API + sysop GUI network name/rename.
//                        QWK offline mail: graphical reader in VirtTermMac and
//                        VirtTerm (Mail menu), BBS Messages [O]ffline (QWK)
//                        Zmodem menu, VirtAnd queue/compose/detail enhancements.
//                        Sysop GUI: scrollable edit panes, FidoNet tab fix,
//                        VirtNet secondary-network null-safe editing.
//   v0.15.1 2026-06-28  BinkP dedicated session log: all client/server/scheduler
//                        BinkP activity is written to <paths.logs>/binkp.log with
//                        RFC3339 timestamps (mirrored to stdout), fido.binkp.log
//                        API for tailing recent lines, and a sysop GUI FidoNet
//                        Operations "BinkP Log" tab; poll/toss/scan actions refresh
//                        the viewer and manual poll shows sent/received/toss counts.
//   v1.0.0  2026-06-28  First 1.0 release: BinkP stats (per network/day/month/year,
//                        uplink/downlink, midnight BINKPDAY/BINKPALL bulletins),
//                        node capability flags editor, local nodelist editor with
//                        import/export and NODEDIFF commit, FidoNet nodelist fetch
//                        scoped to primary network only, BinkP poll EOF fix for
//                        peers that close TCP after send, VirtTermMac (Avalonia
//                        terminal client), VirtTerm (WinForms), and VirtAnd
//                        (Android QWK/offline client).
//   v1.0.1  2026-06-28  Browser-based BBS web UI (internal/web, www/ templates):
//                        login with display-file bulletins, message read/post with
//                        quoted reply fix, file areas, netmail, stats, online users,
//                        and sysop dashboard; display/list.go for bulletin discovery;
//                        web_port/web_bind and paths.www config + sysop GUI fields;
//                        WriteHeader fix for template responses; scripts/build-release.sh.
//   v1.0.2  2026-06-28  Web UI Tier 2: QWK/REP, echo subscriptions, message/file
//                        search, shared links, nav unread badges, PWA manifest/service
//                        worker; new user registration (/register, users.RegisterNew);
//                        www/README feature checklist.
//   v1.1.0  2026-06-28  Web UI Tier 3: forgot/reset password, address book, netmail SPA,
//                        SSE notify stream, sysop admin (users/nodes/BinkP), i18n (en/es).
// ============================================================================

// Package version holds the VirtBBS version number.
// Increment rules:
//   - Patch: bump on every change/fix
//   - Minor: bump on significant feature additions or when directed
//   - Major: bump only on explicit request
package version

// Version is the current VirtBBS release version.
const Version = "1.1.0"
