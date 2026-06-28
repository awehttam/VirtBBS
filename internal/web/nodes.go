package web

import (
	"strings"

	"github.com/virtbbs/virtbbs/internal/node"
	"github.com/virtbbs/virtbbs/internal/users"
)

func webOperationLabel(path string) string {
	switch {
	case path == "/" || path == "/menu":
		return "web.op.dashboard"
	case strings.HasPrefix(path, "/messages"):
		return "web.op.messages"
	case strings.HasPrefix(path, "/netmail"):
		return "web.op.netmail"
	case strings.HasPrefix(path, "/files"):
		return "web.op.files"
	case strings.HasPrefix(path, "/admin"):
		return "web.op.admin"
	case path == "/online":
		return "web.op.online"
	case strings.HasPrefix(path, "/api/"):
		return "web.op.active"
	default:
		return "web.op.path:" + path
	}
}

func (s *Server) bindWebNode(token string, u *users.User) {
	if s.Deps.Nodes == nil || token == "" || u == nil {
		return
	}
	if s.Sessions.NodeID(token) != 0 {
		return
	}
	nodeID, err := s.Deps.Nodes.Register()
	if err != nil {
		return
	}
	s.Sessions.SetNodeID(token, nodeID)
	node.RegisterControl(nodeID, func() { s.Sessions.Delete(token) })
	_ = s.Deps.Nodes.Update(nodeID, node.StatusWeb, "web.op.dashboard", u.ID, u.Name, u.City)
}

func (s *Server) releaseWebNode(token string) {
	if s.Deps.Nodes == nil || token == "" {
		return
	}
	nodeID := s.Sessions.NodeID(token)
	if nodeID == 0 {
		return
	}
	node.UnregisterControl(nodeID)
	_ = s.Deps.Nodes.Unregister(nodeID)
	s.Sessions.SetNodeID(token, 0)
}

func (s *Server) touchWebNode(token string, u *users.User, path string) {
	if s.Deps.Nodes == nil || token == "" || u == nil {
		return
	}
	if s.Sessions.NodeID(token) == 0 {
		s.bindWebNode(token, u)
	}
	nodeID := s.Sessions.NodeID(token)
	if nodeID == 0 {
		return
	}
	_ = s.Deps.Nodes.Update(nodeID, node.StatusWeb, webOperationLabel(path), u.ID, u.Name, u.City)
}
