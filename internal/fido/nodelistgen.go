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
//   v0.13.0 2026-06-27  VirtNet: nodelist.go only ever imports a nodelist
//                        (ImportFile) — this is the missing encoder half,
//                        generating VirtBBS's own outbound nodelist (full +
//                        a day-over-day diff) for a network it hosts.
//   v1.4.1  2026-06-28  Host (/0) lines per FTS-0005; sync fido_members into
//                        fido_nodes for hub search/export; Host AKA pairing.
// ============================================================================

// Package fido — nodelistgen.go
//
// Generates this BBS's own outbound nodelist for a network it hosts
// (NetworkDef.IsHub()), sourced from fido_members rather than a parsed
// file. Filename convention: NODELIST.Z## / NODEDIFF.Z## where ## is
// day-of-year mod 100 (FidoNet style). Full lists publish weekly (Friday);
// other days publish diffs only.
package fido

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GenerateNodelist rebuilds fido_nodes from fido_members for hub network nd.
// On weekly nodelist days (Friday) it also writes NODELIST.Z## to nodelist_dir.
// Returns the encoded nodelist bytes and the filename when written (empty on
// non-weekly days).
func GenerateNodelist(db *sql.DB, nd *NetworkDef, hubBBSName, hubSysopName string) ([]byte, string, error) {
	data, entries, err := buildHubNodelist(db, nd, hubBBSName, hubSysopName)
	if err != nil {
		return nil, "", err
	}
	if err := rebuildHubNodelistDB(db, nd.Name, entries); err != nil {
		return data, "", err
	}
	now := time.Now()
	if !IsWeeklyNodelistDay(now) {
		return data, "", nil
	}
	filename := NodelistFullFilename(now)
	if err := os.MkdirAll(nd.NodelistDir, 0755); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(filepath.Join(nd.NodelistDir, filename), data, 0644); err != nil {
		return nil, "", err
	}
	return data, filename, nil
}

// UpdateHubNodelistFromMembers syncs fido_nodes from fido_members and writes
// the appropriate on-disk file: NODELIST.Z## on weekly days, NODEDIFF.Z## on
// other days when there are changes.
func UpdateHubNodelistFromMembers(db *sql.DB, nd *NetworkDef, hubBBSName, hubSysopName string) error {
	_, fullName, err := GenerateNodelist(db, nd, hubBBSName, hubSysopName)
	if err != nil {
		return err
	}
	if fullName != "" {
		return nil
	}
	_, _, err = GenerateNodelistDiff(db, nd)
	return err
}

func buildHubNodelist(db *sql.DB, nd *NetworkDef, hubBBSName, hubSysopName string) ([]byte, []NodeEntry, error) {
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return nil, nil, fmt.Errorf("invalid network address %q", nd.Address)
	}
	mdb := OpenMembersDB(db)
	members, err := mdb.ListMembers(nd.Name)
	if err != nil {
		return nil, nil, err
	}
	entries := hubNodelistEntries(nd, members, hubBBSName, hubSysopName)
	return encodeNodelistEntries(nd.Name, entries), entries, nil
}

// RebuildHubNodelistDB refreshes fido_nodes from fido_members for a hub
// network so web/API nodelist search sees every VirtNet member (and each
// net's Host at zone:net/0 per FTS-0005).
func RebuildHubNodelistDB(db *sql.DB, nd *NetworkDef, hubBBSName, hubSysopName string) error {
	if !nd.UsesMemberNodelist() {
		return nil
	}
	mdb := OpenMembersDB(db)
	members, err := mdb.ListMembers(nd.Name)
	if err != nil {
		return err
	}
	return rebuildHubNodelistDB(db, nd.Name, hubNodelistEntries(nd, members, hubBBSName, hubSysopName))
}

func rebuildHubNodelistDB(db *sql.DB, network string, entries []NodeEntry) error {
	if _, err := db.Exec(`DELETE FROM fido_nodes WHERE network=?`, network); err != nil {
		return err
	}
	ndb := OpenNodelistDB(db)
	for i := range entries {
		if err := ndb.UpsertLocalNode(&entries[i]); err != nil {
			return err
		}
	}
	return RecordNodelistVersionFromMembers(db, network, len(entries))
}

// hubNodelistEntries builds VirtNet nodelist rows from fido_members,
// including Zone and Host (/0) lines per FTS-0005. A net coordinator
// (IsHost) is listed as Host,N,... (address zone:net/0) and, when their
// assigned node number is non-zero, also as a regular node line (AKA).
func hubNodelistEntries(nd *NetworkDef, members []*Member, hubBBSName, hubSysopName string) []NodeEntry {
	our := nd.NodeAddr()
	var out []NodeEntry

	out = append(out, NodeEntry{
		Network: nd.Name, Zone: our.Zone, Net: our.Zone, Node: 0,
		Name: hubBBSName, Location: "Internet", Sysop: hubSysopName,
		Phone: "-Unpublished-", Baud: 33600, Flags: "CM", Type: "Zone", Active: true,
	})

	byNet := groupByNet(members)
	for _, net := range sortedNets(byNet) {
		netMembers := byNet[net]
		host := findHost(netMembers)

		if host != nil {
			out = append(out, memberAsHostEntry(host))
			if host.NodeNum != 0 {
				e := memberAsNodeEntry(host)
				e.AKA = hostHostAKA(host)
				out = append(out, e)
			}
		} else if net == our.Net {
			out = append(out, NodeEntry{
				Network: nd.Name, Zone: our.Zone, Net: net, Node: 0,
				Name: hubBBSName, Location: "Internet", Sysop: hubSysopName,
				Phone: "-Unpublished-", Baud: 33600, Type: "Host", Active: true,
			})
		}

		for _, m := range netMembers {
			if host != nil && m.ID == host.ID {
				continue
			}
			out = append(out, memberAsNodeEntry(m))
		}
	}
	LinkHostAKAs(out)
	return out
}

func memberAsHostEntry(m *Member) NodeEntry {
	return NodeEntry{
		Network: m.Network, Zone: m.Zone, Net: m.Net, Node: 0,
		Name: m.BBSName, Location: m.Location, Sysop: m.SysopName,
		Phone: "-Unpublished-", Baud: 33600, Flags: memberIBNFlags(m),
		Type: "Host", Active: m.IsActive,
	}
}

func memberAsNodeEntry(m *Member) NodeEntry {
	return NodeEntry{
		Network: m.Network, Zone: m.Zone, Net: m.Net, Node: m.NodeNum, Point: m.Point,
		Name: m.BBSName, Location: m.Location, Sysop: m.SysopName,
		Phone: "-Unpublished-", Baud: 33600, Flags: memberIBNFlags(m),
		Type: "Node", Active: m.IsActive,
	}
}

func hostHostAKA(m *Member) string {
	return fmt.Sprintf("%d:%d/0", m.Zone, m.Net)
}

func memberIBNFlags(m *Member) string {
	if m.BinkpHost == "" {
		return ""
	}
	return ",IBN:" + m.BinkpHost
}

// LinkHostAKAs sets NodeEntry.AKA on paired Host (/0) and regular node rows
// for the same net coordinator within one result set.
func LinkHostAKAs(entries []NodeEntry) {
	type nodeRef struct {
		idx  int
		addr string
	}
	hostByNet := map[int]int{}
	nodeByNet := map[int]nodeRef{}
	for i, e := range entries {
		if e.Type == "Host" && e.Node == 0 {
			hostByNet[e.Net] = i
		}
		if e.Type == "Node" && e.Node != 0 {
			if hIdx, ok := hostByNet[e.Net]; ok && entries[hIdx].Name == e.Name {
				nodeByNet[e.Net] = nodeRef{i, e.Addr4D()}
			}
		}
	}
	for net, hIdx := range hostByNet {
		if nr, ok := nodeByNet[net]; ok {
			entries[hIdx].AKA = nr.addr
			entries[nr.idx].AKA = hostHostAKAFromEntry(&entries[hIdx])
		}
	}
}

// LinkHostAKAsPtrs is LinkHostAKAs for a slice of pointers from Search.
func LinkHostAKAsPtrs(nodes []*NodeEntry) {
	if len(nodes) == 0 {
		return
	}
	entries := make([]NodeEntry, len(nodes))
	for i, n := range nodes {
		if n != nil {
			entries[i] = *n
		}
	}
	LinkHostAKAs(entries)
	for i := range entries {
		if nodes[i] != nil {
			nodes[i].AKA = entries[i].AKA
		}
	}
}

func hostHostAKAFromEntry(e *NodeEntry) string {
	return fmt.Sprintf("%d:%d/0", e.Zone, e.Net)
}

// LinkConfiguredAKAs sets NodeEntry.AKA for rows matching this BBS's configured
// local addresses when LinkHostAKAs did not already pair them.
func LinkConfiguredAKAs(nodes []*NodeEntry, nd *NetworkDef) {
	if nd == nil || len(nodes) == 0 {
		return
	}
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return
	}
	local := nd.AllAddrs()
	if len(local) <= 1 {
		return
	}
	localSet := make(map[string]bool, len(local))
	for _, a := range local {
		localSet[a.String()] = true
	}
	primary := our.String()
	var companions []string
	for _, a := range local {
		if s := a.String(); s != primary {
			companions = append(companions, s)
		}
	}
	companionStr := strings.Join(companions, ", ")
	for _, n := range nodes {
		if n == nil {
			continue
		}
		addr := n.Addr4D()
		if !localSet[addr] {
			continue
		}
		if addr == primary {
			if n.AKA == "" {
				n.AKA = companionStr
			}
		} else if n.AKA == "" {
			n.AKA = primary
		}
	}
}

func encodeNodelistEntries(network string, entries []NodeEntry) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, ";VirtNet nodelist for %q, generated %s\r\n", network, time.Now().Format(time.RFC3339))
	for i := range entries {
		fmt.Fprintf(&b, "%s\r\n", encodeHubNodeLine(&entries[i]))
	}
	return []byte(b.String())
}

func encodeHubNodeLine(e *NodeEntry) string {
	keyword := nodelistKeyword(e.Type)
	num := e.Node
	switch strings.ToLower(e.Type) {
	case "zone":
		num = e.Zone
	case "region", "host":
		num = e.Net
	}
	phone := e.Phone
	if phone == "" {
		phone = "-Unpublished-"
	}
	baud := e.Baud
	if baud == 0 {
		baud = 33600
	}
	flags := e.Flags
	if keyword == "" {
		return fmt.Sprintf(",%d,%s,%s,%s,%s,%d,%s",
			num, nlEncode(e.Name), nlEncode(e.Location), nlEncode(e.Sysop),
			phone, baud, flags)
	}
	return fmt.Sprintf("%s,%d,%s,%s,%s,%s,%d,%s",
		keyword, num, nlEncode(e.Name), nlEncode(e.Location), nlEncode(e.Sysop),
		phone, baud, flags)
}

func groupByNet(members []*Member) map[int][]*Member {
	out := map[int][]*Member{}
	for _, m := range members {
		out[m.Net] = append(out[m.Net], m)
	}
	return out
}

func sortedNets(byNet map[int][]*Member) []int {
	nets := make([]int, 0, len(byNet))
	for n := range byNet {
		nets = append(nets, n)
	}
	sort.Ints(nets)
	return nets
}

func findHost(members []*Member) *Member {
	for _, m := range members {
		if m.IsHost {
			return m
		}
	}
	return nil
}

func writeHostMemberLine(b *strings.Builder, m *Member) {
	writeNodelistLine(b, "Host", m.Net, m.BBSName, m.Location, m.SysopName, memberIBNFlags(m))
}

func writeMemberLine(b *strings.Builder, keyword string, m *Member) {
	if keyword == "Host" || (keyword == "" && m.IsHost) {
		writeHostMemberLine(b, m)
		if m.NodeNum != 0 {
			writeNodelistLine(b, "", m.NodeNum, m.BBSName, m.Location, m.SysopName, memberIBNFlags(m))
		}
		return
	}
	writeNodelistLine(b, keyword, m.NodeNum, m.BBSName, m.Location, m.SysopName, memberIBNFlags(m))
}

func writeNodelistLine(b *strings.Builder, keyword string, num int, bbsName, location, sysop, extraFlags string) {
	fmt.Fprintf(b, "%s,%d,%s,%s,%s,-Unpublished-,33600%s\r\n",
		keyword, num, nlEncode(bbsName), nlEncode(location), nlEncode(sysop), extraFlags)
}

// SnapshotMembers records today's fido_members rows into
// fido_members_snapshot for network, for tomorrow's GenerateNodelistDiff
// to compare against. Called once per day, right after GenerateNodelist.
func SnapshotMembers(db *sql.DB, network string) error {
	now := time.Now()
	mdb := OpenMembersDB(db)
	members, err := mdb.ListMembers(network)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO fido_members_snapshot
		(network, year, day_of_year, zone, net, node_num, point, bbs_name, sysop_name, location, flags)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, m := range members {
		flags := ""
		if m.BinkpHost != "" {
			flags = "IBN:" + m.BinkpHost
		}
		if _, err := stmt.Exec(network, now.Year(), now.YearDay(), m.Zone, m.Net, m.NodeNum, m.Point,
			m.BBSName, m.SysopName, m.Location, flags); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

type snapshotEntry struct {
	zone, net, node, point       int
	bbsName, sysopName, location string
	flags                        string
}

// GenerateNodelistDiff diffs today's fido_members against the most recent
// fido_members_snapshot rows for network, producing a VirtNet-flavored
// NODEDIFF-style file: added/changed nodes as full FTS lines, removed
// nodes as an address-only deletion marker ("-Zone:Net/Node"). This is not
// byte-compatible with the historical binary NODEDIFF patch format — not
// needed for an internal, single-network use case.
func GenerateNodelistDiff(db *sql.DB, nd *NetworkDef) ([]byte, string, error) {
	mdb := OpenMembersDB(db)
	current, err := mdb.ListMembers(nd.Name)
	if err != nil {
		return nil, "", err
	}

	prevYear, prevDay, err := mostRecentSnapshotDay(db, nd.Name, time.Now())
	prev := map[Addr]snapshotEntry{}
	if err == nil && prevYear > 0 {
		prev, err = loadSnapshot(db, nd.Name, prevYear, prevDay)
		if err != nil {
			return nil, "", err
		}
	}

	curByAddr := map[Addr]*Member{}
	for _, m := range current {
		curByAddr[m.Addr()] = m
	}

	var b strings.Builder
	fmt.Fprintf(&b, ";%s nodelist diff, generated %s\r\n", nd.Name, time.Now().Format(time.RFC3339))

	for addr, m := range curByAddr {
		old, existed := prev[addr]
		if !existed || old.bbsName != m.BBSName || old.sysopName != m.SysopName ||
			old.location != m.Location || (old.flags == "") != (m.BinkpHost == "") {
			writeMemberLine(&b, "", m)
		}
	}
	for addr := range prev {
		if _, stillThere := curByAddr[addr]; !stillThere {
			fmt.Fprintf(&b, "-%s\r\n", addr.String())
		}
	}

	filename := NodelistDiffFilename(time.Now())
	data := []byte(b.String())
	if !nodelistBodyHasChanges(data) {
		return nil, "", nil
	}
	if err := os.MkdirAll(nd.NodelistDir, 0755); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(filepath.Join(nd.NodelistDir, filename), data, 0644); err != nil {
		return nil, "", err
	}
	return data, filename, nil
}

func mostRecentSnapshotDay(db *sql.DB, network string, before time.Time) (year, day int, err error) {
	err = db.QueryRow(`SELECT year, day_of_year FROM fido_members_snapshot
		WHERE network=? AND (year < ? OR (year = ? AND day_of_year < ?))
		ORDER BY year DESC, day_of_year DESC LIMIT 1`,
		network, before.Year(), before.Year(), before.YearDay()).Scan(&year, &day)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	return year, day, err
}

func loadSnapshot(db *sql.DB, network string, year, day int) (map[Addr]snapshotEntry, error) {
	rows, err := db.Query(`SELECT zone, net, node_num, point, bbs_name, sysop_name, location, flags
		FROM fido_members_snapshot WHERE network=? AND year=? AND day_of_year=?`, network, year, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[Addr]snapshotEntry{}
	for rows.Next() {
		var a Addr
		var e snapshotEntry
		if err := rows.Scan(&a.Zone, &a.Net, &a.Node, &a.Point, &e.bbsName, &e.sysopName, &e.location, &e.flags); err != nil {
			return nil, err
		}
		out[a] = e
	}
	return out, rows.Err()
}
