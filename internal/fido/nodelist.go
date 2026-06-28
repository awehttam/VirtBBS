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
//   v0.0.6  2026-06-24  Initial implementation — NODELIST.xxx parser, SQLite store, search
//   v0.6.0  2026-06-26  Phase 0 (VirtAnd/VirtTerm): record a fido_nodelist_versions row
//                        on every successful ImportFile, and add GetNodelistVersion so
//                        clients can detect "has this network's nodelist changed".
// ============================================================================

// Package fido — nodelist.go
//
// Imports and queries FidoNet-compatible nodelists.
//
// Standard FidoNet NODELIST format (FTS-0005):
//   ; comment lines start with semicolon
//   keyword,number,name,location,sysop,phone,baud[,flags...]
//
// Keywords and their meaning:
//   Zone   — sets current zone; address is Zone:Zone/0
//   Region — regional coordinator; treated as a Host within the zone
//   Host   — net host; address is Zone:Net/0
//   Hub    — hub within net; address is Zone:Net/Hub
//   (blank)— regular node; address is Zone:Net/Node
//   Pvt    — private node (unlisted phone)
//   Hold   — node currently held (mail held at host)
//   Down   — node is down
//   Boss   — point boss; address is Zone:Net/Node (host of points)
//
// Names and locations use underscores in place of spaces.
package fido

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// NodeEntry represents one entry from a FidoNet nodelist.
type NodeEntry struct {
	ID       int64  `json:"id"`
	Network  string `json:"network"`
	Zone     int    `json:"zone"`
	Net      int    `json:"net"`
	Node     int    `json:"node"`
	Point    int    `json:"point"`
	Name     string `json:"name"`
	Location string `json:"location"`
	Sysop    string `json:"sysop"`
	Phone    string `json:"phone"`
	Baud     int    `json:"baud"`
	Flags    string `json:"flags"`
	Type     string `json:"type"` // Zone/Host/Hub/Pvt/Hold/Down/Boss/Node
	Active   bool   `json:"active"`
}

// Addr4D returns the 4D address string for this node.
func (n *NodeEntry) Addr4D() string {
	if n.Point != 0 {
		return fmt.Sprintf("%d:%d/%d.%d", n.Zone, n.Net, n.Node, n.Point)
	}
	return fmt.Sprintf("%d:%d/%d", n.Zone, n.Net, n.Node)
}

// NodelistDB wraps the VirtBBS SQLite database for nodelist operations.
type NodelistDB struct {
	db *sql.DB
}

// OpenNodelistDB opens the nodelist store using the shared messages database.
func OpenNodelistDB(db *sql.DB) *NodelistDB {
	return &NodelistDB{db: db}
}

// ─── Import ───────────────────────────────────────────────────────────────────

// ImportResult summarises a nodelist import.
type ImportResult struct {
	Inserted int
	Updated  int
	Skipped  int
	Errors   []string
}

// ImportFile parses one NODELIST file and upserts all entries into the DB.
// network is the logical network name (e.g. "FidoNet", "LovlyNet").
// If path is a directory, the most-recently-modified NODELIST.* file is used.
func ImportFile(db *sql.DB, path, network string) (*ImportResult, error) {
	// Resolve directory → latest NODELIST.xxx file.
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		resolved, err := findLatestNodelist(path)
		if err != nil {
			return nil, err
		}
		path = resolved
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open nodelist %s: %w", path, err)
	}
	defer f.Close()

	result := &ImportResult{}
	ndb := OpenNodelistDB(db)

	// Delete all existing entries for this network before re-importing.
	if _, err := db.Exec(`DELETE FROM fido_nodes WHERE network=?`, network); err != nil {
		return nil, err
	}

	// Parse state.
	var curZone, curNet int
	sc := bufio.NewScanner(f)

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if result != nil {
			_ = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO fido_nodes
		(network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,1)`)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	defer stmt.Close()

	for sc.Scan() {
		line := sc.Text()

		// Skip comments and blank lines.
		if strings.HasPrefix(line, ";") || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, ",")
		if len(fields) < 7 {
			result.Skipped++
			continue
		}

		keyword := strings.TrimSpace(fields[0])
		numStr := strings.TrimSpace(fields[1])
		name := nlDecode(fields[2])
		location := nlDecode(fields[3])
		sysop := nlDecode(fields[4])
		phone := strings.TrimSpace(fields[5])
		baudStr := strings.TrimSpace(fields[6])
		flags := ""
		if len(fields) > 7 {
			flags = strings.Join(fields[7:], ",")
		}

		num, _ := strconv.Atoi(numStr)
		baud, _ := strconv.Atoi(baudStr)

		nodeType := "Node"
		var nodeNum, netNum, zoneNum int
		active := true

		switch strings.ToLower(keyword) {
		case "zone":
			curZone = num
			curNet = num // zone coordinator's net == zone number
			nodeType = "Zone"
			zoneNum, netNum, nodeNum = num, num, 0
		case "region":
			curNet = num // region acts like a host
			nodeType = "Region"
			zoneNum, netNum, nodeNum = curZone, num, 0
		case "host":
			curNet = num
			nodeType = "Host"
			zoneNum, netNum, nodeNum = curZone, num, 0
		case "hub":
			nodeType = "Hub"
			zoneNum, netNum, nodeNum = curZone, curNet, num
		case "pvt":
			nodeType = "Pvt"
			zoneNum, netNum, nodeNum = curZone, curNet, num
		case "hold":
			nodeType = "Hold"
			zoneNum, netNum, nodeNum = curZone, curNet, num
			active = false
		case "down":
			nodeType = "Down"
			zoneNum, netNum, nodeNum = curZone, curNet, num
			active = false
		case "boss":
			nodeType = "Boss"
			zoneNum, netNum, nodeNum = curZone, curNet, num
		case "":
			// Regular node.
			zoneNum, netNum, nodeNum = curZone, curNet, num
		default:
			result.Skipped++
			continue
		}

		activeInt := 0
		if active {
			activeInt = 1
		}

		if _, err := stmt.Exec(network, zoneNum, netNum, nodeNum, 0,
			name, location, sysop, phone, baud, flags, nodeType, activeInt); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%d:%d/%d: %v", zoneNum, netNum, nodeNum, err))
			result.Skipped++
		} else {
			result.Inserted++
		}
	}

	if err := sc.Err(); err != nil {
		return nil, err
	}

	_ = ndb // suppress unused warning

	if err := RecordNodelistVersion(db, network, result.Inserted); err != nil {
		return result, err
	}
	return result, nil
}

// RecordNodelistVersion upserts the fido_nodelist_versions row for network,
// recording the current time and node count. Called by ImportFile after
// every successful import.
func RecordNodelistVersion(db *sql.DB, network string, nodeCount int) error {
	_, err := db.Exec(`
		INSERT INTO fido_nodelist_versions (network, imported_at, node_count)
		VALUES (?,?,?)
		ON CONFLICT(network) DO UPDATE SET imported_at=excluded.imported_at, node_count=excluded.node_count`,
		network, time.Now().Format(time.RFC3339), nodeCount)
	return err
}

// NodelistVersion describes the most recent successful import for a network.
type NodelistVersion struct {
	Network    string `json:"network"`
	ImportedAt string `json:"imported_at"`
	NodeCount  int    `json:"node_count"`
}

// GetNodelistVersion returns the most recent import record for network, or
// nil if the network has never been successfully imported.
func GetNodelistVersion(db *sql.DB, network string) (*NodelistVersion, error) {
	v := &NodelistVersion{Network: network}
	err := db.QueryRow(`SELECT imported_at, node_count FROM fido_nodelist_versions WHERE network=?`, network).
		Scan(&v.ImportedAt, &v.NodeCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

// findLatestNodelist finds the most recently modified NODELIST.* file in dir.
func findLatestNodelist(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var best string
	var bestTime int64
	for _, e := range entries {
		name := strings.ToUpper(e.Name())
		if !strings.HasPrefix(name, "NODELIST") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if fi.ModTime().UnixNano() > bestTime {
			bestTime = fi.ModTime().UnixNano()
			best = filepath.Join(dir, e.Name())
		}
	}
	if best == "" {
		return "", fmt.Errorf("no NODELIST.* file found in %s", dir)
	}
	return best, nil
}

// nlDecode replaces underscores with spaces (FidoNet nodelist convention).
func nlDecode(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "_", " ")
}

// nlEncode replaces spaces with underscores — the inverse of nlDecode, used
// by the nodelist generator (nodelistgen.go) to write a name/location/sysop
// field back out in FTS-0005 form.
func nlEncode(s string) string {
	if s == "" {
		return "-"
	}
	return strings.ReplaceAll(strings.TrimSpace(s), " ", "_")
}

// ─── Query ────────────────────────────────────────────────────────────────────

// SearchResult holds a page of node entries plus total count.
type SearchResult struct {
	Nodes []*NodeEntry `json:"nodes"`
	Total int          `json:"total"`
	Page  int          `json:"page"`
	Pages int          `json:"pages"`
}

// Search returns a page of nodes matching a query string (matched against
// sysop, name, location, or exact address like "1:234/567").
// An empty query or "*" matches all nodes in the network.
// pageSize = 0 defaults to 25.
func (ndb *NodelistDB) Search(network, query string, page, pageSize int) (*SearchResult, error) {
	query = NormalizeSearchQuery(query)
	if pageSize <= 0 {
		pageSize = 25
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	// Try to parse as an address first (non-empty query only).
	if query != "" {
		if a, err := ParseAddr(query); err == nil {
			return ndb.searchByAddr(network, a, page, pageSize, offset)
		}
	}

	like := "%" + query + "%"
	networkCond := ""
	args := []any{like, like, like, like}
	if network != "" {
		networkCond = " AND network=?"
		args = append(args, network)
	}

	countSQL := `SELECT COUNT(*) FROM fido_nodes
		WHERE (sysop LIKE ? OR name LIKE ? OR location LIKE ? OR flags LIKE ?)` + networkCond

	var total int
	if err := ndb.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, err
	}

	querySQL := `SELECT id, network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active
		FROM fido_nodes
		WHERE (sysop LIKE ? OR name LIKE ? OR location LIKE ? OR flags LIKE ?)` +
		networkCond +
		` ORDER BY zone, net, node_num LIMIT ? OFFSET ?`

	limitArgs := append(args, pageSize, offset)
	rows, err := ndb.db.Query(querySQL, limitArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes, err := scanNodes(rows)
	if err != nil {
		return nil, err
	}

	pages := (total + pageSize - 1) / pageSize
	return &SearchResult{Nodes: nodes, Total: total, Page: page, Pages: pages}, nil
}

// NormalizeSearchQuery treats blank and "*" as match-all.
func NormalizeSearchQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "*" {
		return ""
	}
	return query
}

// SearchAll returns every node matching query (no pagination).
func (ndb *NodelistDB) SearchAll(network, query string) ([]NodeEntry, error) {
	query = NormalizeSearchQuery(query)
	if query != "" {
		if a, err := ParseAddr(query); err == nil {
			e, err := ndb.LookupAddr(network, a)
			if err != nil {
				return nil, err
			}
			if e == nil {
				return nil, nil
			}
			return []NodeEntry{*e}, nil
		}
	}
	like := "%" + query + "%"
	networkCond := ""
	args := []any{like, like, like, like}
	if network != "" {
		networkCond = " AND network=?"
		args = append(args, network)
	}
	querySQL := `SELECT id, network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active
		FROM fido_nodes
		WHERE (sysop LIKE ? OR name LIKE ? OR location LIKE ? OR flags LIKE ?)` +
		networkCond +
		` ORDER BY zone, net, node_num`
	rows, err := ndb.db.Query(querySQL, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ptrs, err := scanNodes(rows)
	if err != nil {
		return nil, err
	}
	out := make([]NodeEntry, len(ptrs))
	for i, p := range ptrs {
		out[i] = *p
	}
	return out, nil
}

func (ndb *NodelistDB) searchByAddr(network string, a Addr, page, pageSize, offset int) (*SearchResult, error) {
	args := []any{a.Zone, a.Net, a.Node, a.Point}
	netCond := ""
	if network != "" {
		netCond = " AND network=?"
		args = append(args, network)
	}
	countSQL := `SELECT COUNT(*) FROM fido_nodes WHERE zone=? AND net=? AND node_num=? AND point=?` + netCond
	var total int
	if err := ndb.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, err
	}

	querySQL := `SELECT id, network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active
		FROM fido_nodes WHERE zone=? AND net=? AND node_num=? AND point=?` + netCond +
		` ORDER BY zone, net, node_num LIMIT ? OFFSET ?`
	limitArgs := append(args, pageSize, offset)
	rows, err := ndb.db.Query(querySQL, limitArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes, err := scanNodes(rows)
	if err != nil {
		return nil, err
	}
	pages := (total + pageSize - 1) / pageSize
	return &SearchResult{Nodes: nodes, Total: total, Page: page, Pages: pages}, nil
}

// LookupAddr returns the NodeEntry for a specific address, or nil if not found.
func (ndb *NodelistDB) LookupAddr(network string, a Addr) (*NodeEntry, error) {
	args := []any{a.Zone, a.Net, a.Node, a.Point}
	netCond := ""
	if network != "" {
		netCond = " AND network=?"
		args = append(args, network)
	}
	row := ndb.db.QueryRow(`SELECT id, network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active
		FROM fido_nodes WHERE zone=? AND net=? AND node_num=? AND point=?`+netCond, args...)
	nodes, err := scanNodes(singleRow{row})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}
	return nodes[0], nil
}

// LookupHub returns the hub (or Host) for a given zone:net.
// Used for routing: if no hub is known, returns the host (node 0).
func (ndb *NodelistDB) LookupHub(network string, zone, net int) (*NodeEntry, error) {
	row := ndb.db.QueryRow(`SELECT id, network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active
		FROM fido_nodes WHERE network=? AND zone=? AND net=? AND node_type IN ('Hub','Host') AND is_active=1
		ORDER BY CASE node_type WHEN 'Host' THEN 1 ELSE 0 END LIMIT 1`,
		network, zone, net)
	nodes, err := scanNodes(singleRow{row})
	if err != nil || len(nodes) == 0 {
		return nil, err
	}
	return nodes[0], nil
}

// LookupZoneGate returns the zone coordinator for a given zone (Zone:Zone/0).
func (ndb *NodelistDB) LookupZoneGate(network string, zone int) (*NodeEntry, error) {
	row := ndb.db.QueryRow(`SELECT id, network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active
		FROM fido_nodes WHERE network=? AND zone=? AND node_type='Zone' LIMIT 1`, network, zone)
	nodes, err := scanNodes(singleRow{row})
	if err != nil || len(nodes) == 0 {
		return nil, err
	}
	return nodes[0], nil
}

// Count returns total nodes in the nodelist for a network.
func (ndb *NodelistDB) Count(network string) (int, error) {
	var n int
	err := ndb.db.QueryRow(`SELECT COUNT(*) FROM fido_nodes WHERE network=?`, network).Scan(&n)
	return n, err
}

// ListAll returns every node for a network, sorted by address.
func (ndb *NodelistDB) ListAll(network string) ([]NodeEntry, error) {
	rows, err := ndb.db.Query(`SELECT id, network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active
		FROM fido_nodes WHERE network=? ORDER BY zone, net, node_num, point`, network)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ptrs, err := scanNodes(rows)
	if err != nil {
		return nil, err
	}
	out := make([]NodeEntry, len(ptrs))
	for i, p := range ptrs {
		out[i] = *p
	}
	return out, nil
}

// DeleteAddr removes one node from the local nodelist.
func (ndb *NodelistDB) DeleteAddr(network string, a Addr) error {
	_, err := ndb.db.Exec(`DELETE FROM fido_nodes WHERE network=? AND zone=? AND net=? AND node_num=? AND point=?`,
		network, a.Zone, a.Net, a.Node, a.Point)
	return err
}

// singleRow adapts *sql.Row to the Scan interface used by scanNodes.
type singleRow struct{ *sql.Row }

// scanner interface for both *sql.Rows and singleRow.
type rowScanner interface {
	Scan(...any) error
}

func scanNodes(rows interface{}) ([]*NodeEntry, error) {
	// singleRow must be handled before the rowIter branch — singleRow used
	// to define Next() which accidentally made it satisfy rowIter, causing
	// a second Scan on an already-consumed *sql.Row (ErrNoRows).
	if sr, ok := rows.(singleRow); ok {
		n, err := scanOneNode(sr)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return []*NodeEntry{n}, nil
	}

	type rowIter interface {
		Next() bool
		Scan(...any) error
		Err() error
	}

	if ri, ok := rows.(rowIter); ok {
		var out []*NodeEntry
		for ri.Next() {
			n, err := scanOneNode(ri)
			if err != nil {
				return out, err
			}
			out = append(out, n)
		}
		return out, ri.Err()
	}

	return nil, fmt.Errorf("unsupported rows type")
}

func scanOneNode(sc rowScanner) (*NodeEntry, error) {
	n := &NodeEntry{}
	var active int
	err := sc.Scan(&n.ID, &n.Network, &n.Zone, &n.Net, &n.Node, &n.Point,
		&n.Name, &n.Location, &n.Sysop, &n.Phone, &n.Baud, &n.Flags,
		&n.Type, &active)
	if err != nil {
		return nil, err
	}
	n.Active = active != 0
	return n, nil
}
