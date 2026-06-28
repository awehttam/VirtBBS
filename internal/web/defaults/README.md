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

### Tier 2 — Deferred

- [ ] QWK web UI
- [ ] Echo subscriptions / area management
- [ ] Shared message/file links (public URLs)
- [ ] Full-text message search
- [ ] Unread notification badges (poll/SSE)
- [ ] PWA (manifest, service worker)

### Tier 3 — Out of scope for now

- [ ] Doors / games (WebDoor, DOS, xterm)
- [ ] Chat rooms
- [ ] Shoutbox / polls
- [ ] BinkP admin web panel
- [ ] Full sysop admin web panel (Avalonia GUI is primary)
- [ ] Credits / economy
- [ ] PGP / keyserver
- [ ] AI assistant / MCP / bots
- [ ] PacketBBS / mesh
- [ ] Interests onboarding
- [ ] Ads / broadcasting
- [ ] Dedicated netmail SPA (separate from conference messages)
- [ ] Address book
- [ ] Self-service registration / forgot-password
- [ ] Multiple themes / appearance editor
- [ ] i18n
- [ ] Realtime WebSocket/SSE (BinkStream-style)

## Related docs

- `BUILDING.md` — build instructions and default ports
- `VirtBBS.DAT.example` — sample configuration
- Terminal parity reference: `internal/session/session.go` (login flow, stats, bulletins)
