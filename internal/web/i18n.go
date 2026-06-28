package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/fido"
	"github.com/virtbbs/virtbbs/internal/users"
)

//go:embed locales/en.json
var localeEN []byte

//go:embed locales/es.json
var localeES []byte

//go:embed locales/af.json
var localeAF []byte

var localeData = map[string]map[string]string{}

func initLocales() {
	if len(localeData) > 0 {
		return
	}
	for tag, raw := range map[string][]byte{"en": localeEN, "es": localeES, "af": localeAF} {
		m := map[string]string{}
		if err := json.Unmarshal(raw, &m); err == nil {
			localeData[tag] = m
		}
	}
}

func localeFromRequest(r *http.Request) string {
	if c, err := r.Cookie("virtbbs_lang"); err == nil && localeData[c.Value] != nil {
		return c.Value
	}
	al := strings.ToLower(r.Header.Get("Accept-Language"))
	for _, pref := range []string{"af", "es"} {
		if strings.HasPrefix(al, pref) && localeData[pref] != nil {
			return pref
		}
	}
	return "en"
}

func effectiveUserLang(u *users.User, r *http.Request) string {
	if u != nil {
		if loc := strings.TrimSpace(u.Locale); loc != "" {
			initLocales()
			if localeData[loc] != nil {
				return loc
			}
		}
	}
	if r != nil {
		return localeFromRequest(r)
	}
	return "en"
}

func authorLangCode(u *users.User, r *http.Request) string {
	return fido.NormalizeLangCode(effectiveUserLang(u, r))
}

func tr(locale, key string) string {
	initLocales()
	if m := localeData[locale]; m != nil {
		if s, ok := m[key]; ok {
			return s
		}
	}
	if m := localeData["en"]; m != nil {
		if s, ok := m[key]; ok {
			return s
		}
	}
	return key
}

func trf(locale, key string, args ...any) string {
	s := tr(locale, key)
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

var legacyWebOpKeys = map[string]string{
	"Web: Dashboard":    "web.op.dashboard",
	"Web: Messages":     "web.op.messages",
	"Web: Netmail":      "web.op.netmail",
	"Web: Files":        "web.op.files",
	"Web: Admin":        "web.op.admin",
	"Web: Who's online": "web.op.online",
	"Web: Active":       "web.op.active",
}

func translateWebOp(locale, op string) string {
	if strings.HasPrefix(op, "web.op.path:") {
		return trf(locale, "web.op.path", strings.TrimPrefix(op, "web.op.path:"))
	}
	if strings.HasPrefix(op, "web.op.") {
		return tr(locale, op)
	}
	if key, ok := legacyWebOpKeys[op]; ok {
		return tr(locale, key)
	}
	if strings.HasPrefix(op, "Web: ") {
		return trf(locale, "web.op.path", strings.TrimPrefix(op, "Web: "))
	}
	return op
}

func translateAPIError(locale, msg string) string {
	keys := map[string]string{
		"name is required":                   "register.error.name_required",
		"name must be 25 characters or less": "register.error.name_length",
		"real name is required":            "register.error.real_name_required",
		"real name must be 36 characters or less": "register.error.real_name_length",
		"password is required":               "register.error.password_required",
		"user not found":                     "reset.error.user_not_found",
		"invalid or expired reset link":      "reset.error.invalid_token",
		"real name is required for this echo area": "post.error.real_name_required",
	}
	if key, ok := keys[msg]; ok {
		return tr(locale, key)
	}
	return msg
}

func (s *Server) page(r *http.Request) pageData {
	u, _ := s.currentUser(r)
	return pageData{
		BBSName: config.Get().BBS.Name,
		User:    u,
		Locale:  localeFromRequest(r),
	}
}

func (s *Server) handleSetLocale(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	lang := strings.TrimSpace(r.FormValue("lang"))
	initLocales()
	if localeData[lang] == nil {
		lang = "en"
	}
	http.SetCookie(w, &http.Cookie{Name: "virtbbs_lang", Value: lang, Path: "/", MaxAge: 365 * 24 * 3600})
	if u, ok := s.currentUser(r); ok {
		_ = s.Deps.Users.SetLocale(u.ID, lang)
	}
	next := r.FormValue("next")
	if next == "" || !strings.HasPrefix(next, "/") {
		next = "/menu"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}
