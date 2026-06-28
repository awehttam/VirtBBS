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
//   v0.6.0  2026-06-26  Phase 0 (VirtAnd/VirtTerm): user_api_tokens table +
//                        CreateAPIToken/AuthenticateToken/RevokeAPIToken/ListAPITokens
//                        for the new user-facing userapi package.
//   v0.9.0  2026-06-26  Sysop GUI gap-fill: ListAllAPITokens/RevokeAPITokenByID for
//                        sysop-side token administration (internal/api, GUI tab).
// ============================================================================

// Package users manages the VirtBBS user database.
package users

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schema string

// User represents a VirtBBS user account.
type User struct {
	ID              int64
	Name            string
	City            string
	PasswordHash    string
	PhoneBusiness   string
	PhoneHome       string
	LastLoginDate   string
	LastLoginTime   string
	SecurityLevel   int
	TimesOnline     int
	PageLength      int
	Uploads         int
	Downloads       int
	BytesUploaded   int64
	BytesDownloaded int64
	Comment1        string
	Comment2        string
	ElapsedTime     int
	ExpirationDate  string
	ExpertMode      bool
	XferProtocol    string
	ANSI            bool
	FullScreenEditor bool
	EditorType      string // "simple" or "full" (see internal/editor package)
	Deleted         bool
	Sysop           bool
}

// Store wraps a SQLite database for user operations.
type Store struct {
	db *sql.DB
}

// Open attaches to the shared database handle and applies the schema.
func Open(db *sql.DB) (*Store, error) {
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("users migration: %w", err)
	}
	return s, nil
}

// migrate applies backwards-compatible ALTER TABLE statements.
// Errors for columns that already exist are silently ignored.
func (s *Store) migrate() error {
	alters := []string{
		`ALTER TABLE users ADD COLUMN editor_type TEXT NOT NULL DEFAULT 'simple'`,
	}
	for _, stmt := range alters {
		if _, err := s.db.Exec(stmt); err != nil {
			msg := err.Error()
			if !containsAny(msg, "duplicate column", "already exists") {
				return err
			}
		}
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

// Create inserts a new user, hashing the plain-text password.
func (s *Store) Create(u *User, plainPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	res, err := s.db.Exec(`
		INSERT INTO users (name, city, password_hash, phone_business, phone_home,
		    security_level, page_length, expert_mode, xfer_protocol, ansi,
		    full_screen_editor, editor_type, sysop, comment1, comment2, expiration_date)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		u.Name, u.City, u.PasswordHash, u.PhoneBusiness, u.PhoneHome,
		u.SecurityLevel, u.PageLength, boolInt(u.ExpertMode), u.XferProtocol,
		boolInt(u.ANSI), boolInt(u.FullScreenEditor), u.EditorType, boolInt(u.Sysop),
		u.Comment1, u.Comment2, u.ExpirationDate,
	)
	if err != nil {
		return fmt.Errorf("create user %q: %w", u.Name, err)
	}
	u.ID, _ = res.LastInsertId()
	return nil
}

// Authenticate returns the user if name+password match; ErrBadCredentials otherwise.
var ErrBadCredentials = fmt.Errorf("invalid username or password")

func (s *Store) Authenticate(name, plainPassword string) (*User, error) {
	u, err := s.GetByName(name)
	if err != nil {
		return nil, ErrBadCredentials
	}
	if u.Deleted {
		return nil, ErrBadCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(plainPassword)); err != nil {
		return nil, ErrBadCredentials
	}
	return u, nil
}

// GetByName fetches a user by exact name (case-sensitive, space-padded names are trimmed on insert).
func (s *Store) GetByName(name string) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE name = ? AND deleted = 0`, name)
	return scanUser(row)
}

// GetByID fetches a user by primary key.
func (s *Store) GetByID(id int64) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// List returns all non-deleted users, ordered by name.
func (s *Store) List() ([]*User, error) {
	rows, err := s.db.Query(`SELECT ` + userCols + ` FROM users WHERE deleted = 0 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Update saves changed fields for an existing user.
func (s *Store) Update(u *User) error {
	_, err := s.db.Exec(`
		UPDATE users SET city=?, phone_business=?, phone_home=?, security_level=?,
		    page_length=?, expert_mode=?, xfer_protocol=?, ansi=?, full_screen_editor=?,
		    editor_type=?, sysop=?, comment1=?, comment2=?, expiration_date=?, deleted=?,
		    times_online=?, uploads=?, downloads=?, bytes_uploaded=?, bytes_downloaded=?,
		    elapsed_time=?, updated_at=?
		WHERE id=?`,
		u.City, u.PhoneBusiness, u.PhoneHome, u.SecurityLevel,
		u.PageLength, boolInt(u.ExpertMode), u.XferProtocol, boolInt(u.ANSI),
		boolInt(u.FullScreenEditor), u.EditorType, boolInt(u.Sysop), u.Comment1, u.Comment2,
		u.ExpirationDate, boolInt(u.Deleted),
		u.TimesOnline, u.Uploads, u.Downloads, u.BytesUploaded, u.BytesDownloaded,
		u.ElapsedTime, time.Now().Format("2006-01-02 15:04:05"),
		u.ID,
	)
	return err
}

// SetPassword re-hashes and stores a new password for the user.
func (s *Store) SetPassword(id int64, plainPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE users SET password_hash=?, updated_at=? WHERE id=?`,
		string(hash), time.Now().Format("2006-01-02 15:04:05"), id)
	return err
}

// RecordLogin updates last_login_date/time and increments times_online.
func (s *Store) RecordLogin(id int64) error {
	now := time.Now()
	_, err := s.db.Exec(`
		UPDATE users SET last_login_date=?, last_login_time=?, times_online=times_online+1, updated_at=?
		WHERE id=?`,
		now.Format("2006-01-02"), now.Format("15:04"), now.Format("2006-01-02 15:04:05"), id,
	)
	return err
}

// Delete marks a user as deleted (soft delete).
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`UPDATE users SET deleted=1, updated_at=? WHERE id=?`,
		time.Now().Format("2006-01-02 15:04:05"), id)
	return err
}

// scanner interface satisfied by both *sql.Row and *sql.Rows
type scanner interface {
	Scan(...any) error
}

const userCols = `id, name, city, password_hash, phone_business, phone_home,
	last_login_date, last_login_time, security_level, times_online, page_length,
	uploads, downloads, bytes_uploaded, bytes_downloaded, comment1, comment2,
	elapsed_time, expiration_date, expert_mode, xfer_protocol, ansi,
	full_screen_editor, editor_type, deleted, sysop`

func scanUser(sc scanner) (*User, error) {
	var u User
	var expertMode, ansi, fullScreen, deleted, sysop int
	err := sc.Scan(
		&u.ID, &u.Name, &u.City, &u.PasswordHash, &u.PhoneBusiness, &u.PhoneHome,
		&u.LastLoginDate, &u.LastLoginTime, &u.SecurityLevel, &u.TimesOnline, &u.PageLength,
		&u.Uploads, &u.Downloads, &u.BytesUploaded, &u.BytesDownloaded,
		&u.Comment1, &u.Comment2, &u.ElapsedTime, &u.ExpirationDate,
		&expertMode, &u.XferProtocol, &ansi, &fullScreen, &u.EditorType, &deleted, &sysop,
	)
	if err != nil {
		return nil, err
	}
	u.ExpertMode = expertMode != 0
	u.ANSI = ansi != 0
	u.FullScreenEditor = fullScreen != 0
	u.Deleted = deleted != 0
	u.Sysop = sysop != 0
	if u.EditorType == "" {
		u.EditorType = "simple"
	}
	return &u, nil
}

// ── Conference tracking ───────────────────────────────────────────────────────

// GetLastRead returns the last message number read by this user in a conference.
// Returns 0 if no record exists.
func (s *Store) GetLastRead(userID int64, conferenceID int) int {
	var n int
	_ = s.db.QueryRow(
		`SELECT last_msg_read FROM user_conferences WHERE user_id=? AND conference_id=?`,
		userID, conferenceID,
	).Scan(&n)
	return n
}

// SetLastRead updates (or creates) the last-read record for a user/conference.
func (s *Store) SetLastRead(userID int64, conferenceID, lastMsgRead int) error {
	_, err := s.db.Exec(`
		INSERT INTO user_conferences (user_id, conference_id, last_msg_read)
		VALUES (?,?,?)
		ON CONFLICT(user_id, conference_id) DO UPDATE SET last_msg_read=excluded.last_msg_read`,
		userID, conferenceID, lastMsgRead)
	return err
}

// SetRegistered marks whether a user is subscribed to a conference (echo area).
func (s *Store) SetRegistered(userID int64, conferenceID int, registered bool) error {
	_, err := s.db.Exec(`
		INSERT INTO user_conferences (user_id, conference_id, registered, last_msg_read)
		VALUES (?,?,?,0)
		ON CONFLICT(user_id, conference_id) DO UPDATE SET registered=excluded.registered`,
		userID, conferenceID, boolInt(registered))
	return err
}

// ListRegistered returns conference IDs the user has subscribed to.
func (s *Store) ListRegistered(userID int64) (map[int]bool, error) {
	rows, err := s.db.Query(
		`SELECT conference_id FROM user_conferences WHERE user_id=? AND registered=1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var cid int
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		out[cid] = true
	}
	return out, rows.Err()
}

// NewMessageCounts returns, for each conference the user has a record in,
// the count of messages with msg_number > last_msg_read.
// Also returns any conference with new messages even if no user_conference record exists,
// using 0 as the baseline.
// Map: conferenceID → count of new messages.
func (s *Store) NewMessageCounts(userID int64) (map[int]int, error) {
	// Get last_msg_read per conference for this user.
	rows, err := s.db.Query(
		`SELECT conference_id, last_msg_read FROM user_conferences WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lastRead := map[int]int{}
	for rows.Next() {
		var cid, last int
		if err := rows.Scan(&cid, &last); err == nil {
			lastRead[cid] = last
		}
	}

	// Count new messages per conference.
	msgRows, err := s.db.Query(
		`SELECT conference_id, MAX(msg_number) FROM messages WHERE status!='D' GROUP BY conference_id`)
	if err != nil {
		return nil, err
	}
	defer msgRows.Close()
	counts := map[int]int{}
	for msgRows.Next() {
		var cid, high int
		if err := msgRows.Scan(&cid, &high); err == nil {
			baseline := lastRead[cid] // 0 if no record
			if high > baseline {
				counts[cid] = high - baseline
			}
		}
	}
	return counts, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ── API tokens (internal/userapi: VirtAnd, VirtTerm) ──────────────────────────

// APIToken describes a user-issued API token (the raw token value is never
// stored — only its SHA-256 hash, in the same spirit as the bcrypt password hash).
type APIToken struct {
	ID          int64
	UserID      int64
	DeviceLabel string
	CreatedAt   string
	RevokedAt   string // empty if still active
}

// hashToken returns the hex-encoded SHA-256 hash of a raw token value.
// SHA-256 (not bcrypt) is used here because tokens are high-entropy random
// values, not low-entropy human passwords — there's no brute-force risk to
// slow down, and a fast deterministic hash lets AuthenticateToken look the
// token up by hash directly instead of bcrypt-comparing against every row.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// CreateAPIToken generates a new random API token for userID, stores only
// its hash, and returns the raw token value (shown to the user exactly once).
func (s *Store) CreateAPIToken(userID int64, deviceLabel string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	raw := hex.EncodeToString(buf)
	_, err := s.db.Exec(`
		INSERT INTO user_api_tokens (user_id, token_hash, device_label)
		VALUES (?,?,?)`,
		userID, hashToken(raw), deviceLabel)
	if err != nil {
		return "", fmt.Errorf("create api token: %w", err)
	}
	return raw, nil
}

// AuthenticateToken looks up the user owning an active (non-revoked) raw
// token value. Returns ErrBadCredentials if not found or revoked.
func (s *Store) AuthenticateToken(raw string) (*User, error) {
	var userID int64
	err := s.db.QueryRow(`
		SELECT user_id FROM user_api_tokens
		WHERE token_hash = ? AND revoked_at IS NULL`,
		hashToken(raw)).Scan(&userID)
	if err != nil {
		return nil, ErrBadCredentials
	}
	u, err := s.GetByID(userID)
	if err != nil || u.Deleted {
		return nil, ErrBadCredentials
	}
	return u, nil
}

// RevokeAPIToken marks a token (by ID, scoped to userID) as revoked.
func (s *Store) RevokeAPIToken(userID, tokenID int64) error {
	_, err := s.db.Exec(`
		UPDATE user_api_tokens SET revoked_at=?
		WHERE id=? AND user_id=? AND revoked_at IS NULL`,
		time.Now().Format("2006-01-02 15:04:05"), tokenID, userID)
	return err
}

// RevokeAPITokenByID marks a token revoked by its ID alone, with no userID
// scoping check — for sysop-side administration (internal/api), where the
// caller is trusted to revoke any user's token, not just their own.
func (s *Store) RevokeAPITokenByID(tokenID int64) error {
	_, err := s.db.Exec(`
		UPDATE user_api_tokens SET revoked_at=?
		WHERE id=? AND revoked_at IS NULL`,
		time.Now().Format("2006-01-02 15:04:05"), tokenID)
	return err
}

// APITokenAdmin is an APIToken annotated with its owning user's name, for
// the sysop-side "all tokens" administrative view.
type APITokenAdmin struct {
	APIToken
	UserName string
}

// ListAllAPITokens returns every issued token across all users (active and
// revoked), newest first, for sysop administration.
func (s *Store) ListAllAPITokens() ([]*APITokenAdmin, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.user_id, t.device_label, t.created_at, COALESCE(t.revoked_at, ''), u.name
		FROM user_api_tokens t
		JOIN users u ON u.id = t.user_id
		ORDER BY t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*APITokenAdmin
	for rows.Next() {
		t := &APITokenAdmin{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.DeviceLabel, &t.CreatedAt, &t.RevokedAt, &t.UserName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListAPITokens returns all tokens (active and revoked) issued to userID,
// newest first.
func (s *Store) ListAPITokens(userID int64) ([]*APIToken, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, device_label, created_at, COALESCE(revoked_at, '')
		FROM user_api_tokens WHERE user_id=? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*APIToken
	for rows.Next() {
		t := &APIToken{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.DeviceLabel, &t.CreatedAt, &t.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
