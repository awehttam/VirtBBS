package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/fido"
)

type nodeDetailJSON struct {
	ID          int64                      `json:"id"`
	Network     string                     `json:"network"`
	Address     string                     `json:"address"`
	AKA         string                     `json:"aka,omitempty"`
	Name        string                     `json:"name"`
	Location    string                     `json:"location"`
	Sysop       string                     `json:"sysop"`
	Phone       string                     `json:"phone"`
	Baud        int                        `json:"baud"`
	Flags       string                     `json:"flags"`
	Type        string                     `json:"type"`
	Active      bool                       `json:"active"`
	FlagDetails []fido.NodelistFlagDisplay `json:"flag_details"`
}

type nodelistPageConfig struct {
	APIURL   string            `json:"apiUrl"`
	SaveURL  string            `json:"saveUrl,omitempty"`
	Editable bool              `json:"editable"`
	Query    string            `json:"query,omitempty"`
	I18n     map[string]string `json:"i18n"`
}

// nodelistPageJSON returns page config for nodelist.js (safe for embedding in HTML).
func nodelistPageJSON(apiURL, saveURL, query, locale string, editable bool) template.JS {
	keys := []struct{ jsKey, locKey string }{
		{"address", "nodelist.col.address"},
		{"aka", "nodelist.col.aka"},
		{"network", "common.network"},
		{"name", "common.name"},
		{"location", "nodelist.col.location"},
		{"sysop", "nodelist.col.sysop"},
		{"phone", "nodelist.col.phone"},
		{"baud", "nodelist.col.baud"},
		{"type", "nodelist.col.type"},
		{"active", "common.active"},
		{"flags", "common.flags"},
		{"capabilities", "nodelist.col.capabilities"},
		{"yes", "common.yes"},
		{"no", "common.no"},
		{"close", "common.close"},
		{"edit", "common.edit"},
		{"save", "common.save"},
		{"loading", "common.loading"},
		{"load_error", "nodelist.load_error"},
	}
	i18n := make(map[string]string, len(keys)+3)
	for _, pair := range keys {
		i18n[pair.jsKey] = tr(locale, pair.locKey)
	}
	if editable {
		i18n["flags_help"] = tr(locale, "nodelist.flags_help")
		i18n["commit"] = tr(locale, "admin_fido_nodelist.commit_checkbox")
		i18n["save_error"] = tr(locale, "admin_fido_nodelist.save_error")
	}
	cfg := nodelistPageConfig{
		APIURL:   apiURL,
		SaveURL:  saveURL,
		Editable: editable,
		Query:    query,
		I18n:     i18n,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(b)
}

func (s *Server) maybeRebuildHubNodelist(network string) error {
	nd, err := networkDefByName(network)
	if err != nil || !nd.UsesMemberNodelist() {
		return err
	}
	if s.Deps.Messages != nil && fido.ShouldPreserveImportedNodelist(s.Deps.Messages.DB(), nd) {
		return nil
	}
	cfg := config.Get()
	return fido.RebuildHubNodelistDB(s.Deps.Messages.DB(), nd, cfg.BBS.Name, cfg.Sysop.Name)
}

func (s *Server) lookupNodeDetail(network, addrStr string) (*nodeDetailJSON, error) {
	addrStr = strings.TrimSpace(addrStr)
	if addrStr == "" {
		return nil, nil
	}
	a, err := fido.ParseAddr(addrStr)
	if err != nil {
		return nil, err
	}
	ndb := fido.OpenNodelistDB(s.Deps.Messages.DB())
	entry, err := ndb.LookupAddr(network, a)
	if err != nil || entry == nil {
		return nil, err
	}
	aka := entry.AKA
	linked := []*fido.NodeEntry{entry}
	fido.LinkHostAKAsPtrs(linked)
	if nd, nerr := networkDefByName(network); nerr == nil {
		fido.LinkConfiguredAKAs(linked, nd)
	}
	if linked[0].AKA != "" {
		aka = linked[0].AKA
	}
	return nodeEntryToDetail(entry, aka), nil
}

func nodeEntryToDetail(e *fido.NodeEntry, aka string) *nodeDetailJSON {
	return &nodeDetailJSON{
		ID:          e.ID,
		Network:     e.Network,
		Address:     e.Addr4D(),
		AKA:         aka,
		Name:        e.Name,
		Location:    e.Location,
		Sysop:       e.Sysop,
		Phone:       e.Phone,
		Baud:        e.Baud,
		Flags:       e.Flags,
		Type:        e.Type,
		Active:      e.Active,
		FlagDetails: fido.DescribeNodelistFlags(e.Flags),
	}
}

func localNodeFromForm(r *http.Request) fido.LocalNodeInput {
	baud, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("baud")))
	return fido.LocalNodeInput{
		Address:  strings.TrimSpace(r.FormValue("address")),
		Name:     r.FormValue("name"),
		Location: r.FormValue("location"),
		Sysop:    r.FormValue("sysop"),
		Phone:    r.FormValue("phone"),
		Baud:     baud,
		Flags:    r.FormValue("flags"),
		Type:     r.FormValue("type"),
		Active:   r.FormValue("active") == "1",
	}
}

func (s *Server) handleNodelistNode(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireUser(w, r); !ok {
		return
	}
	s.writeNodeDetailJSON(w, r)
}

func (s *Server) handleAdminFidoNodelistNode(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireSysop(w, r); !ok {
		return
	}
	s.writeNodeDetailJSON(w, r)
}

func (s *Server) writeNodeDetailJSON(w http.ResponseWriter, r *http.Request) {
	network := strings.TrimSpace(r.URL.Query().Get("network"))
	addr := strings.TrimSpace(r.URL.Query().Get("addr"))
	if network == "" || addr == "" {
		http.Error(w, "network and addr required", http.StatusBadRequest)
		return
	}
	_ = s.maybeRebuildHubNodelist(network)
	detail, err := s.lookupNodeDetail(network, addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if detail == nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}
