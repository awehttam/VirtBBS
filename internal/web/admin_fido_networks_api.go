package web

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/fido"
)

type idNameJSON struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Network string `json:"network,omitempty"`
}

type echoAreaJSON struct {
	Tag      string `json:"tag"`
	ConfID   int    `json:"conf_id"`
	ConfName string `json:"conf_name,omitempty"`
}

type fileAreaJSON struct {
	Tag     string `json:"tag"`
	DirID   int    `json:"dir_id"`
	DirName string `json:"dir_name,omitempty"`
}

type downlinkJSON struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	Password string `json:"password"`
}

type mappingsResponse struct {
	Network     string         `json:"network"`
	EchoAreas   []echoAreaJSON `json:"echo_areas"`
	FileAreas   []fileAreaJSON `json:"file_areas"`
	Downlinks   []downlinkJSON `json:"downlinks"`
	Conferences []idNameJSON   `json:"conferences"`
	FileDirs    []idNameJSON   `json:"file_dirs"`
}

func writeAdminJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAdminJSONError(w http.ResponseWriter, status int, msg string) {
	writeAdminJSON(w, status, map[string]string{"error": msg})
}

func adminFidoNetworksI18nJSON(locale string) string {
	keys := []string{
		"common.add", "common.edit", "common.delete", "common.save", "common.close",
		"common.name", "common.password",
		"admin_fido_networks.echo_areas", "admin_fido_networks.file_areas", "admin_fido_networks.downlinks",
		"admin_fido_networks.col.tag", "admin_fido_networks.col.conf_id", "admin_fido_networks.col.dir_id",
		"admin_fido_networks.col.address", "admin_fido_networks.modal.add_echo", "admin_fido_networks.modal.edit_echo",
		"admin_fido_networks.modal.add_file", "admin_fido_networks.modal.edit_file",
		"admin_fido_networks.modal.add_downlink", "admin_fido_networks.modal.edit_downlink",
		"admin_fido_networks.empty.echo", "admin_fido_networks.empty.file", "admin_fido_networks.empty.downlinks",
		"admin_fido_networks.confirm.delete_echo", "admin_fido_networks.confirm.delete_file",
		"admin_fido_downlinks.confirm_remove", "admin_fido_downlinks.password_keep",
		"admin_fido_networks.flash.echo_saved", "admin_fido_networks.flash.file_saved",
		"admin_fido_networks.flash.echo_removed", "admin_fido_networks.flash.file_removed",
		"admin_fido_downlinks.flash_added", "admin_fido_downlinks.flash_updated", "admin_fido_downlinks.flash_removed",
		"admin_fido_downlinks.error.required", "admin_fido_downlinks.error.bad_addr",
		"admin_fido_downlinks.error.duplicate", "admin_fido_downlinks.error.not_found",
		"admin_fido_networks.error.bad_tag", "admin_fido_networks.error.bad_id",
		"conferences.group.local",
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		short := key
		if i := stringsLastDot(key); i >= 0 {
			short = key[i+1:]
		}
		out[short] = tr(locale, key)
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func (s *Server) handleAPIAdminFidoNetworksMappings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireSysop(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	network := strings.TrimSpace(r.URL.Query().Get("network"))
	if network == "" {
		writeAdminJSONError(w, http.StatusBadRequest, "network required")
		return
	}
	nd, err := networkDefByName(network)
	if err != nil {
		writeAdminJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	confNames := map[int]string{}
	var allConfs []*conferences.Conference
	if confs, err := s.Deps.Conferences.List(); err == nil {
		allConfs = confs
		for _, c := range confs {
			if c != nil {
				confNames[c.ID] = c.Name
			}
		}
	}
	dirNames := map[int]string{}
	if dirs, err := s.Deps.Files.ListDirs(); err == nil {
		for _, d := range dirs {
			dirNames[int(d.ID)] = d.Name
		}
	}

	resp := mappingsResponse{
		Network:     network,
		EchoAreas:   []echoAreaJSON{},
		FileAreas:   []fileAreaJSON{},
		Downlinks:   []downlinkJSON{},
		Conferences: []idNameJSON{},
		FileDirs:    []idNameJSON{},
	}
	for _, g := range groupConferences(allConfs) {
		for _, c := range g.List {
			resp.Conferences = append(resp.Conferences, idNameJSON{
				ID: c.ID, Name: c.Name, Network: g.Network,
			})
		}
	}
	for id, name := range dirNames {
		resp.FileDirs = append(resp.FileDirs, idNameJSON{ID: id, Name: name})
	}
	sort.Slice(resp.FileDirs, func(i, j int) bool { return resp.FileDirs[i].ID < resp.FileDirs[j].ID })

	for tag, confID := range nd.Areas {
		resp.EchoAreas = append(resp.EchoAreas, echoAreaJSON{
			Tag: tag, ConfID: confID, ConfName: confNames[confID],
		})
	}
	sort.Slice(resp.EchoAreas, func(i, j int) bool { return resp.EchoAreas[i].Tag < resp.EchoAreas[j].Tag })

	for tag, dirID := range nd.FileAreas {
		resp.FileAreas = append(resp.FileAreas, fileAreaJSON{
			Tag: tag, DirID: dirID, DirName: dirNames[dirID],
		})
	}
	sort.Slice(resp.FileAreas, func(i, j int) bool { return resp.FileAreas[i].Tag < resp.FileAreas[j].Tag })

	for _, dl := range nd.Downlinks {
		resp.Downlinks = append(resp.Downlinks, downlinkJSON{
			Name: dl.Name, Address: dl.Address, Password: dl.Password,
		})
	}
	sort.Slice(resp.Downlinks, func(i, j int) bool {
		return strings.ToLower(resp.Downlinks[i].Name) < strings.ToLower(resp.Downlinks[j].Name)
	})

	writeAdminJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAPIAdminFidoNetworksAreas(w http.ResponseWriter, r *http.Request) {
	locale := localeFromRequest(r)
	if _, ok := s.requireSysop(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Network string `json:"network"`
			Tag     string `json:"tag"`
			OldTag  string `json:"old_tag"`
			ConfID  int    `json:"conf_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeAdminJSONError(w, http.StatusBadRequest, "bad json")
			return
		}
		tag := strings.TrimSpace(body.Tag)
		oldTag := strings.TrimSpace(body.OldTag)
		if tag == "" {
			writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_networks.error.bad_tag"))
			return
		}
		if body.ConfID <= 0 {
			writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_networks.error.bad_id"))
			return
		}
		if _, err := s.Deps.Conferences.Get(body.ConfID); err != nil {
			writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_networks.error.bad_id"))
			return
		}
		if oldTag != "" && !strings.EqualFold(oldTag, tag) {
			if err := config.SaveAreaMapping(body.Network, oldTag, 0, true); err != nil {
				writeAdminJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if err := config.SaveAreaMapping(body.Network, tag, body.ConfID, false); err != nil {
			writeAdminJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true, "message": tr(locale, "admin_fido_networks.flash.echo_saved")})
	case http.MethodDelete:
		network := strings.TrimSpace(r.URL.Query().Get("network"))
		tag := strings.TrimSpace(r.URL.Query().Get("tag"))
		if network == "" || tag == "" {
			writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_networks.error.bad_tag"))
			return
		}
		if err := config.SaveAreaMapping(network, tag, 0, true); err != nil {
			writeAdminJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true, "message": tr(locale, "admin_fido_networks.flash.echo_removed")})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPIAdminFidoNetworksFileAreas(w http.ResponseWriter, r *http.Request) {
	locale := localeFromRequest(r)
	if _, ok := s.requireSysop(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Network string `json:"network"`
			Tag     string `json:"tag"`
			OldTag  string `json:"old_tag"`
			DirID   int    `json:"dir_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeAdminJSONError(w, http.StatusBadRequest, "bad json")
			return
		}
		tag := strings.TrimSpace(body.Tag)
		oldTag := strings.TrimSpace(body.OldTag)
		if tag == "" {
			writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_networks.error.bad_tag"))
			return
		}
		if body.DirID <= 0 {
			writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_networks.error.bad_id"))
			return
		}
		dirs, err := s.Deps.Files.ListDirs()
		if err != nil {
			writeAdminJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		found := false
		for _, d := range dirs {
			if int(d.ID) == body.DirID {
				found = true
				break
			}
		}
		if !found {
			writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_networks.error.bad_id"))
			return
		}
		if oldTag != "" && !strings.EqualFold(oldTag, tag) {
			if err := config.SaveFileAreaMapping(body.Network, oldTag, 0, true); err != nil {
				writeAdminJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if err := config.SaveFileAreaMapping(body.Network, tag, int64(body.DirID), false); err != nil {
			writeAdminJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true, "message": tr(locale, "admin_fido_networks.flash.file_saved")})
	case http.MethodDelete:
		network := strings.TrimSpace(r.URL.Query().Get("network"))
		tag := strings.TrimSpace(r.URL.Query().Get("tag"))
		if network == "" || tag == "" {
			writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_networks.error.bad_tag"))
			return
		}
		if err := config.SaveFileAreaMapping(network, tag, 0, true); err != nil {
			writeAdminJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true, "message": tr(locale, "admin_fido_networks.flash.file_removed")})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPIAdminFidoNetworksDownlinks(w http.ResponseWriter, r *http.Request) {
	locale := localeFromRequest(r)
	db := s.Deps.Messages.DB()
	if _, ok := s.requireSysop(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Network  string `json:"network"`
			Action   string `json:"action"`
			Name     string `json:"name"`
			Address  string `json:"address"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeAdminJSONError(w, http.StatusBadRequest, "bad json")
			return
		}
		network := strings.TrimSpace(body.Network)
		name := strings.TrimSpace(body.Name)
		addr := strings.TrimSpace(body.Address)
		password := strings.TrimSpace(body.Password)
		action := strings.TrimSpace(body.Action)
		if action == "" {
			action = "add"
		}

		switch action {
		case "add":
			if name == "" || addr == "" {
				writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_downlinks.error.required"))
				return
			}
			if _, parseErr := fido.ParseAddr(addr); parseErr != nil {
				writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_downlinks.error.bad_addr"))
				return
			}
			nd, err := networkDefByName(network)
			if err != nil {
				writeAdminJSONError(w, http.StatusNotFound, err.Error())
				return
			}
			if nd.DownlinkByAddr(mustParseAddr(addr)) != nil {
				writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_downlinks.error.duplicate"))
				return
			}
			if password == "" {
				password = randomMemberPassword()
			}
			if err := saveNetworkDownlink(network, fido.Downlink{Name: name, Address: addr, Password: password}); err != nil {
				writeAdminJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true, "message": tr(locale, "admin_fido_downlinks.flash_added")})
		case "update":
			if addr == "" || name == "" {
				writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_downlinks.error.required"))
				return
			}
			updated := false
			err := updateNetworkDownlinks(network, func(cur []fido.Downlink) []fido.Downlink {
				out := make([]fido.Downlink, len(cur))
				copy(out, cur)
				for i := range out {
					if strings.EqualFold(out[i].Address, addr) {
						out[i].Name = name
						if password != "" {
							out[i].Password = password
						}
						updated = true
					}
				}
				return out
			})
			if err != nil {
				writeAdminJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !updated {
				writeAdminJSONError(w, http.StatusNotFound, tr(locale, "admin_fido_downlinks.error.not_found"))
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true, "message": tr(locale, "admin_fido_downlinks.flash_updated")})
		default:
			writeAdminJSONError(w, http.StatusBadRequest, "unknown action")
		}
	case http.MethodDelete:
		network := strings.TrimSpace(r.URL.Query().Get("network"))
		addr := strings.TrimSpace(r.URL.Query().Get("address"))
		if network == "" || addr == "" {
			writeAdminJSONError(w, http.StatusBadRequest, tr(locale, "admin_fido_downlinks.error.required"))
			return
		}
		removed, remErr := removeNetworkDownlink(db, network, addr)
		if remErr != nil {
			writeAdminJSONError(w, http.StatusInternalServerError, remErr.Error())
			return
		}
		if !removed {
			writeAdminJSONError(w, http.StatusNotFound, tr(locale, "admin_fido_downlinks.error.not_found"))
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true, "message": tr(locale, "admin_fido_downlinks.flash_removed")})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
