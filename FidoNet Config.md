# VirtBBS — FidoNet Configuration Guide

This guide covers every FidoNet setting in `VirtBBS.DAT`, how echomail/netmail
routing works, the BinkP server, how to add additional FidoNet-compatible
networks, AreaFix, FileFix, the PING/TRACE test utilities, and automatic
nodelist updates. It covers VirtBBS **1.6.0**.

---

## 1. Enabling FidoNet

All FidoNet settings live under the `[fido]` table in `VirtBBS.DAT`:

```toml
[fido]
  name         = "FidoNet"          # display name for the primary network (default "FidoNet")
  enabled      = true
  address      = "1:1/1"
  uplink       = "1:1/100"
  password     = ""
  inbound_dir  = "fido/inbound"
  outbound_dir = "fido/outbound"
  nodelist_dir = "fido/nodelist"
  holding_dir  = ""                 # optional; default is <inbound_dir>/.holding
  binkp_port   = 24554
  taglines_file = ""
  areafix_password = ""
  filefix_password = ""
  tic_password = ""                 # password WE send to OUR uplink's TIC processor

  [fido.areas]
```

| Field | Meaning |
|---|---|
| `name` | Display name for the **primary** network. Defaults to `"FidoNet"` if blank. Renamable via the sysop web admin or `fido.network.rename` API. |
| `enabled` | Master on/off switch. When `false`, all FidoNet menus, the toss/scan/poll commands, and the management API's `fido.*` endpoints refuse to run. |
| `address` | **This BBS's own FidoNet address**, in `zone:net/node` or `zone:net/node.point` form (e.g. `1:234/567` or `1:234/567.1` for a point system). |
| `uplink` | The address of the system this BBS exchanges mail with — your boss node or hub. All routed (non-crash) netmail and all echomail go here. |
| `password` | The session/packet password shared with your uplink. Leave blank if your uplink doesn't require one. |
| `inbound_dir` | Directory `.pkt` files are read from when tossing (see §4). Created automatically if missing. |
| `outbound_dir` | Directory `.pkt` files are written to when scanning/sending (see §5). Created automatically if missing. |
| `nodelist_dir` | Directory containing `NODELIST.*` files for address lookups (sysop name, BBS name, phone, flags) shown in the in-BBS nodelist browser and used by `[I]Ping a node`. |
| `holding_dir` | Optional override for where **orphaned** inbound mail is held for sysop review (§4.1). Blank = `<inbound_dir>/.holding`. |
| `binkp_port` | TCP port used both when **polling** your uplink over BinkP, and **listened on** for inbound BinkP connections from your uplink/downlinks (§6.1). Defaults to `24554` if zero/unset. |
| `taglines_file` | Optional path to a text file, one tagline per line. A random line is inserted above the tear line on every outgoing echomail message. Leave blank to disable. |
| `areafix_password` | Password **we** send when requesting areas from **our own uplink's** AreaFix — see §8.4. |
| `filefix_password` | Password **we** send when requesting file areas from **our own uplink's** FileFix — see §9. |
| `tic_password` | Password **we** send when requesting file transfers from **our own uplink's** TIC processor (FTS-5005). Downlinks authenticate with the same `password` as AreaFix/FileFix. See §9.3. |
| `poll_interval_mins` | Overrides how often the automatic scheduler polls this network's uplink, in minutes. `0`/unset = 6 hours. Any value below 5 is clamped up to 5 — see §6.2. |
| `nodelist_url` | Direct file URL or (primary FidoNet only) discovery page for automatic updates. Blank on additional networks = no automatic fetch — use Import File or the Local Nodelist editor. See §12. |
| `nodelist_update_interval_hours` | Overrides how often the scheduler fetches a fresh nodelist, in hours. `0`/unset = 24 hours. Any value below 1 is clamped up to 1 — see §12.2. |
| `[fido.file_areas]` | Maps FileFix tags to local file directory IDs — see §9.1. |
| `[fido.areas]` | Maps echomail `AREA:` tags to local conference IDs — see §3. |
| `[[fido.downlinks]]` | Systems that subscribe to our echomail areas via AreaFix — see §8.1. |
| `node_flags` | Capability flags for **this BBS's own** nodelist entry — see §1.1. |
| `binkp_host` | Hostname:port advertised in IBN/INA flags when those flags are enabled. |

> **Address format reminder:** VirtBBS only understands the standard 4D form `zone:net/node[.point]`. There is no separate "domain" field (e.g. `.fidonet.org`) — that's a FidoNet Internet-gateway concept VirtBBS does not implement.

### 1.1 Node capability flags (`node_flags`)

Each network entry can declare what services this node offers via standard FTS-0005 nodelist flags. Configure them in the sysop web admin **Network Setup → Node Capabilities** section, in `VirtBBS.DAT`, or via the `fido.network.flags.update` API.

| Flag | Meaning |
|------|---------|
| IBN | Internet BinkP Node — accepts BinkP connections; may include hostname and port |
| INA | Internet Address — hostname for internet connectivity |
| ITN | Internet Telnet Node |
| CM | Continuous Mail — accepts connections 24 hours |
| MO | Mail Only — no interactive users |
| BEER | Sysop drinks beer |
| TRACE | Trace requests honoured |
| PING | Ping requests honoured |

**Defaults for new VirtBBS networks:** `IBN`, `ITN`, `BEER`, `TRACE`, `PING`.

Example:

```toml
[fido]
  node_flags = ["IBN", "ITN", "BEER", "TRACE", "PING"]
  binkp_host = "bbs.example.com:24554"
```

When you save node capabilities (GUI **Save Node Capabilities** or `fido.network.flags.update`):

1. Flags are persisted in `VirtBBS.DAT` under `node_flags` / `binkp_host`.
2. The **local nodelist** (`fido_nodes` table) entry for this node's address is updated (system name, sysop, flags).
3. A **NODEDIFF** file (`NODEDIFF.DDD` in `nodelist_dir`) is written for this node only.
4. **Netmail** is queued to the configured **uplink** with subject `NODEDIFF for zone:net/node` and the diff in the body. Hub networks with no uplink skip netmail and keep the diff file locally.

API helpers:

- `fido.network.flags.list` — returns all known flags with descriptions (for the GUI).
- `fido.network.flags.update` — `{ "network": "FidoNet", "node_flags": ["IBN", ...], "binkp_host": "host:24554" }`.

---

## 2. Conferences and echomail flags

Each VirtBBS conference can be linked to a FidoNet echomail area independently
of the `[fido.areas]` map (this is the mechanism used by the **scan** step —
see §5). Configure it via the sysop **[E]cho flags** menu, or through the GUI's
FidoNet → Echo Flags tab, or the `conferences.update` API call:

| Field | Meaning |
|---|---|
| `Echo` | `true` marks this conference as an echomail area (vs. a local-only conference). |
| `EchoTag` | The `AREA:` tag for this conference, e.g. `VIRTBBS_SUPPORT`. Must match the tag your uplink/downlinks use for the same area. |
| `UplinkAddr` | Per-conference uplink override. Leave blank to use the network's default `uplink`. Useful if you receive different echo areas from different systems. |
| `Network` | Which configured network this conference's echomail belongs to. Leave blank (VirtBBS will store it as `"FidoNet"`) for the primary network, or set it to match a `[[fido.networks]] name` (see §6) for additional networks. |

---

## 3. `[fido.areas]` — inbound area routing (toss)

`[fido.areas]` is a simple map from `AREA:` tag to conference ID, used **only
by the toss step** (inbound mail) to decide which conference an incoming
echomail message belongs to:

```toml
[fido.areas]
  FIDO_GENERAL    = 1
  VIRTBBS_SUPPORT = 2
```

- The key is the exact `AREA:` tag as it appears in the inbound packet (case-sensitive, no `AREA:` prefix).
- The value is the numeric conference ID (see conference list in the sysop menu or `conferences.list` API).
- Any inbound echomail whose `AREA:` tag isn't listed here is **held for sysop review** in the network's holding directory (§4.1) rather than discarded.

> **Why two area mappings?** `[fido.areas]` (toss/inbound) and each conference's `EchoTag` field (scan/outbound) are independent on purpose — a conference can be a *recipient* of an echo area without VirtBBS originating/relaying traffic for it, and vice versa. For a normal two-way echo area, set up both: add the tag to `[fido.areas]` so inbound mail is filed correctly, **and** set the conference's `Echo=true`/`EchoTag` so locally-posted replies get scanned back out.

---

## 4. Tossing (processing inbound mail)

"Tossing" reads every `.pkt` file in `inbound_dir`, imports recognised
messages, and moves processed packets to `<inbound_dir>/.tossed/`.

Ways to trigger a toss:
- **In-BBS:** Sysop menu → FidoNet → `[T]oss inbound`
- **CLI:** `virtbbs -fido-toss`
- **API:** `fido.toss`

What happens during toss:
- **Netmail** (no `AREA:` line) addressed to **this node** is filed into conference 0 (General).
- **Netmail not addressed to this node** is held in the holding directory (§4.1) for sysop review — mail from anyone is accepted, not rejected at toss time.
- **Echomail** is routed via `[fido.areas]` (§3); unknown areas are held (§4.1), not discarded.
- Each message's `^AMSGID`, `SEEN-BY:`, and `^APATH` are parsed out and stored as structured metadata (not shown in the message body) so they can be correctly re-emitted if you relay the message onward. The tear line (`--- ...`) and `* Origin: ...` line are **kept visible** in the stored body, matching how real FidoNet readers display them.
- Duplicate packets (same `^AMSGID` re-processed twice, e.g. after a crash) are detected and skipped automatically.
- A netmail with **Subject `PING`** triggers an automatic `PONG` reply (§10), and **Subject `TRACE`** triggers a routing-info reply (§11).
- A netmail addressed to **"AreaFix"** (§8) or **"FileFix"** (§9) is processed by its responder.

### 4.1 Orphan / holding directory

Mail that cannot be imported automatically is saved as a one-message `.PKT`
file under the network's holding directory (default `<inbound_dir>/.holding/`).
Each hold is logged in `ORPHANS.log`. After toss, the sysop receives a
**NetMail summary** in conference 0 listing what was held and why.

Held when:
- Echomail `AREA:` tag is not in `[fido.areas]`
- Netmail destination address does not match this node (AreaFix/FileFix/PING/TRACE handled first)
- Database insert fails

Global fallback when the source network is unknown: `fido/holding/`.

---

## 5. Scanning (sending outbound echomail)

"Scanning" exports every not-yet-sent echo-flagged message into outbound
`.pkt` file(s) in `outbound_dir`, bundling multiple conferences addressed to
the same uplink into a single packet. Any AreaFix-subscribed downlinks (§8)
for an area also get their own packet automatically.

Ways to trigger a scan:
- **In-BBS:** Sysop menu → FidoNet → `[S]can outbound`
- **CLI:** `virtbbs -fido-scan`
- **API:** `fido.scan`

What gets added to each outgoing message automatically:
- `AREA:<tag>`, `^AMSGID`, `^ATZUTC` (your local UTC offset, e.g. `+0200`)
- `^ALANG: <code>` — experimental VirtBBS kludge with the author's UI language (`en`, `es`, `af`, …) on locally originated mail
- A random line from `taglines_file`, if configured
- A standard tear line (`--- VirtBBS <version>`) and Origin line (`* Origin: <BBS name> (<your address>)`)
- `SEEN-BY:` and `^APATH:` lines — merged with whatever was already present if the message arrived via toss and is being relayed onward, or starting fresh (just your own address) for locally-authored posts

Once a message has been successfully written into a packet, it is marked
internally so it will **not** be re-sent on the next scan — each message is
exported to a given uplink exactly once.

Netmail is sent immediately at compose time (not via the scan step) — see
the `[K]NetMail` option in the Messages menu, or `fido.netmail.send` via the
API.

### 5.1 File scan (TIC outbound)

**File scan** exports unexported files from `[fido.file_areas]` as FTS-5006
`.TIC` tickets plus payload copies in `outbound_dir`, fanning out to your
uplink and FileFix-subscribed downlinks (§9.3). Tracked in `fido_file_exports`.

Ways to trigger file scan:
- **In-BBS:** Sysop menu → FidoNet → `[O]` TIC file scan
- **CLI:** `virtbbs -fido-filescan`
- **Web:** FidoNet → Operations → **File scan (TIC)**, or **TIC** page

Run file scan after uploads land in a mapped file directory and before polling.

---

## 6. Polling your uplink (BinkP)

"Polling" connects to your uplink over BinkP, sends any outbound `.pkt`
files **and TIC/payload pairs**, and receives anything waiting for you. **Every poll is immediately
followed by a toss of that network's inbound directory** — whether
triggered manually, via the API, or by the automatic scheduler (§6.2) —
so newly-received mail (and anything left over from a previous partial
failure) is imported without a separate step. If the poll itself fails
(can't connect, auth rejected, etc.) the toss is skipped for that attempt.

Ways to trigger a poll:
- **In-BBS:** Sysop menu → FidoNet → `[P]oll uplink`
- **API:** `fido.poll` (params: `{"network": "<name>"}`, default network if blank)
- **Automatically:** see §6.2

### 6.1 BinkP server — do we listen for incoming connections?

**Yes.** VirtBBS runs a BinkP server alongside the dial-out client, so other
systems can poll *us* instead of only the reverse. It starts automatically
whenever `[fido] enabled = true`, listening on every distinct `binkp_port`
configured among your enabled networks (one network per port is typical,
but several networks can share a port — the caller's announced address
disambiguates which one they belong to).

How an inbound connection is handled:

1. The caller is identified by its `M_ADR` announcement and matched against every enabled network's configured `uplink` address and `[[fido.downlinks]]` list.
2. An unrecognised address is rejected (`M_ERR`) and logged.
3. If the matched link has a password (the network's own `password` if the caller is our uplink, or that specific downlink's `password`), the caller must supply it correctly via `M_PWD`, or the session is rejected.
4. Whatever the caller sends is received into that network's `inbound_dir`.
5. VirtBBS sends back whatever is queued for that specific caller:
   - **If the caller is a downlink:** only files the AreaFix scan-time fan-out (§8.3) tagged with that downlink's address, plus anything queued for them via crash-routed netmail.
   - **If the caller is our uplink:** everything in `outbound_dir` that *isn't* specifically tagged for one of our own downlinks, plus crash-routed netmail addressed to the uplink.
6. Successfully sent files are deleted; received files are immediately tossed (matching the "every poll completes with a toss" rule in §6).

Session activity and errors are written to **`binkp.log`** under your
configured `[paths] logs` directory (default `./logs/binkp.log`), with
RFC3339 timestamps. The same lines are also mirrored to the server's
stdout log. In the sysop web admin, open **FidoNet → Operations → BinkP Log**
to view recent sessions, or call the `fido.binkp.log` management API
(params: `{"lines": 200}`).

Lines are prefixed `binkp server:` or `binkp client:` / `fido scheduler:`
as appropriate, with the network name in brackets where relevant.

### 6.1.1 BinkP statistics and bulletins

VirtBBS records BinkP/FidoNet counters in SQLite (`fido_binkp_stats` and
`fido_binkp_link_stats`), aggregated per network by **day**, **month**,
**year**, and **all-time**. Counters include outbound poll success/failure,
inbound uplink/downlink sessions, files sent/received, netmail and echomail
sent/received, toss imported/skipped/held, and session errors. Per-link
breakdowns are kept for configured uplinks and downlinks.

- **web admin (`/admin/fido/*`):** FidoNet → Operations → **BinkP Stats** (period selector)
- **API:** `fido.binkp.stats` (params: `{"network":"","period":"day","period_key":"2026-06-27"}`)
- **ANSI bulletins:** at local midnight the server overwrites
  `<session.display_dir>/BINKPDAY.ANS` (previous calendar day's stats) and
  `<session.display_dir>/BINKPALL.ANS` (all-time). Both are also refreshed
  once at startup. Display them like any other PCBoard display file.

> **Limitation:** outbound routing is filename-based (matching the destination address tag scan.go embeds in the filename), not directory-based — there's no per-link outbound subdirectory structure (the BSO "outbound.flo" convention). This is sufficient for the AreaFix downlink fan-out this server was built to support, but isn't a full general-purpose FTN mailer's routing model.

### 6.2 Automatic scheduler

VirtBBS automatically polls every **enabled network that has a configured
uplink**, on a per-network schedule, with a toss immediately following each
poll (see above) — no manual action required. This starts automatically
when the server starts, as long as `[fido] enabled = true`.

- **Default interval: every 6 hours**, for every qualifying network.
- **Per-network override:** set `poll_interval_mins` under `[fido]` (primary network) or inside a `[[fido.networks]]` block, in minutes. Any value below 5 is clamped up to 5 — the scheduler will never poll more often than every 5 minutes.

```toml
[fido]
  ...
  poll_interval_mins = 30   # poll this network every 30 minutes instead of every 6 hours
```

Notes:
- A network with no `uplink` configured, or with `enabled = false`, is skipped — it gets no scheduler goroutine.
- Networks are detected once at server startup. A network added at runtime (e.g. via `config.update`) won't be scheduled until the server restarts; changes to an *existing* scheduled network's `enabled` flag, `uplink`, or `poll_interval_mins` take effect on that network's very next tick, no restart needed.
- Scheduler activity (poll/toss results, errors) is written to the server's standard log output, prefixed `fido scheduler: <network name>`.

---

## 7. Multiple networks

VirtBBS can participate in more than one FidoNet-compatible network (e.g.
FidoNet plus a regional/hobby net) at the same time. The top-level `[fido]`
table describes your **primary** network. Its display name defaults to
`"FidoNet"` but is configurable via `name = "..."`. Add others under
`[[fido.networks]]`:

```toml
[fido]
  name         = "FidoNet"
  enabled      = true
  address      = "1:1/1"
  uplink       = "1:1/100"
  inbound_dir  = "fido/inbound"
  outbound_dir = "fido/outbound"
  nodelist_dir = "fido/nodelist"

  [fido.areas]
    FIDO_GENERAL = 1

[[fido.networks]]
  name         = "LovelyNet"
  enabled      = true
  address      = "80:774/1"
  uplink       = "80:774/100"
  password     = ""
  inbound_dir  = "fido/LovelyNet_inbound"
  outbound_dir = "fido/LovelyNet_outbound"
  nodelist_dir = "fido/LovelyNet_nodelist"
  binkp_port   = 24554
  taglines_file = ""

  [fido.networks.areas]
    LOVELY_CHAT = 3
```

Each `[[fido.networks]]` entry is a **fully independent** network: its own
address, uplink, inbound/outbound directories, nodelist, and area map. Use
a distinct `inbound_dir`/`outbound_dir` per network so packets don't collide.

When adding a network via the sysop web admin, directories default to
`fido/<NetworkName>_inbound`, `_outbound`, and `_nodelist`. Saving config
(via GUI or `config.update` API) **creates** inbound, outbound, nodelist,
`.tossed`, and `.holding` directories automatically.

Rename a network via the sysop web admin **Networks** tab or the `fido.network.rename`
API — updates config and SQLite references (routes, members, subscriptions).

- **Scanning** (§5) iterates every enabled network and writes separate `.pkt` files for each — link a conference to a specific network via its `Network` field (§2) so the scanner knows which network's address/uplink to use for it.
- **Tossing** (§4) — `[T]oss inbound`, `fido.toss`, and `-fido-toss` all toss **every enabled network's** `inbound_dir` in one pass; there's no need to toss each network separately.
- **Polling** (§6) takes a `network` parameter specifically so you can poll each uplink independently, and the automatic scheduler (§6.2) runs one poll/toss cycle per network on its own schedule.
- **AreaFix/FileFix downlinks** (§8/§9) are configured **per network** — a `[[fido.downlinks]]` entry under `[fido]` only applies to the primary network; add a separate `[[fido.networks.downlinks]]` list for each additional network's own downlinks. The same physical system can be a downlink of more than one of your networks with completely independent subscriptions/passwords for each.

---

## 8. AreaFix

> See also: [`AreaFix FileFix TIC.md`](AreaFix%20FileFix%20TIC.md) for a focused guide to the three robots.

VirtBBS implements AreaFix in both directions: **responding** to subscription
requests from your downlinks, and **requesting** areas from your own uplink.

### 8.1 Configuring downlinks (systems that subscribe to your areas)

Add each downlink under `[fido]` (or `[[fido.networks]]` for a non-primary
network):

```toml
[fido]
  ...
  [[fido.downlinks]]
    name     = "Bob's BBS"
    address  = "1:2/4"
    password = "letmein"
```

| Field | Meaning |
|---|---|
| `name` | Display name only, shown in the sysop AreaFix menu. |
| `address` | The downlink's `zone:net/node`. Must match exactly (point ignored) for AreaFix requests from this system to be accepted. |
| `password` | What the downlink must supply as the first non-blank line of its AreaFix netmail. Leave blank to allow unauthenticated requests from this address (not recommended). |

There's no separate config needed for *which* areas a downlink can have —
any area with a matching `EchoTag` (§2) can be requested; VirtBBS validates
the tag against your conferences (or `[fido.areas]` as a fallback) before
accepting a subscription.

### 8.2 How the responder works

A downlink emails `AreaFix` at your address with a netmail body like:

```
letmein
+VIRTBBS_SUPPORT
-OLD_AREA
%QUERY
```

- The **first non-blank line** must match the downlink's configured `password` (or be skipped entirely if the downlink has no password configured).
- `+TAG` subscribes, `-TAG` unsubscribes, `%LIST` lists every area available, `%QUERY` lists current subscriptions, `%HELP` shows command help.
- VirtBBS replies immediately (not via the scan step) with a netmail confirming what changed and the resulting subscription list.
- The original request is also stored as ordinary netmail (conference 0) so the sysop can audit what's been requested.

This all happens automatically during **toss** (§4) — no extra step required.

### 8.3 How fan-out to downlinks works

Once a downlink is subscribed to an area, the **scan** step (§5) automatically
includes them: every time a message is exported for that area, VirtBBS writes
an additional `.pkt` addressed directly to each subscribed downlink, alongside
the normal one addressed to your uplink. No separate uplink override or
per-conference configuration is needed for this — it's purely subscription-
driven.

> **Note:** export tracking (`fido_exported_at`, §5) is per-message, not
> per-destination. If the uplink's packet write succeeds but a downlink's
> packet write fails (e.g. a permissions error), the message is still marked
> exported and will not be retried for that downlink on the next scan. This
> is a known simplification, not a typical real-world failure mode (the same
> directory and write path is used for both).

### 8.4 Requesting areas from your own uplink

If your uplink also runs AreaFix, VirtBBS can act as a downlink of theirs.
Set the password they issued you:

```toml
[fido]
  ...
  areafix_password = "whatever-your-uplink-gave-you"
```

Then, in-BBS: Sysop menu → FidoNet → `[A]reaFix` → `[U]pstream request` —
enter the area tags to subscribe/unsubscribe, space-separated. VirtBBS sends
the request immediately; your uplink's own AreaFix will reply by netmail
once it's processed it.

### 8.5 Sysop menu reference

Sysop menu → FidoNet → `[A]reaFix`:

| Key | Action |
|---|---|
| `[D]` | Add a downlink (name, address, password) — saved to `VirtBBS.DAT`. |
| `[R]` | Remove a downlink by address — clears AreaFix and FileFix subscriptions. |
| `[U]` | Send an AreaFix subscribe/unsubscribe request to your own uplink. |

The main listing shows each configured downlink alongside its current
subscriptions.

### 8.6 Downlinks on the web

Downlink maintenance is available at **`/admin/fido/downlinks`** (add/edit/remove, view AreaFix and FileFix subscriptions, nodelist type) as well as the in-BBS sysop AreaFix menu (`[D]`/`[R]`). Removing a downlink clears **both** AreaFix and FileFix subscription rows.

AreaFix works for every enabled network — pick which one in the `[A]reaFix` menu (§8.5) or the web network selector.

---

## 9. FileFix — file-area subscriptions

> Details: [`AreaFix FileFix TIC.md`](AreaFix%20FileFix%20TIC.md) § FileFix and § TIC.

FileFix is the file-echo equivalent of AreaFix (§8), with the identical
command protocol (`+TAG`/`-TAG`/`%LIST`/`%QUERY`/`%HELP`, password-first),
just addressed to **"FileFix"** instead of **"AreaFix"**, and operating on
file areas instead of echomail conferences.

### 9.1 Configuring file areas

Map FileFix tags to local file directory IDs (`internal/files.Dir.ID`, see
the sysop Files menu or `files.list` API for IDs):

```toml
[fido]
  ...
  [fido.file_areas]
    GAMES = 1
    UTILS = 2
```

Downlinks requesting file areas use the **same `[[fido.downlinks]]` list**
AreaFix uses (§8.1) — there's no separate downlink list for FileFix, since
it's the same remote system either way, just requesting a different kind
of area. Add/remove downlinks via the AreaFix menu, web **Downlinks** page,
or the downlinks textarea on the Networks page; manage file subscriptions
via the `[F]ileFix` menu or by having downlinks send FileFix netmail.

### 9.2 Sysop menu reference

Sysop menu → FidoNet → `[F]ileFix`:

| Key | Action |
|---|---|
| `[U]` | Send a FileFix subscribe/unsubscribe request to your own uplink. |

The main listing shows each configured downlink alongside its current
**file-area** subscriptions.

### 9.3 TIC — file distribution

VirtBBS implements FTS-5006-style **TIC** file echo distribution. See
[`AreaFix FileFix TIC.md`](AreaFix%20FileFix%20TIC.md) for full detail.

Configure `tic_password` (under `[fido]` or `[[fido.networks]]`) for the
password **this BBS sends** on outbound TIC tickets to its uplink. Downlinks
authenticate inbound TIC with the same `password` as AreaFix/FileFix in
`[[fido.downlinks]]`.

**Outbound:** run **file scan** after new uploads appear in a mapped file
area — sysop FidoNet `[O]`, web **Operations → File scan (TIC)**, web
**TIC** page, or `virtbbs -fido-filescan`. Unexported files (tracked in
`fido_file_exports`) are hatched as `.TIC` + payload pairs in
`outbound_dir`, fanning out to the uplink and FileFix-subscribed downlinks.

**Inbound:** `.TIC` files received via BinkP are processed automatically
during toss/poll (same pass as `.PKT` netmail). Payloads are installed into
the local directory mapped by the ticket's `Area` tag. Use web **TIC → Process
inbound** or `-fido-toss` to run TIC processing alone.

**BinkP:** outbound poll and inbound server sessions transfer `.tic` files
and their referenced payloads using the same address-tag routing as echomail
`.pkt` fan-out (§6).

### 9.4 Limitations

- Export tracking is per source file, not per destination (same simplification
  as echomail scan §8.3).
- File scan is manual/CLI — not scheduled automatically (mirrors echomail scan).

---

## 10. PING — netmail connectivity test

VirtBBS implements the long-standing FidoNet "ping" netmail convention (not
an official FTS standard, but widely supported by classic mailers): a
netmail with **Subject `PING`** sent to a node triggers an automatic
**Subject `PONG`** reply, confirming mail flow between two systems.

- **Sending a ping:** Sysop menu → FidoNet → `[I]Ping a node`, then enter the
  destination address (`zone:net/node`). VirtBBS looks up the sysop name from
  your local nodelist if available, builds a `PING` netmail, and routes it
  via your configured uplink immediately (no scan step needed).
- **Receiving a ping:** handled automatically during toss (§4) — any inbound
  netmail with Subject `PING` (matched case-insensitively) gets an immediate
  `PONG` reply queued to your outbound directory, addressed back to the
  sender, reporting the time it was received and the original PING's
  timestamp.
- **No loop risk:** the auto-responder only ever triggers on Subject `PING`
  exactly — it never replies to a `PONG`, so two systems both running the
  auto-responder won't ping-pong forever.

You can also originate a `PING` manually through the ordinary netmail
composer (`[K]NetMail`) by simply typing `PING` as the subject — the
dedicated `[I]Ping a node` menu option just saves a few steps (address
lookup + immediate send).

---

## 11. TRACE — routing diagnostics

TRACE mirrors PING (§10) exactly — same convention, same loop-safety — but
the automatic reply reports this system's own routing details (its
address, its configured uplink, and the BBS software it's running) rather
than just confirming receipt.

- **Sending a trace:** Sysop menu → FidoNet → `[X]Trace a node`, then enter the destination address. Works identically to `[I]Ping a node`.
- **Receiving a trace:** any inbound netmail with **Subject `TRACE`** gets an immediate **Subject `TRACE REPLY`** reply during toss, reporting this node's address and uplink.
- **No loop risk:** same guarantee as PING/PONG — the auto-responder only triggers on Subject `TRACE` exactly, never on `TRACE REPLY`.

> **Limitation:** this is a single-hop test, like PING — VirtBBS cannot
> orchestrate a true multi-system traceroute, since that requires every
> intermediate system along a route to cooperatively relay the TRACE
> onward and report back. The reply only ever describes the system you
> sent the TRACE directly to.

---

## 12. Automatic nodelist updates

VirtBBS can automatically keep each network's nodelist current, without
any manual download/import step.

For the full picture — including **VirtNet hub generation**, echomail
distribution, inbound echo processing, and SQLite storage — see
[`VirtNet Nodelist Processing.md`](VirtNet%20Nodelist%20Processing.md).
This section covers **FidoNet HTTP fetch** only.

### 12.1 How it works

By default (no configuration needed), VirtBBS fetches from
**`https://www.darkrealms.ca/`** — a publicly available FidoNet Zone 1
nodelist mirror — once per day, per enabled network:

1. The discovery page's HTML is scanned for a table row containing the text **"Fidonet Daily Nodelist (Z1/ZIP) day NNN"**; that row's download link is resolved into a full URL. The day number changes daily and the URL isn't derivable from a fixed pattern, so the page is scanned fresh on every fetch rather than guessed.
2. The linked file is downloaded and sniffed for a ZIP signature (not trusted by extension — darkrealms.ca serves its daily file under a `.Z##`-style name despite it being a ZIP archive).
3. If it's a ZIP, the nodelist file inside is extracted; otherwise the response is used as-is.
4. The result is imported via the same logic as a manual `[N]odelist` import, upserting into the nodelist database for that network.

### 12.2 Configuration

```toml
[fido]
  ...
  nodelist_url                   = ""   # blank = use the darkrealms.ca discovery page
  nodelist_update_interval_hours = 0    # 0 = scheduler default (24h); else clamped to >=1
```

| Field | Meaning |
|---|---|
| `nodelist_url` | Either a direct file URL (recognised by extension — `.zip`, `.lzh`, a classic `NODELIST.###`, or a `.Z##`-style suffix) downloaded as-is, or a discovery page (the default if left blank) scanned per §12.1. Override this if darkrealms.ca ever becomes unavailable or you prefer a different source/mirror. |
| `nodelist_update_interval_hours` | Overrides how often the scheduler fetches a fresh nodelist for this network. `0`/unset = 24 hours. Any value below 1 is clamped up to 1. |

Each `[[fido.networks]]` entry has its own independent `nodelist_url` /
`nodelist_update_interval_hours` — every enabled network gets its own
fetch schedule, same as the poll scheduler (§6.2).

### 12.3 Manual fetch

- **In-BBS:** Sysop menu → FidoNet → `[L]oad nodelist now` (pick a network if more than one is configured)
- **API:** `fido.nodelist.fetch` (params: `{"network": "<name>"}`, default network if blank)

### 12.4 Limitations

- Only full nodelist replacement is supported — no NODEDIFF incremental patching. Each fetch downloads and imports the complete current list, which is simpler and self-contained at the cost of a slightly larger download (typically under 200KB) each cycle.
- The default source's discovery-page scan depends on darkrealms.ca's current page wording ("Fidonet Daily Nodelist (Z1/ZIP) day NNN") and table layout. If they restructure their page, the scan will fail gracefully (logged, the scheduler keeps retrying on its normal interval) rather than crash — but won't recover until either their page reverts or you override `nodelist_url` with a different source.

---

## 13. Quick reference — all `[fido]` fields

```toml
[fido]
  name             = "FidoNet"          # primary network display name
  enabled          = false              # master on/off switch
  address          = "1:1/1"            # this BBS's own FidoNet address
  uplink           = ""                 # your boss/hub node's address
  password         = ""                 # shared session/packet password
  inbound_dir      = "fido/inbound"     # where toss reads .pkt files from
  outbound_dir     = "fido/outbound"    # where scan/netmail writes .pkt files to
  nodelist_dir     = "fido/nodelist"    # NODELIST.* files for address lookups
  binkp_port         = 24554            # BinkP port used when polling AND listened on for inbound polls
  taglines_file      = ""               # optional taglines, one per line
  areafix_password   = ""               # password WE send to OUR uplink's AreaFix
  filefix_password   = ""               # password WE send to OUR uplink's FileFix
  tic_password       = ""               # password WE send to OUR uplink's TIC
  holding_dir        = ""               # optional; default <inbound>/.holding
  poll_interval_mins = 0                # 0 = scheduler default (6h); else clamped to >=5
  nodelist_url                   = ""   # blank = scan https://www.darkrealms.ca/
  nodelist_update_interval_hours = 0    # 0 = scheduler default (24h); else clamped to >=1

  [fido.areas]                       # AREA: tag → conference ID (inbound routing)
    TAG_NAME = 1

  [fido.file_areas]                  # FileFix tag → file directory ID
    GAMES = 1

  [[fido.downlinks]]                 # zero or more systems that subscribe to our areas (AreaFix + FileFix)
    name     = "Bob's BBS"
    address  = "1:2/4"
    password = "letmein"

[[fido.networks]]                    # zero or more additional networks
  name               = "NetworkName"
  enabled            = true
  address            = "..."
  uplink             = "..."
  password           = ""
  inbound_dir        = "fido/NetworkName_inbound"
  outbound_dir       = "fido/NetworkName_outbound"
  nodelist_dir       = "fido/NetworkName_nodelist"
  holding_dir        = ""
  binkp_port         = 24554
  taglines_file      = ""
  areafix_password   = ""
  filefix_password   = ""
  tic_password       = ""
  poll_interval_mins = 0
  nodelist_url                   = ""
  nodelist_update_interval_hours = 0

  [fido.networks.areas]
    TAG_NAME = 3

  [fido.networks.file_areas]
    TAG_NAME = 2

  [[fido.networks.downlinks]]
    name     = "..."
    address  = "..."
    password = "..."
```
