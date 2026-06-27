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
//   v0.0.6  2026-06-24  Add EchoTag, UplinkAddr, Network fields; ADD COLUMN migrations
// ============================================================================

// Package conferences manages BBS conferences (message areas).
package conferences

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Conference represents a message conference.
type Conference struct {
	ID          int    `json:"ID"`
	Name        string `json:"Name"`
	Description string `json:"Description"`
	Public      bool   `json:"Public"`
	ReadSec     int    `json:"ReadSec"`
	WriteSec    int    `json:"WriteSec"`
	SysopSec    int    `json:"SysopSec"`
	Echo        bool   `json:"Echo"`       // true = echomail area
	EchoTag     string `json:"EchoTag"`    // AREA: tag for this conference
	UplinkAddr  string `json:"UplinkAddr"` // override uplink (blank = default)
	Network     string `json:"Network"`    // network name (blank = primary)
}

// Store wraps the SQLite messages DB for conference operations.
type Store struct {
	db *sql.DB
}

// Open attaches to the shared database handle and runs any pending migrations.
// The messages schema (conferences table) must already be applied.
func Open(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("conferences migration: %w", err)
	}
	return s, nil
}

// migrate applies ALTER TABLE statements to add columns that may not exist
// in databases created before v0.0.6. Errors for duplicate columns are
// silently ignored.
func (s *Store) migrate() error {
	alters := []string{
		`ALTER TABLE conferences ADD COLUMN echo_tag    TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE conferences ADD COLUMN uplink_addr TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE conferences ADD COLUMN network     TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range alters {
		if _, err := s.db.Exec(stmt); err != nil {
			// "duplicate column name" is expected on fresh schemas — ignore it.
			if !isDuplicateCol(err) {
				return err
			}
		}
	}
	return nil
}

func isDuplicateCol(err error) bool {
	if err == nil {
		return false
	}
	e := err.Error()
	return len(e) > 0 && (contains(e, "duplicate column") || contains(e, "already exists"))
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Close is a no-op; the shared *sql.DB is owned by the caller.
func (s *Store) Close() error { return nil }

const confCols = `id, name, description, public, read_sec, write_sec, sysop_sec,
	echo, echo_tag, uplink_addr, network`

func scanConf(row interface{ Scan(...any) error }) (*Conference, error) {
	c := &Conference{}
	var pub, echo int
	err := row.Scan(&c.ID, &c.Name, &c.Description, &pub,
		&c.ReadSec, &c.WriteSec, &c.SysopSec,
		&echo, &c.EchoTag, &c.UplinkAddr, &c.Network)
	if err != nil {
		return nil, err
	}
	c.Public = pub != 0
	c.Echo = echo != 0
	return c, nil
}

// List returns all conferences ordered by ID.
func (s *Store) List() ([]*Conference, error) {
	rows, err := s.db.Query(`SELECT ` + confCols + ` FROM conferences ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Conference
	for rows.Next() {
		c, err := scanConf(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListEcho returns all echomail conferences for a given network
// (blank network = all echomail conferences).
func (s *Store) ListEcho(network string) ([]*Conference, error) {
	var rows *sql.Rows
	var err error
	if network == "" {
		rows, err = s.db.Query(`SELECT `+confCols+` FROM conferences WHERE echo=1 ORDER BY id`)
	} else {
		rows, err = s.db.Query(`SELECT `+confCols+` FROM conferences WHERE echo=1 AND network=? ORDER BY id`, network)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Conference
	for rows.Next() {
		c, err := scanConf(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get fetches a single conference by ID.
func (s *Store) Get(id int) (*Conference, error) {
	row := s.db.QueryRow(`SELECT `+confCols+` FROM conferences WHERE id=?`, id)
	return scanConf(row)
}

// GetByName finds a conference by its exact name, or nil if none exists.
// Used by fido.EnsureConference to find-or-create a conference by name
// rather than by echo tag.
func (s *Store) GetByName(name string) (*Conference, error) {
	row := s.db.QueryRow(`SELECT `+confCols+` FROM conferences WHERE name=?`, name)
	c, err := scanConf(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// GetByTag finds an echomail conference by its AREA: tag and network.
func (s *Store) GetByTag(tag, network string) (*Conference, error) {
	row := s.db.QueryRow(`SELECT `+confCols+` FROM conferences WHERE echo_tag=? AND network=? AND echo=1`, tag, network)
	return scanConf(row)
}

// Create adds a new conference.
func (s *Store) Create(c *Conference) error {
	res, err := s.db.Exec(
		`INSERT INTO conferences (name, description, public, read_sec, write_sec, sysop_sec, echo, echo_tag, uplink_addr, network)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		c.Name, c.Description, boolInt(c.Public),
		c.ReadSec, c.WriteSec, c.SysopSec,
		boolInt(c.Echo), c.EchoTag, c.UplinkAddr, c.Network,
	)
	if err != nil {
		return fmt.Errorf("create conference %q: %w", c.Name, err)
	}
	id, _ := res.LastInsertId()
	c.ID = int(id)
	return nil
}

// Update saves changes to a conference.
func (s *Store) Update(c *Conference) error {
	_, err := s.db.Exec(
		`UPDATE conferences SET name=?, description=?, public=?, read_sec=?, write_sec=?, sysop_sec=?,
		  echo=?, echo_tag=?, uplink_addr=?, network=? WHERE id=?`,
		c.Name, c.Description, boolInt(c.Public),
		c.ReadSec, c.WriteSec, c.SysopSec,
		boolInt(c.Echo), c.EchoTag, c.UplinkAddr, c.Network,
		c.ID,
	)
	return err
}

// Delete removes a conference (messages cascade via FK if enabled).
func (s *Store) Delete(id int) error {
	_, err := s.db.Exec(`DELETE FROM conferences WHERE id=?`, id)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
