package fido

import (
	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/messages"
)

// ApplyLocalEchoMeta assigns MSGID, REPLY, echo flag, and origin kludges (LANG, TZUTC)
// for a locally posted conference message. orig must be the sending node's address;
// pass Addr{} to skip when FidoNet is disabled or unconfigured.
func ApplyLocalEchoMeta(m *messages.Message, conf *conferences.Conference, orig Addr, lang string, replyTo *messages.Message) {
	if m == nil || orig == (Addr{}) {
		return
	}
	if conf != nil && conf.Echo {
		m.Echo = true
	}
	m.FidoMsgID = FormatMSGID(orig, NewMSGIDSerial())
	m.FidoKludges = MergeOriginKludges(m.FidoKludges, lang)
	if replyTo != nil && replyTo.FidoMsgID != "" {
		m.FidoReply = replyTo.FidoMsgID
	}
}
