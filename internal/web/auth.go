package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/virtbbs/virtbbs/internal/users"
)

const (
	sessionCookie = "virtbbs_session"
	sessionTTL    = 24 * time.Hour
)

type sessionEntry struct {
	userID    int64
	expiresAt time.Time
}

// SessionStore holds active web sessions in memory.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]sessionEntry
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]sessionEntry)}
}

func (s *SessionStore) Create(userID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	s.mu.Lock()
	s.sessions[token] = sessionEntry{userID: userID, expiresAt: time.Now().Add(sessionTTL)}
	s.mu.Unlock()
	return token, nil
}

func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func (s *SessionStore) UserID(token string) (int64, bool) {
	s.mu.RLock()
	e, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		if ok {
			s.Delete(token)
		}
		return 0, false
	}
	return e.userID, true
}

func sessionToken(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func (s *Server) currentUser(r *http.Request) (*users.User, bool) {
	token := sessionToken(r)
	if token == "" {
		return nil, false
	}
	uid, ok := s.Sessions.UserID(token)
	if !ok {
		return nil, false
	}
	u, err := s.Deps.Users.GetByID(uid)
	if err != nil || u.Deleted {
		return nil, false
	}
	return u, true
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (*users.User, bool) {
	u, ok := s.currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return nil, false
	}
	return u, true
}
