package web

import (
	"bytes"
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
	"github.com/virtbbs/virtbbs/internal/fido"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/node"
	"github.com/virtbbs/virtbbs/internal/users"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if u, ok := s.currentUser(r); ok && r.Method == http.MethodGet {
		_ = u
		http.Redirect(w, r, "/menu", http.StatusSeeOther)
		return
	}
	data := s.page(r)
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data.Error = "Invalid form"
			s.render(w, "login.html", data)
			return
		}
		name := strings.TrimSpace(r.FormValue("username"))
		pass := r.FormValue("password")
		u, err := s.Deps.Users.Authenticate(name, pass)
		if err != nil {
			data.Error = "Invalid username or password"
			s.render(w, "login.html", data)
			return
		}
		token, err := s.Sessions.Create(u.ID)
		if err != nil {
			data.Error = "Session error"
			s.render(w, "login.html", data)
			return
		}
		setSessionCookie(w, token)
		_ = s.Deps.Users.RecordLogin(u.ID)
		http.Redirect(w, r, "/menu", http.StatusSeeOther)
		return
	}
	s.render(w, "login.html", data)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := sessionToken(r); token != "" {
		s.Sessions.Delete(token)
	}
	clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleMenu(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	data := struct {
		pageData
		NetmailCount int
		NewMessages  []NewMessageLine
		Stats        DashboardStats
		Bulletins    []BulletinView
		LogonHTML    string
	}{
		pageData: pageData{BBSName: config.Get().BBS.Name, User: u},
	}
	if n, err := s.Deps.Messages.CountNetmail(u.Name, u.Sysop); err == nil {
		data.NetmailCount = n
	}
	data.NewMessages = s.gatherNewMessageLines(u)
	data.Stats = s.gatherDashboardStats(u)
	data.Bulletins = s.listBulletins()
	if html, err := s.renderDisplayHTML("LOGON", u); err == nil {
		data.LogonHTML = html
	}
	s.render(w, "menu.html", data)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	data := struct {
		pageData
		Stats       DashboardStats
		NewMessages []NewMessageLine
	}{
		pageData:    pageData{BBSName: config.Get().BBS.Name, User: u},
		Stats:       s.gatherDashboardStats(u),
		NewMessages: s.gatherNewMessageLines(u),
	}
	s.render(w, "stats.html", data)
}

func (s *Server) handleOnline(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	nodes, _ := s.onlineUsers()
	data := struct {
		pageData
		Nodes []*node.NodeInfo
	}{
		pageData: pageData{BBSName: config.Get().BBS.Name, User: u},
		Nodes:    nodes,
	}
	s.render(w, "online.html", data)
}

func (s *Server) handleBulletins(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	data := struct {
		pageData
		Bulletins []BulletinView
	}{
		pageData:  pageData{BBSName: config.Get().BBS.Name, User: u},
		Bulletins: s.listBulletins(),
	}
	s.render(w, "bulletins.html", data)
}

func (s *Server) handleBulletinView(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("name")))
	if name == "" {
		http.Redirect(w, r, "/bulletins", http.StatusSeeOther)
		return
	}
	html, err := s.renderDisplayHTML(name, u)
	if err != nil {
		http.Error(w, "bulletin not found", http.StatusNotFound)
		return
	}
	data := struct {
		pageData
		Name  string
		Title string
		HTML  string
	}{
		pageData: pageData{BBSName: config.Get().BBS.Name, User: u},
		Name:     name,
		Title:    bulletinTitle(name),
		HTML:     html,
	}
	s.render(w, "bulletin_view.html", data)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	confID, _ := strconv.Atoi(r.URL.Query().Get("conf"))
	all, err := s.Deps.Conferences.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var visible []*conferences.Conference
	for _, c := range all {
		if canReadConference(u, c) {
			visible = append(visible, c)
		}
	}
	if confID == 0 {
		data := struct {
			pageData
			Conferences []*conferences.Conference
		}{
			pageData:    pageData{BBSName: config.Get().BBS.Name, User: u},
			Conferences: visible,
		}
		s.render(w, "messages.html", data)
		return
	}
	c, err := s.Deps.Conferences.Get(confID)
	if err != nil || !canReadConference(u, c) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}
	msgs, err := s.Deps.Messages.List(confID, 50, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	canWrite := canWriteConference(u, c)
	data := struct {
		pageData
		Conference *conferences.Conference
		Messages   []*messages.Message
		CanWrite   bool
	}{
		pageData:   pageData{BBSName: config.Get().BBS.Name, User: u},
		Conference: c,
		Messages:   msgs,
		CanWrite:   canWrite,
	}
	s.render(w, "messages_list.html", data)
}

func (s *Server) handleMessageRead(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	confID, _ := strconv.Atoi(r.URL.Query().Get("conf"))
	msgNum, _ := strconv.Atoi(r.URL.Query().Get("num"))
	c, err := s.Deps.Conferences.Get(confID)
	if err != nil || !canReadConference(u, c) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}
	msg, err := s.Deps.Messages.Get(confID, msgNum)
	if err != nil {
		http.Error(w, "message not found", http.StatusNotFound)
		return
	}
	_ = s.Deps.Users.SetLastRead(u.ID, confID, msgNum)
	data := struct {
		pageData
		Conference *conferences.Conference
		Message    *messages.Message
		CanWrite   bool
	}{
		pageData:   pageData{BBSName: config.Get().BBS.Name, User: u},
		Conference: c,
		Message:    msg,
		CanWrite:   canWriteConference(u, c),
	}
	s.render(w, "read.html", data)
}

func (s *Server) handleMessagePost(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	confID, _ := strconv.Atoi(r.URL.Query().Get("conf"))
	replyNum, _ := strconv.Atoi(r.URL.Query().Get("reply"))
	c, err := s.Deps.Conferences.Get(confID)
	if err != nil || !canWriteConference(u, c) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}
	subject := ""
	toName := "All"
	body := ""
	if replyNum > 0 {
		if orig, err := s.Deps.Messages.Get(confID, replyNum); err == nil {
			toName = orig.FromName
			subject = replySubject(orig.Subject)
			body = quoteReplyBody(orig)
		}
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		subject = strings.TrimSpace(r.FormValue("subject"))
		toName = strings.TrimSpace(r.FormValue("to"))
		if toName == "" {
			toName = "All"
		}
		body := r.FormValue("body")
		if subject == "" || body == "" {
			data := struct {
				pageData
				Conference *conferences.Conference
				Subject    string
				ToName     string
				Body       string
				ReplyNum   int
				Error      string
			}{
				pageData:   pageData{BBSName: config.Get().BBS.Name, User: u},
				Conference: c,
				Subject:    subject,
				ToName:     toName,
				Body:       body,
				ReplyNum:   replyNum,
				Error:      "Subject and message body are required",
			}
			s.render(w, "post.html", data)
			return
		}
		m := &messages.Message{
			ConferenceID: confID,
			FromName:     u.Name,
			ToName:       toName,
			Subject:      subject,
			Body:         body,
			Status:       "P",
			Echo:         c.Echo,
		}
		if err := s.Deps.Messages.Post(m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/messages/read?conf=%d&num=%d", confID, m.MsgNumber), http.StatusSeeOther)
		return
	}
	data := struct {
		pageData
		Conference *conferences.Conference
		Subject    string
		ToName     string
		Body       string
		ReplyNum   int
	}{
		pageData:   pageData{BBSName: config.Get().BBS.Name, User: u},
		Conference: c,
		Subject:    subject,
		ToName:     toName,
		Body:       body,
		ReplyNum:   replyNum,
	}
	s.render(w, "post.html", data)
}

func (s *Server) handleNetmail(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	msgs, err := s.Deps.Messages.ListNetmail(u.Name, u.Sysop, 0, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := struct {
		pageData
		Messages []*messages.Message
	}{
		pageData: pageData{BBSName: config.Get().BBS.Name, User: u},
		Messages: msgs,
	}
	s.render(w, "netmail.html", data)
}

func (s *Server) handleNetmailRead(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	msgNum, _ := strconv.Atoi(r.URL.Query().Get("num"))
	msgs, err := s.Deps.Messages.ListNetmail(u.Name, u.Sysop, msgNum, 1)
	if err != nil || len(msgs) == 0 {
		http.Error(w, "message not found", http.StatusNotFound)
		return
	}
	data := struct {
		pageData
		Message *messages.Message
	}{
		pageData: pageData{BBSName: config.Get().BBS.Name, User: u},
		Message:  msgs[0],
	}
	s.render(w, "netmail_read.html", data)
}

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	all, err := s.Deps.Files.ListDirs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var dirs []*files.Dir
	for _, d := range all {
		if canReadDir(u, d) {
			dirs = append(dirs, d)
		}
	}
	data := struct {
		pageData
		Dirs []*files.Dir
	}{
		pageData: pageData{BBSName: config.Get().BBS.Name, User: u},
		Dirs:     dirs,
	}
	s.render(w, "files.html", data)
}

func (s *Server) handleFilesBrowse(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	dirID, _ := strconv.ParseInt(r.URL.Query().Get("dir"), 10, 64)
	dir, err := s.Deps.Files.GetDir(dirID)
	if err != nil || !canReadDir(u, dir) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}
	filesList, err := s.Deps.Files.ListFiles(dirID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := struct {
		pageData
		Dir      *files.Dir
		Files    []*files.File
		CanUpload bool
	}{
		pageData:  pageData{BBSName: config.Get().BBS.Name, User: u},
		Dir:       dir,
		Files:     filesList,
		CanUpload: canUploadDir(u, dir),
	}
	s.render(w, "files_browse.html", data)
}

func (s *Server) handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	dirID, _ := strconv.ParseInt(r.URL.Query().Get("dir"), 10, 64)
	filename := r.URL.Query().Get("file")
	dir, err := s.Deps.Files.GetDir(dirID)
	if err != nil || !canReadDir(u, dir) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}
	path := s.Deps.Files.AbsPath(dirID, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if dirFiles, err := s.Deps.Files.ListFiles(dirID); err == nil {
		for _, f := range dirFiles {
			if f.Filename == filename {
				_ = s.Deps.Files.IncrementDownloads(f.ID)
				break
			}
		}
	}
	modTime := time.Now()
	if fi, err := os.Stat(path); err == nil {
		modTime = fi.ModTime()
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(filename)))
	http.ServeContent(w, r, filename, modTime, bytes.NewReader(data))
}

func (s *Server) handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dirID, _ := strconv.ParseInt(r.FormValue("dir"), 10, 64)
	dir, err := s.Deps.Files.GetDir(dirID)
	if err != nil || !canUploadDir(u, dir) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "bad upload", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filename := filepath.Base(header.Filename)
	desc := r.FormValue("description")
	if err := s.Deps.Files.EnsureDirPath(dirID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dest := s.Deps.Files.AbsPath(dirID, filename)
	if err := os.WriteFile(dest, data, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.Deps.Files.RegisterUpload(dirID, filename, desc, u.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.Deps.Files.BuildLocalFile(config.Get().BBS.Name)
	http.Redirect(w, r, fmt.Sprintf("/files/browse?dir=%d", dirID), http.StatusSeeOther)
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	data := struct {
		pageData
		Profile *users.User
	}{
		pageData: pageData{BBSName: config.Get().BBS.Name, User: u},
		Profile:  u,
	}
	s.render(w, "profile.html", data)
}

func (s *Server) handleNodelist(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	network := strings.TrimSpace(r.URL.Query().Get("network"))
	if network == "" {
		network = "FidoNet"
	}
	var results *fido.SearchResult
	if query != "" {
		ndb := fido.OpenNodelistDB(s.Deps.Messages.DB())
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		results, _ = ndb.Search(network, query, page, 25)
	}
	data := struct {
		pageData
		Query   string
		Network string
		Results *fido.SearchResult
	}{
		pageData: pageData{BBSName: config.Get().BBS.Name, User: u},
		Query:    query,
		Network:  network,
		Results:  results,
	}
	s.render(w, "nodelist.html", data)
}
