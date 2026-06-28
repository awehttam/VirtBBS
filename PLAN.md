# VirtBBS — Go Rewrite of PCBoard 15.3

> **Note:** This document is a historical planning artifact from the initial rewrite.
> The original plan called for a separate Fyne sysop GUI (`virtbbs-gui`); that was
> superseded by the built-in web admin (`internal/web`, `/admin/*`) in v1.1–v1.3.
> User access is via Telnet/SSH, the web UI, and the VirtAnd Android client — not
> a dedicated desktop terminal app.

## Context

PCBoard 15.3 is a classic DOS BBS (~568k lines, Borland C/C++, 1993–1996). The user wants a clean from-scratch rewrite in Go named **VirtBBS**, treating PCBoard as a reference implementation. All modem I/O is replaced by built-in Telnet and SSH servers. Two executables: a headless BBS server and a remote-capable sysop/admin GUI (Fyne, pure Go).

---

## Project Layout

Create `/Volumes/JohnDovey/Projects/BBS/VirtBBS/` with the following structure:

```
VirtBBS/
├── cmd/
│   ├── virtbbs/          # BBS server executable (Go)
│   │   └── main.go
│   └── virtbbs-gui/      # Sysop GUI executable (Fyne, pure Go)
│       └── main.go
├── internal/
│   ├── telnet/           # Telnet server + IAC negotiation
│   ├── sshsrv/           # SSH server (golang.org/x/crypto/ssh)
│   ├── session/          # Per-user session state machine
│   ├── auth/             # User authentication
│   ├── users/            # SQLite user store + USERS binary importer
│   ├── messages/         # Message base (SQLite + MSGS importer)
│   ├── conferences/      # Conference management (CNAMES importer + SQLite)
│   ├── files/            # File directory, upload/download (DIR.LST)
│   ├── transfer/         # Pure-Go Zmodem (upload + download)
│   ├── ppl/              # PPL interpreter (PPE execution)
│   ├── fido/             # FidoNet message routing
│   ├── node/             # Multi-node status (SQLite)
│   ├── config/           # VirtBBS.DAT config + PCBOARD.DAT importer
│   ├── ansi/             # ANSI escape sequence renderer
│   ├── callers/          # Callers log
│   └── api/              # JSON-over-TCP API for remote GUI communication
├── pkg/
│   └── pcbformat/        # Shared PCBoard binary format decoders
│       ├── float4.go     # 4-byte BASIC single-precision float conversion
│       ├── strings.go    # Space-padded fixed-length string helpers
│       └── dates.go      # YYMMDD / Julian date helpers
├── gui/                  # Fyne sysop console
│   └── windows/          # Individual GUI panels
├── go.mod
└── README.md
```

---

## Architecture

### BBS Server (`cmd/virtbbs`)
- Pure Go, no cgo
- **Telnet**: TCP listener, configurable port, default **2323**
- **SSH**: TCP listener, configurable port, default **3232** (golang.org/x/crypto/ssh)
- Both Telnet and SSH ports configured in `VirtBBS.DAT` (with CLI flag override)
- One goroutine per connected user session
- Session state machine: LOGIN → MAIN_MENU → CONFERENCE → MESSAGE → FILE → DOOR → LOGOFF
- Multi-node coordination via SQLite (replaces USERNET.XXX binary file)
- Exposes a JSON-over-TCP management API (`internal/api`) for remote GUI

### Telnet Layer (`internal/telnet`)
- RFC 854 Telnet + IAC negotiation
- Negotiate: ECHO, SGA, NAWS (window size), TTYPE (terminal type)
- Wrap `net.Conn` → expose `io.ReadWriter` to session layer

### SSH Layer (`internal/sshsrv`)
- `golang.org/x/crypto/ssh` server
- Allocate PTY, negotiate terminal type
- Same session state machine as Telnet — shared via common `io.ReadWriter` interface

### Configuration (`internal/config`)
- **VirtBBS.DAT** — TOML format (replaces PCBOARD.DAT line-oriented format)
- Key fields:
  ```toml
  [network]
  telnet_port = 2323
  ssh_port    = 3232
  api_port    = 9999       # remote GUI API port
  api_bind    = "0.0.0.0" # allow remote connections

  [paths]
  db       = "./data/virtbbs.db"
  files    = "./files"
  logs     = "./logs"

  [sysop]
  name     = "Sysop"
  password = "..."   # bcrypt hash

  [bbs]
  name        = "My VirtBBS"
  max_nodes   = 10
  ```
- **Importer**: `pcbformat` package reads PCBOARD.DAT line-by-line and produces a VirtBBS.DAT
- **Sysop CRUD via GUI**: all config fields editable in GUI Settings panel; GUI saves updated TOML and signals server to reload (via API)

### Data Storage
| Data | Storage | Importer |
|---|---|---|
| Users | SQLite (`users` table) | Reads 400-byte USERS records + USERS.INF |
| Messages | SQLite | Reads 128-byte MSGS header/body blocks |
| Conferences | SQLite + TOML config | Reads CNAMES.@@@ variable records |
| Callers log | Append-only text file | Direct compat with PCBoard format |
| Node status | SQLite `nodes` table | N/A (runtime only) |
| Config | VirtBBS.DAT (TOML) | Reads PCBOARD.DAT line-by-line |

### File Transfer (`internal/transfer`)
- **Pure Go Zmodem** implementation (no external `sz`/`rz` binaries)
  - Implements ZRQINIT/ZRINIT/ZFILE/ZDATA/ZEOF/ZFIN packet flow
  - CRC-16 and CRC-32 modes
  - Crash recovery (resume partial downloads)
- Xmodem/Ymodem as fallback (simpler, also pure Go)
- Protocol selected per-session (stored in user record)

### Remote GUI API (`internal/api`)
- JSON-over-TCP on configurable port (default 9999), bind address configurable (default `0.0.0.0` for remote access)
- TLS optional (self-signed cert generated on first run)
- Authentication: sysop username + password (bcrypt)
- Endpoints (JSON RPC style):
  - `nodes.list` — active node status
  - `users.list/get/update/delete`
  - `messages.list/get/delete`
  - `conferences.list/get/update`
  - `callers.list` — recent callers log
  - `config.get/update` — VirtBBS.DAT CRUD
  - `node.chat` — send message to a node
  - `node.kick` — disconnect a node

### Sysop GUI (`cmd/virtbbs-gui`)
- **Library**: [Fyne](https://fyne.io) v2 — pure Go, cross-platform (macOS/Linux/Windows), no cgo
- Connects to BBS server API over TCP (remote or local); connection host/port/credentials in GUI preferences
- **Panels**:
  - Node Monitor — live node list (polls `nodes.list` every 2s)
  - User Manager — search/edit/delete users, reset passwords
  - Message Viewer — browse conferences and messages
  - Conference Editor — CRUD on conferences
  - Caller Log — scrollable recent callers
  - Sysop Chat — broadcast or DM to a node
  - Settings — full VirtBBS.DAT CRUD with form fields + Save button
  - Server Status — uptime, connected nodes count, version

---

## Key PCBoard Format Details to Implement

From `Develop 04-15-97/` documentation in `/Volumes/JohnDovey/Projects/BBS/PCBOARD/pcboard/`:

- **`pkg/pcbformat/float4.go`** — 4-byte BASIC single-precision float (message numbers, byte counts, dates)
- **`pkg/pcbformat/strings.go`** — space-padded fixed-length strings (NOT null-terminated)
- **Users (400 bytes/record)**: name[25], city[24], password[12], phone[13×2], lastdate[6], security level, conference bitmaps[5], etc.
- **USERS.INF**: extended per-user conference flags (up to 65,495 conferences)
- **Messages**: 128-byte header + 128-byte body blocks; MSGS.NDX index (2-byte record numbers)
- **CNAMES.@@@**: 2-byte record-size header per entry, variable-length conference records
- **PCBOARD.DAT**: 348+ line-oriented text file; importer reads sequentially by line number

---

## Implementation Phases

| Phase | Scope | Est. Duration |
|---|---|---|
| 1 | Scaffold, VirtBBS.DAT (TOML), Telnet server on port 2323, login prompt | Week 1–2 |
| 2 | SSH server on port 3232, shared session interface | Week 2–3 |
| 3 | SQLite user store, USERS importer, login/auth flow | Week 3–5 |
| 4 | Session state machine, ANSI menus, main command loop | Week 5–7 |
| 5 | SQLite message store, MSGS importer, read/enter/reply messages | Week 7–10 |
| 6 | File directories, pure-Go Zmodem up/download | Week 10–13 |
| 7 | Remote API server, Fyne GUI (node monitor, user manager, config CRUD) | Week 13–17 |
| 8 | PPL interpreter (port from SCREXEC.CPP / EVALP.CPP) | Week 17–22 |
| 9 | FidoNet import/export, PCBOARD.DAT importer | Week 22–26 |
| 10 | Multi-node chat, node kick, callers log | Week 26–28 |

---

## Technology Choices

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go 1.22+ | Clean concurrency; single binary; excellent cross-compile |
| DB | SQLite via `modernc.org/sqlite` | Pure Go driver (no cgo for server binary) |
| Telnet | Custom `internal/telnet` | Full IAC control, ~200 lines |
| SSH | `golang.org/x/crypto/ssh` | Stdlib-quality SSH server in pure Go |
| GUI | [Fyne v2](https://fyne.io) | Pure Go, cross-platform, no cgo, native widgets |
| Config | TOML (`github.com/BurntSushi/toml`) | Human-editable; replaces PCBOARD.DAT |
| Zmodem | Pure Go (custom `internal/transfer`) | No external binary dependency |
| Cross-compile | `GOOS/GOARCH` env vars | Trivial with pure Go stack |
| Testing | `go test` + golden files from real PCBoard data | Validate format decoders against real binaries |

---

## Reference Files During Implementation

| What | Source location |
|---|---|
| User record layout | `pcboard/Develop 04-15-97/USERS.DOC` |
| Message format | `pcboard/Develop 04-15-97/MSGS.DOC` |
| Conference format | `pcboard/Develop 04-15-97/CNAMES.DOC` |
| PCBOARD.DAT fields | `pcboard/Develop 04-15-97/PCBOARD.DOC` |
| 4-byte float | `pcboard/pcb-real/` |
| PPL interpreter | `pcboard15.3sourcev0.014/.../PPL/SCREXEC.CPP`, `EVALP.CPP` |
| Zmodem | `pcboard/pcb-misc/ZMODEM/` |
| ANSI rendering | `pcboard15.3sourcev0.014/.../DISPLAY/` |
| Node status format | `pcboard/pcb-misc/USERNET/` |

---

## Verification Plan

1. `telnet localhost 2323` → ANSI login screen with color
2. SSH client → `ssh user@localhost -p 3232` → same login flow
3. Import real USERS binary from reference PCBoard → confirm SQLite records match
4. Import real MSGS file → read messages back with correct from/to/subject/body
5. Zmodem download: transfer a known file, verify CRC and byte count match
6. GUI on remote machine: set host/port to BBS server IP → connects, shows live node list
7. GUI Settings: change BBS name, save → server reloads config, new callers see updated name
8. Cross-compile: `GOOS=windows GOARCH=amd64 go build ./cmd/...` and `GOOS=linux GOARCH=amd64 go build ./cmd/...`

---

## Out of Scope (for now)
- RIP graphics protocol
- UUCP/Usenet gateway
- Physical modem/COM port support (replaced by Telnet + SSH)
- Windows 9x/DOS compatibility of the new code
