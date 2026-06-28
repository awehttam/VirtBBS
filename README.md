# VirtBBS

**VirtBBS** is a modern, from-scratch rewrite of the classic PCBoard BBS system. The server is written entirely in Go. It replaces modem-based dial-up access with Telnet and SSH servers, migrates all data to SQLite, and provides a full sysop administration interface in the built-in web UI.

---

## Inspiration & Heritage

PCBoard was one of the most influential Bulletin Board Systems (BBS) of its era, developed by Clark Development Company from 1987 through the mid-1990s. At its peak, PCBoard powered thousands of boards worldwide and pioneered features such as conferences, file areas, door games, and the PCBoard Programming Language (PPL). VirtBBS draws directly on the PCBoard 15.3 source code and documentation as its reference implementation, reimagining the system for the modern era while preserving the familiar BBS experience.

---

## Features

| Feature | Details |
|---|---|
| **Telnet server** | RFC 854 with full IAC negotiation (character mode, echo, NAWS, TTYPE). Default port 2323 |
| **SSH server** | `golang.org/x/crypto/ssh`, PTY support. Default port 3232 |
| **User database** | SQLite — supports importer from PCBoard binary USERS file |
| **Message base** | SQLite with multi-conference support. Importer from PCBoard MSGS format |
| **File areas** | Per-directory file listings, upload/download tracking |
| **File catalog tools** | Sysop directory scan (registers disk files, flags missing ones, pulls `FILE_ID.DIZ` from ZIPs), `[E]dit desc`, and a daily `LOCALFIL.ZIP` (SLDIR-style master listing) |
| **Stats** | `[S]tats` main-menu screen and PPL `GETSTATS` — session/account/system counters (see `ppe/stats.md`) |
| **Zmodem transfer** | Pure-Go Zmodem implementation (no external `sz`/`rz` binaries) — Telnet/SSH and web |
| **PPL interpreter** | Tree-walking interpreter for PCBoard Programming Language `.PPS` source files |
| **ANSI colour** | Full ANSI escape sequence rendering for menus and displays |
| **Multi-node** | Node status tracking via SQLite (replaces PCBoard's `USERNET.XXX`) |
| **Callers log** | Compatible 64-byte record format |
| **Web interface** | Browser-based BBS UI (`internal/web`) — login, messages, files, QWK, full sysop admin |
| **Sysop admin (web)** | `/admin/*` — users, nodes, config, conferences, FidoNet, callers, API tokens |
| **FidoNet mail** | BinkP poll/server, toss/scan, AreaFix/FileFix, orphan-mail holding, multi-network support |
| **QWK offline mail** | Real QWK/REP packets via `internal/qwk`; BBS `[O]ffline (QWK)` menu; web `/qwk`; VirtAnd sync |
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
│   ├── ansi/              # ANSI escape sequence helpers
│   ├── api/               # JSON-over-TCP sysop management API
│   ├── callers/           # Callers log (64-byte record format)
│   ├── conferences/       # Conference (message area) management
│   ├── config/            # VirtBBS.DAT TOML config + PCBoard importer
│   ├── files/             # File directory and transfer tracking
│   ├── messages/          # Message base (SQLite)
│   ├── node/              # Multi-node status tracking
│   ├── ppl/               # PCBoard Programming Language interpreter
│   │   ├── lexer.go       # Tokeniser
│   │   ├── parser.go      # Pratt parser → AST
│   │   ├── ast.go         # AST node types
│   │   ├── types.go       # PPL value types
│   │   ├── interpreter.go # Tree-walking interpreter + built-ins
│   │   └── runner.go      # High-level Run() / EnvFromSession()
│   ├── qwk/               # Real legacy QWK/REP binary offline-mail packets
│   ├── session/           # Per-user session state machine
│   ├── sshsrv/            # SSH server
│   ├── telnet/            # Telnet server + IAC negotiation
│   ├── transfer/          # Pure-Go Zmodem file transfer
│   ├── userapi/           # Token-authenticated JSON-over-TCP API (VirtAnd)
│   ├── users/             # User store + PCBoard USERS importer
│   ├── version/           # Version constant
│   └── web/               # Browser-based BBS UI (HTTP)
├── pkg/
│   └── pcbformat/         # PCBoard binary format decoders
│       ├── dates.go       # YYMMDD / HHMM helpers
│       ├── float4.go      # 4-byte BASIC single-precision float
│       └── strings.go     # Space-padded fixed-length strings
├── ppe/                   # Sample .PPS source files
├── releases/              # Versioned release packages (binaries + config)
├── update/                # Versioned update packages (upgrade from prior version)
├── VirtBBS.DAT            # Default configuration (TOML)
├── README.md
├── Installation.md
└── go.mod
```

---

## Architecture

### Server executable

**`virtbbs`** — The BBS server. Runs as a headless daemon:
- Listens for Telnet connections (port 2323) and SSH connections (port 3232)
- Serves one goroutine per connected user
- Maintains multi-node status in SQLite
- Exposes a JSON-over-TCP management API (port 9999) for scripts and automation
- Serves the browser-based BBS and sysop admin UI (port 8081)
- `--init-sysop` flag bootstraps the first sysop account

### Network Ports (defaults)

| Service | Port |
|---|---|
| Telnet | 2323 |
| SSH | 3232 |
| Sysop API | 9999 |
| User API (VirtAnd) | 9998 |
| Web UI (HTTP) | 8081 |

All ports are configurable in `VirtBBS.DAT`.

### Data Storage

All persistent data lives in a single SQLite database (`data/virtbbs.db`):

| Data | Table(s) |
|---|---|
| Users | `users`, `user_conferences` |
| Messages | `messages`, `conferences` |
| File directories | `file_dirs`, `files` |
| Node status | `nodes` (runtime only, cleared on restart) |

### PPL Interpreter

VirtBBS interprets PCBoard Programming Language (`.PPS`) source files directly using a tree-walking AST interpreter. This avoids the proprietary encryption used in compiled `.PPE` bytecode files. The interpreter supports:
- All control structures: `IF/ELSEIF/ELSE/ENDIF`, `WHILE/WEND`, `FOR/TO/STEP/NEXT`, `GOTO`, `GOSUB/RETURN`
- All PCBoard types: `INTEGER`, `STRING`, `BOOLEAN`, `REAL`, `DATE`, `TIME`, `MONEY`
- User variables: `U_NAME`, `U_CITY`, `U_SEC`, `U_TIMESON`
- BBS variables: `BBSNAME`, `SYSOPNAME`, `NODENUM`
- File I/O, string manipulation, math, date/time built-ins
- `DISPFILE`, `INPUTSTR`, `GETUSER`/`PUTUSER`, `HANGUP`, `LOG`, `DELAY`

---

## Technology Stack

| Concern | Library/Tool |
|---|---|
| Language (server) | Go 1.22+ |
| SQLite | `modernc.org/sqlite` (pure Go, no cgo) |
| SSH | `golang.org/x/crypto/ssh` |
| Config | `github.com/BurntSushi/toml` |
| Passwords | `golang.org/x/crypto/bcrypt` |

The server binary (`virtbbs`) requires **no cgo** and cross-compiles cleanly for macOS, Linux, and Windows.

---

## Building from Source

```bash
# Clone / enter the project
cd /path/to/VirtBBS

# Build the server (macOS native)
GOPATH=/Volumes/JohnDovey/go go build -o virtbbs ./cmd/virtbbs

# Cross-compile server for Linux amd64
GOOS=linux GOARCH=amd64 GOPATH=/Volumes/JohnDovey/go go build -o virtbbs-linux-amd64 ./cmd/virtbbs

# Cross-compile server for Windows amd64
GOOS=windows GOARCH=amd64 GOPATH=/Volumes/JohnDovey/go go build -o virtbbs-windows-amd64.exe ./cmd/virtbbs
```

---

## Quick Start

See [Installation.md](Installation.md) for full instructions.

```bash
# 1. Initialise the sysop account
./virtbbs --init-sysop

# 2. Start the BBS server
./virtbbs

# 3. Connect via Telnet
telnet localhost 2323

# 4. Open the web UI (user or sysop admin)
open http://localhost:8081/
```

---

## Versioning

| Component | Rule |
|---|---|
| Patch (x.x.**N**) | Bumped on every change or fix |
| Minor (x.**N**.0) | Bumped on significant feature additions |
| Major (**N**.0.0) | Bumped on explicit request |

Current version: **1.2.0**

---

## License

MIT License — Copyright (c) 2026 John Dovey <dovey.john@gmail.com>

See source file headers for full licence text.

---

## Acknowledgements

PCBoard BBS was created by Clark Development Company. The PCBoard 15.3 source code and developer documentation were used as a reference implementation and format specification. No original PCBoard code is included in VirtBBS.
