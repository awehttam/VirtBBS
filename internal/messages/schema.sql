-- VirtBBS message base schema

CREATE TABLE IF NOT EXISTS conferences (
    id           INTEGER PRIMARY KEY,
    name         TEXT    NOT NULL,
    description  TEXT    NOT NULL DEFAULT '',
    public       INTEGER NOT NULL DEFAULT 1,
    read_sec     INTEGER NOT NULL DEFAULT 10,
    write_sec    INTEGER NOT NULL DEFAULT 10,
    sysop_sec    INTEGER NOT NULL DEFAULT 110,
    echo         INTEGER NOT NULL DEFAULT 0,  -- 1 = echomail area
    echo_tag     TEXT    NOT NULL DEFAULT '',  -- AREA: tag (e.g. FIDO_GENERAL)
    echo_from_name TEXT NOT NULL DEFAULT 'real', -- real | alias | anonymous
    uplink_addr  TEXT    NOT NULL DEFAULT '',  -- override uplink (blank = use default)
    network      TEXT    NOT NULL DEFAULT '',  -- network name (blank = primary)
    created_at   TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- Insert default General conference
INSERT OR IGNORE INTO conferences (id, name, description, public)
VALUES (0, 'General', 'General discussion', 1);

CREATE TABLE IF NOT EXISTS messages (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    conference_id   INTEGER NOT NULL REFERENCES conferences(id),
    msg_number      INTEGER NOT NULL,
    from_name       TEXT    NOT NULL,
    to_name         TEXT    NOT NULL DEFAULT 'ALL',
    subject         TEXT    NOT NULL DEFAULT '',
    date_posted     TEXT    NOT NULL,
    status          TEXT    NOT NULL DEFAULT 'A',
    echo            INTEGER NOT NULL DEFAULT 0,
    has_attachment  INTEGER NOT NULL DEFAULT 0,
    body            TEXT    NOT NULL DEFAULT '',
    fido_msgid      TEXT,    -- FidoNet ^AMSGID kludge value, for dedupe/threading
    fido_reply      TEXT,    -- parent MSGID from ^AREPLY kludge
    fido_kludges    TEXT,    -- other ^A kludge lines (TZUTC, INTL, etc.)
    fido_seenby     TEXT,    -- space-separated net/node tokens from SEEN-BY lines
    fido_path       TEXT,    -- space-separated net/node tokens from ^APATH kludge
    fido_origin     TEXT,    -- originating zone:net/node if received via FidoNet toss
    fido_exported_at TEXT,   -- set once this message has been written to an outbound PKT
    UNIQUE (conference_id, msg_number)
);

CREATE INDEX IF NOT EXISTS idx_messages_conf ON messages(conference_id, msg_number);
CREATE INDEX IF NOT EXISTS idx_messages_to   ON messages(to_name);
-- idx_messages_fido_msgid is created in store.go's migrate(), AFTER the
-- ALTER TABLE statements that add fido_msgid to pre-existing databases —
-- creating it here would fail on an old DB before migrate() has run.

-- FidoNet nodelist: imported from NODELIST.xxx files
CREATE TABLE IF NOT EXISTS fido_nodes (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    network   TEXT    NOT NULL DEFAULT 'FidoNet',
    zone      INTEGER NOT NULL,
    net       INTEGER NOT NULL,
    node_num  INTEGER NOT NULL,
    point     INTEGER NOT NULL DEFAULT 0,
    name      TEXT    NOT NULL DEFAULT '',
    location  TEXT    NOT NULL DEFAULT '',
    sysop     TEXT    NOT NULL DEFAULT '',
    phone     TEXT    NOT NULL DEFAULT '',
    baud      INTEGER NOT NULL DEFAULT 0,
    flags     TEXT    NOT NULL DEFAULT '',
    node_type TEXT    NOT NULL DEFAULT 'Node', -- Zone/Host/Hub/Pvt/Hold/Down/Boss/Node
    is_active INTEGER NOT NULL DEFAULT 1,
    UNIQUE(network, zone, net, node_num, point)
);

CREATE INDEX IF NOT EXISTS idx_fido_nodes_addr  ON fido_nodes(zone, net, node_num, point);
CREATE INDEX IF NOT EXISTS idx_fido_nodes_sysop ON fido_nodes(sysop);
CREATE INDEX IF NOT EXISTS idx_fido_nodes_name  ON fido_nodes(name);
CREATE INDEX IF NOT EXISTS idx_fido_nodes_net   ON fido_nodes(network, zone, net);

-- Outbound netmail queue
CREATE TABLE IF NOT EXISTS fido_netmail (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    from_name  TEXT    NOT NULL,
    from_addr  TEXT    NOT NULL,
    to_name    TEXT    NOT NULL,
    to_addr    TEXT    NOT NULL,
    subject    TEXT    NOT NULL DEFAULT '',
    body       TEXT    NOT NULL DEFAULT '',
    crash      INTEGER NOT NULL DEFAULT 0,   -- 1 = send directly (no routing)
    network    TEXT    NOT NULL DEFAULT '',  -- which network
    author_lang TEXT   NOT NULL DEFAULT 'en', -- ^ALANG kludge (en, es, af)
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    sent_at    TEXT
);

-- AreaFix subscriptions: which echomail areas each downlink receives from us.
CREATE TABLE IF NOT EXISTS fido_areafix_subs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    network       TEXT    NOT NULL DEFAULT 'FidoNet',
    downlink_addr TEXT    NOT NULL,  -- zone:net/node of the downlink (no point)
    area_tag      TEXT    NOT NULL,  -- AREA: tag, matches conferences.echo_tag
    created_at    TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(network, downlink_addr, area_tag)
);

CREATE INDEX IF NOT EXISTS idx_areafix_subs_downlink ON fido_areafix_subs(network, downlink_addr);
CREATE INDEX IF NOT EXISTS idx_areafix_subs_area     ON fido_areafix_subs(network, area_tag);

-- FileFix subscriptions: which file areas each downlink receives from us.
CREATE TABLE IF NOT EXISTS fido_filefix_subs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    network       TEXT    NOT NULL DEFAULT 'FidoNet',
    downlink_addr TEXT    NOT NULL,  -- zone:net/node of the downlink (no point)
    file_tag      TEXT    NOT NULL,  -- file-area tag, matches fido.file_areas key
    created_at    TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(network, downlink_addr, file_tag)
);

CREATE INDEX IF NOT EXISTS idx_filefix_subs_downlink ON fido_filefix_subs(network, downlink_addr);
CREATE INDEX IF NOT EXISTS idx_filefix_subs_area     ON fido_filefix_subs(network, file_tag);

-- TIC export tracking: local files already hatched/sent for a network.
CREATE TABLE IF NOT EXISTS fido_file_exports (
    network     TEXT    NOT NULL,
    dir_id      INTEGER NOT NULL,
    filename    TEXT    NOT NULL,
    exported_at TEXT    NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (network, dir_id, filename)
);

-- Nodelist version tracking: one row per successful import, written by
-- fido.ImportFile (and therefore also fido.FetchAndImport). Lets clients
-- (internal/userapi, used by VirtAnd) cheaply ask "has this
-- network's nodelist changed since I last synced" without re-downloading.
CREATE TABLE IF NOT EXISTS fido_nodelist_versions (
    network     TEXT    NOT NULL PRIMARY KEY,
    imported_at TEXT    NOT NULL,
    node_count  INTEGER NOT NULL DEFAULT 0
);

-- VirtNet hub support: pending applications to join a network this BBS
-- hosts (Uplink == ""). See internal/fido/members.go.
CREATE TABLE IF NOT EXISTS fido_join_requests (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    network              TEXT    NOT NULL,
    requested_by_user_id INTEGER NOT NULL,
    bbs_name             TEXT    NOT NULL,
    sysop_name           TEXT    NOT NULL,
    location             TEXT    NOT NULL DEFAULT '',
    contact              TEXT    NOT NULL DEFAULT '',
    requested_net        INTEGER,                          -- NULL = sysop's choice
    binkp_host           TEXT    NOT NULL DEFAULT '',       -- optional host:port
    status               TEXT    NOT NULL DEFAULT 'pending', -- pending/approved/denied
    created_at           TEXT    NOT NULL DEFAULT (datetime('now')),
    decided_at           TEXT,
    decided_by           TEXT
);

CREATE INDEX IF NOT EXISTS idx_join_requests_status ON fido_join_requests(network, status);

-- VirtNet hub support: approved members of a network this BBS hosts.
-- This is both the routing table (binkp_host/password) and the source of
-- truth for nodelist generation (internal/fido/nodelistgen.go).
CREATE TABLE IF NOT EXISTS fido_members (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    network      TEXT    NOT NULL,
    zone         INTEGER NOT NULL,
    net          INTEGER NOT NULL,
    node_num     INTEGER NOT NULL,
    point        INTEGER NOT NULL DEFAULT 0,
    bbs_name     TEXT    NOT NULL,
    sysop_name   TEXT    NOT NULL,
    location     TEXT    NOT NULL DEFAULT '',
    contact      TEXT    NOT NULL DEFAULT '',
    binkp_host   TEXT    NOT NULL DEFAULT '',  -- host:port — the routing table itself
    password     TEXT    NOT NULL DEFAULT '',  -- mirrors fido.Downlink.Password
    is_host      INTEGER NOT NULL DEFAULT 0,   -- net Host flag for nodelist generation
    is_active    INTEGER NOT NULL DEFAULT 1,
    is_delegated INTEGER NOT NULL DEFAULT 0,   -- 1 = arrived via inbound NodeAnnounce
                                                -- netmail, not locally approved here
    joined_at    TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(network, zone, net, node_num, point)
);

CREATE INDEX IF NOT EXISTS idx_fido_members_net ON fido_members(network, net);

-- Arriving "<NetworkName> Nodelists" echomail (see internal/fido/
-- nodelistecho.go) is queued here at toss time rather than processed
-- inline, since the file-area work it needs (writing the file, registering
-- it, calling ImportFile) requires internal/files, which internal/fido
-- cannot import (import cycle — see ensure.go). A periodic drain step in
-- the caller (which already imports both) processes and clears this queue.
-- One line per new/changed VirtNet node, written by
-- internal/fido.LogNodeChange (called from ApplyNodeAnnounceInfo). The
-- full table contents are dumped into NodeChgs.txt and re-zipped on every
-- day-rollover regeneration (internal/fido.BuildNodeChgsText).
CREATE TABLE IF NOT EXISTS fido_node_change_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    network    TEXT    NOT NULL,
    line       TEXT    NOT NULL,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS fido_nodelist_echo_pending (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    network    TEXT    NOT NULL,
    subject    TEXT    NOT NULL,
    body       TEXT    NOT NULL,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- One row per network per calendar day a nodelist was generated — the
-- "yesterday" snapshot GenerateNodelistDiff compares today's fido_members
-- against, to produce the daily diff file.
CREATE TABLE IF NOT EXISTS fido_members_snapshot (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    network     TEXT    NOT NULL,
    year        INTEGER NOT NULL,
    day_of_year INTEGER NOT NULL,
    zone        INTEGER NOT NULL,
    net         INTEGER NOT NULL,
    node_num    INTEGER NOT NULL,
    point       INTEGER NOT NULL DEFAULT 0,
    bbs_name    TEXT    NOT NULL,
    sysop_name  TEXT    NOT NULL,
    location    TEXT    NOT NULL DEFAULT '',
    flags       TEXT    NOT NULL DEFAULT '',
    UNIQUE(network, year, day_of_year, zone, net, node_num, point)
);

-- ROUTES.BBS-style static routing table (BinkleyTerm/FrontDoor convention):
-- wildcard address patterns mapped to a "route via this address instead"
-- next-hop, used when a destination isn't directly reachable — e.g. mail
-- for a node behind a delegated sub-hub gets physically handed to that
-- sub-hub. See internal/fido/routes.go.
CREATE TABLE IF NOT EXISTS fido_routes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    network    TEXT    NOT NULL,
    pattern    TEXT    NOT NULL,                  -- "*", "300:*", "300:1005/*", or an exact "300:1005/3"
    route_to   TEXT    NOT NULL,                  -- address to actually hand the packet to
    is_default INTEGER NOT NULL DEFAULT 0,         -- 1 = auto-seeded net->Host route, 0 = sysop/import-added
    created_at TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE(network, pattern)
);

-- BinkP / FidoNet session statistics (per network, per period).
CREATE TABLE IF NOT EXISTS fido_binkp_stats (
    network                    TEXT NOT NULL,
    period                     TEXT NOT NULL,  -- day, month, year, all
    period_key                 TEXT NOT NULL,  -- YYYY-MM-DD, YYYY-MM, YYYY, or empty for all
    poll_client_ok             INTEGER NOT NULL DEFAULT 0,
    poll_client_fail           INTEGER NOT NULL DEFAULT 0,
    poll_client_files_sent     INTEGER NOT NULL DEFAULT 0,
    poll_client_files_recv     INTEGER NOT NULL DEFAULT 0,
    poll_server_uplink_ok      INTEGER NOT NULL DEFAULT 0,
    poll_server_uplink_fail    INTEGER NOT NULL DEFAULT 0,
    poll_server_uplink_sent    INTEGER NOT NULL DEFAULT 0,
    poll_server_uplink_recv    INTEGER NOT NULL DEFAULT 0,
    poll_server_downlink_ok    INTEGER NOT NULL DEFAULT 0,
    poll_server_downlink_fail  INTEGER NOT NULL DEFAULT 0,
    poll_server_downlink_sent  INTEGER NOT NULL DEFAULT 0,
    poll_server_downlink_recv  INTEGER NOT NULL DEFAULT 0,
    netmail_recv               INTEGER NOT NULL DEFAULT 0,
    echomail_recv              INTEGER NOT NULL DEFAULT 0,
    netmail_sent               INTEGER NOT NULL DEFAULT 0,
    echomail_sent              INTEGER NOT NULL DEFAULT 0,
    toss_imported              INTEGER NOT NULL DEFAULT 0,
    toss_skipped               INTEGER NOT NULL DEFAULT 0,
    toss_held                  INTEGER NOT NULL DEFAULT 0,
    toss_packets               INTEGER NOT NULL DEFAULT 0,
    session_errors             INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (network, period, period_key)
);

-- Per-link BinkP stats (uplink or downlink peer).
CREATE TABLE IF NOT EXISTS fido_binkp_link_stats (
    network        TEXT NOT NULL,
    period         TEXT NOT NULL,
    period_key     TEXT NOT NULL,
    link_type      TEXT NOT NULL,  -- uplink or downlink
    peer_key       TEXT NOT NULL,  -- Fido address or downlink name
    poll_ok        INTEGER NOT NULL DEFAULT 0,
    poll_fail      INTEGER NOT NULL DEFAULT 0,
    files_sent     INTEGER NOT NULL DEFAULT 0,
    files_recv     INTEGER NOT NULL DEFAULT 0,
    netmail_sent   INTEGER NOT NULL DEFAULT 0,
    echomail_sent  INTEGER NOT NULL DEFAULT 0,
    netmail_recv   INTEGER NOT NULL DEFAULT 0,
    echomail_recv  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (network, period, period_key, link_type, peer_key)
);
