package web

import (
	"fmt"
	"strings"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/display"
	"github.com/virtbbs/virtbbs/internal/node"
	"github.com/virtbbs/virtbbs/internal/users"
)

// NewMessageLine is one conference with unread messages for the dashboard.
type NewMessageLine struct {
	ConferenceID int
	Name         string
	Count        int
}

// ConferenceListRow is one message area on the conference picker with read stats.
type ConferenceListRow struct {
	Conference *conferences.Conference
	Total      int // highest message number in the area
	Unread     int // messages since last read
	LastRead   int // last message number read by this user
}

// DashboardStats mirrors the terminal main-menu Stats screen (session.gatherStats).
type DashboardStats struct {
	NewMsgsTotal   int
	BBSCallsToday  int
	BBSUniqueToday int
	BBSMsgTotal    int
	BBSConfCount   int
	BBSFileTotal   int
	BBSFileToday   int
	BBSFileMonth   int
	OnlineNodes    int
}

// BulletinView is a rendered display file for the web UI.
type BulletinView struct {
	Name    string
	Title   string
	HTML    string // empty if not pre-rendered inline
	HasFile bool
}

func (s *Server) displayVars(u *users.User) *display.Vars {
	cfg := config.Get()
	v := &display.Vars{
		BBSName:   cfg.BBS.Name,
		SysopName: cfg.Sysop.Name,
		TimeLeft:  cfg.Session.TimePerCallMins,
	}
	if u != nil {
		v.Name = u.Name
		v.City = u.City
		v.Security = u.SecurityLevel
		v.NumCalls = u.TimesOnline
	}
	return v
}

func (s *Server) renderDisplayHTML(name string, u *users.User) (string, error) {
	cfg := config.Get()
	text, err := display.Render(cfg.Session.DisplayDir, name, s.displayVars(u))
	if err != nil {
		return "", err
	}
	return ansiToHTML(text), nil
}

func (s *Server) gatherDashboardStats(u *users.User) DashboardStats {
	var st DashboardStats
	if counts, err := s.Deps.Users.NewMessageCounts(u.ID); err == nil {
		for _, n := range counts {
			st.NewMsgsTotal += n
		}
	}
	if s.Deps.Callers != nil {
		if unique, total, err := s.Deps.Callers.DailyStats(); err == nil {
			st.BBSUniqueToday = unique
			st.BBSCallsToday = total
		}
	}
	if n, err := s.Deps.Messages.TotalCount(); err == nil {
		st.BBSMsgTotal = n
	}
	if confs, err := s.Deps.Conferences.List(); err == nil {
		st.BBSConfCount = len(confs)
	}
	if cat, err := s.Deps.Files.GetCatalogStats(); err == nil {
		st.BBSFileTotal = cat.Total
		st.BBSFileToday = cat.Today
		st.BBSFileMonth = cat.LastMonth
	}
	if s.Deps.Nodes != nil {
		if nodes, err := s.Deps.Nodes.List(); err == nil {
			st.OnlineNodes = len(nodes)
		}
	}
	return st
}

func (s *Server) gatherNewMessageLines(u *users.User) []NewMessageLine {
	counts, err := s.Deps.Users.NewMessageCounts(u.ID)
	if err != nil || len(counts) == 0 {
		return nil
	}
	var lines []NewMessageLine
	for confID, n := range counts {
		if n == 0 {
			continue
		}
		name := fmt.Sprintf("Conference %d", confID)
		if c, err := s.Deps.Conferences.Get(confID); err == nil {
			name = c.Name
		}
		lines = append(lines, NewMessageLine{ConferenceID: confID, Name: name, Count: n})
	}
	return lines
}

func (s *Server) buildConferenceListRows(u *users.User, confs []*conferences.Conference) []ConferenceListRow {
	unread, _ := s.Deps.Users.NewMessageCounts(u.ID)
	highMap, _ := s.Deps.Messages.HighMsgNumberByConference()
	lastMap := s.Deps.Users.LastReadMap(u.ID)
	rows := make([]ConferenceListRow, 0, len(confs))
	for _, c := range confs {
		total := highMap[c.ID]
		lastRead := lastMap[c.ID]
		if lastRead > total {
			_ = s.Deps.Users.SetLastRead(u.ID, c.ID, total)
			lastRead = total
		}
		rows = append(rows, ConferenceListRow{
			Conference: c,
			Total:      total,
			Unread:     unread[c.ID],
			LastRead:   lastRead,
		})
	}
	return rows
}

func (s *Server) listBulletins() []BulletinView {
	cfg := config.Get()
	bulletins, err := display.ListBulletins(cfg.Session.DisplayDir)
	if err != nil {
		return nil
	}
	out := make([]BulletinView, 0, len(bulletins))
	for _, b := range bulletins {
		out = append(out, BulletinView{Name: b.Name, Title: b.Title, HasFile: true})
	}
	return out
}

func bulletinTitle(name string) string {
	upper := strings.ToUpper(name)
	titles := map[string]string{
		"LOGON": "Logon Message", "BINKPDAY": "BinkP Statistics (24h)",
		"BINKPALL": "BinkP Statistics (All Time)", "BULLETIN": "Bulletin",
	}
	if t, ok := titles[upper]; ok {
		return t
	}
	return name
}

// onlineUsers returns active nodes for the who's-online page.
func (s *Server) onlineUsers() ([]*node.NodeInfo, error) {
	if s.Deps.Nodes == nil {
		return nil, nil
	}
	return s.Deps.Nodes.List()
}
