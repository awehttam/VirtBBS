package web

import (
	"net/http"
	"strings"
)

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if u, ok := s.currentUser(r); ok && r.Method == http.MethodGet {
		_ = u
		http.Redirect(w, r, "/menu", http.StatusSeeOther)
		return
	}
	data := struct {
		pageData
		Name     string
		RealName string
		City     string
		Error    string
	}{
		pageData: s.page(r),
	}
	if r.Method == http.MethodPost {
		if r.FormValue("website") != "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if err := r.ParseForm(); err != nil {
			data.Error = tr(data.Locale, "register.error.form")
			s.render(w, "register.html", data)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		realName := strings.TrimSpace(r.FormValue("real_name"))
		city := strings.TrimSpace(r.FormValue("city"))
		pass := r.FormValue("password")
		pass2 := r.FormValue("password_confirm")
		data.Name = name
		data.RealName = realName
		data.City = city
		if pass != pass2 || pass == "" {
			data.Error = tr(data.Locale, "register.error.password_mismatch")
			s.render(w, "register.html", data)
			return
		}
		u, err := s.Deps.Users.RegisterNew(name, realName, city, pass, localeFromRequest(r))
		if err != nil {
			data.Error = translateAPIError(data.Locale, err.Error())
			s.render(w, "register.html", data)
			return
		}
		token, err := s.Sessions.Create(u.ID)
		if err != nil {
			data.Error = tr(data.Locale, "register.error.login_failed")
			s.render(w, "register.html", data)
			return
		}
		setSessionCookie(w, token)
		s.bindWebNode(token, u)
		initLocales()
		if localeData[u.Locale] != nil {
			http.SetCookie(w, &http.Cookie{Name: "virtbbs_lang", Value: u.Locale, Path: "/", MaxAge: 365 * 24 * 3600})
		}
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
		pageData: s.page(r),
		WelcomeHTML: html,
	}
	s.render(w, "register_welcome.html", data)
}
