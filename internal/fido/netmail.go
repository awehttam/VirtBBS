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
//   v0.0.6  2026-06-24  Initial implementation — netmail compose, route, PKT write
//   v0.1.0  2026-06-25  Add TZUTC kludge (FTS-4001) to outbound netmail
// ============================================================================

// Package fido — netmail.go
//
// Composes and routes FidoNet netmail (personal messages between nodes).
//
// Routing rules (zone-aware):
//   Crash flag  → write PKT directly to outbound/<destAddr>/  (direct delivery)
//   Same zone   → route via uplink (uplink handles local delivery)
//   Other zone  → route via uplink (uplink contacts zone gate)
//   Point addr  → strip point, deliver to boss node (zone:net/node)
//
// PKT format: FTS-0001 Type-2 packet
//   58-byte packet header
//   N × message records (null-terminated fields)
//   0x0000 end-of-packet marker
package fido

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NetmailMsg holds the fields for one outbound netmail message.
type NetmailMsg struct {
	FromName string `json:"FromName"`
	FromAddr string `json:"FromAddr"`
	ToName   string `json:"ToName"`
	ToAddr   string `json:"ToAddr"`
	Subject  string `json:"Subject"`
	Body     string `json:"Body"`

	MsgID      string `json:"MsgID,omitempty"`
	ReplyMsgID string `json:"ReplyMsgID,omitempty"`

	Crash   bool   `json:"Crash"`
	Network string `json:"Network"`

	// AuthorLang is the origin user's ISO 639-1 code (^ALANG kludge).
	AuthorLang string `json:"AuthorLang,omitempty"`
}

// NetmailDB wraps the database for netmail queue operations.
type NetmailDB struct{ db *sql.DB }

// OpenNetmailDB returns a NetmailDB using the shared database connection.
func OpenNetmailDB(db *sql.DB) *NetmailDB { return &NetmailDB{db: db} }

// Enqueue stores a netmail in the queue for the next poll cycle.
func (ndb *NetmailDB) Enqueue(m *NetmailMsg) (int64, error) {
	crash := 0
	if m.Crash {
		crash = 1
	}
	res, err := ndb.db.Exec(`INSERT INTO fido_netmail
		(from_name, from_addr, to_name, to_addr, subject, body, crash, network, author_lang)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		m.FromName, m.FromAddr, m.ToName, m.ToAddr,
		m.Subject, m.Body, crash, m.Network, NormalizeLangCode(m.AuthorLang))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Pending returns all unsent netmails.
func (ndb *NetmailDB) Pending() ([]*NetmailMsg, []int64, error) {
	rows, err := ndb.db.Query(`SELECT id, from_name, from_addr, to_name, to_addr, subject, body, crash, network, author_lang
		FROM fido_netmail WHERE sent_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var msgs []*NetmailMsg
	var ids []int64
	for rows.Next() {
		m := &NetmailMsg{}
		var id int64
		var crash int
		if err := rows.Scan(&id, &m.FromName, &m.FromAddr, &m.ToName, &m.ToAddr,
			&m.Subject, &m.Body, &crash, &m.Network, &m.AuthorLang); err != nil {
			return nil, nil, err
		}
		m.Crash = crash != 0
		msgs = append(msgs, m)
		ids = append(ids, id)
	}
	return msgs, ids, rows.Err()
}

// MarkSent marks a queued netmail as sent.
func (ndb *NetmailDB) MarkSent(id int64) error {
	_, err := ndb.db.Exec(`UPDATE fido_netmail SET sent_at=datetime('now') WHERE id=?`, id)
	return err
}

// ScanNetmailResult summarises flushing the fido_netmail queue to outbound PKTs.
type ScanNetmailResult struct {
	Exported int
	Errors   []string
}

// ScanNetmailQueue writes pending netmail for nd to outbound .PKT files so the
// next BinkP poll can send them. Web and admin compose enqueue rows instead of
// writing PKTs immediately (unlike the terminal compose path).
func ScanNetmailQueue(nd *NetworkDef, db *sql.DB) *ScanNetmailResult {
	result := &ScanNetmailResult{}
	if nd == nil || db == nil {
		return result
	}
	ndb := OpenNetmailDB(db)
	msgs, ids, err := ndb.Pending()
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	origAddr := nd.NodeAddr()
	if origAddr == (Addr{}) {
		result.Errors = append(result.Errors, fmt.Sprintf("invalid node address %q", nd.Address))
		return result
	}
	uplink := nd.UplinkAddr()
	for i, m := range msgs {
		if m.Network != nd.Name {
			continue
		}
		nextHop, err := RouteAddr(db, m, nd)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("id %d: %v", ids[i], err))
			continue
		}
		outDir := OutboundDir(nd.OutboundDir, nextHop, uplink, m.Crash)
		if _, err := WritePKT(origAddr, nextHop, nd.Password, outDir, []*NetmailMsg{m}, nd.Name); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("id %d: %v", ids[i], err))
			continue
		}
		if err := ndb.MarkSent(ids[i]); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("id %d mark sent: %v", ids[i], err))
			continue
		}
		result.Exported++
	}
	return result
}

// ─── PKT writer ──────────────────────────────────────────────────────────────

// WritePKT writes a single FTS-0001 Type-2 PKT file containing the given
// messages.  Returns the path of the created file.
//
//   origAddr  — address of the sending system (us)
//   destAddr  — address of the next-hop system (uplink or direct dest)
//   password  — session password for the PKT header
//   outDir    — directory to write the .pkt file into (created if absent)
func WritePKT(origAddr, destAddr Addr, password, outDir string, msgs []*NetmailMsg, network ...string) (string, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", err
	}

	fname := fmt.Sprintf("%08X.pkt", time.Now().UnixNano()&0xFFFFFFFF)
	path := filepath.Join(outDir, fname)

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Convert netmail messages to the shared Message type and write them
	// using the same FTS-0001 record layout as echomail (WritePacket).
	pktMsgs := make([]*Message, 0, len(msgs))
	for _, m := range msgs {
		from, _ := ParseAddr(m.FromAddr)
		to, _ := ParseAddr(m.ToAddr)

		// Attribute: Private (0x0002) always set for netmail; Crash (0x0100) if flagged.
		attr := uint16(AttribPrivate)
		if m.Crash {
			attr |= AttribCrash
		}

		pktMsgs = append(pktMsgs, &Message{
			OrigAddr: from,
			DestAddr: to,
			DateTime: time.Now().Format("02 Jan 06  15:04:05"),
			ToName:   m.ToName,
			FromName: m.FromName,
			Subject:  m.Subject,
			Body:     buildBody(m, from, to, origAddr.Zone),
			Attrib:   attr,
		})
	}

	if err := WritePacket(f, origAddr, destAddr, password, pktMsgs); err != nil {
		return "", err
	}
	if len(network) > 0 && network[0] != "" && len(msgs) > 0 {
		RecordNetmailSent(network[0], destAddr.String(), len(msgs))
	}

	return path, nil
}

// buildBody prepends FTS KLUDGE lines and MSGID to the body.
func buildBody(m *NetmailMsg, from, to Addr, localZone int) string {
	var sb strings.Builder

	msgID := m.MsgID
	if msgID == "" {
		msgID = FormatMSGID(from, NewMSGIDSerial())
	}
	fmt.Fprintf(&sb, "\x01MSGID: %s\r\n", msgID)
	if m.ReplyMsgID != "" {
		fmt.Fprintf(&sb, "\x01REPLY: %s\r\n", m.ReplyMsgID)
	}

	// TZUTC kludge (FTS-4001): local UTC offset at composition time, e.g. "+0200".
	sb.WriteString(fmt.Sprintf("\x01TZUTC: %s\r\n", time.Now().Format("-0700")))

	// ^ALANG: origin author's UI language (VirtBBS experimental kludge).
	sb.WriteString(LangKludgeLine(m.AuthorLang))
	sb.WriteByte('\r')
	sb.WriteByte('\n')

	// INTL kludge: required when source/dest zones differ or either is non-local.
	if from.Zone != to.Zone || from.Zone != localZone {
		intl := fmt.Sprintf("\x01INTL %d:%d/%d %d:%d/%d\r\n",
			to.Zone, to.Net, to.Node,
			from.Zone, from.Net, from.Node)
		sb.WriteString(intl)
	}

	// FMPT kludge for point source.
	if from.Point != 0 {
		sb.WriteString(fmt.Sprintf("\x01FMPT %d\r\n", from.Point))
	}
	// TOPT kludge for point destination.
	if to.Point != 0 {
		sb.WriteString(fmt.Sprintf("\x01TOPT %d\r\n", to.Point))
	}

	sb.WriteString(m.Body)
	return sb.String()
}

// ─── Routing helper ─────────────────────────────────────────────────────────

// RouteAddr returns the next-hop address for a netmail message:
//   - Crash: direct to destination (strip point → boss node)
//   - A direct, configured Downlink: deliver straight to them
//   - Otherwise, an indirect destination: consult the ROUTES.BBS-style
//     routing table (routes.go) for a next-hop — e.g. a node behind a
//     delegated sub-hub gets physically handed to that sub-hub
//   - No route found: fall back to the uplink, as before
//   - Point: stripped from the destination address throughout
//
// db may be nil (skips the routing-table lookup, falling straight to the
// uplink) — kept optional so any future caller without a database handle
// degrades to the pre-routing-table behavior rather than panicking.
func RouteAddr(db *sql.DB, m *NetmailMsg, nd *NetworkDef) (Addr, error) {
	dest, err := ParseAddr(m.ToAddr)
	if err != nil {
		return Addr{}, fmt.Errorf("invalid destination address %q: %w", m.ToAddr, err)
	}

	if m.Crash {
		// Crash netmail: go directly to the boss (strip point).
		boss := Addr{Zone: dest.Zone, Net: dest.Net, Node: dest.Node}
		return boss, nil
	}

	if dl := nd.DownlinkByAddr(dest); dl != nil {
		// Already a direct, known member — no need to route further.
		return Addr{Zone: dest.Zone, Net: dest.Net, Node: dest.Node}, nil
	}

	if db != nil {
		if hop, ok, err := RouteFor(db, nd.Name, dest); err == nil && ok {
			return hop, nil
		}
	}

	// No direct match, no route: fall back to the uplink (unchanged
	// pre-routing-table behavior).
	uplink := nd.UplinkAddr()
	if uplink.Zone == 0 {
		return Addr{}, fmt.Errorf("no uplink configured for network %s", nd.Name)
	}
	return uplink, nil
}

// OutboundDir returns the per-next-hop outbound subdirectory whenever the
// message must go directly to a specific peer rather than the generic
// uplink-bound pool — true for crash netmail, and now also true whenever
// RouteAddr resolved an indirect next-hop that isn't the uplink (e.g. a
// node behind a delegated sub-hub gets handed to that sub-hub specifically,
// not lumped in with everything else destined for the uplink). Otherwise
// returns the general outbound dir.
func OutboundDir(baseOutbound string, nextHop, uplink Addr, crash bool) string {
	indirect := uplink != (Addr{}) && nextHop != uplink
	if crash || indirect {
		sub := fmt.Sprintf("%04X%04X.OUT", nextHop.Zone*0x100+nextHop.Net, nextHop.Node)
		return filepath.Join(baseOutbound, sub)
	}
	return baseOutbound
}
