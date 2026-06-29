package web

import (
	"net/http"

	"github.com/virtbbs/virtbbs/internal/fido"
	"github.com/virtbbs/virtbbs/internal/messages"
)

// messageLangLabel returns a localized language name when the message carries
// a ^ALANG/LANG kludge (FidoKludges), or "" if none.
func messageLangLabel(viewerLocale string, m *messages.Message) string {
	if m == nil {
		return ""
	}
	code := fido.ParseLangFromKludges(m.FidoKludges)
	if code == "" {
		return ""
	}
	label := tr(viewerLocale, "lang."+code)
	if label == "" || label == "lang."+code {
		return code
	}
	return label
}

type messageReadData struct {
	pageData
	LangLabel string
}

func readPageData(s *Server, r *http.Request, m *messages.Message) messageReadData {
	return messageReadData{
		pageData:  s.page(r),
		LangLabel: messageLangLabel(localeFromRequest(r), m),
	}
}

type messageViewResponse struct {
	*messages.Message
	DisplayBody string            `json:"DisplayBody,omitempty"`
	LangLabel   string            `json:"LangLabel,omitempty"`
	LangCode    string            `json:"LangCode,omitempty"`
	Reply       *netmailReplyInfo `json:"Reply,omitempty"`
}

func buildMessageViewJSON(locale string, m *messages.Message, displayBody string) messageViewResponse {
	langCode := fido.ParseLangFromKludges(m.FidoKludges)
	return messageViewResponse{
		Message:     m,
		DisplayBody: displayBody,
		LangLabel:   messageLangLabel(locale, m),
		LangCode:    langCode,
	}
}
