package web

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/virtbbs/virtbbs/internal/config"
)

//go:embed locales/en.json
var localeEN []byte

//go:embed locales/es.json
var localeES []byte

var localeData = map[string]map[string]string{}

func initLocales() {
	if len(localeData) > 0 {
		return
	}
	for tag, raw := range map[string][]byte{"en": localeEN, "es": localeES} {
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
	al := r.Header.Get("Accept-Language")
	if strings.HasPrefix(strings.ToLower(al), "es") && localeData["es"] != nil {
		return "es"
	}
	return "en"
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
	next := r.FormValue("next")
	if next == "" || !strings.HasPrefix(next, "/") {
		next = "/menu"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}
