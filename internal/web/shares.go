package web

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// ShareStore manages expiring public share links for messages and files.
type ShareStore struct {
	db *sql.DB
}

// Share is a public link payload.
type Share struct {
	Key       string
	Kind      string // "message" or "file"
	ConfID    int
	MsgNum    int
	DirID     int64
	Filename  string
	CreatedBy int64
	ExpiresAt time.Time
}

// OpenShareStore ensures the schema exists and returns a store.
func OpenShareStore(db *sql.DB) (*ShareStore, error) {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS web_shares (
		key         TEXT PRIMARY KEY,
		kind        TEXT NOT NULL,
		conf_id     INTEGER NOT NULL DEFAULT 0,
		msg_num     INTEGER NOT NULL DEFAULT 0,
		dir_id      INTEGER NOT NULL DEFAULT 0,
		filename    TEXT NOT NULL DEFAULT '',
		created_by  INTEGER NOT NULL,
		expires_at  TEXT NOT NULL,
		created_at  TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return nil, err
	}
	return &ShareStore{db: db}, nil
}

func newShareKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// CreateMessageShare stores a share link for a conference message (7-day expiry).
func (st *ShareStore) CreateMessageShare(createdBy int64, confID, msgNum int) (string, error) {
	key, err := newShareKey()
	if err != nil {
		return "", err
	}
	exp := time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	_, err = st.db.Exec(`INSERT INTO web_shares (key, kind, conf_id, msg_num, created_by, expires_at)
		VALUES (?,?,?,?,?,?)`, key, "message", confID, msgNum, createdBy, exp)
	return key, err
}

// CreateFileShare stores a share link for a file download (7-day expiry).
func (st *ShareStore) CreateFileShare(createdBy int64, dirID int64, filename string) (string, error) {
	key, err := newShareKey()
	if err != nil {
		return "", err
	}
	exp := time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	_, err = st.db.Exec(`INSERT INTO web_shares (key, kind, dir_id, filename, created_by, expires_at)
		VALUES (?,?,?,?,?,?)`, key, "file", dirID, filename, createdBy, exp)
	return key, err
}

// Get loads a non-expired share by key.
func (st *ShareStore) Get(key string) (*Share, error) {
	var sh Share
	var expStr string
	err := st.db.QueryRow(`SELECT key, kind, conf_id, msg_num, dir_id, filename, created_by, expires_at
		FROM web_shares WHERE key=?`, key).Scan(
		&sh.Key, &sh.Kind, &sh.ConfID, &sh.MsgNum, &sh.DirID, &sh.Filename, &sh.CreatedBy, &expStr)
	if err != nil {
		return nil, err
	}
	sh.ExpiresAt, err = time.Parse(time.RFC3339, expStr)
	if err != nil || time.Now().After(sh.ExpiresAt) {
		return nil, fmt.Errorf("share expired or invalid")
	}
	return &sh, nil
}
