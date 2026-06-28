package web

import (
	"github.com/virtbbs/virtbbs/internal/fido"
	"github.com/virtbbs/virtbbs/internal/messages"
)

func fidoSourceOpts(m *messages.Message, areaTag string) fido.SourceOpts {
	return fido.SourceOpts{
		Body:        m.Body,
		FidoMsgID:   m.FidoMsgID,
		FidoReply:   m.FidoReply,
		FidoKludges: m.FidoKludges,
		FidoSeenBy:  m.FidoSeenBy,
		FidoPath:    m.FidoPath,
		AreaTag:     areaTag,
	}
}
