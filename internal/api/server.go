// ============================================================================
// VirtBBS — A modern BBS server inspired by PCBoard BBS
//           (Clark Development Company, 1987-1996)
//
// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// MIT License
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
// OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.
//
// Change History:
//   v0.0.1  2026-06-24  Initial implementation
//   v0.0.2  2026-06-24  Phase 10: Implement node.kick and node.broadcast endpoints
//   v0.0.5  2026-06-24  Phase 14: callers.search, callers.stats endpoints
// ============================================================================

// Package api provides a JSON-over-TCP management API for remote sysop access.
package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/virtbbs/virtbbs/internal/callers"
	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/fido"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/node"
	"github.com/virtbbs/virtbbs/internal/users"
)

// Request is a JSON-RPC-style request.
type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	Auth   AuthParams      `json:"auth"`
}

// AuthParams carries sysop credentials on every request.
type AuthParams struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// Response wraps the result or an error string.
type Response struct {
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Deps bundles store dependencies.
type Deps struct {
	Users       *users.Store
	Messages    *messages.Store
	Nodes       *node.Store
	Callers     *callers.Log
	Conferences *conferences.Store
	// MsgStore is the same as Messages — kept for fido toss/scan which needs *messages.Store.
}

// Server listens for API connections.
type Server struct {
	Addr string
	Deps Deps
}

func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("api listen %s: %w", s.Addr, err)
	}
	defer ln.Close()
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(c)
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	sc := bufio.NewScanner(c)
	enc := json.NewEncoder(c)

	for sc.Scan() {
		var req Request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			_ = enc.Encode(Response{Error: "invalid JSON"})
			continue
		}
		if !s.authenticate(req.Auth) {
			_ = enc.Encode(Response{Error: "unauthorized"})
			continue
		}
		result, err := s.dispatch(req)
		if err != nil {
			_ = enc.Encode(Response{Error: err.Error()})
		} else {
			_ = enc.Encode(Response{Result: result})
		}
	}
}

func (s *Server) authenticate(auth AuthParams) bool {
	cfg := config.Get()
	if !strings.EqualFold(auth.User, cfg.Sysop.Name) {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(cfg.Sysop.PasswordHash), []byte(auth.Password)) == nil
}

func (s *Server) dispatch(req Request) (any, error) {
	switch req.Method {
	case "nodes.list":
		return s.Deps.Nodes.List()

	case "users.list":
		return s.Deps.Users.List()

	case "users.get":
		var p struct{ ID int64 }
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		return s.Deps.Users.GetByID(p.ID)

	case "users.delete":
		var p struct{ ID int64 }
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		return nil, s.Deps.Users.Delete(p.ID)

	case "users.setpassword":
		var p struct {
			ID       int64
			Password string
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		return nil, s.Deps.Users.SetPassword(p.ID, p.Password)

	case "messages.list":
		var p struct {
			ConferenceID int
			Limit        int
			Offset       int
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Limit == 0 {
			p.Limit = 50
		}
		return s.Deps.Messages.List(p.ConferenceID, p.Limit, p.Offset)

	case "callers.list":
		var p struct{ N int }
		_ = json.Unmarshal(req.Params, &p)
		if p.N == 0 {
			p.N = 50
		}
		return s.Deps.Callers.Recent(p.N)

	case "callers.search":
		var p struct {
			Query string
			N     int
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.N == 0 {
			p.N = 100
		}
		return s.Deps.Callers.Search(p.Query, p.N)

	case "callers.stats":
		unique, total, err := s.Deps.Callers.DailyStats()
		if err != nil {
			return nil, err
		}
		return map[string]int{"unique": unique, "total": total}, nil

	case "fido.toss":
		cfg := config.Get()
		if !cfg.Fido.Enabled {
			return nil, fmt.Errorf("FidoNet is not enabled")
		}
		return fido.TossAll(&cfg.Fido, s.Deps.Messages, s.Deps.Conferences), nil

	case "fido.scan":
		cfg := config.Get()
		if !cfg.Fido.Enabled {
			return nil, fmt.Errorf("FidoNet is not enabled")
		}
		result, err := fido.ScanAll(&cfg.Fido, s.Deps.Messages, s.Deps.Conferences, cfg.BBS.Name)
		if err != nil {
			return nil, err
		}
		return result, nil

	case "config.get":
		return config.Get(), nil

	case "config.update":
		// Merge into the live config so un-sent fields are preserved.
		current := config.Get()
		merged := *current // shallow copy
		if err := json.Unmarshal(req.Params, &merged); err != nil {
			return nil, err
		}
		// Never let the API zero out the password hash.
		if merged.Sysop.PasswordHash == "" {
			merged.Sysop.PasswordHash = current.Sysop.PasswordHash
		}
		return nil, config.Save(&merged)

	case "node.kick":
		var p struct{ NodeID int }
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		return nil, node.KickNode(p.NodeID)

	case "node.broadcast":
		var p struct {
			From    string
			Message string
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		if p.From == "" {
			p.From = "Sysop"
		}
		node.BroadcastAll(p.From, p.Message)
		return "broadcast sent", nil

	case "conferences.list":
		return s.Deps.Conferences.List()

	case "conferences.create":
		var p conferences.Conference
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		if err := s.Deps.Conferences.Create(&p); err != nil {
			return nil, err
		}
		return p, nil

	case "conferences.delete":
		var p struct{ ID int }
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		return nil, s.Deps.Conferences.Delete(p.ID)

	case "conferences.update":
		var p conferences.Conference
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		return nil, s.Deps.Conferences.Update(&p)

	// ── FidoNet nodelist ────────────────────────────────────────────────────

	case "fido.nodes.search":
		var p struct {
			Network string `json:"network"`
			Query   string `json:"query"`
			Page    int    `json:"page"`
			Size    int    `json:"size"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		ndb := fido.OpenNodelistDB(s.Deps.Messages.DB())
		return ndb.Search(p.Network, p.Query, p.Page, p.Size)

	case "fido.nodes.get":
		var p struct {
			Network string `json:"network"`
			Addr    string `json:"addr"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		a, err := fido.ParseAddr(p.Addr)
		if err != nil {
			return nil, err
		}
		ndb := fido.OpenNodelistDB(s.Deps.Messages.DB())
		return ndb.LookupAddr(p.Network, a)

	case "fido.nodes.count":
		var p struct{ Network string }
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		ndb := fido.OpenNodelistDB(s.Deps.Messages.DB())
		n, err := ndb.Count(p.Network)
		return map[string]int{"count": n}, err

	case "fido.poll":
		var p struct{ Network string }
		_ = json.Unmarshal(req.Params, &p)
		cfg := config.Get()
		if !cfg.Fido.Enabled {
			return nil, fmt.Errorf("FidoNet is not enabled")
		}
		nd := cfg.Fido.NetworkByName(p.Network)
		if nd == nil {
			return nil, fmt.Errorf("network %q not found", p.Network)
		}
		result := fido.PollAndToss(nd, s.Deps.Messages, s.Deps.Conferences)
		return result, result.Poll.Error

	case "fido.netmail.send":
		var m fido.NetmailMsg
		if err := json.Unmarshal(req.Params, &m); err != nil {
			return nil, err
		}
		cfg := config.Get()
		ndb := fido.OpenNetmailDB(s.Deps.Messages.DB())
		id, err := ndb.Enqueue(&m)
		if err != nil {
			return nil, err
		}
		// Also write PKT immediately.
		nd := cfg.Fido.NetworkByName(m.Network)
		if nd == nil {
			return map[string]int64{"id": id}, nil
		}
		nextHop, err := fido.RouteAddr(&m, nd)
		if err != nil {
			return map[string]int64{"id": id}, nil
		}
		outDir := fido.OutboundDir(nd.OutboundDir, nextHop, m.Crash)
		origAddr, _ := fido.ParseAddr(nd.Address)
		pktPath, err := fido.WritePKT(origAddr, nextHop, nd.Password, outDir, []*fido.NetmailMsg{&m})
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": id, "pkt": pktPath}, nil

	case "fido.import.nodelist":
		var p struct {
			Path    string `json:"path"`
			Network string `json:"network"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Network == "" {
			p.Network = "FidoNet"
		}
		return fido.ImportFile(s.Deps.Messages.DB(), p.Path, p.Network)

	case "fido.nodelist.fetch":
		var p struct{ Network string }
		_ = json.Unmarshal(req.Params, &p)
		cfg := config.Get()
		if !cfg.Fido.Enabled {
			return nil, fmt.Errorf("FidoNet is not enabled")
		}
		nd := cfg.Fido.NetworkByName(p.Network)
		if nd == nil {
			return nil, fmt.Errorf("network %q not found", p.Network)
		}
		return fido.FetchAndImport(nd, s.Deps.Messages.DB())

	default:
		return nil, fmt.Errorf("unknown method: %s", req.Method)
	}
}
