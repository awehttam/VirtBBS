package users

import (
	"fmt"
	"strings"
)

// AddressBookEntry is a user's saved contact.
type AddressBookEntry struct {
	ID       int64
	UserID   int64
	Name     string
	FidoAddr string
	Email    string
	Notes    string
	Language string
	Created  string
}

func (s *Store) ensureAddressBookSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS user_address_book (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name       TEXT NOT NULL,
		fido_addr  TEXT NOT NULL DEFAULT '',
		email      TEXT NOT NULL DEFAULT '',
		notes      TEXT NOT NULL DEFAULT '',
		language   TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`ALTER TABLE user_address_book ADD COLUMN language TEXT NOT NULL DEFAULT ''`)
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_address_book_user ON user_address_book(user_id)`)
	return err
}

// ListAddressBook returns entries for a user, optionally filtered by query.
func (s *Store) ListAddressBook(userID int64, query string) ([]*AddressBookEntry, error) {
	if err := s.ensureAddressBookSchema(); err != nil {
		return nil, err
	}
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		r, e := s.db.Query(`SELECT id, user_id, name, fido_addr, email, notes, language, created_at
			FROM user_address_book WHERE user_id=? ORDER BY name`, userID)
		if e != nil {
			return nil, e
		}
		defer r.Close()
		return scanAddressRows(r)
	}
	like := "%" + q + "%"
	r, err := s.db.Query(`SELECT id, user_id, name, fido_addr, email, notes, language, created_at
		FROM user_address_book WHERE user_id=?
		AND (lower(name) LIKE ? OR lower(fido_addr) LIKE ? OR lower(email) LIKE ? OR lower(notes) LIKE ? OR lower(language) LIKE ?)
		ORDER BY name`, userID, like, like, like, like, like)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return scanAddressRows(r)
}

func scanAddressRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]*AddressBookEntry, error) {
	var out []*AddressBookEntry
	for rows.Next() {
		e := &AddressBookEntry{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.Name, &e.FidoAddr, &e.Email, &e.Notes, &e.Language, &e.Created); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddAddressBookEntry inserts a contact for userID.
func (s *Store) AddAddressBookEntry(userID int64, name, fidoAddr, email, notes, language string) error {
	if err := s.ensureAddressBookSchema(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	_, err := s.db.Exec(`INSERT INTO user_address_book (user_id, name, fido_addr, email, notes, language)
		VALUES (?,?,?,?,?,?)`, userID, name, strings.TrimSpace(fidoAddr), strings.TrimSpace(email),
		strings.TrimSpace(notes), strings.TrimSpace(language))
	return err
}

// DeleteAddressBookEntry removes an entry owned by userID.
func (s *Store) DeleteAddressBookEntry(userID, entryID int64) error {
	if err := s.ensureAddressBookSchema(); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM user_address_book WHERE id=? AND user_id=?`, entryID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("entry not found")
	}
	return nil
}

// UpdateAddressBookEntry updates an entry owned by userID.
func (s *Store) UpdateAddressBookEntry(userID, entryID int64, name, fidoAddr, email, notes, language string) error {
	if err := s.ensureAddressBookSchema(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	res, err := s.db.Exec(`UPDATE user_address_book SET name=?, fido_addr=?, email=?, notes=?, language=?
		WHERE id=? AND user_id=?`, name, strings.TrimSpace(fidoAddr), strings.TrimSpace(email),
		strings.TrimSpace(notes), strings.TrimSpace(language), entryID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("entry not found")
	}
	return nil
}
