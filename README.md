# VirtBBS

**VirtBBS** is a modern, from-scratch rewrite of the classic PCBoard BBS system. The server is written entirely in Go. It replaces modem-based dial-up access with Telnet and SSH servers, migrates all data to SQLite, and provides a full user and sysop experience through a built-in responsive web interface.

---

## Inspiration & Heritage

PCBoard was one of the most influential Bulletin Board Systems (BBS) of its era, developed by Clark Development Company from 1987 through the mid-1990s. At its peak, PCBoard powered thousands of boards worldwide and pioneered features such as conferences, file areas, door games, and the PCBoard Programming Language (PPL). VirtBBS draws directly on the PCBoard 15.3 source code and documentation as its reference implementation, reimagining the system for the modern era while preserving the familiar BBS experience.

The browser-based BBS UI was inspired in part by **[BinktermPHP](https://lovelybits.org/binktermphp)** — a modern web-first FTN platform that showed how echomail, netmail, file areas, and sysop tools can work well in a responsive HTML interface alongside classic Telnet/SSH access. VirtBBS pursues a similar goal in Go: native BinkP, a self-contained server, and no PHP stack required.

---

## How to Connect

| Client | How |
|--------|-----|
| **Web browser** | `http://localhost:8081/` — login, messages, files, netmail, QWK, profile, sysop admin |
| **Telnet** | `telnet localhost 2323` — full ANSI terminal session (SyncTerm, NetRunner, etc.) |
| **SSH** | `ssh -p 3232 user@localhost` — same session over SSH |
| **VirtAnd (Android)** | Point client using token auth + QWK/REP sync on port 9998 |

The **web UI** is the primary interface for everyday use and all sysop administration (`/admin/*`). Telnet and SSH remain for the classic terminal experience, PPL doors, and Zmodem transfers.

---

## Features

| Feature | Details |
|---------|---------|
| **Web interface** | Bootstrap 5 responsive UI — desktop, tablet, and mobile; English, Spanish, and Afrikaans |
| **Sysop admin (web)** | `/admin/*` — users, nodes, config, conferences, FidoNet, BinkP, callers, API tokens |
| **Telnet server** | RFC 854 with full IAC negotiation (character mode, echo, NAWS, TTYPE). Default port 2323 |
| **SSH server** | `golang.org/x/crypto/ssh`, PTY support. Default port 3232 |
| **User database** | SQLite — supports importer from PCBoard binary USERS file |
| **Message base** | SQLite with multi-conference support. Importer from PCBoard MSGS format |
| **File areas** | Per-directory file listings, upload/download tracking, Zmodem |
| **File catalog tools** | Sysop directory scan, `[E]dit desc`, daily `LOCALFIL.ZIP` (SLDIR-style listing) |
| **Stats** | `[S]tats` main-menu screen, PPL `GETSTATS`, web `/stats` |
| **Zmodem transfer** | Pure-Go Zmodem (no external `sz`/`rz`) — Telnet/SSH and web |
| **PPL interpreter** | Tree-walking interpreter for PCBoard Programming Language `.PPS` source files |
| **ANSI colour** | Full ANSI escape sequence rendering for menus and displays |
| **Multi-node** | Node status tracking via SQLite |
| **Callers log** | Compatible 64-byte record format |
| **FidoNet mail** | BinkP poll/server, toss/scan, AreaFix/FileFix, orphan-mail holding, multi-network |
| **QWK offline mail** | Real QWK/REP packets via `internal/qwk`; BBS menu, web `/qwk`, VirtAnd sync |
| **Netmail & address book** | Web netmail SPA, FidoNet address book, nodelist search/export |
| **i18n** | Web UI locales: `en`, `es`, `af`; user language persisted; experimental `^ALANG` kludge on outbound FTN mail |
| **PWA** | Installable web app with service worker and SSE notification badges |
| **Config** | TOML format `VirtBBS.DAT` (replaces PCBoard's line-oriented `PCBOARD.DAT`) |

---

## Project Structure

```
VirtBBS/
├── cmd/
│   └── virtbbs/           # BBS server (Telnet + SSH + API + userapi + web)
├── android/
│   └── VirtAnd/           # Android point client (Kotlin) — offline-first, QWK/REP sync
├── www/                   # Web UI templates and static assets (seeded from internal/web/defaults)
├── internal/
│   ├── api/               # JSON-over-TCP sysop management API (scripts/automation)
│   ├── fido/              # FidoNet BinkP, toss, scan, netmail, nodelist
│   ├── messages/          # Message base (SQLite)
│   ├── qwk/               # Real legacy QWK/REP binary offline-mail packets
│   ├── session/           # Per-user Telnet/SSH session state machine
│   ├── userapi/           # Token-authenticated JSON API (VirtAnd)
│   ├── web/               # Browser-based BBS UI and sysop admin (HTTP)
│   ├── telnet/            # Telnet server + IAC negotiation
│   ├── sshsrv/            # SSH server
│   ├── ppl/               # PPL interpreter
│   └── …                  # ansi, callers, conferences, config, files, node, users, …
├── pkg/pcbformat/         # PCBoard binary format decoders
├── ppe/                   # Sample .PPS source files
├── VirtBBS.DAT            # Configuration (TOML)
├── README.md
├── Installation.md
├── BUILDING.md
└── go.mod
```

See `www/README.md` for the full web route list and feature checklist.

---

## Architecture

### Server executable

**`virtbbs`** — single headless daemon:

- Telnet (2323) and SSH (3232) — one goroutine per caller
- Web UI (8081) — HTML templates + static assets from `paths.www`
- User API (9998) — VirtAnd token-authenticated JSON-over-TCP
- Sysop API (9999) — JSON-over-TCP for scripts/automation (optional; web admin uses Go handlers directly)
- FidoNet BinkP, toss/scan scheduler when `[fido] enabled = true`
- `--init-sysop` bootstraps the first sysop account

### Network ports (defaults)

| Service | Port |
|---------|------|
| Telnet | 2323 |
| SSH | 3232 |
| Sysop API | 9999 |
| User API (VirtAnd) | 9998 |
| Web UI (HTTP) | 8081 |
| BinkP (FidoNet, per network) | 24554 |

All ports are configurable in `VirtBBS.DAT`.

### Data storage

All persistent data lives in SQLite (`data/virtbbs.db` by default): users, messages, conferences, files, FidoNet nodelist, netmail queue, and callers log.

---

## Technology Stack

| Concern | Library/Tool |
|---------|--------------|
| Language (server) | Go 1.22+ |
| SQLite | `modernc.org/sqlite` (pure Go, no cgo) |
| SSH | `golang.org/x/crypto/ssh` |
| Web UI | Go `html/template`, Bootstrap 5, jQuery |
| Config | `github.com/BurntSushi/toml` |
| Passwords | `golang.org/x/crypto/bcrypt` |
| Android (VirtAnd) | Kotlin, Jetpack Compose, Room |

The server binary requires **no cgo** and cross-compiles cleanly for macOS, Linux, and Windows.

---

## Building from Source

See [BUILDING.md](BUILDING.md) for full instructions.

```bash
go mod tidy
go build ./cmd/virtbbs
./virtbbs -config VirtBBS.DAT
```

---

## Quick Start

See [Installation.md](Installation.md) for full instructions.

```bash
# 1. Initialise the sysop account
./virtbbs --init-sysop

# 2. Start the BBS server
./virtbbs

# 3. Connect via Telnet (optional)
telnet localhost 2323

# 4. Open the web UI (recommended)
open http://localhost:8081/
```

Log in with your BBS username and password. Sysops see **Admin** in the navigation bar.

---

## Documentation

| Document | Contents |
|----------|----------|
| [Installation.md](Installation.md) | Fresh install, upgrade, ports, Graphviz |
| [BUILDING.md](BUILDING.md) | Build, cross-compile, PCBoard import |
| [FidoNet Config.md](FidoNet%20Config.md) | FidoNet, BinkP, AreaFix, nodelist |
| [www/README.md](www/README.md) | Web UI routes, seeding, feature checklist |
| [CLAUDE.md](CLAUDE.md) | Notes for AI assistants and toolchain paths |
| [android/VirtAnd/README.md](android/VirtAnd/README.md) | Android point client |

---

## Versioning

| Component | Rule |
|-----------|------|
| Patch (x.x.**N**) | Bumped on every change or fix |
| Minor (x.**N**.0) | Bumped on significant feature additions |
| Major (**N**.0.0) | Bumped on explicit request |

Current version: **1.4.0**

---

## License

MIT License — Copyright (c) 2026 John Dovey <dovey.john@gmail.com>

See source file headers for full licence text.

---

## Acknowledgements

- **PCBoard BBS** — Clark Development Company. The PCBoard 15.3 source code and developer documentation were used as a reference implementation and format specification. No original PCBoard code is included in VirtBBS.

- **[BinktermPHP](https://lovelybits.org/binktermphp)** — by Matthew A. Wecht (awehttam) and the LovelyBits community. VirtBBS's web-first BBS design — responsive browser access to echomail, netmail, file areas, address books, shared links, and sysop administration alongside Telnet/SSH — draws inspiration from BinktermPHP's demonstration that modern FTN systems can serve users equally well through the web and the terminal. BinktermPHP is open source; see [github.com/awehttam/binkterm-php](https://github.com/awehttam/binkterm-php).
