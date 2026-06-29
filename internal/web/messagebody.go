package web

import (
	"html"
	"strings"
)

func bodyHasANSI(raw string) bool {
	return strings.Contains(raw, "\x1b[")
}

// FormatMessageBodyHTML renders a message body for safe HTML display.
// ANSI sequences take precedence, then StyleCodes, else plain escaped text.
func FormatMessageBodyHTML(body string) string {
	if body == "" {
		return ""
	}
	if bodyHasANSI(body) {
		return `<div class="ansi-screen">` + ansiToHTML(body) + `</div>`
	}
	if hasStyleCodes(body) {
		return styleCodesToHTML(body)
	}
	escaped := html.EscapeString(body)
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\n")
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	return escaped
}
