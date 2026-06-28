package web

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/virtbbs/virtbbs/internal/callers"
	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/config"
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
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(s.Root, "static")))))
	log.Printf("Web UI www root: %s", s.Root)
	return http.ListenAndServe(s.Addr, mux)
}

func (s *Server) templates() (*template.Template, error) {
	s.tmplOnce.Do(func() {
		pattern := filepath.Join(s.Root, "templates", "*.html")
		funcs := template.FuncMap{
			"add": func(a, b int) int { return a + b },
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
	BBSName string
	User    *users.User
	Flash   string
	Error   string
}

func (s *Server) base(r *http.Request) pageData {
	u, _ := s.currentUser(r)
	return pageData{BBSName: config.Get().BBS.Name, User: u}
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
