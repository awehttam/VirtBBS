package web

import (
	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/config"
)

// ConferenceNetworkGroup is a Fido network (or local) section for templates.
type ConferenceNetworkGroup struct {
	Network string
	Rows    []ConferenceListRow
}

// SubscriptionNetworkGroup groups echo subscription rows by network.
type SubscriptionNetworkGroup struct {
	Network string
	Rows    []SubscriptionRow
}

// SubscriptionRow is one echomail area on the subscriptions page.
type SubscriptionRow struct {
	Conference *conferences.Conference
	Subscribed bool
	NewCount   int
}

func conferenceNetworkOrder() []string {
	return fidoNetworkNamesList()
}

func primaryFidoNetworkName() string {
	return config.Get().Fido.EffectivePrimaryName()
}

func groupConferenceListRows(rows []ConferenceListRow) []ConferenceNetworkGroup {
	if len(rows) == 0 {
		return nil
	}
	confs := make([]*conferences.Conference, 0, len(rows))
	rowByID := make(map[int]ConferenceListRow, len(rows))
	for _, row := range rows {
		if row.Conference == nil {
			continue
		}
		confs = append(confs, row.Conference)
		rowByID[row.Conference.ID] = row
	}
	groups := conferences.GroupByNetwork(confs, conferenceNetworkOrder(), primaryFidoNetworkName())
	out := make([]ConferenceNetworkGroup, 0, len(groups))
	for _, g := range groups {
		section := ConferenceNetworkGroup{Network: g.Network}
		for _, c := range g.List {
			if row, ok := rowByID[c.ID]; ok {
				section.Rows = append(section.Rows, row)
			}
		}
		if len(section.Rows) > 0 {
			out = append(out, section)
		}
	}
	return out
}

func groupConferences(confs []*conferences.Conference) []conferences.NetworkGroup {
	return conferences.GroupByNetwork(confs, conferenceNetworkOrder(), primaryFidoNetworkName())
}

func sortConferencesByNetwork(confs []*conferences.Conference) []*conferences.Conference {
	return conferences.SortByNetwork(confs, conferenceNetworkOrder(), primaryFidoNetworkName())
}

func groupSubscriptionRows(rows []SubscriptionRow) []SubscriptionNetworkGroup {
	if len(rows) == 0 {
		return nil
	}
	confs := make([]*conferences.Conference, 0, len(rows))
	rowByID := make(map[int]SubscriptionRow, len(rows))
	for _, row := range rows {
		if row.Conference == nil {
			continue
		}
		confs = append(confs, row.Conference)
		rowByID[row.Conference.ID] = row
	}
	groups := conferences.GroupByNetwork(confs, conferenceNetworkOrder(), primaryFidoNetworkName())
	out := make([]SubscriptionNetworkGroup, 0, len(groups))
	for _, g := range groups {
		section := SubscriptionNetworkGroup{Network: g.Network}
		for _, c := range g.List {
			if row, ok := rowByID[c.ID]; ok {
				section.Rows = append(section.Rows, row)
			}
		}
		if len(section.Rows) > 0 {
			out = append(out, section)
		}
	}
	return out
}
