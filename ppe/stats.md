# VirtBBS Statistics

VirtBBS collects connection, message, and file-transfer statistics at several
levels. Most of this data was always recorded internally; PPL/PPE programs can
read it via the `GETSTATS` statement (added for `stats.pps`).

## What is tracked

### Per user (lifetime)

Stored in the `users` SQLite table and updated across calls.

| Field | Description |
|-------|-------------|
| `times_online` | Total number of logins |
| `uploads` / `downloads` | File transfer counts |
| `bytes_uploaded` / `bytes_downloaded` | Cumulative bytes transferred |
| `last_login_date` / `last_login_time` | Previous login timestamp |

### Per call (session)

Counted in memory during the call and written to the callers log on logoff.

| Field | Description |
|-------|-------------|
| Messages read | Messages viewed this session |
| Messages posted | Messages left this session |
| Files downloaded | Files downloaded this session |
| Files uploaded | Files uploaded this session |
| Duration | Session length in seconds |
| Time on / time left | Minutes on this call; minutes remaining if a per-call limit is set |

### Mail waiting

Per conference, the server compares each user's `last_msg_read` against the
highest message number in that conference. The sum across conferences is your
**new mail** count (same logic as the `*** You have N new message(s) ***`
banner at login).

### BBS today (callers log)

Derived from `CALLERS.DAT` (newline-delimited JSON companion to `CALLERS.LOG`).

| Stat | Description |
|------|-------------|
| Calls today | Total logins/logoffs recorded today |
| Unique users today | Distinct usernames seen today |

### Message base (system-wide)

| Stat | Description |
|------|-------------|
| Total messages | Non-deleted rows in `messages` |
| Conferences | Number of configured conferences |

## Where data lives

| Storage | Contents |
|---------|----------|
| `data/virtbbs.db` | Users, messages, conferences, per-user read pointers |
| `CALLERS.LOG` | Fixed-width text callers log (PCBoard-compatible) |
| `CALLERS.DAT` | Structured JSON callers log (duration, msgs read/left, files up/down) |

On logoff, the session writes a rich `LOGOFF` entry to both callers files with
that call's statistics.

## PPL / PPE: `GETSTATS`

Run **`GETUSER`** first (user identity and `NODENUM`), then **`GETSTATS`** to
load the counters into PPL variables.

### This call

| Variable | Type | Description |
|----------|------|-------------|
| `S_MSGREAD` | integer | Messages read this session |
| `S_MSGLEFT` | integer | Messages posted this session |
| `S_FILEDOWN` | integer | Files downloaded this session |
| `S_FILEUP` | integer | Files uploaded this session |
| `S_TIMEON` | integer | Minutes on this call |
| `S_TIMELEFT` | integer | Minutes remaining (0 if unlimited) |

### Your account

| Variable | Type | Description |
|----------|------|-------------|
| `U_UPLOADS` | integer | Lifetime upload count |
| `U_DOWNLOADS` | integer | Lifetime download count |
| `U_KUP` | integer | Kilobytes uploaded (bytes ÷ 1024) |
| `U_KDOWN` | integer | Kilobytes downloaded (bytes ÷ 1024) |
| `U_LASTDATE` | string | Last login date |
| `U_LASTTIME` | string | Last login time |
| `U_NEWMSG` | integer | Total new messages waiting (all conferences) |

`GETUSER` still provides: `U_NAME`, `U_CITY`, `U_SEC`, `U_TIMESON`, `U_MAILW`.

### System

| Variable | Type | Description |
|----------|------|-------------|
| `BBS_TODAYCALLS` | integer | Calls recorded today |
| `BBS_TODAYUNIQUE` | integer | Unique callers today |
| `BBS_MSGS` | integer | Total messages in the message base |
| `BBS_CONFS` | integer | Number of conferences |

BBS identity from `GETUSER` / builtins: `BBSNAME`, `SYSOPNAME`, `NODENUM`.

### Minimal example

```ppl
GETUSER
GETSTATS
PRINTLN "Welcome back, " + U_NAME + "!"
PRINTLN "You have " + STR(U_NEWMSG) + " new message(s)."
PRINTLN "Read " + STR(S_MSGREAD) + " messages so far this call."
END
```

## Main menu: `[S]tats`

The main BBS menu includes a **Stats** option (`S`) that shows the same
counters as `stats.pps`, rendered with ANSI colours (cyan headers, yellow
section titles, highlighted new-mail count). The screen **pauses every 23
lines** with “Press a key to continue…” before showing the next page. Sysop
functions moved to **`!`** on the main menu so `S` is available to all users.

## Sample program: `stats.pps`

`ppe/stats.pps` is a PPE equivalent using `GETUSER` and `GETSTATS`. It shows:

1. **This call** — node, time on/left, msgs read/posted, files up/down  
2. **Your account** — times on, last login, lifetime transfers, new mail  
3. **System today** — calls and unique users  
4. **Message base** — conference and message totals  

### Running it

From the sysop or user PPE menu:

```
PPE file path (.PPS): ppe/stats.pps
```

Requires a rebuilt server after `GETSTATS` was added (`go build ./cmd/virtbbs`).

### Display notes

- Box-drawing characters in `.PPS` sources should be **UTF-8** (as in
  `userinfo.pps` and `hello.pps`).
- **SSH** terminals receive UTF-8; **Telnet** clients (SyncTerm, etc.) receive
  CP437 translation automatically.

## Implementation reference

| Component | Role |
|-----------|------|
| `internal/session/session.go` | Session counters; `populatePplStats()` fills the PPL environment |
| `internal/ppl/interpreter.go` | `GETSTATS` statement |
| `internal/callers/log.go` | Callers log and `DailyStats()` |
| `internal/users/store.go` | `NewMessageCounts()` |
| `internal/messages/store.go` | `TotalCount()` |

## Related sysop views

The sysop menu and `VirtBBS.GUI` expose overlapping data (callers log,
user list, message areas). `GETSTATS` is the end-user/PPE-facing slice of the
same underlying counters — not a separate stats database.