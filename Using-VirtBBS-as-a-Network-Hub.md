# Using VirtBBS as a Network Hub

This guide explains how to configure VirtBBS as the **zone coordinator and hub** for your own Fido-compatible network (e.g. VirtNet), including addressing conventions, `VirtBBS.DAT` setup, member registry, and what the generated nodelist should look like.

## `300:0/0` vs what Fido/VirtBBS actually use

VirtBBS only understands **`zone:net/node`** (optional `.point`).

| Role | What people often say | Standard Fido form | Nodelist line |
|------|------------------------|--------------------|---------------|
| **Zone coordinator** (your hub for zone 300) | "300:0/0" | **`300:300/0`** | `Zone,300,...` |
| **Net 1 coordinator (NC)** | "net /1" | **`300:1/0`** | `Host,1,...` |
| **Your BBS on net 1** | "I'm on net 1" | **`300:1/1`** (or `/17`, etc.) | `,1,...` |

So: **zone 300 hub** → think **`300:300/0`** in the nodelist, not `300:0/0`.  
**Net 1 NC** → **`300:1/0`**.  
**This BBS as a real node on net 1** → e.g. **`300:1/1`**.

VirtBBS generates the Zone line from your hub's zone and always uses **`zone:zone/0`** for it (see `hubNodelistEntries` in `internal/fido/nodelistgen.go`).

---

## Sensible VirtNet setup

### 1. Hub network in `VirtBBS.DAT`

Use a **hub** network (`uplink = ""`), no `nodelist_url` (member-based nodelist, not HTTP fetch):

```toml
[[fido.networks]]
  name         = "VirtNet"          # or your network name
  enabled      = true
  address      = "300:1/1"          # this BBS: node 1 on net 1 (NC)
  uplink       = ""                 # hub — no uplink
  password     = "your-binkp-password"
  inbound_dir  = "fido/VirtNet_inbound"
  outbound_dir = "fido/VirtNet_outbound"
  nodelist_dir = "fido/VirtNet_nodelist"
  nodelist_url = ""                 # leave blank for hub/member nodelist
  akas         = ["300:300/0", "300:1/0"]
```

- **`address`** — your primary node (e.g. `300:1/1`).
- **`akas`** (optional but useful):
  - `300:300/0` — zone coordinator (mail/BinkP at zone level)
  - `300:1/0` — net 1 host (also added automatically when `address` is `300:1/N` and `N ≠ 0`)

### 2. Register **this BBS** in `fido_members`

Generated hub nodelists come from **`fido_members`**, not only from config.

Add yourself as **net 1, node 1, Host checked**:

- **In-BBS:** FidoNet sysop menu → join requests / routing (approve yourself), or member edit
- **Web admin:** Fido → join requests — approve with **net = 1**, **node = 1**, **Host** enabled

That gives:

- `Host,1,...` → **`300:1/0`**
- `,1,...` → **`300:1/1`** (with AKA pairing in the UI)

### 3. Add other nodes on net 1

For each downlink:

- Approve join → **net = 1**, **node = 2, 3, …**, **Host = off**
- They become downlinks (BinkP + AreaFix) and appear as regular node lines

### 4. Later: more nets with their own NCs

When you add **net 2** with a different NC:

| Member | Net | Node | Host? | Nodelist addresses |
|--------|-----|------|-------|-------------------|
| Net 2 NC BBS | 2 | 1 (or 17, etc.) | **Yes** | `300:2/0` + `300:2/1` |
| Ordinary node | 2 | 2 | No | `300:2/2` |

VirtBBS emits one **`Host,N`** line per net that has a member with **`IsHost`**.

Your hub stays zone coordinator via the automatic **`Zone,300`** line; you don't need a separate "zone member" row for that.

---

## What the nodelist should look like

Example for zone **300**, you as net **1** NC at **`300:1/1`**, one other node on net 1:

```text
; VirtNet
Zone,300,YourBBS,Internet,YourSysop,-Unpublished-,33600,CM,IBN:your.host:24554
Host,1,YourBBS,Internet,YourSysop,-Unpublished-,33600,IBN:your.host:24554,CM
,1,YourBBS,Internet,YourSysop,-Unpublished-,33600,IBN:your.host:24554,CM
,2,OtherBBS,Internet,OtherSysop,-Unpublished-,33600,IBN:other.host:24554,CM
```

**Publishing schedule (VirtBBS hub):**

- **Weekly (Friday):** full `NODELIST.Z##` (day-of-year mod 100)
- **Other days:** `NODEDIFF.Z##` when members changed
- **Authoritative source:** `fido_members` — use **Rebuild from members** on `/admin/fido/nodelist` after fixing members if an old import is still showing

Generated archives in the Nodelist Files area are ZIP files containing the plain nodelist/diff plus `FILE_ID.DIZ`.

---

## Practical checklist

1. Set hub network: `address = "300:1/1"`, `uplink = ""`, `akas` as above.
2. Add hub to **`fido_members`**: net **1**, node **1**, **Host**.
3. Approve net 1 nodes at **net 1**, nodes **2+**, not Host.
4. Click **Rebuild from members** on the VirtNet nodelist admin page.
5. Downlinks: `uplink = "300:1/1"` (or your hub address), poll your BinkP port.

---

## If you imported a nodelist with different numbers

Imports can disagree with your plan (e.g. zone 90, net 227). For a **member hub**, treat **`fido_members` + rebuild** as truth, not the imported file. Use **Rebuild from members** so the published list matches zone **300** / net **1**.

---

## Related documentation

- [`FidoNet Config.md`](FidoNet%20Config.md) — full `[fido]` field reference
- [`VirtNet Nodelist Processing.md`](VirtNet%20Nodelist%20Processing.md) — automatic generation, echomail distribution, and import
