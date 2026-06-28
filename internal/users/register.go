package users

import (
	"fmt"
	"strings"

	"github.com/virtbbs/virtbbs/internal/config"
)

// RegisterNew creates a new user account using the same defaults as the
// terminal "NEW" login flow (session.newUser).
func (s *Store) RegisterNew(name, city, password string) (*User, error) {
	name = strings.TrimSpace(name)
	city = strings.TrimSpace(city)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if len(name) > 25 {
		return nil, fmt.Errorf("name must be 25 characters or less")
	}
	if password == "" {
		return nil, fmt.Errorf("password is required")
	}
	secLevel := config.Get().Session.NewUserSecurity
	if secLevel <= 0 {
		secLevel = 10
	}
	u := &User{
		Name:           name,
		City:           city,
		SecurityLevel:  secLevel,
		PageLength:     24,
		XferProtocol:   "Z",
		ANSI:           true,
		EditorType:     "simple",
	}
	if err := s.Create(u, password); err != nil {
		return nil, err
	}
	return u, nil
}
