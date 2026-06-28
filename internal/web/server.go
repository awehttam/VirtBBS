package web

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/virtbbs/virtbbs/internal/callers"
	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/files"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/node"
	"github.com/virtbbs/virtbbs/internal/users"
)

// Deps bundles store dependencies for the web UI.
type Deps struct {
	Users       *users.Store
	Messages    *messages.Store
	Conferences *conferences.Store
	Files       *files.Store
	Nodes       *node.Store
	Callers     *callers.Log
}

// Server serves the browser-based BBS interface.
type Server struct {
	Addr     string
	Root     string // www root (templates + static), relative to install dir
	Deps     Deps
	Sessions *SessionStore

	tmplOnce sync.Once
	tmpl     *template.Template
	tmplErr  error
}

// ListenAndServe starts the HTTP listener.
func (s *Server) ListenAndServe() error {
	if s.Sessions == nil {
		s.Sessions = NewSessionStore()
	}
	if err := SeedWWW(s.Root); err != nil {
		log.Printf("web: seed www: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/register", s.handleRegister)
	mux.HandleFunc("/register/welcome", s.handleRegisterWelcome)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/menu", s.handleMenu)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/online", s.handleOnline)
	mux.HandleFunc("/bulletins", s.handleBulletins)
	mux.HandleFunc("/bulletins/view", s.handleBulletinView)
	mux.HandleFunc("/messages", s.handleMessages)
	mux.HandleFunc("/messages/read", s.handleMessageRead)
	mux.HandleFunc("/messages/post", s.handleMessagePost)
	mux.HandleFunc("/netmail", s.handleNetmail)
	mux.HandleFunc("/netmail/read", s.handleNetmailRead)
	mux.HandleFunc("/files", s.handleFiles)
	mux.HandleFunc("/files/browse", s.handleFilesBrowse)
	mux.HandleFunc("/files/download", s.handleFilesDownload)
	mux.HandleFunc("/files/upload", s.handleFilesUpload)
	mux.HandleFunc("/profile", s.handleProfile)
	mux.HandleFunc("/nodelist", s.handleNodelist)
	mux.HandleFunc("/nodelist/node", s.handleNodelistNode)
	mux.HandleFunc("/nodelist/export", s.handleNodelistExport)
	mux.HandleFunc("/networks/about", s.handleNetworkAbout)
	mux.HandleFunc("/networks/map", s.handleNetworkMap)
	mux.HandleFunc("/networks/diagram", s.handleNetworkDiagram)
	mux.HandleFunc("/qwk", s.handleQWK)
	mux.HandleFunc("/subscriptions", s.handleSubscriptions)
	mux.HandleFunc("/search", s.handleSearch)
	mux.HandleFunc("/share/create", s.handleShareCreate)
	mux.HandleFunc("/share/created", s.handleShareCreated)
	mux.HandleFunc("/shared/", s.handleShared)
	mux.HandleFunc("/api/notify", s.handleNotify)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/manifest.webmanifest", s.handleManifest)
	mux.HandleFunc("/forgot-password", s.handleForgotPassword)
	mux.HandleFunc("/reset-password", s.handleResetPassword)
	mux.HandleFunc("/addressbook", s.handleAddressBook)
	mux.HandleFunc("/netmail/app", s.handleNetmailApp)
	mux.HandleFunc("/api/netmail", s.handleAPINetmail)
	mux.HandleFunc("/api/netmail/compose", s.handleAPINetmailCompose)
	mux.HandleFunc("/admin", s.handleAdmin)
	mux.HandleFunc("/admin/binkp", s.handleAdminBinkP)
	mux.HandleFunc("/admin/users", s.handleAdminUsers)
	mux.HandleFunc("/admin/users/edit", s.handleAdminUserEdit)
	mux.HandleFunc("/admin/nodes", s.handleAdminNodes)
	mux.HandleFunc("/admin/broadcast", s.handleAdminBroadcast)
	mux.HandleFunc("/admin/config", s.handleAdminConfig)
	mux.HandleFunc("/admin/conferences", s.handleAdminConferences)
	mux.HandleFunc("/admin/messages", s.handleAdminMessages)
	mux.HandleFunc("/admin/files", s.handleAdminFiles)
	mux.HandleFunc("/admin/callers", s.handleAdminCallers)
	mux.HandleFunc("/admin/tokens", s.handleAdminTokens)
	mux.HandleFunc("/admin/fido", s.handleAdminFido)
	mux.HandleFunc("/admin/fido/ops", s.handleAdminFidoOps)
	mux.HandleFunc("/admin/fido/networks", s.handleAdminFidoNetworks)
	mux.HandleFunc("/admin/fido/routing", s.handleAdminFidoRouting)
	mux.HandleFunc("/admin/fido/join", s.handleAdminFidoJoin)
	mux.HandleFunc("/admin/fido/downlinks", s.handleAdminFidoDownlinks)
	mux.HandleFunc("/admin/fido/tools", s.handleAdminFidoTools)
	mux.HandleFunc("/admin/fido/import", s.handleAdminFidoImportUpload)
	mux.HandleFunc("/admin/fido/nodelist", s.handleAdminFidoNodelist)
	mux.HandleFunc("/admin/fido/nodelist/add", s.handleAdminFidoNodelistAdd)
	mux.HandleFunc("/admin/fido/nodelist/node", s.handleAdminFidoNodelistNode)
	mux.HandleFunc("/set-locale", s.handleSetLocale)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(s.Root, "static")))))
	log.Printf("Web UI www root: %s", s.Root)
	return http.ListenAndServe(s.Addr, mux)
}

func (s *Server) templates() (*template.Template, error) {
	s.tmplOnce.Do(func() {
		initLocales()
		pattern := filepath.Join(s.Root, "templates", "*.html")
		funcs := template.FuncMap{
			"add": func(a, b int) int { return a + b },
			"sub": func(a, b int) int { return a - b },
			"urlquery": func(s string) string {
				return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
			},
			"t": func(locale, key string) string { return tr(locale, key) },
			"tf": func(locale, key string, args ...any) string { return trf(locale, key, args...) },
			"formatSize": func(locale string, bytes int64) string { return formatDataSize(bytes, locale) },
			"chartData": func(c StatsCharts) template.JS { return template.JS(c.ChartJSON()) },
			"chartJSON": func(s string) template.JS { return template.JS(s) },
			"webOp": func(locale, op string) string { return translateWebOp(locale, op) },
			"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		}
		s.tmpl, s.tmplErr = template.New("").Funcs(funcs).ParseGlob(pattern)
	})
	return s.tmpl, s.tmplErr
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	tmpl, err := s.templates()
	if err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "render error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

type pageData struct {
	BBSName     string
	User        *users.User
	Flash       string
	Error       string
	Locale      string
	NavNetworks []NetworkNavItem
}

func (s *Server) base(r *http.Request) pageData {
	return s.page(r)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if _, ok := s.currentUser(r); ok {
		http.Redirect(w, r, "/menu", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func canReadConference(u *users.User, c *conferences.Conference) bool {
	return u.SecurityLevel >= c.ReadSec
}

func canWriteConference(u *users.User, c *conferences.Conference) bool {
	return u.SecurityLevel >= c.WriteSec
}

func canReadDir(u *users.User, d *files.Dir) bool {
	return u.SecurityLevel >= d.ReadSec
}

func canUploadDir(u *users.User, d *files.Dir) bool {
	return u.SecurityLevel >= d.UploadSec
}

// absWWW resolves the configured www path to an absolute path for logging.
func absWWW(root string) string {
	if filepath.IsAbs(root) {
		return root
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return abs
}

// WWWPath returns the resolved absolute www directory (for diagnostics).
func WWWPath(root string) string {
	return absWWW(root)
}

// EnsureRoot creates the www directory if missing.
func EnsureRoot(root string) error {
	return os.MkdirAll(root, 0755)
}

// AddrString formats bind address like other VirtBBS listeners.
func AddrString(bind string, port int) string {
	return fmt.Sprintf("%s:%d", bind, port)
}
