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
	"strings"
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
	FidoReply      string // parent MSGID from ^AREPLY kludge
	FidoKludges    string // other ^A kludge lines (TZUTC, INTL, etc.)
	FidoSeenBy     string // space-separated net/node tokens from SEEN-BY lines
	FidoPath       string // space-separated net/node tokens from ^APATH kludge
	FidoOrigin     string // originating zone:net/node if received via FidoNet toss
	FidoNetwork    string // Fido network name when received via toss
	FidoExportedAt time.Time // zero if not yet written to an outbound PKT
}

// Store manages messages in SQLite.
type Store struct {
	db *sql.DB
}

// Open attaches to the shared database handle and applies the schema.
func Open(db *sql.DB) (*Store, error) {
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
		`ALTER TABLE messages ADD COLUMN fido_reply TEXT`,
		`ALTER TABLE messages ADD COLUMN fido_kludges TEXT`,
		`ALTER TABLE messages ADD COLUMN fido_seenby TEXT`,
		`ALTER TABLE messages ADD COLUMN fido_path TEXT`,
		`ALTER TABLE messages ADD COLUMN fido_origin TEXT`,
		`ALTER TABLE messages ADD COLUMN fido_network TEXT`,
		`ALTER TABLE messages ADD COLUMN fido_exported_at TEXT`,
		`ALTER TABLE fido_netmail ADD COLUMN author_lang TEXT NOT NULL DEFAULT 'en'`,
		`ALTER TABLE fido_nodelist_versions ADD COLUMN source TEXT NOT NULL DEFAULT 'import'`,
		`CREATE TABLE IF NOT EXISTS fido_nodelist_applied (
			network TEXT NOT NULL,
			source TEXT NOT NULL,
			source_key TEXT NOT NULL,
			filename TEXT NOT NULL,
			applied_at TEXT NOT NULL,
			PRIMARY KEY (network, source, source_key)
		)`,
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
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_fido_reply ON messages(fido_reply)`); err != nil {
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

// Close is a no-op; the shared *sql.DB is owned by the caller.
func (s *Store) Close() error { return nil }

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
		   fido_msgid, fido_reply, fido_kludges, fido_seenby, fido_path, fido_origin, fido_network)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ConferenceID, m.MsgNumber, m.FromName, m.ToName, m.Subject,
		m.DatePosted.Format(time.RFC3339), m.Status, boolInt(m.Echo), m.Body,
		nullable(m.FidoMsgID), nullable(m.FidoReply), nullable(m.FidoKludges),
		nullable(m.FidoSeenBy), nullable(m.FidoPath), nullable(m.FidoOrigin), nullable(m.FidoNetwork),
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
		                       fido_msgid, fido_reply, fido_kludges, fido_seenby, fido_path, fido_origin, fido_network)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ConferenceID, m.MsgNumber, m.FromName, m.ToName, m.Subject,
		m.DatePosted.Format(time.RFC3339), m.Status, boolInt(m.Echo), m.Body,
		nullable(m.FidoMsgID), nullable(m.FidoReply), nullable(m.FidoKludges),
		nullable(m.FidoSeenBy), nullable(m.FidoPath), nullable(m.FidoOrigin), nullable(m.FidoNetwork),
	)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	return nil
}

const messageCols = `id, conference_id, msg_number, from_name, to_name, subject, date_posted, status, echo, body,
		fido_msgid, fido_reply, fido_kludges, fido_seenby, fido_path, fido_origin, fido_network, fido_exported_at`

// IsNetmail reports whether m is inbound FidoNet netmail (stored in conference 0).
func IsNetmail(m *Message) bool {
	return m != nil && m.ConferenceID == 0 && !m.Echo && strings.TrimSpace(m.FidoOrigin) != ""
}

// CanViewNetmail reports whether forUser may read inbound netmail m.
func CanViewNetmail(forUser string, sysop bool, m *Message) bool {
	if !IsNetmail(m) {
		return true
	}
	if sysop {
		return true
	}
	tn := strings.ToLower(strings.TrimSpace(m.ToName))
	u := strings.ToLower(strings.TrimSpace(forUser))
	return tn == u || tn == "all"
}

// generalExcludeNetmailSQL keeps inbound FidoNet netmail out of General conference
// browsing; use ListNetmail / the netmail UI instead.
const generalExcludeNetmailSQL = ` AND NOT (echo=0 AND fido_origin IS NOT NULL AND fido_origin != '')`

// List returns messages in a conference, newest first.
func (s *Store) List(conferenceID, limit, offset int) ([]*Message, error) {
	extra := ""
	if conferenceID == 0 {
		extra = generalExcludeNetmailSQL
	}
	rows, err := s.db.Query(`
		SELECT `+messageCols+`
		FROM messages WHERE conference_id=? AND status!='D'`+extra+`
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
	extra := ""
	if conferenceID == 0 {
		extra = generalExcludeNetmailSQL
	}
	rows, err := s.db.Query(`
		SELECT `+messageCols+`
		FROM messages WHERE conference_id=? AND msg_number>=? AND status!='D'`+extra+`
		ORDER BY msg_number ASC LIMIT ?`,
		conferenceID, startNum, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// netmailBaseSQL is the shared WHERE clause for inbound FidoNet netmail stored
// in conference 0 (General) by the toss pipeline — echo=0 plus a Fido origin.
const netmailBaseSQL = `conference_id=0 AND echo=0 AND status!='D'
		AND fido_origin IS NOT NULL AND fido_origin != ''`

// CountNetmail returns how many netmail messages are visible to forUser.
// Sysops see all netmail; other users see mail addressed to them or "All".
func (s *Store) CountNetmail(forUser string, sysop bool) (int, error) {
	where, args := netmailRecipientFilter(forUser, sysop)
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE `+netmailBaseSQL+where, args...).Scan(&n)
	return n, err
}

// CountNetmailUnread returns netmail with msg_number greater than afterMsgNum.
func (s *Store) CountNetmailUnread(forUser string, sysop bool, afterMsgNum int) (int, error) {
	where, args := netmailRecipientFilter(forUser, sysop)
	args = append(args, afterMsgNum)
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE `+netmailBaseSQL+where+` AND msg_number > ?`, args...).Scan(&n)
	return n, err
}

// GetNetmail returns one inbound netmail message visible to forUser.
func (s *Store) GetNetmail(forUser string, sysop bool, msgNum int) (*Message, error) {
	where, args := netmailRecipientFilter(forUser, sysop)
	args = append(args, msgNum)
	row := s.db.QueryRow(`
		SELECT `+messageCols+`
		FROM messages WHERE `+netmailBaseSQL+where+` AND msg_number=?`, args...)
	return scanMessage(row)
}

// ListNetmail returns inbound netmail for forUser, oldest first.
func (s *Store) ListNetmail(forUser string, sysop bool, startNum, limit int) ([]*Message, error) {
	where, args := netmailRecipientFilter(forUser, sysop)
	args = append(args, startNum, limit)
	rows, err := s.db.Query(`
		SELECT `+messageCols+`
		FROM messages WHERE `+netmailBaseSQL+where+`
		AND msg_number>=? ORDER BY msg_number ASC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func netmailRecipientFilter(forUser string, sysop bool) (clause string, args []any) {
	if sysop {
		return "", nil
	}
	return ` AND (lower(to_name)=lower(?) OR lower(to_name)='all')`, []any{forUser}
}

// GetByFidoMsgID fetches a message by its FidoNet MSGID within a conference.
func (s *Store) GetByFidoMsgID(conferenceID int, msgID string) (*Message, error) {
	if msgID == "" {
		return nil, sql.ErrNoRows
	}
	row := s.db.QueryRow(`
		SELECT `+messageCols+`
		FROM messages WHERE conference_id=? AND fido_msgid=? AND status!='D'`,
		conferenceID, msgID)
	return scanMessage(row)
}

// CountReplies returns how many messages reply to the given parent MSGID.
func (s *Store) CountReplies(conferenceID int, parentMsgID string) (int, error) {
	if parentMsgID == "" {
		return 0, nil
	}
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages
		WHERE conference_id=? AND fido_reply=? AND status!='D'`,
		conferenceID, parentMsgID).Scan(&n)
	return n, err
}

// FindThread returns all messages in the conversation thread containing
// startMsgNumber, ordered oldest-first. Thread membership follows ^AREPLY
// links via stored fido_reply / fido_msgid values.
func (s *Store) FindThread(conferenceID, startMsgNumber int) ([]*Message, error) {
	start, err := s.Get(conferenceID, startMsgNumber)
	if err != nil {
		return nil, err
	}

	root := start
	for root.FidoReply != "" {
		parent, err := s.GetByFidoMsgID(conferenceID, root.FidoReply)
		if err != nil {
			break
		}
		root = parent
	}

	threadIDs := map[string]bool{}
	if root.FidoMsgID != "" {
		threadIDs[root.FidoMsgID] = true
	}

	for {
		if len(threadIDs) == 0 {
			break
		}
		placeholders := make([]string, 0, len(threadIDs))
		args := []any{conferenceID}
		for id := range threadIDs {
			placeholders = append(placeholders, "?")
			args = append(args, id)
		}
		query := fmt.Sprintf(`SELECT fido_msgid FROM messages
			WHERE conference_id=? AND status!='D' AND fido_reply IN (%s)`,
			strings.Join(placeholders, ","))
		rows, err := s.db.Query(query, args...)
		if err != nil {
			return nil, err
		}
		added := false
		for rows.Next() {
			var childID sql.NullString
			if err := rows.Scan(&childID); err != nil {
				rows.Close()
				return nil, err
			}
			if childID.Valid && childID.String != "" && !threadIDs[childID.String] {
				threadIDs[childID.String] = true
				added = true
			}
		}
		rows.Close()
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		if !added {
			break
		}
	}

	if len(threadIDs) == 0 {
		return []*Message{start}, nil
	}

	placeholders := make([]string, 0, len(threadIDs))
	args := []any{conferenceID}
	for id := range threadIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	query := fmt.Sprintf(`SELECT `+messageCols+` FROM messages
		WHERE conference_id=? AND status!='D' AND fido_msgid IN (%s)
		ORDER BY msg_number ASC`, strings.Join(placeholders, ","))
	rows, err := s.db.Query(query, args...)
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

// Delete marks a message as deleted and returns its conference ID.
func (s *Store) Delete(id int64) (conferenceID int, err error) {
	if err := s.db.QueryRow(`SELECT conference_id FROM messages WHERE id=?`, id).Scan(&conferenceID); err != nil {
		return 0, err
	}
	_, err = s.db.Exec(`UPDATE messages SET status='D' WHERE id=?`, id)
	return conferenceID, err
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

// ListEchoRescan returns echo-flagged messages in a conference including those
// already exported, oldest first. limit 0 means no row cap. Used by AreaFix
// %RESCAN to rebuild backlog packets for a downlink without re-marking export
// state (which would suppress uplink retransmission).
func (s *Store) ListEchoRescan(conferenceID, limit int) ([]*Message, error) {
	q := `SELECT ` + messageCols + `
		FROM messages WHERE conference_id=? AND echo=1 AND status!='D'
		ORDER BY msg_number ASC`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		q += ` LIMIT ?`
		rows, err = s.db.Query(q, conferenceID, limit)
	} else {
		rows, err = s.db.Query(q, conferenceID)
	}
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

// CountPostsByDay returns message post counts keyed by YYYY-MM-DD for the
// last days calendar days (non-deleted messages only).
func (s *Store) CountPostsByDay(days int) (map[string]int, error) {
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := s.db.Query(`SELECT date(date_posted) AS d, COUNT(*) FROM messages
		WHERE status!='D' AND date(date_posted) >= ?
		GROUP BY d`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var date string
		var n int
		if err := rows.Scan(&date, &n); err != nil {
			return nil, err
		}
		out[date] = n
	}
	return out, rows.Err()
}

// Search finds messages whose subject, body, or from_name contains query
// (case-insensitive), newest first.
func (s *Store) Search(query string, limit int) ([]*Message, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	like := "%" + strings.ToLower(q) + "%"
	rows, err := s.db.Query(`
		SELECT `+messageCols+`
		FROM messages WHERE status!='D'
		AND (lower(subject) LIKE ? OR lower(body) LIKE ? OR lower(from_name) LIKE ?)
		ORDER BY date_posted DESC LIMIT ?`, like, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// HighMsgNumber returns the highest active (non-deleted) message number in a conference.
func (s *Store) HighMsgNumber(conferenceID int) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COALESCE(MAX(msg_number),0) FROM messages WHERE conference_id=? AND status!='D'`, conferenceID).Scan(&n)
	return n, err
}

// HighMsgNumberByConference returns the highest msg_number per conference (non-deleted only).
func (s *Store) HighMsgNumberByConference() (map[int]int, error) {
	rows, err := s.db.Query(`
		SELECT conference_id, COALESCE(MAX(msg_number),0)
		FROM messages WHERE status!='D'
		GROUP BY conference_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var cid, high int
		if err := rows.Scan(&cid, &high); err != nil {
			return nil, err
		}
		out[cid] = high
	}
	return out, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanMessage(sc scanner) (*Message, error) {
	var m Message
	var dateStr string
	var echo int
	var msgID, reply, kludges, seenBy, path, origin, network, exportedAt sql.NullString
	err := sc.Scan(&m.ID, &m.ConferenceID, &m.MsgNumber, &m.FromName, &m.ToName,
		&m.Subject, &dateStr, &m.Status, &echo, &m.Body,
		&msgID, &reply, &kludges, &seenBy, &path, &origin, &network, &exportedAt)
	if err != nil {
		return nil, err
	}
	m.DatePosted, _ = time.Parse(time.RFC3339, dateStr)
	m.Echo = echo != 0
	m.FidoMsgID = msgID.String
	m.FidoReply = reply.String
	m.FidoKludges = kludges.String
	m.FidoSeenBy = seenBy.String
	m.FidoPath = path.String
	m.FidoOrigin = origin.String
	m.FidoNetwork = network.String
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
