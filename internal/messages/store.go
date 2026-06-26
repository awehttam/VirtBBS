// ============================================================================
// VirtBBS — A modern BBS server inspired by PCBoard BBS
//           (Clark Development Company, 1987-1996)
//
// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// MIT License
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
// OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.
//
// Change History:
//   v0.0.1  2026-06-24  Initial implementation
// ============================================================================

// Package messages manages the VirtBBS message base.
package messages

import (
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Message represents a single BBS message.
type Message struct {
	ID           int64
	ConferenceID int
	MsgNumber    int
	FromName     string
	ToName       string
	Subject      string
	DatePosted   time.Time
	Status       string
	Echo         bool
	Body         string

	// FidoNet metadata. Empty/zero for locally-authored messages.
	FidoMsgID      string // ^AMSGID kludge value, for dedupe/threading
	FidoSeenBy     string // space-separated net/node tokens from SEEN-BY lines
	FidoPath       string // space-separated net/node tokens from ^APATH kludge
	FidoOrigin     string // originating zone:net/node if received via FidoNet toss
	FidoExportedAt time.Time // zero if not yet written to an outbound PKT
}

// Store manages messages in SQLite.
type Store struct {
	db *sql.DB
}

// Open opens the database and applies the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("messages schema: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("messages migration: %w", err)
	}
	return s, nil
}

// migrate applies backwards-compatible ALTER TABLE statements for databases
// created before the FidoNet metadata columns existed.
// Errors for columns that already exist are silently ignored.
func (s *Store) migrate() error {
	alters := []string{
		`ALTER TABLE messages ADD COLUMN fido_msgid TEXT`,
		`ALTER TABLE messages ADD COLUMN fido_seenby TEXT`,
		`ALTER TABLE messages ADD COLUMN fido_path TEXT`,
		`ALTER TABLE messages ADD COLUMN fido_origin TEXT`,
		`ALTER TABLE messages ADD COLUMN fido_exported_at TEXT`,
	}
	for _, stmt := range alters {
		if _, err := s.db.Exec(stmt); err != nil {
			if !containsAny(err.Error(), "duplicate column", "already exists") {
				return err
			}
		}
	}

	// Must run after the ALTER TABLE statements above — creating this index
	// in schema.sql (which runs before migrate()) fails on a pre-existing
	// database that doesn't have fido_msgid yet.
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_fido_msgid ON messages(fido_msgid)`); err != nil {
		return err
	}
	return nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}

func (s *Store) Close() error { return s.db.Close() }

// DB returns the underlying *sql.DB for packages that need direct access
// (e.g. fido nodelist operations that share the same database file).
func (s *Store) DB() *sql.DB { return s.db }

// PostWithNumber inserts a message preserving its existing MsgNumber.
// Used by importers that need to retain original PCBoard message numbers.
// On conflict (duplicate msg_number in the same conference) the message is skipped.
func (s *Store) PostWithNumber(m *Message) error {
	if m.DatePosted.IsZero() {
		m.DatePosted = time.Now()
	}
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO messages
		  (conference_id, msg_number, from_name, to_name, subject, date_posted, status, echo, body,
		   fido_msgid, fido_seenby, fido_path, fido_origin)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ConferenceID, m.MsgNumber, m.FromName, m.ToName, m.Subject,
		m.DatePosted.Format(time.RFC3339), m.Status, boolInt(m.Echo), m.Body,
		nullable(m.FidoMsgID), nullable(m.FidoSeenBy), nullable(m.FidoPath), nullable(m.FidoOrigin),
	)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	return nil
}

// Post inserts a new message into the given conference.
func (s *Store) Post(m *Message) error {
	var nextNum int
	row := s.db.QueryRow(`SELECT COALESCE(MAX(msg_number),0)+1 FROM messages WHERE conference_id=?`, m.ConferenceID)
	if err := row.Scan(&nextNum); err != nil {
		return err
	}
	m.MsgNumber = nextNum
	if m.DatePosted.IsZero() {
		m.DatePosted = time.Now()
	}
	res, err := s.db.Exec(`
		INSERT INTO messages (conference_id, msg_number, from_name, to_name, subject, date_posted, status, echo, body,
		                       fido_msgid, fido_seenby, fido_path, fido_origin)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ConferenceID, m.MsgNumber, m.FromName, m.ToName, m.Subject,
		m.DatePosted.Format(time.RFC3339), m.Status, boolInt(m.Echo), m.Body,
		nullable(m.FidoMsgID), nullable(m.FidoSeenBy), nullable(m.FidoPath), nullable(m.FidoOrigin),
	)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	return nil
}

const messageCols = `id, conference_id, msg_number, from_name, to_name, subject, date_posted, status, echo, body,
		fido_msgid, fido_seenby, fido_path, fido_origin, fido_exported_at`

// List returns messages in a conference, newest first.
func (s *Store) List(conferenceID, limit, offset int) ([]*Message, error) {
	rows, err := s.db.Query(`
		SELECT `+messageCols+`
		FROM messages WHERE conference_id=? AND status!='D'
		ORDER BY msg_number DESC LIMIT ? OFFSET ?`,
		conferenceID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// ListFrom returns messages in a conference with msg_number >= startNum, oldest first.
func (s *Store) ListFrom(conferenceID, startNum, limit int) ([]*Message, error) {
	rows, err := s.db.Query(`
		SELECT `+messageCols+`
		FROM messages WHERE conference_id=? AND msg_number>=? AND status!='D'
		ORDER BY msg_number ASC LIMIT ?`,
		conferenceID, startNum, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// Get fetches a single message by conference + number.
func (s *Store) Get(conferenceID, msgNumber int) (*Message, error) {
	row := s.db.QueryRow(`
		SELECT `+messageCols+`
		FROM messages WHERE conference_id=? AND msg_number=?`,
		conferenceID, msgNumber)
	return scanMessage(row)
}

// Delete marks a message as deleted.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`UPDATE messages SET status='D' WHERE id=?`, id)
	return err
}

// ListEcho returns echo-flagged messages in a conference that have not yet
// been written to an outbound FidoNet packet, oldest first.
// Used by the FidoNet scanner when building outbound packets.
func (s *Store) ListEcho(conferenceID, limit, offset int) ([]*Message, error) {
	rows, err := s.db.Query(`
		SELECT `+messageCols+`
		FROM messages WHERE conference_id=? AND echo=1 AND status!='D' AND fido_exported_at IS NULL
		ORDER BY msg_number ASC LIMIT ? OFFSET ?`,
		conferenceID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// HasFidoMsgID reports whether a message with the given FidoNet MSGID
// already exists in the conference. Used by the toss pipeline to detect
// duplicate packet processing (e.g. a crash between import and marking
// the source file as handled).
func (s *Store) HasFidoMsgID(conferenceID int, msgID string) (bool, error) {
	if msgID == "" {
		return false, nil
	}
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM messages WHERE conference_id=? AND fido_msgid=?`,
		conferenceID, msgID).Scan(&n)
	return n > 0, err
}

// MarkExported records that a message has been written into an outbound
// FidoNet packet, so it is not selected again by ListEcho on a later scan.
func (s *Store) MarkExported(id int64) error {
	_, err := s.db.Exec(`UPDATE messages SET fido_exported_at=? WHERE id=?`,
		time.Now().Format(time.RFC3339), id)
	return err
}

// TotalCount returns the number of non-deleted messages across all conferences.
func (s *Store) TotalCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE status!='D'`).Scan(&n)
	return n, err
}

// HighMsgNumber returns the highest message number in a conference.
func (s *Store) HighMsgNumber(conferenceID int) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COALESCE(MAX(msg_number),0) FROM messages WHERE conference_id=?`, conferenceID).Scan(&n)
	return n, err
}

type scanner interface{ Scan(...any) error }

func scanMessage(sc scanner) (*Message, error) {
	var m Message
	var dateStr string
	var echo int
	var msgID, seenBy, path, origin, exportedAt sql.NullString
	err := sc.Scan(&m.ID, &m.ConferenceID, &m.MsgNumber, &m.FromName, &m.ToName,
		&m.Subject, &dateStr, &m.Status, &echo, &m.Body,
		&msgID, &seenBy, &path, &origin, &exportedAt)
	if err != nil {
		return nil, err
	}
	m.DatePosted, _ = time.Parse(time.RFC3339, dateStr)
	m.Echo = echo != 0
	m.FidoMsgID = msgID.String
	m.FidoSeenBy = seenBy.String
	m.FidoPath = path.String
	m.FidoOrigin = origin.String
	if exportedAt.Valid {
		m.FidoExportedAt, _ = time.Parse(time.RFC3339, exportedAt.String)
	}
	return &m, nil
}

func scanMessages(rows *sql.Rows) ([]*Message, error) {
	var out []*Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullable converts an empty string to a SQL NULL so optional FidoNet
// metadata columns stay NULL (rather than "") for locally-authored messages.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
