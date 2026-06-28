package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/virtbbs/virtbbs/internal/fido"
)

type binkpBulletinPageData struct {
	pageData
	Name         string
	Title        string
	Subtitle     string
	GeneratedAt  string
	NetworkViews []BinkpNetworkView
}

func (s *Server) renderBinkpStatsBulletin(w http.ResponseWriter, r *http.Request, name string) {
	period := "24h"
	subtitle := ""
	if name == "BINKPALL" {
		period = "all"
	} else {
		dayKey := time.Now().Add(-24 * time.Hour).Format("2006-01-02")
		subtitle = fmt.Sprintf("Previous 24 hours (%s)", dayKey)
	}

	db := s.Deps.Messages.DB()
	if db == nil {
		http.Error(w, "stats unavailable", http.StatusServiceUnavailable)
		return
	}
	st, err := fido.QueryBinkpStatsForPeriod(db, "", period, time.Now())
	if err != nil {
		http.Error(w, "stats unavailable", http.StatusServiceUnavailable)
		return
	}

	var views []BinkpNetworkView
	for _, n := range st.Networks {
		view := BinkpNetworkView{
			Network: n.Network,
			Stats:   n,
			Links:   fido.LinksForNetwork(st, n.Network),
			ChartID: fido.SanitizeChartID(n.Network),
		}
		if series, err := fido.QueryBinkpDailySeries(db, n.Network, binkpChartDays); err == nil {
			view.ChartJSON = binkpChartJSON(series)
		}
		views = append(views, view)
	}

	data := binkpBulletinPageData{
		pageData:     s.page(r),
		Name:         name,
		Title:        bulletinTitle(name),
		Subtitle:     subtitle,
		GeneratedAt:  time.Now().Format(time.RFC3339),
		NetworkViews: views,
	}
	s.render(w, "binkp_bulletin_view.html", data)
}
