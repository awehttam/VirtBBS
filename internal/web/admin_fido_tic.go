package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/fido"
)

func (s *Server) handleAdminFidoTIC(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	cfg := config.Get()
	network := selectedNetwork(r)
	nd := cfg.Fido.NetworkByName(network)
	if nd == nil {
		http.Error(w, "network not found", http.StatusNotFound)
		return
	}

	exportCount := 0
	if rows, err := s.Deps.Messages.DB().Query(`SELECT COUNT(*) FROM fido_file_exports WHERE network=?`, network); err == nil {
		if rows.Next() {
			_ = rows.Scan(&exportCount)
		}
		rows.Close()
	}

	var areas []struct{ Tag string; DirID int }
	for tag, id := range nd.FileAreas {
		areas = append(areas, struct{ Tag string; DirID int }{Tag: tag, DirID: id})
	}

	data := struct {
		pageData
		Networks    []string
		Network     string
		FileAreas   []struct{ Tag string; DirID int }
		ExportCount int
		Flash       string
		Error       string
	}{
		pageData:    s.page(r),
		Networks:    fidoNetworkNamesList(),
		Network:     network,
		FileAreas:   areas,
		ExportCount: exportCount,
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		network = selectedNetwork(r)
		data.Network = network
		nd = cfg.Fido.NetworkByName(network)
		switch r.FormValue("action") {
		case "file_scan":
			if nd == nil {
				data.Error = "network not found"
			} else if res, err := fido.FileScanAll(&cfg.Fido, s.Deps.Messages.DB(), cfg.Paths.Files); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = fmt.Sprintf("File scan: %d file(s), %d TIC ticket(s)", res.Files, res.TICFiles)
				if len(res.Errors) > 0 {
					data.Error = strings.Join(res.Errors, "; ")
				}
			}
		case "process_inbound":
			if nd == nil {
				data.Error = "network not found"
			} else if s.Deps.Files == nil {
				data.Error = "files store unavailable"
			} else {
				res := fido.ProcessInboundTICs(nd, s.Deps.Messages.DB(), s.Deps.Files)
				data.Flash = fmt.Sprintf("Processed %d inbound TIC file(s)", res.Processed)
				if len(res.Errors) > 0 {
					data.Error = strings.Join(res.Errors, "; ")
				}
			}
		}
	}

	s.render(w, "admin_fido_tic.html", data)
}
