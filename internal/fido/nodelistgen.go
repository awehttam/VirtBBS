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
// ============================================================================

// Package fido — nodelistgen.go
//
// Generates this BBS's own outbound nodelist for a network it hosts
// (NetworkDef.IsHub()), sourced from fido_members rather than a parsed
// file. Filename convention: "VirtNode.Z045" (full) / "VirtNode.D045"
// (diff), day-of-year, mirroring FidoNet's own NODELIST.045/NODEDIFF.045.
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

// GenerateNodelist builds the full nodelist for nd (a hub network) from
// fido_members, writes it to nd.NodelistDir, and returns its bytes and
// filename. hubBBSName/hubSysopName are used for net 1's implicit Host
// line when no member is explicitly marked is_host for that net — net 1's
// host is this BBS itself (config.Config isn't importable here, see
// members.go's saveDownlink comment for why; the caller supplies these
// two strings instead).
func GenerateNodelist(db *sql.DB, nd *NetworkDef, hubBBSName, hubSysopName string) ([]byte, string, error) {
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return nil, "", fmt.Errorf("invalid network address %q", nd.Address)
	}
	mdb := OpenMembersDB(db)
	members, err := mdb.ListMembers(nd.Name)
	if err != nil {
		return nil, "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, ";VirtNet nodelist for %q, generated %s\r\n", nd.Name, time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "Zone,%d,%s,%s,%s,-Unpublished-,33600,CM\r\n",
		our.Zone, nlEncode(hubBBSName), nlEncode("Internet"), nlEncode(hubSysopName))

	byNet := groupByNet(members)
	nets := sortedNets(byNet)
	for _, net := range nets {
		netMembers := byNet[net]
		host := findHost(netMembers)
		if host == nil && net == our.Net {
			// Net 1 (the hub's own net) has an implicit Host: VirtBBS itself.
			writeNodelistLine(&b, "Host", net, hubBBSName, "Internet", hubSysopName, "")
		} else if host != nil {
			writeMemberLine(&b, "Host", host)
		}
		for _, m := range netMembers {
			if host != nil && m.ID == host.ID {
				continue
			}
			writeMemberLine(&b, "", m)
		}
	}

	filename := fmt.Sprintf("VirtNode.Z%03d", time.Now().YearDay())
	data := []byte(b.String())
	if err := os.MkdirAll(nd.NodelistDir, 0755); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(filepath.Join(nd.NodelistDir, filename), data, 0644); err != nil {
		return nil, "", err
	}
	return data, filename, nil
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

func writeMemberLine(b *strings.Builder, keyword string, m *Member) {
	flags := ""
	if m.BinkpHost != "" {
		flags = ",IBN:" + m.BinkpHost
	}
	writeNodelistLine(b, keyword, m.NodeNum, m.BBSName, m.Location, m.SysopName, flags)
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
	zone, net, node, point        int
	bbsName, sysopName, location  string
	flags                          string
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
	fmt.Fprintf(&b, ";VirtNet nodelist diff for %q, generated %s\r\n", nd.Name, time.Now().Format(time.RFC3339))

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

	filename := fmt.Sprintf("VirtNode.D%03d", time.Now().YearDay())
	data := []byte(b.String())
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
