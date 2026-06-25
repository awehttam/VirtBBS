# VirtBBS — Installation Guide

## Requirements

- macOS 12+, Linux (kernel 4.x+), or Windows 10+
- The `virtbbs` server ships as a self-contained binary — no additional runtime required
- The sysop GUI (`VirtBBS.GUI`) requires the [.NET 8 runtime](https://dotnet.microsoft.com/download/dotnet/8.0) on whichever machine runs it

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

From the `releases/0.0.8/` package, copy the following into your installation directory:

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

The sysop GUI (`VirtBBS.GUI`) is run separately from source — see [Open the Sysop Console (GUI)](#8-open-the-sysop-console-gui) below — and does not ship inside the `bin/` release package.

### 3. Review `VirtBBS.DAT`

Open `VirtBBS.DAT` in any text editor and adjust for your system:

```toml
[bbs]
name      = "My VirtBBS"   # Name shown to callers
max_nodes = 10              # Maximum simultaneous connections

[network]
telnet_port = 2323          # Telnet listen port
ssh_port    = 3232          # SSH listen port
api_port    = 9999          # Sysop GUI API port
api_bind    = "0.0.0.0"    # Bind address ("127.0.0.1" to restrict to localhost)

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
2026/06/25 12:00:00 VirtBBS 0.0.8 starting
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

### 8. Open the Sysop Console (GUI)

The sysop GUI (`VirtBBS.GUI`) is a .NET 8 / Avalonia UI application. It can run on the same machine as the server or on any machine with network access to the API port (default 9999), and requires the [.NET 8 SDK or runtime](https://dotnet.microsoft.com/download/dotnet/8.0).

From the `gui-dotnet/VirtBBS.GUI` directory:

```bash
dotnet run
```

The connection bar at the top of the window asks for:

| Field | Value |
|---|---|
| Host | `127.0.0.1` (or remote server IP) |
| Port | `9999` |
| User | Your sysop name |
| Pass | Your sysop password |

Click **Connect**. The Nodes, Users, Messages, Conferences, Callers, Config, and FidoNet tabs will populate with live data.

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

## Version

This guide covers VirtBBS **0.0.8**.
