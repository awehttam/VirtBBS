package web

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/virtbbs/virtbbs/internal/messages"
)

// quoteReplyBody builds a compose textarea prefill for a reply, following the
// binkterm-php pattern: attribution line plus FSC-0032-style quoted lines.
func quoteReplyBody(orig *messages.Message) string {
	date := orig.DatePosted.Format("January 2 2006")
	attribution := fmt.Sprintf("\nOn %s, %s wrote:\n", date, orig.FromName)
	initials := replyInitials(orig.FromName)
	prefix := " " + initials + "> "

	var quoted []string
	for _, line := range strings.Split(orig.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			quoted = append(quoted, "")
			continue
		}
		// Drop Fido kludge lines from the quoted block.
		if strings.HasPrefix(trimmed, "\x01") || strings.HasPrefix(trimmed, "^A") {
			continue
		}
		quoted = append(quoted, prefix+trimmed)
	}
	return attribution + "\n" + strings.Join(quoted, "\n")
}

func replyInitials(name string) string {
	var letters []rune
	for _, r := range name {
		if unicode.IsLetter(r) {
			letters = append(letters, unicode.ToUpper(r))
			if len(letters) == 2 {
				break
			}
		}
	}
	if len(letters) == 0 {
		return "??"
	}
	return string(letters)
}

func replySubject(origSubject string) string {
	subject := strings.TrimSpace(origSubject)
	if len(subject) >= 3 && strings.EqualFold(subject[:3], "re:") {
		subject = strings.TrimSpace(subject[3:])
	}
	return "Re: " + subject
}
