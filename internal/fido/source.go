package fido

import (
	"fmt"
	"strings"
)

// SourceOpts holds stored Fido message columns used to reconstruct FTS body text.
type SourceOpts struct {
	Body        string
	FidoMsgID   string
	FidoReply   string
	FidoKludges string // \r-separated ^A kludge lines (TZUTC, LANG, etc.)
	FidoSeenBy  string // space-separated net/node tokens
	FidoPath    string // space-separated net/node tokens
	AreaTag     string // optional echo AREA: tag
}

// ReconstructSource rebuilds FTS-style message body from stored columns.
// Order: AREA (if echo), MSGID, REPLY, kludge lines, body, SEEN-BY, PATH.
func ReconstructSource(opts SourceOpts) string {
	var sb strings.Builder

	if opts.AreaTag != "" {
		fmt.Fprintf(&sb, "AREA:%s\r", opts.AreaTag)
	}
	if opts.FidoMsgID != "" {
		fmt.Fprintf(&sb, "\x01MSGID: %s\r", opts.FidoMsgID)
	}
	if opts.FidoReply != "" {
		fmt.Fprintf(&sb, "\x01REPLY: %s\r", opts.FidoReply)
	}
	if opts.FidoKludges != "" {
		for _, line := range strings.Split(opts.FidoKludges, "\r") {
			line = strings.TrimRight(line, "\n")
			if line != "" {
				sb.WriteString(line)
				sb.WriteString("\r")
			}
		}
	}

	body := opts.Body
	sb.WriteString(body)
	if body != "" && !strings.HasSuffix(body, "\r") {
		sb.WriteString("\r")
	}

	if opts.FidoSeenBy != "" {
		fmt.Fprintf(&sb, "SEEN-BY: %s\r", opts.FidoSeenBy)
	}
	if opts.FidoPath != "" {
		fmt.Fprintf(&sb, "\x01PATH: %s\r", opts.FidoPath)
	}

	return sb.String()
}
