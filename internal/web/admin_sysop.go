package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/virtbbs/virtbbs/internal/callers"
	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/files"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/node"
	"github.com/virtbbs/virtbbs/internal/users"
)

func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	cfg := config.Get()
	data := struct {
		pageData
		Config *config.Config
	}{
		pageData: s.page(r),
		Config:   cfg,
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		merged := *cfg
		merged.BBS.Name = strings.TrimSpace(r.FormValue("bbs_name"))
		merged.BBS.MaxNodes = formInt(r, "max_nodes", merged.BBS.MaxNodes)
		merged.Network.TelnetPort = formInt(r, "telnet_port", merged.Network.TelnetPort)
		merged.Network.SSHPort = formInt(r, "ssh_port", merged.Network.SSHPort)
		merged.Network.UserAPIPort = formInt(r, "userapi_port", merged.Network.UserAPIPort)
		merged.Network.UserAPIBind = strings.TrimSpace(r.FormValue("userapi_bind"))
		merged.Network.WebPort = formInt(r, "web_port", merged.Network.WebPort)
		merged.Network.WebBind = strings.TrimSpace(r.FormValue("web_bind"))
		merged.Paths.DB = strings.TrimSpace(r.FormValue("db_path"))
		merged.Paths.Files = strings.TrimSpace(r.FormValue("files_path"))
		merged.Paths.Logs = strings.TrimSpace(r.FormValue("logs_path"))
		merged.Paths.WWW = strings.TrimSpace(r.FormValue("www_path"))
		merged.Session.TimePerCallMins = formInt(r, "time_per_call_mins", merged.Session.TimePerCallMins)
		merged.Session.IdleTimeoutMins = formInt(r, "idle_timeout_mins", merged.Session.IdleTimeoutMins)
		merged.Session.NewUserSecurity = formInt(r, "new_user_security", merged.Session.NewUserSecurity)
		merged.Sysop.Name = strings.TrimSpace(r.FormValue("sysop_name"))
		if merged.Sysop.PasswordHash == "" {
			merged.Sysop.PasswordHash = cfg.Sysop.PasswordHash
		}
		if err := config.Save(&merged); err != nil {
			data.Error = err.Error()
		} else {
			data.Flash = "Configuration saved."
			data.Config = config.Get()
		}
	}
	s.render(w, "admin_config.html", data)
}

func (s *Server) handleAdminConferences(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		switch r.FormValue("action") {
		case "create", "update":
			c := conferences.Conference{
				Name:         strings.TrimSpace(r.FormValue("name")),
				Description:  strings.TrimSpace(r.FormValue("description")),
				Public:       formBool(r, "public"),
				ReadSec:      formInt(r, "read_sec", 10),
				WriteSec:     formInt(r, "write_sec", 10),
				SysopSec:     formInt(r, "sysop_sec", 110),
				Echo:         formBool(r, "echo"),
				EchoTag:      strings.TrimSpace(r.FormValue("echo_tag")),
				EchoFromName: conferences.NormalizeEchoFromName(r.FormValue("echo_from_name")),
				UplinkAddr:   strings.TrimSpace(r.FormValue("uplink_addr")),
				Network:      strings.TrimSpace(r.FormValue("network")),
			}
			if c.Echo && r.FormValue("echo_from_name") == "" {
				c.EchoFromName = conferences.EchoFromReal
			}
			if r.FormValue("action") == "update" {
				c.ID, _ = strconv.Atoi(r.FormValue("id"))
				_ = s.Deps.Conferences.Update(&c)
			} else {
				_ = s.Deps.Conferences.Create(&c)
			}
		case "set_echo_from":
			id, _ := strconv.Atoi(r.FormValue("id"))
			if conf, err := s.Deps.Conferences.Get(id); err == nil && conf != nil {
				conf.EchoFromName = conferences.NormalizeEchoFromName(r.FormValue("echo_from_name"))
				_ = s.Deps.Conferences.Update(conf)
			}
		case "delete":
			id, _ := strconv.Atoi(r.FormValue("id"))
			_ = s.Deps.Conferences.Delete(id)
		}
		http.Redirect(w, r, "/admin/conferences", http.StatusSeeOther)
		return
	}
	list, _ := s.Deps.Conferences.List()
	data := struct {
		pageData
		Conferences []*conferences.Conference
		Networks    []string
	}{
		pageData:    s.page(r),
		Conferences: list,
		Networks:    fidoNetworkNamesList(),
	}
	s.render(w, "admin_conferences.html", data)
}

func (s *Server) handleAdminMessages(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		if r.FormValue("action") == "delete" {
			id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
			if confID, err := s.Deps.Messages.Delete(id); err == nil && confID != 0 {
				if high, err := s.Deps.Messages.HighMsgNumber(confID); err == nil {
					_ = s.Deps.Users.ClampLastReadForConference(confID, high)
				}
			}
		}
		confID := r.FormValue("conf")
		http.Redirect(w, r, "/admin/messages?conf="+confID, http.StatusSeeOther)
		return
	}
	confID, confSelected := queryConfSelected(r)
	allConfs, _ := s.Deps.Conferences.List()
	var msgs []*messages.Message
	if confSelected {
		msgs, _ = s.Deps.Messages.List(confID, 50, 0)
	}
	data := struct {
		pageData
		Conferences []*conferences.Conference
		Messages    []*messages.Message
		ConfID      int
	}{
		pageData:    s.page(r),
		Conferences: allConfs,
		Messages:    msgs,
		ConfID:      confID,
	}
	s.render(w, "admin_messages.html", data)
}

func (s *Server) handleAdminFiles(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	if s.Deps.Files == nil {
		http.Error(w, "files store not available", http.StatusInternalServerError)
		return
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		switch r.FormValue("action") {
		case "create":
			name := strings.TrimSpace(r.FormValue("name"))
			path := strings.TrimSpace(r.FormValue("path"))
			if path == "" {
				path = strings.ToLower(name)
			}
			_, _ = s.Deps.Files.CreateDir(name, strings.TrimSpace(r.FormValue("description")),
				path, formInt(r, "read_sec", 0), formInt(r, "upload_sec", 0))
		case "update":
			d := files.Dir{
				ID:          int64(formInt(r, "id", 0)),
				Name:        strings.TrimSpace(r.FormValue("name")),
				Description: strings.TrimSpace(r.FormValue("description")),
				Path:        strings.TrimSpace(r.FormValue("path")),
				ReadSec:     formInt(r, "read_sec", 0),
				UploadSec:   formInt(r, "upload_sec", 0),
				Active:      formBool(r, "active"),
			}
			_ = s.Deps.Files.UpdateDir(&d)
		}
		http.Redirect(w, r, "/admin/files", http.StatusSeeOther)
		return
	}
	list, _ := s.Deps.Files.ListAllDirs()
	data := struct {
		pageData
		Dirs []*files.Dir
	}{
		pageData: s.page(r),
		Dirs:     list,
	}
	s.render(w, "admin_files.html", data)
}

func (s *Server) handleAdminCallers(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	var entries []*callers.Entry
	var statsText string
	if s.Deps.Callers != nil {
		if query != "" {
			entries, _ = s.Deps.Callers.Search(query, 100)
		} else {
			entries, _ = s.Deps.Callers.Recent(100)
		}
		if unique, total, err := s.Deps.Callers.DailyStats(); err == nil {
			statsText = fmt.Sprintf("Today: %d unique, %d total calls", unique, total)
		}
	}
	data := struct {
		pageData
		Entries   []*callers.Entry
		Query     string
		StatsText string
	}{
		pageData:  s.page(r),
		Entries:   entries,
		Query:     query,
		StatsText: statsText,
	}
	s.render(w, "admin_callers.html", data)
}

func (s *Server) handleAdminTokens(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		if r.FormValue("action") == "revoke" {
			id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
			_ = s.Deps.Users.RevokeAPITokenByID(id)
		}
		http.Redirect(w, r, "/admin/tokens", http.StatusSeeOther)
		return
	}
	tokens, _ := s.Deps.Users.ListAllAPITokens()
	data := struct {
		pageData
		Tokens []*users.APITokenAdmin
	}{
		pageData: s.page(r),
		Tokens:   tokens,
	}
	s.render(w, "admin_tokens.html", data)
}

func (s *Server) handleAdminUserEdit(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if id == 0 {
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}
	u, err := s.Deps.Users.GetByID(id)
	if err != nil || u == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	data := struct {
		pageData
		Edit *users.User
	}{
		pageData: s.page(r),
		Edit:     u,
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		u.City = strings.TrimSpace(r.FormValue("city"))
		u.RealName = strings.TrimSpace(r.FormValue("real_name"))
		u.PhoneBusiness = strings.TrimSpace(r.FormValue("phone"))
		u.SecurityLevel = formInt(r, "security", u.SecurityLevel)
		u.PageLength = formInt(r, "page_length", u.PageLength)
		u.ANSI = formBool(r, "ansi")
		u.Sysop = formBool(r, "sysop")
		u.Deleted = formBool(r, "deleted")
		u.Comment1 = strings.TrimSpace(r.FormValue("comment"))
		u.EditorType = strings.TrimSpace(r.FormValue("editor_type"))
		if u.EditorType == "" {
			u.EditorType = "simple"
		}
		if err := s.Deps.Users.Update(u); err != nil {
			data.Error = err.Error()
		} else {
			data.Flash = "User saved."
			data.Edit, _ = s.Deps.Users.GetByID(id)
		}
	}
	s.render(w, "admin_user_edit.html", data)
}

func (s *Server) handleAdminBroadcast(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		msg := strings.TrimSpace(r.FormValue("message"))
		if msg != "" {
			from := strings.TrimSpace(r.FormValue("from"))
			if from == "" {
				from = "Sysop"
			}
			node.BroadcastAll(from, msg)
		}
	}
	http.Redirect(w, r, "/admin/nodes", http.StatusSeeOther)
}
