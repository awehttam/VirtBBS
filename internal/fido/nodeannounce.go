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
//   v0.13.0 2026-06-27  VirtNet: delegated sub-nets — a downstream member
//                        running its own VirtBBS as a sub-hub announces
//                        every node it registers under its own net to
//                        SysOp@<central hub>, so the central hub's nodelist
//                        stays authoritative even for nodes it never
//                        directly approved. Mirrors the existing AreaFix/
//                        FileFix ToName-dispatch convention and the Ping/
//                        Trace outbound NetmailMsg+WritePKT composition.
// ============================================================================

// Package fido — nodeannounce.go
package fido

import (
	"bufio"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/messages"
)

// NodeAnnounceToName is the ToName a NodeAnnounce netmail is addressed to,
// mirroring AreaFix/FileFix's ToName-based recognition convention.
const NodeAnnounceToName = "SysOp"

// NodeAnnounceSubject is the fixed subject line toss.go matches on.
const NodeAnnounceSubject = "NODE ANNOUNCE"

// IsNodeAnnounceRequest reports whether subject is a NodeAnnounce netmail.
func IsNodeAnnounceRequest(subject string) bool {
	return strings.EqualFold(strings.TrimSpace(subject), NodeAnnounceSubject)
}

// SendNodeAnnounce composes and queues a NODE ANNOUNCE netmail to
// SysOp@nd.Uplink, carrying m's full current info. changeType is "NEW" or
// "CHANGE" — informational only (used for §10/§11's wording), since the
// receiving ProcessNodeAnnounce always upserts regardless of what's stated.
// Only meaningful when nd.Uplink != "" — the real central hub never calls
// this (nothing to announce upward from the top).
func SendNodeAnnounce(nd *NetworkDef, m *Member, changeType string) error {
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return fmt.Errorf("invalid local address %q", nd.Address)
	}
	uplink := nd.UplinkAddr()
	if uplink == (Addr{}) {
		return fmt.Errorf("no uplink configured")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "TYPE: %s\r\n", changeType)
	fmt.Fprintf(&b, "ADDR: %s\r\n", m.Addr4D())
	fmt.Fprintf(&b, "BBS: %s\r\n", m.BBSName)
	fmt.Fprintf(&b, "SYSOP: %s\r\n", m.SysopName)
	fmt.Fprintf(&b, "LOCATION: %s\r\n", m.Location)
	fmt.Fprintf(&b, "CONTACT: %s\r\n", m.Contact)
	if m.BinkpHost != "" {
		fmt.Fprintf(&b, "IBN: %s\r\n", m.BinkpHost)
	}

	msg := &NetmailMsg{
		FromName: "VirtBBS NodeAnnounce",
		FromAddr: our.String(),
		ToName:   NodeAnnounceToName,
		ToAddr:   uplink.String(),
		Subject:  NodeAnnounceSubject,
		Body:     b.String(),
		Network:  nd.Name,
	}
	outDir := OutboundDir(nd.OutboundDir, uplink, false)
	_, err := WritePKT(our, uplink, nd.Password, outDir, []*NetmailMsg{msg})
	return err
}

// nodeAnnounceFields parses a NODE ANNOUNCE body's "KEY: value" lines.
func nodeAnnounceFields(body string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(line[:idx]))
		val := strings.TrimSpace(line[idx+1:])
		out[key] = val
	}
	return out
}

// ProcessNodeAnnounce parses pm's body and upserts a fido_members row with
// is_delegated=1, then posts the §10 welcome/change echomail. Called both
// for genuinely inbound netmails (toss.go) and, for symmetry, from purely
// local approval/edit flows at the hub itself (with isDelegated=false in
// that case — see ApplyNodeAnnounceInfo).
func ProcessNodeAnnounce(nd *NetworkDef, db *sql.DB, confStore *conferences.Store, msgStore *messages.Store, pm *Message) error {
	fields := nodeAnnounceFields(pm.Body)
	addr, err := ParseAddr(fields["ADDR"])
	if err != nil {
		return fmt.Errorf("node announce: invalid ADDR %q: %w", fields["ADDR"], err)
	}
	changeType := fields["TYPE"]
	if changeType == "" {
		changeType = "CHANGE"
	}

	m := &Member{
		Network:     nd.Name,
		Zone:        addr.Zone,
		Net:         addr.Net,
		NodeNum:     addr.Node,
		Point:       addr.Point,
		BBSName:     fields["BBS"],
		SysopName:   fields["SYSOP"],
		Location:    fields["LOCATION"],
		Contact:     fields["CONTACT"],
		BinkpHost:   fields["IBN"],
		IsActive:    true,
		IsDelegated: true,
	}
	return ApplyNodeAnnounceInfo(nd, db, confStore, msgStore, m, changeType)
}

// ApplyNodeAnnounceInfo upserts m (keyed by address) and posts the §10
// welcome/change echomail into "<NetworkName> Sysops" — the shared logic
// behind both inbound NodeAnnounce processing and local approval/edit
// flows, so both paths produce identical announcements.
func ApplyNodeAnnounceInfo(nd *NetworkDef, db *sql.DB, confStore *conferences.Store, msgStore *messages.Store, m *Member, changeType string) error {
	mdb := OpenMembersDB(db)
	saved, isNew, err := mdb.UpsertMember(m)
	if err != nil {
		return err
	}
	if isNew {
		changeType = "NEW"
		if err := OpenAreaFixDB(db).Subscribe(nd.Name, saved.Addr4D(), nd.EffectiveNodelistEchoTag()); err != nil {
			return fmt.Errorf("member created but failed to subscribe to nodelist updates: %w", err)
		}
	}

	sysopsConf, err := EnsureConference(confStore, nd.Name+" Sysops", nd.Name)
	if err != nil {
		return fmt.Errorf("node announce: ensure Sysops conference: %w", err)
	}

	subject := fmt.Sprintf("VirtNet Node Updated: %s", saved.Addr4D())
	if changeType == "NEW" {
		subject = fmt.Sprintf("Welcome to the new VirtNet Node: %s", saved.Addr4D())
	}
	body := formatMemberInfo(saved)

	if err := msgStore.Post(&messages.Message{
		ConferenceID: sysopsConf.ID,
		FromName:     "VirtBBS NodeAnnounce",
		ToName:       "All",
		Subject:      subject,
		Status:       "A",
		Echo:         true,
		Body:         body,
	}); err != nil {
		return err
	}

	return LogNodeChange(db, nd.Name, fmt.Sprintf("%s  %-16s  %-7s  %s (%s)",
		time.Now().Format("2006-01-02 15:04"), saved.Addr4D(), changeType, saved.BBSName, saved.SysopName))
}

func formatMemberInfo(m *Member) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Address:  %s\r\n", m.Addr4D())
	fmt.Fprintf(&b, "BBS:      %s\r\n", m.BBSName)
	fmt.Fprintf(&b, "Sysop:    %s\r\n", m.SysopName)
	fmt.Fprintf(&b, "Location: %s\r\n", m.Location)
	if m.BinkpHost != "" {
		fmt.Fprintf(&b, "BinkP:    %s\r\n", m.BinkpHost)
	}
	return b.String()
}
