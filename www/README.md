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

Also configurable in the Sysop GUI **Config** tab (Web Port, Web Bind, Web Root).

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

### Tier 2 routes (v1.0.2+)

| Route | Purpose |
|-------|---------|
| `/qwk` | Download QWK packet, upload REP replies |
| `/subscriptions` | Subscribe/unsubscribe echo conferences |
| `/search?q=` | Search messages and files |
| `/share/create` | POST — create 7-day public share link |
| `/shared/{key}` | Public shared message or file download |
| `/api/notify` | JSON unread counts (nav badge polling) |
| `/manifest.webmanifest` | PWA manifest |
| `/register` | New user signup (Telnet **NEW** equivalent) |
| `/register/welcome` | Post-registration NEWUSER display |

### Tier 3 routes (v1.1.0+)

| Route | Purpose |
|-------|---------|
| `/forgot-password` | Request password reset link |
| `/reset-password?token=` | Set new password |
| `/addressbook` | Personal FTN/contact address book |
| `/netmail/app` | Netmail SPA (list, read, compose) |
| `/api/netmail` | JSON netmail list/read |
| `/api/netmail/compose` | POST — queue outbound netmail |
| `/api/stream` | SSE unread notification stream |
| `/admin` | Sysop admin hub |
| `/admin/binkp` | BinkP poll, stats, log |
| `/admin/users` | User list, password reset, delete |
| `/admin/nodes` | Online nodes, kick |
| `/set-locale` | Language preference (en/es) |

## Feature checklist

Legend: `[x]` done · `[ ]` not yet implemented

### Tier 1 — Core (shipped)

- [x] Login / logout + server-side cookie sessions
- [x] Dashboard / main menu (`/menu`)
- [x] Message areas: list conferences, read, post/reply
- [x] File areas: browse, download
- [x] User profile (`/profile`)
- [x] Who's online (`/online`)
- [x] LOGON display + bulletins list/view (`/bulletins`, `/bulletins/view`)
- [x] Stats summary on dashboard + `/stats` page
- [x] Sysop panel on dashboard (links, not full admin)
- [x] WriteHeader/render fix (buffered template output)
- [x] Reply quoting on post (prefill `Re:` subject + quoted body)

### Tier 2 — Extended web features

- [x] QWK web UI (`/qwk` — download QWK, upload REP)
- [x] Echo subscriptions / area management (`/subscriptions`)
- [x] Shared message/file links (public `/shared/{key}` URLs, 7-day expiry)
- [x] Full-text message search (`/search` — messages + files)
- [x] Unread notification badges (nav badges via `/api/notify` polling)
- [x] PWA (manifest + service worker for static asset cache)
- [x] New user self-registration (`/register` — same as Telnet **NEW**)

### Tier 3 — Extended (partial)

- [x] Forgot / reset password (`/forgot-password`, `/reset-password`)
- [x] Address book (`/addressbook`)
- [x] Dedicated netmail SPA (`/netmail/app`)
- [x] Realtime SSE notifications (`/api/stream`)
- [x] BinkP admin web panel (`/admin/binkp`)
- [x] Sysop admin web panel — users, nodes, hub (`/admin/*`; full config via GUI)
- [x] i18n — English + Spanish (`/set-locale`, nav strings)

### Tier 3 — Still out of scope

- [ ] Doors / games (WebDoor, DOS, xterm)
- [ ] Chat rooms
- [ ] Shoutbox / polls
- [ ] Credits / economy
- [ ] PGP / keyserver
- [ ] AI assistant / MCP / bots
- [ ] PacketBBS / mesh
- [ ] Interests onboarding
- [ ] Ads / broadcasting
- [ ] Multiple themes / appearance editor

## Related docs

- `BUILDING.md` — build instructions and default ports
- `VirtBBS.DAT.example` — sample configuration
- Terminal parity reference: `internal/session/session.go` (login flow, stats, bulletins)
