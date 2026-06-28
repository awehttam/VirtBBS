package web

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/fido"
)

func (s *Server) handleAdminFido(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	_ = u
	cfg := config.Get()
	data := struct {
		pageData
		FidoEnabled bool
	}{
		pageData:    s.page(r),
		FidoEnabled: cfg.Fido.Enabled,
	}
	s.render(w, "admin_fido.html", data)
}

func (s *Server) handleAdminFidoOps(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	cfg := config.Get()
	network := selectedNetwork(r)
	data := struct {
		pageData
		Networks       []string
		Network        string
		IsHub          bool
		LocalNodes     []fido.NodeEntry
		VersionText    string
		ImportPath     string
		Flash          string
		Error          string
		BinkpLogLines  []string
		BinkpLogPath   string
		BinkpStatsText string
	}{
		pageData: s.page(r),
		Networks: fidoNetworkNamesList(),
		Network:  network,
	}
	if nd := cfg.Fido.NetworkByName(network); nd != nil {
		data.IsHub = nd.IsHub()
	}
	db := s.Deps.Messages.DB()
	if nodes, err := fido.ListLocalNodes(db, network); err == nil {
		data.LocalNodes = nodes
	}
	if v, err := fido.GetNodelistVersion(db, network); err == nil && v != nil {
		data.VersionText = fmt.Sprintf("%s — %d nodes", v.ImportedAt, v.NodeCount)
	}
	if lines, path, err := fido.ReadBinkpLogTail(40); err == nil {
		data.BinkpLogLines, data.BinkpLogPath = lines, path
	}
	if st, err := fido.QueryBinkpStatsForPeriod(db, network, "day", time.Now()); err == nil && st != nil {
		data.BinkpStatsText = fmt.Sprintf("%d network(s), %d link(s) today", len(st.Networks), len(st.Links))
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		network = selectedNetwork(r)
		data.Network = network
		if nd := cfg.Fido.NetworkByName(network); nd != nil {
			data.IsHub = nd.IsHub()
		}
		action := r.FormValue("action")
		switch action {
		case "toss":
			if !cfg.Fido.Enabled {
				data.Error = "FidoNet not enabled"
			} else {
				res := fido.TossAll(&cfg.Fido, s.Deps.Messages, s.Deps.Conferences, cfg.Sysop.Name, s.Deps.Files, cfg.Paths.Files)
				data.Flash = fmt.Sprintf("Toss complete — imported %d message(s), %d TIC file(s)", res.Imported, res.TICProcessed)
			}
		case "scan":
			if !cfg.Fido.Enabled {
				data.Error = "FidoNet not enabled"
			} else if res, err := fido.ScanAll(&cfg.Fido, s.Deps.Messages, s.Deps.Conferences, cfg.BBS.Name); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = fmt.Sprintf("Scan complete — exported %d message(s), %d PKT file(s)", res.Scanned, res.PKTFiles)
			}
		case "file_scan":
			if !cfg.Fido.Enabled {
				data.Error = "FidoNet not enabled"
			} else if res, err := fido.FileScanAll(&cfg.Fido, db, config.Get().Paths.Files); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = fmt.Sprintf("File scan complete — %d file(s), %d TIC ticket(s)", res.Files, res.TICFiles)
				if len(res.Errors) > 0 {
					data.Error = strings.Join(res.Errors, "; ")
				}
			}
		case "rebuild_maps":
			nd := cfg.Fido.NetworkByName(network)
			if nd == nil {
				data.Error = "network not found"
			} else if count, warns := fido.RebuildNetworkDiagrams(nd, db, s.Deps.Files, cfg.BBS.Name, cfg.Sysop.Name); count == 0 && len(warns) > 0 {
				data.Error = strings.Join(warns, "; ")
			} else {
				data.Flash = fmt.Sprintf("Network maps rebuilt — %d diagram(s) in VirtDiag.zip", count)
				if len(warns) > 0 {
					data.Error = strings.Join(warns, "; ")
				}
			}
		case "poll":
			nd := cfg.Fido.NetworkByName(network)
			if nd == nil {
				data.Error = "network not found"
			} else {
				res := fido.PollAndToss(nd, s.Deps.Messages, s.Deps.Conferences, cfg.Sysop.Name, s.Deps.Files, cfg.Paths.Files)
				if res.Poll.Error != nil {
					data.Error = res.Poll.Error.Error()
				} else {
					tossed := 0
					if res.Toss != nil {
						tossed = res.Toss.Imported
					}
					data.Flash = fmt.Sprintf("Poll OK — sent: %d, received: %d, tossed: %d",
						len(res.Poll.Sent), len(res.Poll.Received), tossed)
				}
			}
		case "fetch":
			nd := cfg.Fido.NetworkByName(network)
			if nd == nil {
				data.Error = "network not found"
			} else if !nd.NodelistFetchEnabled() {
				data.Error = "no nodelist_url configured"
			} else if _, err := fido.FetchAndImport(nd, db); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = "Nodelist fetched and imported."
			}
		case "import":
			path := strings.TrimSpace(r.FormValue("import_path"))
			if path == "" {
				data.Error = "import path required"
			} else if _, err := fido.ImportFile(db, path, network); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = "Nodelist imported from " + path
			}
		case "netmail":
			nd := cfg.Fido.NetworkByName(network)
			if nd == nil {
				data.Error = "network not found"
			} else {
				m := fido.NetmailMsg{
					Network:  network,
					FromName: "Sysop",
					FromAddr: nd.Address,
					ToAddr:   strings.TrimSpace(r.FormValue("to_addr")),
					ToName:   strings.TrimSpace(r.FormValue("to_name")),
					Subject:  strings.TrimSpace(r.FormValue("subject")),
					Body:     r.FormValue("body"),
					Crash:    formBool(r, "crash"),
				}
				ndb := fido.OpenNetmailDB(db)
				if id, err := ndb.Enqueue(&m); err != nil {
					data.Error = err.Error()
				} else {
					data.Flash = fmt.Sprintf("Netmail queued (id %d)", id)
				}
			}
		case "commit_local":
			nd, err := networkDefByName(network)
			if err != nil {
				data.Error = err.Error()
				break
			}
			var upsert []fido.LocalNodeInput
			for i := 0; ; i++ {
				addr := strings.TrimSpace(r.FormValue(fmt.Sprintf("addr_%d", i)))
				if addr == "" && i > 0 {
					break
				}
				if addr == "" {
					continue
				}
				upsert = append(upsert, fido.LocalNodeInput{
					Address:  addr,
					Name:     r.FormValue(fmt.Sprintf("name_%d", i)),
					Location: r.FormValue(fmt.Sprintf("location_%d", i)),
					Sysop:    r.FormValue(fmt.Sprintf("sysop_%d", i)),
					Phone:    r.FormValue(fmt.Sprintf("phone_%d", i)),
					Flags:    r.FormValue(fmt.Sprintf("flags_%d", i)),
					Type:     r.FormValue(fmt.Sprintf("type_%d", i)),
					Active:   r.FormValue(fmt.Sprintf("active_%d", i)) == "1",
				})
			}
			params := fido.LocalNodelistCommitParams{Network: network, Upsert: upsert}
			if res, err := fido.CommitLocalNodelist(db, nd, cfg.BBS.Name, cfg.Sysop.Name, cfg.Network.TelnetPort, params); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = res.Message
			}
		case "delete_checked":
			nd, err := networkDefByName(network)
			if err != nil {
				data.Error = err.Error()
				break
			}
			deleteAddrs := r.Form["delete_addr"]
			if len(deleteAddrs) == 0 {
				data.Error = tr(localeFromRequest(r), "admin_fido_ops.error.no_delete_selected")
				break
			}
			params := fido.LocalNodelistCommitParams{Network: network, Delete: deleteAddrs}
			if res, err := fido.CommitLocalNodelist(db, nd, cfg.BBS.Name, cfg.Sysop.Name, cfg.Network.TelnetPort, params); err != nil {
				data.Error = err.Error()
			} else if res.Message != "" {
				data.Flash = res.Message
			} else {
				data.Flash = tr(localeFromRequest(r), "admin_fido_ops.flash_deleted")
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
		}
		if nodes, err := fido.ListLocalNodes(db, network); err == nil {
			data.LocalNodes = nodes
		}
		if lines, path, err := fido.ReadBinkpLogTail(40); err == nil {
			data.BinkpLogLines, data.BinkpLogPath = lines, path
		}
	}
	s.render(w, "admin_fido_ops.html", data)
}

func (s *Server) handleAdminFidoNetworks(w http.ResponseWriter, r *http.Request) {
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

	data := struct {
		pageData
		Networks      []string
		Network       string
		IsPrimary     bool
		Def           fido.NetworkDef
		AreasText     string
		FileAreasText string
		DownlinksText string
		AKAsText      string
		NodeFlags     []fido.NodeFlagDef
		SelectedFlags []string
		FlagSet       map[string]bool
		AreaFixSubs   []areaFixSubView
	}{
		pageData:      s.page(r),
		Networks:      fidoNetworkNamesList(),
		Network:       network,
		IsPrimary:     strings.EqualFold(network, cfg.Fido.EffectivePrimaryName()),
		Def:           *nd,
		AreasText:     formatAreaMap(nd.Areas),
		FileAreasText: formatAreaMap(nd.FileAreas),
		DownlinksText: formatDownlinks(nd.Downlinks),
		AKAsText:      strings.Join(nd.AKAs, "\n"),
		SelectedFlags: nd.NodeFlags,
	}
	data.FlagSet = map[string]bool{}
	for _, f := range nd.NodeFlags {
		data.FlagSet[strings.ToUpper(f)] = true
	}
	data.NodeFlags = fido.KnownNodeFlags()
	areafixDB := fido.OpenAreaFixDB(s.Deps.Messages.DB())
	for _, dl := range nd.Downlinks {
		areas, _ := areafixDB.SubscriptionsFor(network, dl.Address)
		data.AreaFixSubs = append(data.AreaFixSubs, areaFixSubView{
			Downlink: dl.Address, Name: dl.Name, Areas: areas,
		})
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		network = selectedNetwork(r)
		action := r.FormValue("action")
		switch action {
		case "save_flags":
			flags := r.Form["node_flags"]
			validated, err := fido.ValidateNodeFlags(flags)
			if err != nil {
				data.Error = err.Error()
			} else {
				binkpHost := strings.TrimSpace(r.FormValue("binkp_host"))
				if err := saveNetworkNodeFlags(network, validated, binkpHost); err != nil {
					data.Error = err.Error()
				} else {
					nd2, _ := networkDefByName(network)
					if nd2 != nil {
						cfg2 := config.Get()
						_, _ = fido.UpdateNodeFlags(s.Deps.Messages.DB(), nd2,
							cfg2.BBS.Name, cfg2.Sysop.Name, "Internet",
							cfg2.Network.TelnetPort, validated, binkpHost)
					}
					data.Flash = "Node flags saved."
				}
			}
		case "add_network":
			merged := *config.Get()
			name := fmt.Sprintf("Network%d", len(merged.Fido.Networks)+1)
			merged.Fido.Networks = append(merged.Fido.Networks, fido.NetworkDef{
				Name: name, Enabled: true,
				InboundDir:  fmt.Sprintf("fido/%s_inbound", name),
				OutboundDir: fmt.Sprintf("fido/%s_outbound", name),
				NodelistDir: fmt.Sprintf("fido/%s_nodelist", name),
				NodeFlags:   fido.DefaultNodeFlags,
			})
			if err := saveFidoConfig(s.Deps.Messages.DB(), merged.Fido); err != nil {
				data.Error = err.Error()
			} else {
				http.Redirect(w, r, "/admin/fido/networks?network="+name, http.StatusSeeOther)
				return
			}
		case "save":
			merged := *config.Get()
			areas := parseAreaMapLines(r.FormValue("areas"))
			fileAreas := parseAreaMapLines(r.FormValue("file_areas"))
			downlinks := parseDownlinkLines(r.FormValue("downlinks"))
			akas := strings.Split(strings.TrimSpace(r.FormValue("akas")), "\n")
			newName := strings.TrimSpace(r.FormValue("network_name"))
			if strings.EqualFold(network, merged.Fido.EffectivePrimaryName()) {
				merged.Fido.Enabled = formBool(r, "enabled")
				merged.Fido.Address = strings.TrimSpace(r.FormValue("address"))
				merged.Fido.Uplink = strings.TrimSpace(r.FormValue("uplink"))
				merged.Fido.Password = r.FormValue("password")
				merged.Fido.InboundDir = strings.TrimSpace(r.FormValue("inbound_dir"))
				merged.Fido.OutboundDir = strings.TrimSpace(r.FormValue("outbound_dir"))
				merged.Fido.NodelistDir = strings.TrimSpace(r.FormValue("nodelist_dir"))
				merged.Fido.HoldingDir = strings.TrimSpace(r.FormValue("holding_dir"))
				merged.Fido.BinkpPort = formInt(r, "binkp_port", 24554)
				merged.Fido.AreaFixPassword = r.FormValue("areafix_password")
				merged.Fido.FileFixPassword = r.FormValue("filefix_password")
				merged.Fido.TicPassword = r.FormValue("tic_password")
				merged.Fido.PollIntervalMins = formInt(r, "poll_interval_mins", 0)
				merged.Fido.NodelistURL = strings.TrimSpace(r.FormValue("nodelist_url"))
				merged.Fido.NodelistUpdateIntervalHours = formInt(r, "nodelist_update_hours", 0)
				merged.Fido.AKAs = akas
				merged.Fido.Areas = areas
				merged.Fido.FileAreas = fileAreas
				merged.Fido.Downlinks = downlinks
				if newName != "" {
					merged.Fido.Name = newName
				}
			} else {
				for i := range merged.Fido.Networks {
					if !strings.EqualFold(merged.Fido.Networks[i].Name, network) {
						continue
					}
					nd := &merged.Fido.Networks[i]
					if newName != "" {
						nd.Name = newName
					}
					nd.Enabled = formBool(r, "enabled")
					nd.Address = strings.TrimSpace(r.FormValue("address"))
					nd.Uplink = strings.TrimSpace(r.FormValue("uplink"))
					nd.Password = r.FormValue("password")
					nd.InboundDir = strings.TrimSpace(r.FormValue("inbound_dir"))
					nd.OutboundDir = strings.TrimSpace(r.FormValue("outbound_dir"))
					nd.NodelistDir = strings.TrimSpace(r.FormValue("nodelist_dir"))
					nd.HoldingDir = strings.TrimSpace(r.FormValue("holding_dir"))
					nd.BinkpPort = formInt(r, "binkp_port", 24554)
					nd.AreaFixPassword = r.FormValue("areafix_password")
					nd.FileFixPassword = r.FormValue("filefix_password")
					nd.TicPassword = r.FormValue("tic_password")
					nd.PollIntervalMins = formInt(r, "poll_interval_mins", 0)
					nd.NodelistURL = strings.TrimSpace(r.FormValue("nodelist_url"))
					nd.NodelistUpdateIntervalHours = formInt(r, "nodelist_update_hours", 0)
					nd.NodelistEchoTag = strings.TrimSpace(r.FormValue("nodelist_echo_tag"))
					nd.AKAs = akas
					nd.Areas = areas
					nd.FileAreas = fileAreas
					nd.Downlinks = downlinks
					break
				}
			}
			if err := saveFidoConfig(s.Deps.Messages.DB(), merged.Fido); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = "Network saved."
				if newName != "" {
					network = newName
				}
			}
		}
		cfg = config.Get()
		nd = cfg.Fido.NetworkByName(network)
		if nd != nil {
			data.Network = network
			data.Def = *nd
			data.AreasText = formatAreaMap(nd.Areas)
			data.FileAreasText = formatAreaMap(nd.FileAreas)
			data.DownlinksText = formatDownlinks(nd.Downlinks)
			data.AKAsText = strings.Join(nd.AKAs, "\n")
			data.SelectedFlags = nd.NodeFlags
		}
	}
	s.render(w, "admin_fido_networks.html", data)
}

type areaFixSubView struct {
	Downlink string
	Name     string
	Areas    []string
}

func (s *Server) handleAdminFidoRouting(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	network := selectedNetwork(r)
	db := s.Deps.Messages.DB()
	data := struct {
		pageData
		Networks        []string
		Network         string
		Routes          []*fido.Route
		Members         []*fido.Member
		RoutesExport    string
		RoutingExport   string
		EditMemberID    int64
		Flash           string
		Error           string
	}{
		pageData: s.page(r),
		Networks: fidoNetworkNamesList(),
		Network:  network,
	}
	data.Routes, _ = fido.ListRoutes(db, network)
	mdb := fido.OpenMembersDB(db)
	data.Members, _ = mdb.ListMembers(network)

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		network = selectedNetwork(r)
		data.Network = network
		switch r.FormValue("action") {
		case "add_route":
			pattern := strings.TrimSpace(r.FormValue("pattern"))
			routeTo := strings.TrimSpace(r.FormValue("route_to"))
			if err := fido.AddRoute(db, network, pattern, routeTo); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = "Route added."
			}
		case "remove_route":
			_ = fido.RemoveRoute(db, network, strings.TrimSpace(r.FormValue("pattern")))
			data.Flash = "Route removed."
		case "import_routes":
			if res, err := fido.ImportRoutesBBS(db, network, []byte(r.FormValue("routes_text"))); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = fmt.Sprintf("Imported %d route(s)", res.Added)
			}
		case "import_routing":
			if res, err := fido.ImportRoutingTable(db, network, []byte(r.FormValue("routing_text"))); err != nil {
				data.Error = err.Error()
			} else {
				data.Flash = fmt.Sprintf("Updated %d member(s)", res.Updated)
			}
		case "export_routes":
			if b, err := fido.ExportRoutesBBS(db, network); err == nil {
				data.RoutesExport = string(b)
			}
		case "export_routing":
			if b, err := fido.ExportRoutingTable(db, network); err == nil {
				data.RoutingExport = string(b)
			}
		case "update_member":
			id, _ := strconv.ParseInt(r.FormValue("member_id"), 10, 64)
			for _, m := range data.Members {
				if m.ID != id {
					continue
				}
				m.BBSName = strings.TrimSpace(r.FormValue("bbs_name"))
				m.SysopName = strings.TrimSpace(r.FormValue("sysop_name"))
				m.Location = strings.TrimSpace(r.FormValue("location"))
				m.Contact = strings.TrimSpace(r.FormValue("contact"))
				m.BinkpHost = strings.TrimSpace(r.FormValue("binkp_host"))
				m.Password = r.FormValue("password")
				if err := mdb.UpdateMemberInfo(m); err != nil {
					data.Error = err.Error()
				} else {
					data.Flash = "Member updated."
				}
				break
			}
		}
		data.Routes, _ = fido.ListRoutes(db, network)
		data.Members, _ = mdb.ListMembers(network)
	}
	s.render(w, "admin_fido_routing.html", data)
}

func (s *Server) handleAdminFidoJoin(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	network := selectedNetwork(r)
	db := s.Deps.Messages.DB()
	mdb := fido.OpenMembersDB(db)
	pending, _ := mdb.ListPending(network)
	data := struct {
		pageData
		Networks          []string
		Network           string
		Pending           []*fido.JoinRequest
		ApprovedPassword  string
		Flash             string
		Error             string
	}{
		pageData: s.page(r),
		Networks: fidoNetworkNamesList(),
		Network:  network,
		Pending:  pending,
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		network = selectedNetwork(r)
		id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
		switch r.FormValue("action") {
		case "approve":
			nd, err := networkDefByName(network)
			if err != nil {
				data.Error = err.Error()
				break
			}
			if !nd.IsHub() {
				data.Error = fmt.Sprintf("network %q is not a hub", network)
				break
			}
			joinReq, err := mdb.GetJoinRequest(id)
			if err != nil || joinReq == nil || joinReq.Status != "pending" {
				data.Error = "join request not found or already decided"
				break
			}
			net := formInt(r, "net", 0)
			if net == 0 && joinReq.RequestedNet != nil {
				net = *joinReq.RequestedNet
			}
			if net == 0 {
				net = 1
			}
			nodeNum := formInt(r, "node", 0)
			isHost := formBool(r, "is_host")
			if !isHost && nodeNum == 0 {
				nodeNum, err = mdb.NextNodeNum(network, net)
				if err != nil {
					data.Error = err.Error()
					break
				}
			}
			password := strings.TrimSpace(r.FormValue("password"))
			if password == "" {
				password = randomMemberPassword()
			}
			m, err := mdb.ApproveJoinRequest(nd, joinReq, net, nodeNum, isHost, password, saveNetworkDownlink)
			if err != nil {
				data.Error = err.Error()
			} else {
				cfg := config.Get()
				_ = fido.ApplyNodeAnnounceInfo(nd, db, s.Deps.Conferences, s.Deps.Messages, m, "NEW")
				_, _, _ = fido.GenerateNodelist(db, nd, cfg.BBS.Name, cfg.Sysop.Name)
				data.ApprovedPassword = password
				data.Flash = fmt.Sprintf("Approved as %s", m.Addr4D())
			}
		case "deny":
			_ = mdb.Deny(id, "Sysop")
			data.Flash = "Request denied."
		}
		data.Network = network
		data.Pending, _ = mdb.ListPending(network)
	}
	s.render(w, "admin_fido_join.html", data)
}

func (s *Server) handleAdminFidoTools(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	network := selectedNetwork(r)
	data := struct {
		pageData
		Networks []string
		Network  string
		Flash    string
		Error    string
		Result   string
	}{
		pageData: s.page(r),
		Networks: fidoNetworkNamesList(),
		Network:  network,
	}

	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		network = selectedNetwork(r)
		data.Network = network
		nd, err := networkDefByName(network)
		if err != nil {
			data.Error = err.Error()
		} else {
			fromName := strings.TrimSpace(r.FormValue("from_name"))
			if fromName == "" {
				fromName = "Sysop"
			}
			toAddr := strings.TrimSpace(r.FormValue("to_addr"))
			toName := strings.TrimSpace(r.FormValue("to_name"))
			var pkt string
			switch r.FormValue("action") {
			case "ping":
				pkt, err = fido.SendPing(nd, fromName, toName, toAddr)
			case "trace":
				pkt, err = fido.SendTrace(nd, fromName, toName, toAddr)
			case "areafix":
				adds := strings.Split(strings.TrimSpace(r.FormValue("adds")), "\n")
				removes := strings.Split(strings.TrimSpace(r.FormValue("removes")), "\n")
				pkt, err = fido.RequestAreaFix(nd, fromName, adds, removes)
			case "filefix":
				adds := strings.Split(strings.TrimSpace(r.FormValue("adds")), "\n")
				removes := strings.Split(strings.TrimSpace(r.FormValue("removes")), "\n")
				pkt, err = fido.RequestFileFix(nd, fromName, adds, removes)
			}
			if err != nil {
				data.Error = err.Error()
			} else if pkt != "" {
				data.Result = pkt
				data.Flash = "Packet written."
			}
		}
	}
	s.render(w, "admin_fido_tools.html", data)
}

// handleAdminFidoImportUpload accepts a nodelist file upload for import.
func (s *Server) handleAdminFidoImportUpload(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireSysop(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("nodelist")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	tmp, err := os.CreateTemp("", "nodelist-*.txt")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.ReadFrom(file); err != nil {
		_ = tmp.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = tmp.Close()
	network := selectedNetwork(r)
	if _, err := fido.ImportFile(s.Deps.Messages.DB(), tmpPath, network); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = header
	http.Redirect(w, r, "/admin/fido/nodelist?network="+network, http.StatusSeeOther)
}
