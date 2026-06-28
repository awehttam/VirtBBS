package web

import (
	"net/http"
	"strings"

	"github.com/virtbbs/virtbbs/internal/config"
)

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if u, ok := s.currentUser(r); ok && r.Method == http.MethodGet {
		_ = u
		http.Redirect(w, r, "/menu", http.StatusSeeOther)
		return
	}
	data := struct {
		pageData
		Name  string
		City  string
		Error string
	}{
		pageData: s.page(r),
	}
	if r.Method == http.MethodPost {
		if r.FormValue("website") != "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if err := r.ParseForm(); err != nil {
			data.Error = "Invalid form"
			s.render(w, "register.html", data)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		city := strings.TrimSpace(r.FormValue("city"))
		pass := r.FormValue("password")
		pass2 := r.FormValue("password_confirm")
		data.Name = name
		data.City = city
		if pass != pass2 || pass == "" {
			data.Error = "Passwords do not match."
			s.render(w, "register.html", data)
			return
		}
		u, err := s.Deps.Users.RegisterNew(name, city, pass)
		if err != nil {
			data.Error = err.Error()
			s.render(w, "register.html", data)
			return
		}
		token, err := s.Sessions.Create(u.ID)
		if err != nil {
			data.Error = "Account created but login failed — try signing in."
			s.render(w, "register.html", data)
			return
		}
		setSessionCookie(w, token)
		_ = s.Deps.Users.RecordLogin(u.ID)
		http.Redirect(w, r, "/register/welcome", http.StatusSeeOther)
		return
	}
	s.render(w, "register.html", data)
}

func (s *Server) handleRegisterWelcome(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	html, _ := s.renderDisplayHTML("NEWUSER", u)
	data := struct {
		pageData
		WelcomeHTML string
	}{
		pageData:    pageData{BBSName: config.Get().BBS.Name, User: u},
		WelcomeHTML: html,
	}
	s.render(w, "register_welcome.html", data)
}
