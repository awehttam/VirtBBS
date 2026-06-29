package fido

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ApplyNodelistDiffFile applies a text NODEDIFF to fido_nodes for network.
// Added/changed lines are upserted; lines beginning with "-" remove an address.
func ApplyNodelistDiffFile(db *sql.DB, network, path string, nd *NetworkDef) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return ApplyNodelistDiff(db, network, bufio.NewScanner(f), nd)
}

// ApplyNodelistDiff applies diff content from sc to fido_nodes for network.
func ApplyNodelistDiff(db *sql.DB, network string, sc *bufio.Scanner, nd *NetworkDef) error {
	ndb := OpenNodelistDB(db)
	var curZone, curNet int
	if nd != nil {
		if a := nd.NodeAddr(); a != (Addr{}) {
			curZone, curNet = a.Zone, a.Net
		}
	}
	if curZone == 0 {
		if nodes, err := ndb.ListAll(network); err == nil && len(nodes) > 0 {
			curZone, curNet = nodes[0].Zone, nodes[0].Net
		}
	}

	changed := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "-") {
			addr, err := ParseAddr(strings.TrimPrefix(line, "-"))
			if err != nil {
				continue
			}
			if err := ndb.DeleteAddr(network, addr); err != nil {
				return err
			}
			changed++
			continue
		}

		entry, err := parseNodelistLine(line, network, &curZone, &curNet)
		if err != nil || entry == nil {
			continue
		}
		if err := ndb.UpsertLocalNode(entry); err != nil {
			return err
		}
		changed++
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if changed == 0 {
		return nil
	}
	count, err := ndb.Count(network)
	if err != nil {
		return err
	}
	return RecordNodelistVersion(db, network, count)
}

// parseNodelistLine parses one FTS nodelist line into a NodeEntry.
func parseNodelistLine(line, network string, curZone, curNet *int) (*NodeEntry, error) {
	fields := strings.Split(line, ",")
	if len(fields) < 7 {
		return nil, fmt.Errorf("short line")
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
		*curZone = num
		*curNet = num
		nodeType = "Zone"
		zoneNum, netNum, nodeNum = num, num, 0
	case "region":
		*curNet = num
		nodeType = "Region"
		zoneNum, netNum, nodeNum = *curZone, num, 0
	case "host":
		*curNet = num
		nodeType = "Host"
		zoneNum, netNum, nodeNum = *curZone, num, 0
	case "hub":
		nodeType = "Hub"
		zoneNum, netNum, nodeNum = *curZone, *curNet, num
	case "pvt":
		nodeType = "Pvt"
		zoneNum, netNum, nodeNum = *curZone, *curNet, num
	case "hold":
		nodeType = "Hold"
		zoneNum, netNum, nodeNum = *curZone, *curNet, num
		active = false
	case "down":
		nodeType = "Down"
		zoneNum, netNum, nodeNum = *curZone, *curNet, num
		active = false
	case "boss":
		nodeType = "Boss"
		zoneNum, netNum, nodeNum = *curZone, *curNet, num
	case "":
		zoneNum, netNum, nodeNum = *curZone, *curNet, num
	default:
		return nil, fmt.Errorf("unknown keyword %q", keyword)
	}

	return &NodeEntry{
		Network: network, Zone: zoneNum, Net: netNum, Node: nodeNum,
		Name: name, Location: location, Sysop: sysop,
		Phone: phone, Baud: baud, Flags: flags, Type: nodeType, Active: active,
	}, nil
}

func recordNodelistApplied(db *sql.DB, network, source, sourceKey, filename string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO fido_nodelist_applied
		(network, source, source_key, filename, applied_at) VALUES (?,?,?,?,?)`,
		network, source, sourceKey, filename, time.Now().Format(time.RFC3339))
	return err
}

func nodelistAlreadyApplied(db *sql.DB, network, source, sourceKey string) bool {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM fido_nodelist_applied
		WHERE network=? AND source=? AND source_key=?`, network, source, sourceKey).Scan(&n)
	return err == nil && n > 0
}

// nodelistCandidateIsNewer reports whether refTime is after the last import.
func nodelistCandidateIsNewer(db *sql.DB, network string, refTime time.Time, zSuffix int) bool {
	v, err := GetNodelistVersion(db, network)
	if err != nil || v == nil {
		return true
	}
	importedAt, err := time.Parse(time.RFC3339, v.ImportedAt)
	if err != nil {
		return true
	}
	if refTime.After(importedAt) {
		return true
	}
	if refTime.IsZero() && zSuffix >= 0 {
		return true
	}
	return false
}

func skipOwnHubNodelist(nd *NetworkDef, uploader, fromName string) bool {
	if nd == nil || !nd.UsesMemberNodelist() {
		return false
	}
	if fromName == "VirtBBS NodeAnnounce" {
		return true
	}
	return uploader == "VirtBBS"
}
