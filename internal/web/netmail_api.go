package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/fido"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/users"
)

// netmailConferenceID is conference 0 (General), where tossed netmail is stored.
const netmailConferenceID = 0

type netmailListItem struct {
	*messages.Message
	Unread bool `json:"Unread"`
}

type netmailStatsResponse struct {
	Total  int `json:"total"`
	Unread int `json:"unread"`
}

type netmailReplyInfo struct {
	ToName  string `json:"to_name"`
	ToAddr  string `json:"to_addr"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type addressBookItem struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FidoAddr string `json:"fido_addr"`
	Email    string `json:"email"`
	Notes    string `json:"notes"`
	Language string `json:"language"`
}

type nodelistSearchItem struct {
	Addr4D   string `json:"addr"`
	Name     string `json:"name"`
	Sysop    string `json:"sysop"`
	Location string `json:"location"`
}

func (s *Server) netmailLastRead(userID int64) int {
	return s.Deps.Users.GetLastRead(userID, netmailConferenceID)
}

func (s *Server) netmailUnreadCount(u *users.User) int {
	n, err := s.Deps.Messages.CountNetmailUnread(u.Name, u.Sysop, s.netmailLastRead(u.ID))
	if err != nil {
		return 0
	}
	return n
}

func netmailListItems(msgs []*messages.Message, lastRead int) []netmailListItem {
	out := make([]netmailListItem, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, netmailListItem{
			Message: m,
			Unread:  m.MsgNumber > lastRead,
		})
	}
	return out
}

func buildNetmailReplyInfo(m *messages.Message) netmailReplyInfo {
	toName := strings.TrimSpace(m.FromName)
	toAddr := strings.TrimSpace(m.FidoOrigin)
	return netmailReplyInfo{
		ToName:  toName,
		ToAddr:  toAddr,
		Subject: replySubject(m.Subject),
		Body:    quoteReplyBody(m),
	}
}

func loadNetmailTaglines() []string {
	cfg := config.Get()
	path := strings.TrimSpace(cfg.Fido.TaglinesFile)
	if path == "" {
		if nd := cfg.Fido.NetworkByName(cfg.Fido.EffectivePrimaryName()); nd != nil {
			path = strings.TrimSpace(nd.TaglinesFile)
		}
	}
	return fido.LoadTaglines(path)
}

func (s *Server) handleAPINetmailDelete(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Num int `json:"num"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Num <= 0 {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	m, err := s.Deps.Messages.GetNetmail(u.Name, u.Sysop, body.Num)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if _, err := s.Deps.Messages.Delete(m.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.Deps.Users.ClampLastReadForConference(netmailConferenceID, body.Num-1)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"deleted": true})
}

func (s *Server) handleAPIAddressBook(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	locale := localeFromRequest(r)
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		entries, err := s.Deps.Users.ListAddressBook(u.ID, strings.TrimSpace(r.URL.Query().Get("q")))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out := make([]addressBookItem, 0, len(entries))
		for _, e := range entries {
			out = append(out, addressBookItem{
				ID: e.ID, Name: e.Name, FidoAddr: e.FidoAddr,
				Email: e.Email, Notes: e.Notes, Language: e.Language,
			})
		}
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodPost:
		var body addressBookItem
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if err := s.Deps.Users.AddAddressBookEntry(u.ID, body.Name, body.FidoAddr, body.Email, body.Notes, body.Language); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": tr(locale, "addressbook.flash.added")})
	case http.MethodPut:
		var body addressBookItem
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID <= 0 {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if err := s.Deps.Users.UpdateAddressBookEntry(u.ID, body.ID, body.Name, body.FidoAddr, body.Email, body.Notes, body.Language); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": tr(locale, "addressbook.flash.updated")})
	case http.MethodDelete:
		id, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
		if id <= 0 {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		if err := s.Deps.Users.DeleteAddressBookEntry(u.ID, id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": tr(locale, "addressbook.flash.deleted")})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPINodelistSearch(w http.ResponseWriter, r *http.Request) {
	_, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	network := strings.TrimSpace(r.URL.Query().Get("network"))
	if network == "" {
		names := nodelistNetworkNames()
		if len(names) > 0 {
			network = names[0]
		}
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if network == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]nodelistSearchItem{})
		return
	}
	if err := s.maybeRebuildHubNodelist(network); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ndb := fido.OpenNodelistDB(s.Deps.Messages.DB())
	res, err := ndb.Search(network, query, 1, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]nodelistSearchItem, 0)
	if res != nil {
		for _, n := range res.Nodes {
			if n == nil {
				continue
			}
			out = append(out, nodelistSearchItem{
				Addr4D:   n.Addr4D(),
				Name:     n.Name,
				Sysop:    n.Sysop,
				Location: n.Location,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleAPINetmailTaglines(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.currentUser(r); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(loadNetmailTaglines())
}

func (s *Server) handleAPINetmailStats(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	lastRead := s.netmailLastRead(u.ID)
	total, _ := s.Deps.Messages.CountNetmail(u.Name, u.Sysop)
	unread, _ := s.Deps.Messages.CountNetmailUnread(u.Name, u.Sysop, lastRead)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(netmailStatsResponse{Total: total, Unread: unread})
}

func writeNetmailMessageJSON(w http.ResponseWriter, locale string, u *users.User, m *messages.Message) {
	displayBody := FormatMessageBodyHTML(m.Body)
	if u.Sysop {
		displayBody = fido.ReconstructSource(fidoSourceOpts(m, ""))
	}
	resp := buildMessageViewJSON(locale, m, displayBody)
	reply := buildNetmailReplyInfo(m)
	resp.Reply = &reply
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func netmailI18nJSON(locale string) string {
	keys := []string{
		"netmail.empty",
		"netmail.app.select",
		"netmail.app.from",
		"netmail.app.to_prefix",
		"netmail.app.queued",
		"netmail.app.send_failed",
		"netmail.app.load_failed",
		"netmail.app.deleted",
		"netmail.app.delete_failed",
		"netmail.app.delete_confirm",
		"netmail.app.filter_all",
		"netmail.app.filter_unread",
		"netmail.app.stats",
		"netmail.app.reply",
		"netmail.app.delete",
		"netmail.app.nodelist_empty",
		"netmail.app.use_contact",
		"common.delete",
		"common.reply",
		"common.close",
		"common.search",
		"common.loading",
		"common.name",
		"common.add",
		"common.edit",
		"common.delete",
		"common.save",
		"addressbook.empty",
		"addressbook.add_contact",
		"addressbook.edit_contact",
		"addressbook.label.fido",
		"addressbook.label.email",
		"addressbook.label.notes",
		"addressbook.label.language",
		"addressbook.flash.added",
		"addressbook.flash.updated",
		"addressbook.flash.deleted",
		"addressbook.delete_confirm",
		"netmail.app.add_to_contacts",
		"nodelist.col.address",
		"nodelist.col.sysop",
		"read.fido_origin",
	}
	m := map[string]string{}
	for _, k := range keys {
		m[k] = tr(locale, k)
	}
	b, _ := json.Marshal(m)
	return string(b)
}
