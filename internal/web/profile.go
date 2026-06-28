package web

import (
	"net/http"
	"strings"

	"github.com/virtbbs/virtbbs/internal/users"
)

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	data := struct {
		pageData
		Profile *users.User
	}{
		pageData: s.page(r),
		Profile:  u,
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data.Error = tr(data.Locale, "login.error.form")
			s.render(w, "profile.html", data)
			return
		}
		fresh, err := s.Deps.Users.GetByID(u.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fresh.City = strings.TrimSpace(r.FormValue("city"))
		fresh.RealName = strings.TrimSpace(r.FormValue("real_name"))
		if fresh.RealName == "" {
			data.Error = tr(data.Locale, "register.error.real_name_required")
			data.Profile = fresh
			s.render(w, "profile.html", data)
			return
		}

		cur := r.FormValue("current_password")
		newPass := r.FormValue("new_password")
		confirm := r.FormValue("password_confirm")
		changingPassword := cur != "" || newPass != "" || confirm != ""
		if changingPassword {
			switch {
			case cur == "":
				data.Error = tr(data.Locale, "profile.error.current_required")
			case newPass == "":
				data.Error = tr(data.Locale, "profile.error.new_required")
			case newPass != confirm:
				data.Error = tr(data.Locale, "reset.error.password_mismatch")
			default:
				if _, err := s.Deps.Users.Authenticate(fresh.Name, cur); err != nil {
					data.Error = tr(data.Locale, "profile.error.current_password")
				} else if err := s.Deps.Users.SetPassword(fresh.ID, newPass); err != nil {
					data.Error = translateAPIError(data.Locale, err.Error())
				}
			}
		}
		if data.Error == "" {
			if err := s.Deps.Users.Update(fresh); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = tr(data.Locale, "profile.flash.updated")
				if updated, err := s.Deps.Users.GetByID(u.ID); err == nil {
					fresh = updated
				}
			}
		}
		data.Profile = fresh
		data.pageData = s.page(r)
		if data.pageData.User != nil {
			data.pageData.User.City = fresh.City
		}
	}
	s.render(w, "profile.html", data)
}
