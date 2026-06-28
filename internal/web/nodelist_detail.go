package web

import (
	"encoding/json"
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

func (s *Server) maybeRebuildHubNodelist(network string) error {
	nd, err := networkDefByName(network)
	if err != nil || !nd.UsesMemberNodelist() {
		return err
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
	if aka == "" {
		linked := []*fido.NodeEntry{entry}
		fido.LinkHostAKAsPtrs(linked)
		if linked[0].AKA != "" {
			aka = linked[0].AKA
		}
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
