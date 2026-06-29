package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/fido"
)

func (s *Server) handleAdminFidoNodelist(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	network := selectedNetwork(r)
	locale := localeFromRequest(r)
	data := struct {
		pageData
		Networks    []string
		Network     string
		Query       string
		Results     *fido.SearchResult
		VersionText string
		Flash       string
		Error       string
		CanEdit     bool
	}{
		pageData: s.page(r),
		Networks: fidoNetworkNamesList(),
		Network:  network,
		CanEdit:  true,
	}

	db := s.Deps.Messages.DB()
	if v, err := fido.GetNodelistVersion(db, network); err == nil && v != nil {
		data.VersionText = fmt.Sprintf("%s — %d nodes", v.ImportedAt, v.NodeCount)
	}

	importedThisRequest := false
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		network = selectedNetwork(r)
		data.Network = network
		switch r.FormValue("action") {
		case "fetch":
			nd := config.Get().Fido.NetworkByName(network)
			if nd == nil {
				data.Error = tr(locale, "admin_binkp.error.network")
			} else if !nd.NodelistFetchEnabled() {
				data.Error = tr(locale, "admin_fido_ops.error.no_nodelist_url")
			} else if _, err := fido.FetchAndImport(nd, db, s.Deps.Files); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = tr(locale, "admin_fido_nodelist.flash_fetched")
				importedThisRequest = true
			}
		case "import":
			path := strings.TrimSpace(r.FormValue("import_path"))
			if path == "" {
				data.Error = tr(locale, "admin_fido_ops.error.import_path")
			} else if _, err := importNodelistRestoreLocal(db, path, network); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = tr(locale, "admin_fido_nodelist.flash_imported")
				importedThisRequest = true
			}
		case "export_local":
			nd, err := networkDefByName(network)
			if err != nil {
				data.Error = err.Error()
			} else {
				filePath, body, err := fido.ExportNodelistFile(db, nd)
				if err != nil {
					data.Error = err.Error()
				} else {
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
					w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filePath))
					_, _ = w.Write(body)
					return
				}
			}
		case "save_node":
			in := localNodeFromForm(r)
			commit := r.FormValue("commit") == "1"
			msg, err := s.saveLocalNodeInput(network, in, commit, locale)
			if err != nil {
				data.Error = err.Error()
			} else {
				q := url.QueryEscape(strings.TrimSpace(r.FormValue("q")))
				http.Redirect(w, r, fmt.Sprintf("/admin/fido/nodelist?network=%s&q=%s&flash=%s",
					url.QueryEscape(network), q, url.QueryEscape(msg)), http.StatusSeeOther)
				return
			}
		}
		if v, err := fido.GetNodelistVersion(db, network); err == nil && v != nil {
			data.VersionText = fmt.Sprintf("%s — %d nodes", v.ImportedAt, v.NodeCount)
		}
	}

	data.Query = strings.TrimSpace(r.URL.Query().Get("q"))
	if flash := r.URL.Query().Get("flash"); flash != "" {
		data.Flash = flash
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if !importedThisRequest {
		_ = s.maybeRebuildHubNodelist(network)
	}
	ndb := fido.OpenNodelistDB(db)
	results, err := ndb.Search(network, data.Query, page, 25)
	if err != nil {
		data.Error = err.Error()
	} else if results != nil {
		linkNodelistAKAs(results.Nodes, network)
		data.Results = results
	}
	s.render(w, "admin_fido_nodelist.html", data)
}

func (s *Server) handleAdminFidoNodelistAdd(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	network := selectedNetwork(r)
	locale := localeFromRequest(r)
	data := struct {
		pageData
		Networks []string
		Network  string
		Node     fido.LocalNodeInput
		Flash    string
		Error    string
	}{
		pageData: s.page(r),
		Networks: fidoNetworkNamesList(),
		Network:  network,
		Node: fido.LocalNodeInput{
			Type:   "Node",
			Baud:   33600,
			Active: true,
		},
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		network = selectedNetwork(r)
		data.Network = network
		data.Node = localNodeFromForm(r)
		commit := r.FormValue("commit") == "1"
		msg, err := s.saveLocalNodeInput(network, data.Node, commit, locale)
		if err != nil {
			data.Error = err.Error()
		} else {
			data.Flash = msg
			http.Redirect(w, r, "/admin/fido/nodelist?network="+network, http.StatusSeeOther)
			return
		}
	}
	s.render(w, "admin_fido_nodelist_add.html", data)
}

func (s *Server) saveLocalNodeInput(network string, in fido.LocalNodeInput, commit bool, locale string) (string, error) {
	nd, err := networkDefByName(network)
	if err != nil {
		return "", err
	}
	cfg := config.Get()
	params := fido.LocalNodelistCommitParams{
		Network: network,
		Upsert:  []fido.LocalNodeInput{in},
	}
	if commit {
		res, err := fido.CommitLocalNodelist(s.Deps.Messages.DB(), nd, cfg.BBS.Name, cfg.Sysop.Name, cfg.Network.TelnetPort, params)
		if err != nil {
			return "", err
		}
		if res.Message != "" {
			return res.Message, nil
		}
		return tr(locale, "admin_fido_nodelist.flash_committed"), nil
	}
	if err := fido.ApplyLocalNodes(s.Deps.Messages.DB(), network, params.Upsert, nil); err != nil {
		return "", err
	}
	return tr(locale, "admin_fido_nodelist.saved_db"), nil
}
