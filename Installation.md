# VirtBBS — Installation Guide

## Requirements

- macOS 12+, Linux (kernel 4.x+), or Windows 10+
- The `virtbbs` server ships as a self-contained binary — no additional runtime required
- **[Graphviz](https://graphviz.org/)** (optional) — required on the **server** host if you run VirtNet nodelist day-rollover and want `VirtDiag.zip` to include PNG network diagrams. Without the `dot` binary on PATH, rollover still publishes the nodelist and change log; diagram generation is skipped with a warning in the server log.

### Installing Graphviz

VirtBBS shells out to Graphviz's `dot` command to render nodelist topology diagrams during VirtNet day-rollover. Install Graphviz on the machine that runs `virtbbs`, then verify:

```bash
which dot    # macOS/Linux
dot -V
```

On Windows (PowerShell):

```powershell
where.exe dot
dot -V
```

**macOS (Homebrew):**

```bash
brew install graphviz
```

Intel Macs usually install `dot` to `/usr/local/bin`; Apple Silicon Macs use `/opt/homebrew/bin`.

**Linux:**

| Distro | Command |
|---|---|
| Debian / Ubuntu | `sudo apt install graphviz` |
| Fedora / RHEL | `sudo dnf install graphviz` |
| Arch | `sudo pacman -S graphviz` |

**Windows:**

- [Graphviz installer](https://graphviz.org/download/) — check "Add Graphviz to the system PATH" during setup, or
- `winget install graphviz`, or
- `choco install graphviz`

After installing, **restart `virtbbs`** so the server process picks up the updated PATH. If you run VirtBBS as a system service (launchd, systemd) and diagrams are still skipped, ensure the service environment includes the directory containing `dot` (e.g. `/usr/local/bin` or `/opt/homebrew/bin` on macOS).

---

## Fresh Installation

### 1. Choose an installation directory

Pick any directory on the host machine. All VirtBBS runtime data is created relative to the working directory from which you launch the server.

```
Example: /opt/virtbbs      (Linux)
         C:\VirtBBS         (Windows)
         ~/bbs              (macOS)
```

### 2. Copy the release files

From the `releases/0.5.0/` package, copy the following into your installation directory:

```
<install-dir>/
├── bin/
│   ├── virtbbs          # BBS server  (Linux/macOS)
│   └── virtbbs.exe      # BBS server  (Windows)
├── ppe/
│   ├── hello.pps        # Sample PPE: Hello World
│   └── userinfo.pps     # Sample PPE: User Info display
└── VirtBBS.DAT          # Configuration file
```

On macOS/Linux, make the server binary executable:

```bash
chmod +x bin/virtbbs
```

The web-based sysop admin is served by the BBS server itself — no separate install step. See [Open the Sysop Admin (Web)](#8-open-the-sysop-admin-web) below.

### 3. Review `VirtBBS.DAT`

Open `VirtBBS.DAT` in any text editor and adjust for your system:

```toml
[bbs]
name      = "My VirtBBS"   # Name shown to callers
max_nodes = 10              # Maximum simultaneous connections

[network]
telnet_port = 2323          # Telnet listen port
ssh_port    = 3232          # SSH listen port
api_port    = 9999          # JSON/TCP management API (scripts/automation)
web_port    = 8081          # Browser BBS + sysop admin UI
web_bind    = "0.0.0.0"

[paths]
db    = "data/virtbbs.db"  # SQLite database path (relative to install dir)
files = "files"             # File transfer area root
logs  = "logs"              # Log file directory
```

### 4. Initialise the Sysop account

**This step must be completed before starting the BBS for the first time.**

From the installation directory, run:

```bash
bin/virtbbs --init-sysop
```

You will be prompted for:
- **Sysop name** — the sysop username callers will see (default: `Sysop`)
- **Password** — typed twice, hidden (no echo)

This command:
1. Creates the `data/` directory and SQLite database
2. Creates the sysop user record with security level 110 and sysop flag
3. Writes the bcrypt password hash into `VirtBBS.DAT` for the API

Example:

```
=== VirtBBS First-Run Sysop Setup ===
Sysop name [Sysop]: SysAdmin
Password:
Confirm password:
Sysop account 'SysAdmin' created.
VirtBBS.DAT updated with sysop credentials.
Setup complete — you can now start VirtBBS normally.
```

### 5. Start the BBS server

```bash
bin/virtbbs
```

The server will log startup messages:

```
2026/06/25 12:00:00 VirtBBS 0.5.0 starting
2026/06/24 12:00:00 Telnet listening on :2323
2026/06/24 12:00:00 SSH listening on :3232
2026/06/24 12:00:00 Management API listening on 0.0.0.0:9999
```

### 6. Test a Telnet connection

```bash
telnet localhost 2323
```

You should see the VirtBBS login prompt. Log in with the sysop name and password you set in Step 4.

> **Note:** If your Telnet client requires you to set terminal mode, run `mode line` BEFORE connecting (Windows), or ensure your client sends character-at-a-time. Modern clients (PuTTY, NetRunner, SyncTerm) handle this automatically.

### 7. Test an SSH connection

```bash
ssh -p 3232 YourSysopName@localhost
```

Accept the host key fingerprint on first connection. SSH does not require any special terminal mode configuration.

### 8. Open the Sysop Admin (Web)

The built-in web UI includes a full sysop administration panel. Log in with your sysop account and open **Admin** in the navigation bar, or go directly to:

```
http://localhost:8081/admin
```

From there you can manage users, online nodes, BBS configuration, conferences, file areas, callers log, FidoNet settings, and VirtAnd API tokens. See `www/README.md` for the complete route list.

The JSON/TCP management API (port 9999) remains available for scripts and automation; the web admin calls the same underlying server functions directly.

---

## Directory Layout After First Run

```
<install-dir>/
├── bin/
│   └── virtbbs
├── data/
│   ├── virtbbs.db        # SQLite database (users, messages, conferences, files)
│   └── host_key.pem      # SSH host key (auto-generated on first start)
├── files/                # File transfer area (subdirs created per file directory)
├── logs/
│   └── callers.log       # Callers log
├── ppe/
│   ├── hello.pps
│   └── userinfo.pps
└── VirtBBS.DAT
```

---

## Importing from PCBoard

### Import Users

```bash
bin/virtbbs import-users /path/to/PCBOARD/USERS
```

> **Note:** Imported users have their password set to a placeholder. The sysop must reset each user's password via the GUI (Users tab → Set Password) or the user can be prompted to set a new password on next login.

### Import PCBOARD.DAT Configuration

```bash
bin/virtbbs import-config /path/to/PCBOARD.DAT
```

This reads key fields (BBS name, sysop name, max nodes) and updates `VirtBBS.DAT`.

---

## Running as a System Service

### macOS (launchd)

Create `/Library/LaunchDaemons/io.virtbbs.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>         <string>io.virtbbs</string>
  <key>ProgramArguments</key>
  <array>
    <string>/opt/virtbbs/bin/virtbbs</string>
  </array>
  <key>WorkingDirectory</key> <string>/opt/virtbbs</string>
  <key>RunAtLoad</key>        <true/>
  <key>KeepAlive</key>        <true/>
  <key>StandardOutPath</key>  <string>/opt/virtbbs/logs/virtbbs.log</string>
  <key>StandardErrorPath</key><string>/opt/virtbbs/logs/virtbbs.log</string>
</dict>
</plist>
```

```bash
sudo launchctl load /Library/LaunchDaemons/io.virtbbs.plist
```

### Linux (systemd)

Create `/etc/systemd/system/virtbbs.service`:

```ini
[Unit]
Description=VirtBBS BBS Server
After=network.target

[Service]
Type=simple
User=bbs
WorkingDirectory=/opt/virtbbs
ExecStart=/opt/virtbbs/bin/virtbbs
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now virtbbs
```

---

## Firewall / Port Forwarding

To allow callers from the internet, open the following ports on your firewall/router:

| Port | Protocol | Purpose |
|---|---|---|
| 2323 | TCP | Telnet BBS access |
| 3232 | TCP | SSH BBS access |
| 9999 | TCP | Sysop API (restrict to trusted IPs) |

> **Security:** Consider binding `api_bind` to `127.0.0.1` if running the GUI only on the same machine, or use an SSH tunnel for remote access.

---

## Resetting the Sysop Password

If you forget the sysop password, re-run the init command from the installation directory:

```bash
bin/virtbbs --init-sysop
```

Enter the same sysop name and a new password. The record will be updated in the database and `VirtBBS.DAT` will be rewritten with the new hash.

---

## Upgrading / Database Schema Changes

**You don't need to run any migration command.** Every VirtBBS release that
adds new database columns or tables applies the change automatically, the
next time the server (or any CLI command like `--fido-toss`) opens the
database — there's no separate `migrate` step to remember.

How it works, if you're curious: each store (`messages`, `users`,
`conferences`, etc.) embeds a `schema.sql` with `CREATE TABLE IF NOT EXISTS`
statements (safe to re-run — a no-op if the table already exists), plus a
`migrate()` function that runs `ALTER TABLE ... ADD COLUMN ...` for any
columns added since your database was first created. If a column already
exists, SQLite's "duplicate column" error is caught and ignored, so it's
safe to run against a brand-new database, a database from three versions
ago, or anything in between.

To upgrade:

1. Stop the running `virtbbs` process.
2. Replace the `bin/virtbbs` binary with the new release.
3. Start it again — `bin/virtbbs` (or restart your system service).

That's it. New columns/tables appear automatically on that first startup;
existing data is untouched. There's no need to back up before upgrading
for schema reasons specifically, but as always, a copy of `data/virtbbs.db`
and `VirtBBS.DAT` before any upgrade is good practice.

If you ever see an error mentioning a missing column or table after
upgrading, it most likely means the **old** binary is still running
against a database a **newer** binary already migrated (or vice versa) —
make sure you've fully stopped the old process before starting the new one.

---

## Version

This guide covers VirtBBS **1.2.0**.
