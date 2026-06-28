package users

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

func (s *Store) ensurePasswordResetSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS password_reset_tokens (
		token_hash  TEXT PRIMARY KEY,
		user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at  TEXT NOT NULL,
		created_at  TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	return err
}

func hashResetToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// CreatePasswordResetToken issues a one-hour reset token for the named user.
// Returns the raw token (show to user once) or error if user not found.
func (s *Store) CreatePasswordResetToken(username string) (string, error) {
	if err := s.ensurePasswordResetSchema(); err != nil {
		return "", err
	}
	u, err := s.GetByName(username)
	if err != nil {
		return "", fmt.Errorf("user not found")
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	raw := hex.EncodeToString(buf)
	exp := time.Now().Add(time.Hour).Format(time.RFC3339)
	_, err = s.db.Exec(`INSERT INTO password_reset_tokens (token_hash, user_id, expires_at) VALUES (?,?,?)`,
		hashResetToken(raw), u.ID, exp)
	if err != nil {
		return "", err
	}
	return raw, nil
}

// ResetPasswordWithToken validates a reset token and sets a new password.
func (s *Store) ResetPasswordWithToken(rawToken, newPassword string) error {
	if err := s.ensurePasswordResetSchema(); err != nil {
		return err
	}
	if newPassword == "" {
		return fmt.Errorf("password is required")
	}
	var userID int64
	var expStr string
	err := s.db.QueryRow(`SELECT user_id, expires_at FROM password_reset_tokens WHERE token_hash=?`,
		hashResetToken(rawToken)).Scan(&userID, &expStr)
	if err != nil {
		return fmt.Errorf("invalid or expired reset link")
	}
	exp, err := time.Parse(time.RFC3339, expStr)
	if err != nil || time.Now().After(exp) {
		_, _ = s.db.Exec(`DELETE FROM password_reset_tokens WHERE token_hash=?`, hashResetToken(rawToken))
		return fmt.Errorf("invalid or expired reset link")
	}
	if err := s.SetPassword(userID, newPassword); err != nil {
		return err
	}
	_, _ = s.db.Exec(`DELETE FROM password_reset_tokens WHERE token_hash=?`, hashResetToken(rawToken))
	return nil
}
