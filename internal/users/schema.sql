-- VirtBBS users schema

CREATE TABLE IF NOT EXISTS users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT    NOT NULL UNIQUE,         -- BBS handle (up to 25 chars)
    real_name       TEXT    NOT NULL DEFAULT '',     -- FidoNet real name (echomail)
    city            TEXT    NOT NULL DEFAULT '',
    password_hash   TEXT    NOT NULL,                -- bcrypt
    phone_business  TEXT    NOT NULL DEFAULT '',
    phone_home      TEXT    NOT NULL DEFAULT '',
    last_login_date TEXT    NOT NULL DEFAULT '',     -- YYYY-MM-DD
    last_login_time TEXT    NOT NULL DEFAULT '',     -- HH:MM
    security_level  INTEGER NOT NULL DEFAULT 10,
    times_online    INTEGER NOT NULL DEFAULT 0,
    page_length     INTEGER NOT NULL DEFAULT 24,
    uploads         INTEGER NOT NULL DEFAULT 0,
    downloads       INTEGER NOT NULL DEFAULT 0,
    bytes_uploaded  INTEGER NOT NULL DEFAULT 0,
    bytes_downloaded INTEGER NOT NULL DEFAULT 0,
    comment1        TEXT    NOT NULL DEFAULT '',
    comment2        TEXT    NOT NULL DEFAULT '',
    elapsed_time    INTEGER NOT NULL DEFAULT 0,      -- total minutes
    expiration_date TEXT    NOT NULL DEFAULT '',     -- YYYY-MM-DD, empty = no expiry
    expert_mode     INTEGER NOT NULL DEFAULT 0,      -- 0=novice, 1=expert
    xfer_protocol   TEXT    NOT NULL DEFAULT 'Z',   -- default file transfer protocol
    ansi            INTEGER NOT NULL DEFAULT 1,
    full_screen_editor INTEGER NOT NULL DEFAULT 0,
    locale          TEXT    NOT NULL DEFAULT 'en',     -- UI / ^ALANG kludge (en, es, af)
    deleted         INTEGER NOT NULL DEFAULT 0,
    sysop           INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS user_conferences (
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conference_id   INTEGER NOT NULL,
    registered      INTEGER NOT NULL DEFAULT 0,    -- 1 = registered member
    last_msg_read   INTEGER NOT NULL DEFAULT 0,    -- last message number read
    PRIMARY KEY (user_id, conference_id)
);

CREATE INDEX IF NOT EXISTS idx_users_name ON users(name);
CREATE INDEX IF NOT EXISTS idx_users_deleted ON users(deleted);

-- User-generated API tokens for the new user-facing JSON/TCP API (internal/userapi),
-- used by VirtAnd (Android point client).
-- Hash-only storage, same spirit as the bcrypt password_hash column above.
CREATE TABLE IF NOT EXISTS user_api_tokens (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash    TEXT    NOT NULL,
    device_label  TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL DEFAULT (datetime('now')),
    revoked_at    TEXT
);

CREATE INDEX IF NOT EXISTS idx_user_api_tokens_user ON user_api_tokens(user_id);
