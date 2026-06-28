# VirtBBS Web Interface

Browser-based BBS UI served by the Go server (`internal/web`). Templates and
static assets live in the install directory under `paths.www` (default `www/`),
relative to where you launch `virtbbs` — the same pattern as `display/` for
terminal screens.

## Configuration

In `VirtBBS.DAT`:

```toml
[network]
  web_port = 8081
  web_bind = "0.0.0.0"

[paths]
  www = "www"
```

Also configurable in **Admin → BBS config** (`/admin/config`).

Default URL: **http://localhost:8081/**

## Seeding and upgrades

Built-in defaults are embedded in the binary at `internal/web/defaults/` and
copied into your install `www/` on startup by `SeedWWW` — **only when a file
is missing**. Existing customisations are never overwritten.

After upgrading VirtBBS, if templates look stale, delete the affected files
under `www/` (or the whole `www/templates/` folder) and restart; missing files
will be re-seeded from the new defaults.

To customise the UI, edit files in your install `www/templates/` and
`www/static/` directly.

### User routes

| Route | Purpose |
|-------|---------|
| `/login`, `/logout` | Session login |
| `/register` | New user signup |
| `/menu` | Dashboard |
| `/messages`, `/files`, `/profile`, `/online` | Core BBS |
| `/qwk`, `/subscriptions`, `/search` | Extended features |
| `/netmail/app`, `/addressbook` | FidoNet user tools |
| `/nodelist` | Nodelist search and export |

### Sysop admin routes (`/admin/*`)

| Route | Purpose |
|-------|---------|
| `/admin` | Admin hub |
| `/admin/users`, `/admin/users/edit` | User list, edit, password, delete |
| `/admin/nodes` | Online nodes, kick, broadcast |
| `/admin/conferences` | Conference CRUD |
| `/admin/messages` | Message moderation (delete) |
| `/admin/files` | File area CRUD |
| `/admin/callers` | Callers log, search, daily stats |
| `/admin/config` | BBS config (ports, paths, session) |
| `/admin/tokens` | VirtAnd API token revoke |
| `/admin/fido` | FidoNet admin hub |
| `/admin/fido/ops` | Toss, scan, poll, nodelist, netmail |
| `/admin/fido/networks` | Per-network Fido config |
| `/admin/fido/routing` | Routes, members, import/export |
| `/admin/fido/join` | Hub join approve/deny |
| `/admin/fido/tools` | Ping, trace, AreaFix, FileFix |
| `/admin/binkp` | BinkP poll, stats, log |

## Feature checklist

Legend: `[x]` done · `[ ]` not yet implemented

### Tier 1 — Core (shipped)

- [x] Login / logout + server-side cookie sessions
- [x] Dashboard / main menu (`/menu`)
- [x] Message areas: list conferences, read, post/reply
- [x] File areas: browse, download
- [x] User profile (`/profile`)
- [x] Who's online (`/online`)
- [x] LOGON display + bulletins list/view
- [x] Stats summary on dashboard + `/stats` page
- [x] Sysop panel on dashboard + full `/admin/*` web admin
- [x] Reply quoting on post

### Tier 2 — Extended web features

- [x] QWK web UI (`/qwk`)
- [x] Echo subscriptions (`/subscriptions`)
- [x] Shared message/file links (`/shared/{key}`)
- [x] Full-text search (`/search`)
- [x] Unread notification badges (`/api/notify`, `/api/stream`)
- [x] PWA manifest + service worker
- [x] New user self-registration (`/register`)

### Tier 3 — Extended

- [x] Forgot / reset password
- [x] Address book (`/addressbook`)
- [x] Netmail SPA (`/netmail/app`)
- [x] BinkP admin (`/admin/binkp`)
- [x] Full sysop admin web panel (replaces former VirtBBS.GUI)
- [x] i18n — English + Spanish

### Former GUI parity (web admin)

- [x] Nodes: list, kick, broadcast
- [x] Users: list, edit, password, delete
- [x] Messages: list by conference, delete
- [x] Conferences: create, update, delete
- [x] File areas: create, update
- [x] Callers: recent, search, daily stats
- [x] BBS config editor
- [x] API tokens: list, revoke
- [x] FidoNet: networks, toss/scan/poll, nodelist, routing, join, tools

### Still out of scope (terminal-only or future)

- [ ] Doors / games (WebDoor, DOS, xterm)
- [ ] Chat rooms, shoutbox, polls, credits, PGP, AI/MCP, PacketBBS, ads

## Related docs

- `BUILDING.md` — build instructions and default ports
- `VirtBBS.DAT.example` — sample configuration
- Terminal parity reference: `internal/session/session.go`
