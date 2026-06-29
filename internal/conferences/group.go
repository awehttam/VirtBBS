package conferences

import (
	"sort"
	"strings"
)

// NetworkGroup lists conferences belonging to one Fido network (or local-only).
// Network is empty for non-echomail (local) conferences.
type NetworkGroup struct {
	Network string
	List    []*Conference
}

// EffectiveNetwork returns the Fido network name for an echomail conference.
// Non-echo conferences return "" (local). Blank network uses primaryName.
func EffectiveNetwork(c *Conference, primaryName string) string {
	if c == nil || !c.Echo {
		return ""
	}
	if n := strings.TrimSpace(c.Network); n != "" {
		return n
	}
	return strings.TrimSpace(primaryName)
}

// GroupByNetwork groups conferences under local areas first, then each known
// network in networkOrder, then any remaining network names alphabetically.
// Within each group, conferences are sorted by ID.
func GroupByNetwork(confs []*Conference, networkOrder []string, primaryName string) []NetworkGroup {
	local := make([]*Conference, 0)
	byNet := make(map[string][]*Conference)
	for _, c := range confs {
		if c == nil {
			continue
		}
		net := EffectiveNetwork(c, primaryName)
		if net == "" {
			local = append(local, c)
			continue
		}
		byNet[net] = append(byNet[net], c)
	}
	sortConfs := func(list []*Conference) {
		sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	}
	var groups []NetworkGroup
	if len(local) > 0 {
		sortConfs(local)
		groups = append(groups, NetworkGroup{Network: "", List: local})
	}
	seen := make(map[string]bool, len(networkOrder))
	for _, net := range networkOrder {
		net = strings.TrimSpace(net)
		if net == "" || seen[net] {
			continue
		}
		list := byNet[net]
		if len(list) == 0 {
			continue
		}
		sortConfs(list)
		groups = append(groups, NetworkGroup{Network: net, List: list})
		seen[net] = true
	}
	var extra []string
	for net := range byNet {
		if !seen[net] && len(byNet[net]) > 0 {
			extra = append(extra, net)
		}
	}
	sort.Strings(extra)
	for _, net := range extra {
		list := byNet[net]
		sortConfs(list)
		groups = append(groups, NetworkGroup{Network: net, List: list})
	}
	return groups
}

// SortByNetwork returns conferences flattened in network group order.
func SortByNetwork(confs []*Conference, networkOrder []string, primaryName string) []*Conference {
	groups := GroupByNetwork(confs, networkOrder, primaryName)
	out := make([]*Conference, 0, len(confs))
	for _, g := range groups {
		out = append(out, g.List...)
	}
	return out
}
