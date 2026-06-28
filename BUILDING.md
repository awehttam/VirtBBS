# Building VirtBBS

## Prerequisites

- Go 1.22+ (`go version`) — builds the BBS server

## Quick start

```bash
# Fetch Go dependencies
go mod tidy

# Build the BBS server (no cgo, cross-compiles cleanly)
go build ./cmd/virtbbs

# Run the BBS server
./virtbbs -config VirtBBS.DAT
```

## Connecting

- **Telnet**: `telnet localhost 2323`  (or SyncTerm, NetRunner, etc.)
- **SSH**: `ssh -p 3232 username@localhost`
- **Web UI**: open `http://localhost:8081/` in a browser (login with your BBS username/password). Responsive Bootstrap 5 layout — works on desktop, tablet, and mobile.
- **Sysop admin**: log in as sysop on the web UI and open **Admin** in the navigation bar (users, nodes, config, FidoNet, and more)

Web UI design draws inspiration from [BinktermPHP](https://lovelybits.org/binktermphp); see the main README acknowledgements.

## Importing from PCBoard 15.3

```bash
# Import users from a PCBoard USERS binary file
./virtbbs -import-users /path/to/USERS

# Import messages from a PCBoard MSGS file into conference 0
./virtbbs -import-msgs /path/to/MSGS -conference 0

# Import config from PCBOARD.DAT
./virtbbs -import-config /path/to/PCBOARD.DAT -out VirtBBS.DAT
```

## Cross-compilation

```bash
# Windows (server, no cgo)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/virtbbs

# Linux AMD64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/virtbbs
```

## Default ports

| Service | Port |
|---|---|
| Telnet | 2323 |
| SSH | 3232 |
| Sysop API | 9999 |
| User API (VirtAnd) | 9998 |
| Web UI (HTTP) | 8081 |
| BinkP (FidoNet, per network) | 24554 |

Change Telnet/SSH/API ports in `VirtBBS.DAT` under `[network]`. FidoNet
mail directories (`fido/inbound`, `fido/<Name>_inbound`, etc.) are created
automatically when config is saved.

Web UI templates and CSS live under `paths.www` (default `www/`, relative to
the install directory where you launch `virtbbs`). On first start, built-in
defaults are copied into that folder if missing; edit them there to customise
the look and layout without rebuilding the server.
