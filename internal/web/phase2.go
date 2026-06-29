package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/files"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/qwk"
)

func (s *Server) shareStore() (*ShareStore, error) {
	return OpenShareStore(s.Deps.Messages.DB())
}

func (s *Server) handleQWK(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	all, _ := s.Deps.Conferences.List()
	var readable []*conferences.Conference
	for _, c := range all {
		if canReadConference(u, c) {
			readable = append(readable, c)
		}
	}
	data := struct {
		pageData
		Groups []conferences.NetworkGroup
		Flash  string
		Error  string
	}{
		pageData: s.page(r),
		Groups:   groupConferences(readable),
	}
	if r.Method == http.MethodPost {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			_ = r.ParseMultipartForm(8 << 20)
		} else {
			_ = r.ParseForm()
		}
		action := r.FormValue("action")
		switch action {
		case "download":
			var ids []int
			for _, c := range readable {
				if r.FormValue(fmt.Sprintf("conf_%d", c.ID)) == "on" {
					ids = append(ids, c.ID)
				}
			}
			if len(ids) == 0 {
				for _, c := range readable {
					ids = append(ids, c.ID)
				}
			}
			cfg := config.Get()
			meta := qwk.PacketMeta{BBSName: cfg.BBS.Name, SysopName: cfg.Sysop.Name, BBSID: "VBBS"}
			pkt, err := qwk.BuildPacket(meta, s.Deps.Users, s.Deps.Messages, s.Deps.Conferences, u.ID, ids)
			if err != nil {
				data.Error = err.Error()
				s.render(w, "qwk.html", data)
				return
			}
			safe := strings.Map(func(r rune) rune {
				if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
					return r
				}
				return '_'
			}, u.Name)
			fname := safe + ".QWK"
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fname))
			_, _ = w.Write(pkt)
			return
		case "upload":
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				data.Error = tr(data.Locale, "qwk.error.upload")
				s.render(w, "qwk.html", data)
				return
			}
			f, _, err := r.FormFile("rep")
			if err != nil {
				data.Error = tr(data.Locale, "qwk.error.missing_rep")
				s.render(w, "qwk.html", data)
				return
			}
			raw, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				data.Error = err.Error()
				s.render(w, "qwk.html", data)
				return
			}
			replies, err := qwk.ParseRep(raw)
			if err != nil {
				data.Error = err.Error()
				s.render(w, "qwk.html", data)
				return
			}
			var allowed []*qwk.ReplyMsg
			for _, rep := range replies {
				c, err := s.Deps.Conferences.Get(rep.ConferenceID)
				if err != nil {
					continue
				}
				if u.SecurityLevel >= c.WriteSec {
					allowed = append(allowed, rep)
				}
			}
			posted, err := qwk.PostReplies(s.Deps.Messages, s.Deps.Conferences, u, allowed)
			if err != nil {
				data.Error = err.Error()
				s.render(w, "qwk.html", data)
				return
			}
			data.Flash = trf(data.Locale, "qwk.flash.posted", posted)
		}
	}
	s.render(w, "qwk.html", data)
}

func (s *Server) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err == nil {
			if _, ok := r.PostForm["conf"]; ok {
				confID, _ := strconv.Atoi(r.FormValue("conf"))
				sub := r.FormValue("subscribe") == "1"
				_ = s.Deps.Users.SetRegistered(u.ID, confID, sub)
			}
		}
		http.Redirect(w, r, "/subscriptions", http.StatusSeeOther)
		return
	}
	registered, _ := s.Deps.Users.ListRegistered(u.ID)
	all, _ := s.Deps.Conferences.List()
	var rows []SubscriptionRow
	counts, _ := s.Deps.Users.NewMessageCounts(u.ID)
	for _, c := range all {
		if !c.Echo || !canReadConference(u, c) {
			continue
		}
		rows = append(rows, SubscriptionRow{
			Conference: c,
			Subscribed: registered[c.ID],
			NewCount:   counts[c.ID],
		})
	}
	data := struct {
		pageData
		Groups []SubscriptionNetworkGroup
	}{
		pageData: s.page(r),
		Groups:   groupSubscriptionRows(rows),
	}
	s.render(w, "subscriptions.html", data)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	type msgHit struct {
		Message    *messages.Message
		Conference string
	}
	var msgHits []msgHit
	var fileHits []*files.File
	if query != "" {
		raw, err := s.Deps.Messages.Search(query, 50)
		if err == nil {
			for _, m := range raw {
				c, err := s.Deps.Conferences.Get(m.ConferenceID)
				if err != nil || !canReadConference(u, c) {
					continue
				}
				msgHits = append(msgHits, msgHit{Message: m, Conference: c.Name})
			}
		}
		fileMatches, err := s.Deps.Files.Search(query)
		if err == nil {
			dirSec := map[int64]int{}
			if dirs, err := s.Deps.Files.ListDirs(); err == nil {
				for _, d := range dirs {
					dirSec[d.ID] = d.ReadSec
				}
			}
			for _, f := range fileMatches {
				if sec, ok := dirSec[f.DirID]; ok && u.SecurityLevel >= sec {
					fileHits = append(fileHits, f)
				}
			}
		}
	}
	data := struct {
		pageData
		Query    string
		Messages []msgHit
		Files    []*files.File
	}{
		pageData: s.page(r),
		Query:    query,
		Messages: msgHits,
		Files:    fileHits,
	}
	s.render(w, "search.html", data)
}

func (s *Server) handleShareCreate(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st, err := s.shareStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	kind := r.FormValue("kind")
	var key string
	switch kind {
	case "message":
		confID, _ := strconv.Atoi(r.FormValue("conf"))
		msgNum, _ := strconv.Atoi(r.FormValue("num"))
		c, err := s.Deps.Conferences.Get(confID)
		if err != nil || !canReadConference(u, c) {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}
		key, err = st.CreateMessageShare(u.ID, confID, msgNum)
	case "file":
		dirID, _ := strconv.ParseInt(r.FormValue("dir"), 10, 64)
		filename := r.FormValue("file")
		dir, err := s.Deps.Files.GetDir(dirID)
		if err != nil || !canReadDir(u, dir) {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}
		key, err = st.CreateFileShare(u.ID, dirID, filename)
	default:
		http.Error(w, "unknown kind", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/share/created?key="+key, http.StatusSeeOther)
}

func (s *Server) handleShareCreated(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	key := r.URL.Query().Get("key")
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s/shared/%s", scheme, r.Host, key)
	data := struct {
		pageData
		Key string
		URL string
	}{
		pageData: s.page(r),
		Key:      key,
		URL:      url,
	}
	s.render(w, "share_created.html", data)
}

func (s *Server) handleShared(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/shared/")
	if key == "" || strings.Contains(key, "/") {
		http.NotFound(w, r)
		return
	}
	st, err := s.shareStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sh, err := st.Get(key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch sh.Kind {
	case "message":
		c, err := s.Deps.Conferences.Get(sh.ConfID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		msg, err := s.Deps.Messages.Get(sh.ConfID, sh.MsgNum)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		data := struct {
			pageData
			Conference *conferences.Conference
			Message    *messages.Message
			Expires    time.Time
		}{
			pageData:   s.page(r),
			Conference: c,
			Message:    msg,
			Expires:    sh.ExpiresAt,
		}
		s.render(w, "shared_message.html", data)
	case "file":
		dir, err := s.Deps.Files.GetDir(sh.DirID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		path := s.Deps.Files.AbsPath(sh.DirID, sh.Filename)
		data, err := os.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		modTime := time.Now()
		if fi, err := os.Stat(path); err == nil {
			modTime = fi.ModTime()
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(sh.Filename)))
		http.ServeContent(w, r, sh.Filename, modTime, bytes.NewReader(data))
		_ = dir
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUser(r)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
		return
	}
	newTotal := 0
	if counts, err := s.Deps.Users.NewMessageCounts(u.ID); err == nil {
		for _, n := range counts {
			newTotal += n
		}
	}
	netmail := s.netmailUnreadCount(u)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"new_messages": newTotal,
		"netmail":      netmail,
	})
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	manifest := map[string]any{
		"name":             cfg.BBS.Name,
		"short_name":       cfg.BBS.Name,
		"description":      tr(localeFromRequest(r), "pwa.description"),
		"start_url":        "/menu",
		"display":          "standalone",
		"background_color": "#0a0a12",
		"theme_color":      "#12121f",
		"icons": []map[string]string{
			{"src": "/static/icon.svg", "sizes": "any", "type": "image/svg+xml"},
		},
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	_ = json.NewEncoder(w).Encode(manifest)
}
