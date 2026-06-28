package postname

import (
	"fmt"
	"strings"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/users"
)

// ForConference returns the FromName for a new message in this conference.
func ForConference(c *conferences.Conference, u *users.User) string {
	if c == nil || u == nil {
		return ""
	}
	if !c.Echo {
		return u.Name
	}
	return FromPolicy(c.EchoFromName, u)
}

// FromPolicy maps a policy string to a display FromName.
func FromPolicy(policy string, u *users.User) string {
	if u == nil {
		return ""
	}
	switch conferences.NormalizeEchoFromName(policy) {
	case conferences.EchoFromAlias:
		return u.Name
	case conferences.EchoFromAnonymous:
		return "Anonymous"
	default:
		if rn := strings.TrimSpace(u.RealName); rn != "" {
			return rn
		}
		return u.Name
	}
}

// ValidateEchoPost returns an error when the user cannot post to an echo area.
func ValidateEchoPost(c *conferences.Conference, u *users.User) error {
	if c == nil || !c.Echo || u == nil {
		return nil
	}
	if conferences.NormalizeEchoFromName(c.EchoFromName) == conferences.EchoFromReal && strings.TrimSpace(u.RealName) == "" {
		return fmt.Errorf("real name is required for this echo area")
	}
	return nil
}
