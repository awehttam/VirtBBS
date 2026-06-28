package conferences

import "strings"

const (
	EchoFromReal      = "real"
	EchoFromAlias     = "alias"
	EchoFromAnonymous = "anonymous"
)

// NormalizeEchoFromName returns a valid echo FromName policy (default real).
func NormalizeEchoFromName(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case EchoFromAlias, EchoFromAnonymous:
		return strings.ToLower(strings.TrimSpace(policy))
	default:
		return EchoFromReal
	}
}
