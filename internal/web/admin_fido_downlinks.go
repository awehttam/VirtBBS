package web

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/virtbbs/virtbbs/internal/fido"
)

type downlinkRowView struct {
	Name         string
	Address      string
	Password     string
	NodelistType string
	Areas        []string
	FileAreas    []string
}

func (s *Server) handleAdminFidoDownlinks(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	locale := localeFromRequest(r)
	network := selectedNetwork(r)
	nd, err := networkDefByName(network)
	if err != nil {
		http.Error(w, "network not found", http.StatusNotFound)
		return
	}

	data := struct {
		pageData
		Networks  []string
		Network   string
		Downlinks []downlinkRowView
		Flash     string
		Error     string
	}{
		pageData: s.page(r),
		Networks: fidoNetworkNamesList(),
		Network:  network,
	}

	db := s.Deps.Messages.DB()
	data.Downlinks = loadDownlinkRows(db, network, nd.Downlinks)

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		network = selectedNetwork(r)
		data.Network = network
		switch r.FormValue("action") {
		case "add":
			name := strings.TrimSpace(r.FormValue("name"))
			addr := strings.TrimSpace(r.FormValue("address"))
			password := strings.TrimSpace(r.FormValue("password"))
			if name == "" || addr == "" {
				data.Error = tr(locale, "admin_fido_downlinks.error.required")
			} else if _, parseErr := fido.ParseAddr(addr); parseErr != nil {
				data.Error = tr(locale, "admin_fido_downlinks.error.bad_addr")
			} else {
				nd2, err := networkDefByName(network)
				if err != nil {
					data.Error = err.Error()
				} else if nd2.DownlinkByAddr(mustParseAddr(addr)) != nil {
					data.Error = tr(locale, "admin_fido_downlinks.error.duplicate")
				} else {
					if password == "" {
						password = randomMemberPassword()
					}
					if err := saveNetworkDownlink(network, fido.Downlink{
						Name: name, Address: addr, Password: password,
					}); err != nil {
						data.Error = err.Error()
					} else {
						data.Flash = tr(locale, "admin_fido_downlinks.flash_added")
					}
				}
			}
		case "remove":
			addr := strings.TrimSpace(r.FormValue("address"))
			if addr == "" {
				data.Error = tr(locale, "admin_fido_downlinks.error.required")
			} else if removed, remErr := removeNetworkDownlink(db, network, addr); remErr != nil {
				data.Error = remErr.Error()
			} else if !removed {
				data.Error = tr(locale, "admin_fido_downlinks.error.not_found")
			} else {
				data.Flash = tr(locale, "admin_fido_downlinks.flash_removed")
			}
		case "update":
			addr := strings.TrimSpace(r.FormValue("address"))
			name := strings.TrimSpace(r.FormValue("name"))
			password := strings.TrimSpace(r.FormValue("password"))
			if addr == "" || name == "" {
				data.Error = tr(locale, "admin_fido_downlinks.error.required")
			} else {
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
					data.Error = err.Error()
				} else if !updated {
					data.Error = tr(locale, "admin_fido_downlinks.error.not_found")
				} else {
					data.Flash = tr(locale, "admin_fido_downlinks.flash_updated")
				}
			}
		}
		if data.Flash != "" || data.Error != "" {
			if nd, err := networkDefByName(network); err == nil {
				data.Downlinks = loadDownlinkRows(db, network, nd.Downlinks)
			}
		}
	}
	s.render(w, "admin_fido_downlinks.html", data)
}

func loadDownlinkRows(db *sql.DB, network string, dls []fido.Downlink) []downlinkRowView {
	ndb := fido.OpenNodelistDB(db)
	areafixDB := fido.OpenAreaFixDB(db)
	filefixDB := fido.OpenFileFixDB(db)
	var rows []downlinkRowView
	for _, dl := range dls {
		row := downlinkRowView{Name: dl.Name, Address: dl.Address, Password: dl.Password}
		row.Areas, _ = areafixDB.SubscriptionsFor(network, dl.Address)
		row.FileAreas, _ = filefixDB.SubscriptionsFor(network, dl.Address)
		if addr, err := fido.ParseAddr(dl.Address); err == nil {
			if entry, err := ndb.LookupAddr(network, addr); err == nil && entry != nil {
				row.NodelistType = entry.Type
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func mustParseAddr(s string) fido.Addr {
	a, _ := fido.ParseAddr(s)
	return a
}
