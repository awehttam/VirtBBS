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
//   v0.0.2  2026-06-24  Phase 10: Integrate with registry (RegisterControl/UnregisterControl)
// ============================================================================

// Package node manages multi-node status tracking via SQLite.
package node

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS nodes (
    id          INTEGER PRIMARY KEY,
    status      TEXT    NOT NULL DEFAULT 'available',
    user_id     INTEGER,
    user_name   TEXT    NOT NULL DEFAULT '',
    city        TEXT    NOT NULL DEFAULT '',
    operation   TEXT    NOT NULL DEFAULT '',
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);`

// Status values
const (
	StatusAvailable = "available"
	StatusLogin     = "login"
	StatusMain      = "main"
	StatusMessages  = "messages"
	StatusFiles     = "files"
	StatusChat      = "chat"
	StatusDoor      = "door"
	StatusLogoff    = "logoff"
)

// NodeInfo holds the current state of a single BBS node.
type NodeInfo struct {
	ID        int
	Status    string
	UserID    int64
	UserName  string
	City      string
	Operation string
	UpdatedAt time.Time
}

// Store manages node status in SQLite.
type Store struct {
	db *sql.DB
}

func Open(db *sql.DB) (*Store, error) {
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("node schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Register claims a node slot, returning the node ID.
func (s *Store) Register() (int, error) {
	// Find lowest available slot
	res, err := s.db.Exec(`INSERT INTO nodes (status) VALUES (?)`, StatusLogin)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

// Update sets the current node status and operation string.
func (s *Store) Update(nodeID int, status, operation string, userID int64, userName, city string) error {
	_, err := s.db.Exec(`
		UPDATE nodes SET status=?, operation=?, user_id=?, user_name=?, city=?, updated_at=?
		WHERE id=?`,
		status, operation, userID, userName, city,
		time.Now().Format("2006-01-02 15:04:05"), nodeID,
	)
	return err
}

// Unregister removes a node from the active list.
func (s *Store) Unregister(nodeID int) error {
	_, err := s.db.Exec(`DELETE FROM nodes WHERE id=?`, nodeID)
	return err
}

// ClearAll removes every node row. Called at server startup to drop sessions
// that ended without running session cleanup (crash, kill -9, etc.).
func (s *Store) ClearAll() error {
	_, err := s.db.Exec(`DELETE FROM nodes`)
	return err
}

// PurgeInactive deletes node rows that no longer have a live connection in the
// in-memory registry (orphaned SQLite rows left after an unclean disconnect).
func (s *Store) PurgeInactive() error {
	active := ActiveIDs()
	if len(active) == 0 {
		_, err := s.db.Exec(`DELETE FROM nodes`)
		return err
	}
	placeholders := make([]string, len(active))
	args := make([]any, len(active))
	for i, id := range active {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`DELETE FROM nodes WHERE id NOT IN (%s)`, strings.Join(placeholders, ","))
	_, err := s.db.Exec(q, args...)
	return err
}

// List returns all currently active nodes.
func (s *Store) List() ([]*NodeInfo, error) {
	if err := s.PurgeInactive(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, status, COALESCE(user_id,0), user_name, city, operation, updated_at FROM nodes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*NodeInfo
	for rows.Next() {
		n := &NodeInfo{}
		var updStr string
		if err := rows.Scan(&n.ID, &n.Status, &n.UserID, &n.UserName, &n.City, &n.Operation, &updStr); err != nil {
			return nil, err
		}
		n.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updStr)
		out = append(out, n)
	}
	return out, rows.Err()
}
