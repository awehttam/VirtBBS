// Package fido — nodelistlocal.go
//
// Updates this BBS's own entry in the local nodelist DB, generates a
// single-node NODEDIFF, and queues netmail to the uplink.
package fido

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NodeFlagsUpdateResult summarises a node-flags save operation.
type NodeFlagsUpdateResult struct {
	Address      string `json:"address"`
	Flags        string `json:"flags"`
	NodediffFile string `json:"nodediff_file"`
	NetmailSent  bool   `json:"netmail_sent"`
	NetmailTo    string `json:"netmail_to,omitempty"`
	PktPath      string `json:"pkt_path,omitempty"`
	Message      string `json:"message,omitempty"`
}

// UpdateNodeFlags validates flags, updates the local nodelist entry for this
// node, writes a NODEDIFF file, and sends netmail to the uplink when one is
// configured.
func UpdateNodeFlags(db *sql.DB, nd *NetworkDef, bbsName, sysopName, location string,
	telnetPort int, flags []string, binkpHost string) (*NodeFlagsUpdateResult, error) {

	validated, err := ValidateNodeFlags(flags)
	if err != nil {
		return nil, err
	}

	our := nd.NodeAddr()
	if our == (Addr{}) {
		return nil, fmt.Errorf("invalid network address %q", nd.Address)
	}
	if bbsName == "" {
		bbsName = "VirtBBS"
	}
	if sysopName == "" {
		sysopName = "Sysop"
	}
	if location == "" {
		location = "Internet"
	}

	flagsStr := BuildNodelistFlags(validated, binkpHost, nd.Port(), telnetPort)

	ndb := OpenNodelistDB(db)
	old, err := ndb.LookupAddr(nd.Name, our)
	if err != nil {
		return nil, err
	}

	newEntry := &NodeEntry{
		Network:  nd.Name,
		Zone:     our.Zone,
		Net:      our.Net,
		Node:     our.Node,
		Point:    our.Point,
		Name:     bbsName,
		Location: location,
		Sysop:    sysopName,
		Phone:    "-Unpublished-",
		Baud:     33600,
		Flags:    flagsStr,
		Type:     "Node",
		Active:   true,
	}

	if err := ndb.UpsertLocalNode(newEntry); err != nil {
		return nil, fmt.Errorf("update local nodelist: %w", err)
	}

	diffBody, filename, err := writeSingleNodeNodediff(nd, our, old, newEntry)
	if err != nil {
		return nil, err
	}

	result := &NodeFlagsUpdateResult{
		Address:      our.String(),
		Flags:        flagsStr,
		NodediffFile: filename,
	}

	pktPath, sentTo, sendErr := sendNodediffNetmail(nd, our, diffBody, filename)
	if sendErr != nil {
		result.Message = "nodelist updated; netmail failed: " + sendErr.Error()
	} else if sentTo != "" {
		result.NetmailSent = true
		result.NetmailTo = sentTo
		result.PktPath = pktPath
		result.Message = fmt.Sprintf("nodelist updated; NODEDIFF sent to %s", sentTo)
	} else {
		result.Message = "nodelist updated; no uplink configured — NODEDIFF saved locally only"
	}

	// VirtNet hub members table mirrors BinkP host for routing.
	if member, merr := OpenMembersDB(db).GetMemberByAddr(nd.Name, our); merr == nil && member != nil {
		member.BinkpHost = binkpHost
		_ = OpenMembersDB(db).UpdateMemberInfo(member)
	}

	return result, nil
}

// UpsertLocalNode inserts or replaces one fido_nodes row for this BBS.
func (ndb *NodelistDB) UpsertLocalNode(e *NodeEntry) error {
	active := 0
	if e.Active {
		active = 1
	}
	_, err := ndb.db.Exec(`INSERT INTO fido_nodes
		(network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(network, zone, net, node_num, point) DO UPDATE SET
			name=excluded.name, location=excluded.location, sysop=excluded.sysop,
			phone=excluded.phone, baud=excluded.baud, flags=excluded.flags,
			node_type=excluded.node_type, is_active=excluded.is_active`,
		e.Network, e.Zone, e.Net, e.Node, e.Point,
		e.Name, e.Location, e.Sysop, e.Phone, e.Baud, e.Flags, e.Type, active)
	return err
}

func writeSingleNodeNodediff(nd *NetworkDef, our Addr, old, new *NodeEntry) ([]byte, string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, ";NODEDIFF for %s, generated %s\r\n", our.String(), time.Now().Format(time.RFC3339))
	if old != nil && nodeEntryChanged(old, new) {
		fmt.Fprintf(&b, "; previous flags: %s\r\n", old.Flags)
	}
	fmt.Fprintf(&b, ",%d,%s,%s,%s,%s,%d,%s\r\n",
		our.Node,
		nlEncode(new.Name), nlEncode(new.Location), nlEncode(new.Sysop),
		new.Phone, new.Baud, new.Flags)

	filename := NodelistDiffFilename(time.Now())
	data := []byte(b.String())
	if nd.NodelistDir != "" {
		if err := os.MkdirAll(nd.NodelistDir, 0755); err != nil {
			return data, filename, err
		}
		if err := os.WriteFile(filepath.Join(nd.NodelistDir, filename), data, 0644); err != nil {
			return data, filename, err
		}
	}
	return data, filename, nil
}

func nodeEntryChanged(old, new *NodeEntry) bool {
	return old.Name != new.Name || old.Location != new.Location ||
		old.Sysop != new.Sysop || old.Flags != new.Flags
}

func sendNodediffNetmail(nd *NetworkDef, our Addr, diffBody []byte, filename string) (pktPath, toAddr string, err error) {
	uplink := nd.UplinkAddr()
	if uplink == (Addr{}) {
		return "", "", nil
	}

	subject := fmt.Sprintf("NODEDIFF for %s", our.String())
	body := fmt.Sprintf("Attached: %s\r\n\r\n%s", filename, string(diffBody))

	msg := &NetmailMsg{
		FromName: "VirtBBS",
		FromAddr: our.String(),
		ToName:   "Nodelist",
		ToAddr:   uplink.String(),
		Subject:  subject,
		Body:     body,
		Network:  nd.Name,
	}

	outDir := OutboundDir(nd.OutboundDir, uplink, uplink, false)
	pktPath, err = WritePKT(our, uplink, nd.Password, outDir, []*NetmailMsg{msg}, nd.Name)
	if err != nil {
		return "", uplink.String(), err
	}
	return pktPath, uplink.String(), nil
}
