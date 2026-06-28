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
//   v0.4.0  2026-06-28  %RESCAN backlog export, +TAG,R=N subscribe-with-rescan
//   v0.2.0  2026-06-25  Initial implementation — AreaFix responder (for downlink
//                        subscription requests) and request generator (for
//                        subscribing to our own uplink as a downlink)
//   v0.3.0  2026-06-25  ProcessAreaFixRequest/replyAreaFix/areaFixTagExists take a
//                        *NetworkDef instead of *Config, so the responder works for
//                        any configured network, not just primary
// ============================================================================

package fido

// Package fido — areafix.go
//
// Implements AreaFix, the long-standing FidoNet convention for managing
// echomail area subscriptions by netmail. Two independent roles:
//
//   Responder  — other systems ("downlinks") send netmail to "AreaFix"
//                 at OUR address to subscribe/unsubscribe from the echo
//                 areas we feed them. ProcessAreaFixRequest handles this.
//
//   Requester  — THIS BBS sends netmail to "AreaFix" at our UPLINK's
//                 address to subscribe/unsubscribe from areas we want to
//                 receive. RequestAreaFix handles this.
//
// Command syntax (case-insensitive, one command per line, password first):
//
//	<password>
//	+AREA_TAG       subscribe to AREA_TAG (+TAG,R=N sends N old messages)
//	-AREA_TAG       unsubscribe from AREA_TAG
//	%LIST           list all areas available to subscribe to
//	%QUERY          list areas currently subscribed to
//	%RESCAN         rescan subscribed areas (or set rescan mode for +TAG lines)
//	%RESCAN TAG     rescan backlog for subscribed area TAG
//	%HELP           show this command summary

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/messages"
)

// AreaFixRobotName is the netmail ToName that triggers the responder.
const AreaFixRobotName = "AreaFix"

// IsAreaFixRequest reports whether toName addresses the AreaFix robot.
func IsAreaFixRequest(toName string) bool {
	return strings.EqualFold(strings.TrimSpace(toName), AreaFixRobotName)
}

// AreaFixDB manages AreaFix subscription state in SQLite.
type AreaFixDB struct{ db *sql.DB }

// OpenAreaFixDB returns an AreaFixDB using the shared database connection.
func OpenAreaFixDB(db *sql.DB) *AreaFixDB { return &AreaFixDB{db: db} }

// Subscribe records that downlinkAddr (zone:net/node) receives areaTag.
func (a *AreaFixDB) Subscribe(network, downlinkAddr, areaTag string) error {
	_, err := a.db.Exec(`INSERT OR IGNORE INTO fido_areafix_subs (network, downlink_addr, area_tag)
		VALUES (?,?,?)`, network, downlinkAddr, areaTag)
	return err
}

// Unsubscribe removes a downlink's subscription to areaTag.
func (a *AreaFixDB) Unsubscribe(network, downlinkAddr, areaTag string) error {
	_, err := a.db.Exec(`DELETE FROM fido_areafix_subs WHERE network=? AND downlink_addr=? AND area_tag=?`,
		network, downlinkAddr, areaTag)
	return err
}

// SubscriptionsFor returns the area tags downlinkAddr currently subscribes to.
func (a *AreaFixDB) SubscriptionsFor(network, downlinkAddr string) ([]string, error) {
	rows, err := a.db.Query(`SELECT area_tag FROM fido_areafix_subs
		WHERE network=? AND downlink_addr=? ORDER BY area_tag`, network, downlinkAddr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// SubscribedDownlinks returns the addresses of every downlink subscribed to
// areaTag, used by the scanner to fan an outgoing echomail message out to
// downlinks in addition to the conference's normal uplink destination.
func (a *AreaFixDB) SubscribedDownlinks(network, areaTag string) ([]string, error) {
	rows, err := a.db.Query(`SELECT downlink_addr FROM fido_areafix_subs
		WHERE network=? AND area_tag=? ORDER BY downlink_addr`, network, areaTag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var addrs []string
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			return nil, err
		}
		addrs = append(addrs, addr)
	}
	return addrs, rows.Err()
}

// AllDownlinkAddrs returns the distinct set of downlink addresses with at
// least one subscription on this network. Used by the sysop UI.
func (a *AreaFixDB) AllDownlinkAddrs(network string) ([]string, error) {
	rows, err := a.db.Query(`SELECT DISTINCT downlink_addr FROM fido_areafix_subs
		WHERE network=? ORDER BY downlink_addr`, network)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var addrs []string
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			return nil, err
		}
		addrs = append(addrs, addr)
	}
	return addrs, rows.Err()
}

// ── Responder (downlinks managing their subscriptions with us) ─────────────

// ProcessAreaFixRequest handles an inbound netmail addressed to "AreaFix".
// It validates the sender against the network's configured Downlinks list
// and the password supplied as the first non-blank body line, applies any
// +TAG/-TAG/%LIST/%QUERY/%RESCAN/%HELP commands found in the remaining lines,
// and writes an immediate netmail reply summarising the result. When msgStore
// is non-nil, rescan commands queue backlog .pkt files for the downlink.
func ProcessAreaFixRequest(nd *NetworkDef, msgStore *messages.Store, confStore *conferences.Store, networkName, bbsName string, pm *Message) error {
	if msgStore == nil {
		return fmt.Errorf("areafix: message store required")
	}
	db := msgStore.DB()
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return fmt.Errorf("areafix: invalid local address %q", nd.Address)
	}

	dl := nd.DownlinkByAddr(pm.OrigAddr)
	if dl == nil {
		return replyAreaFix(nd, our, pm, "Unknown system — you are not configured as a downlink.\r\n")
	}

	lines := strings.Split(strings.ReplaceAll(pm.Body, "\r\n", "\r"), "\r")
	var cmdLines []string
	passwordOK := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !passwordOK {
			passwordOK = line == dl.Password
			if !passwordOK {
				// First non-blank line wasn't the password — but allow a
				// password-less downlink (Password == "") to skip straight
				// to commands.
				if dl.Password == "" {
					passwordOK = true
					cmdLines = append(cmdLines, line)
				} else {
					return replyAreaFix(nd, our, pm, "Invalid password.\r\n")
				}
			}
			continue
		}
		cmdLines = append(cmdLines, line)
	}
	if !passwordOK {
		return replyAreaFix(nd, our, pm, "Invalid password.\r\n")
	}

	areafixDB := OpenAreaFixDB(db)
	downlinkAddr := pm.OrigAddr.String()

	var out strings.Builder
	fmt.Fprintf(&out, "AreaFix response for %s (%s)\r\n\r\n", dl.Name, downlinkAddr)

	if len(cmdLines) == 0 {
		writeAreaFixHelp(&out)
	}

	rescanMode := false

	flushRescan := func(tags []string, maxMsgs int, prefix string) {
		if msgStore == nil || len(tags) == 0 {
			return
		}
		res, err := RescanEchoToDownlink(nd, msgStore, confStore, bbsName, downlinkAddr, tags, maxMsgs)
		if err != nil {
			fmt.Fprintf(&out, "  %srescan ERROR: %v\r\n", prefix, err)
			return
		}
		for _, e := range res.Errors {
			fmt.Fprintf(&out, "  %srescan WARNING: %s\r\n", prefix, e)
		}
		if res.Messages == 0 {
			fmt.Fprintf(&out, "  %srescan — no messages to send\r\n", prefix)
		} else {
			fmt.Fprintf(&out, "  %srescan — %d message(s) queued\r\n", prefix, res.Messages)
		}
	}

	subscribed := func(tag string) bool {
		tags, err := areafixDB.SubscriptionsFor(networkName, downlinkAddr)
		if err != nil {
			return false
		}
		tag = strings.ToUpper(tag)
		for _, t := range tags {
			if t == tag {
				return true
			}
		}
		return false
	}

	for _, line := range cmdLines {
		upper := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case upper == "%LIST" || upper == "LIST":
			writeAreaFixList(&out, confStore, networkName)
		case upper == "%QUERY" || upper == "QUERY":
			writeAreaFixQuery(&out, areafixDB, networkName, downlinkAddr)
		case upper == "%HELP" || upper == "HELP" || upper == "?":
			writeAreaFixHelp(&out)
		case strings.HasPrefix(upper, "%RESCAN"):
			tag, _ := parseAreaFixRescanLine(line)
			if tag != "" {
				if !subscribed(tag) {
					fmt.Fprintf(&out, "  %-30s NOT SUBSCRIBED — not rescanned\r\n", tag)
				} else {
					flushRescan([]string{tag}, 0, "")
				}
			} else {
				rescanMode = true
				tags, err := areafixDB.SubscriptionsFor(networkName, downlinkAddr)
				if err != nil || len(tags) == 0 {
					out.WriteString("  %RESCAN — no subscribed areas (subsequent +TAG will rescan)\r\n")
				} else {
					flushRescan(tags, 0, "%RESCAN ")
				}
			}
		case strings.HasPrefix(line, "+") || strings.HasPrefix(line, "="):
			add, ok := parseAreaFixAddLine(line)
			if !ok || add.tag == "" {
				continue
			}
			tag := add.tag
			if !areaFixTagExists(confStore, networkName, nd, tag) {
				fmt.Fprintf(&out, "  +%-30s UNKNOWN AREA — not added\r\n", tag)
				continue
			}
			if err := areafixDB.Subscribe(networkName, downlinkAddr, tag); err != nil {
				fmt.Fprintf(&out, "  +%-30s ERROR: %v\r\n", tag, err)
				continue
			}
			fmt.Fprintf(&out, "  +%-30s subscribed\r\n", tag)
			if add.rescanMax >= 0 {
				flushRescan([]string{tag}, add.rescanMax, "")
			} else if rescanMode {
				flushRescan([]string{tag}, 0, "")
			}
		case strings.HasPrefix(line, "-"):
			tag := strings.ToUpper(strings.TrimSpace(line[1:]))
			if tag == "" {
				continue
			}
			if err := areafixDB.Unsubscribe(networkName, downlinkAddr, tag); err != nil {
				fmt.Fprintf(&out, "  -%-30s ERROR: %v\r\n", tag, err)
				continue
			}
			fmt.Fprintf(&out, "  -%-30s unsubscribed\r\n", tag)
		default:
			fmt.Fprintf(&out, "  Unrecognised command: %q\r\n", line)
		}
	}

	out.WriteString("\r\n")
	writeAreaFixQuery(&out, areafixDB, networkName, downlinkAddr)

	return replyAreaFix(nd, our, pm, out.String())
}

// areaFixAddCmd holds a parsed +TAG subscribe line.
type areaFixAddCmd struct {
	tag       string
	rescanMax int // -1 = no rescan; 0 = full backlog; N>0 = oldest N messages
}

// parseAreaFixAddLine parses +TAG or =TAG with optional ,R or ,R=N suffix.
func parseAreaFixAddLine(line string) (areaFixAddCmd, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return areaFixAddCmd{}, false
	}
	if line[0] == '+' || line[0] == '=' {
		line = line[1:]
	}
	parts := strings.Split(line, ",")
	tag := strings.ToUpper(strings.TrimSpace(parts[0]))
	cmd := areaFixAddCmd{tag: tag, rescanMax: -1}
	for _, opt := range parts[1:] {
		opt = strings.ToUpper(strings.TrimSpace(opt))
		if opt == "R" {
			cmd.rescanMax = 0
			continue
		}
		if strings.HasPrefix(opt, "R=") {
			n, err := strconv.Atoi(strings.TrimSpace(opt[2:]))
			if err != nil || n < 0 {
				cmd.rescanMax = 0
			} else {
				cmd.rescanMax = n
			}
		}
	}
	return cmd, tag != ""
}

// parseAreaFixRescanLine parses %RESCAN or %RESCAN TAG.
func parseAreaFixRescanLine(line string) (tag string, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(strings.ToUpper(line), "%RESCAN") {
		return "", false
	}
	rest := strings.TrimSpace(line[len("%RESCAN"):])
	if rest == "" {
		return "", true
	}
	return strings.ToUpper(rest), true
}

// areaFixTagExists reports whether tag is a valid, known echomail area —
// either as a conference's EchoTag (preferred, via confStore) or in the
// legacy nd.Areas map.
func areaFixTagExists(confStore *conferences.Store, networkName string, nd *NetworkDef, tag string) bool {
	if confStore != nil {
		if conf, err := confStore.GetByTag(tag, networkName); err == nil && conf != nil {
			return true
		}
	}
	_, ok := nd.Areas[tag]
	return ok
}

func writeAreaFixList(out *strings.Builder, confStore *conferences.Store, networkName string) {
	out.WriteString("Areas available:\r\n")
	if confStore == nil {
		out.WriteString("  (none configured)\r\n")
		return
	}
	confs, err := confStore.ListEcho(networkName)
	if err != nil || len(confs) == 0 {
		out.WriteString("  (none configured)\r\n")
		return
	}
	tags := make([]string, 0, len(confs))
	for _, c := range confs {
		if c.EchoTag != "" {
			tags = append(tags, c.EchoTag)
		}
	}
	sort.Strings(tags)
	for _, t := range tags {
		fmt.Fprintf(out, "  %s\r\n", t)
	}
}

func writeAreaFixQuery(out *strings.Builder, areafixDB *AreaFixDB, networkName, downlinkAddr string) {
	tags, err := areafixDB.SubscriptionsFor(networkName, downlinkAddr)
	out.WriteString("Currently subscribed:\r\n")
	if err != nil || len(tags) == 0 {
		out.WriteString("  (none)\r\n")
		return
	}
	for _, t := range tags {
		fmt.Fprintf(out, "  %s\r\n", t)
	}
}

func writeAreaFixHelp(out *strings.Builder) {
	out.WriteString("Commands (one per line, after your password):\r\n")
	out.WriteString("  +TAG         subscribe to area TAG\r\n")
	out.WriteString("  +TAG,R=N     subscribe and send N old messages\r\n")
	out.WriteString("  +TAG,R       subscribe and send full backlog\r\n")
	out.WriteString("  -TAG         unsubscribe from area TAG\r\n")
	out.WriteString("  %LIST        list all areas available\r\n")
	out.WriteString("  %QUERY       list your current subscriptions\r\n")
	out.WriteString("  %RESCAN      rescan all subscribed areas (+ sets rescan mode)\r\n")
	out.WriteString("  %RESCAN TAG  rescan one subscribed area\r\n")
	out.WriteString("  %HELP        show this help\r\n\r\n")
}

// replyAreaFix writes an immediate netmail reply from the AreaFix robot
// back to the requester, routed via the network's configured uplink.
func replyAreaFix(nd *NetworkDef, our Addr, pm *Message, body string) error {
	uplink := nd.UplinkAddr()
	if uplink == (Addr{}) {
		return fmt.Errorf("areafix: no uplink configured to route reply")
	}
	reply := &NetmailMsg{
		FromName: AreaFixRobotName,
		FromAddr: our.String(),
		ToName:   pm.FromName,
		ToAddr:   pm.OrigAddr.String(),
		Subject:  "AreaFix response",
		Body:     body,
	}
	outDir := OutboundDir(nd.OutboundDir, uplink, uplink, false)
	_, err := WritePKT(our, uplink, nd.Password, outDir, []*NetmailMsg{reply}, nd.Name)
	return err
}

// ── Requester (us subscribing to our own uplink's AreaFix) ─────────────────

// RequestAreaFix composes and writes a netmail to "AreaFix" at nd's own
// uplink, requesting subscription changes (adds/removes are AREA: tags,
// without +/- prefixes). Used when VirtBBS itself is a downlink of nd.
func RequestAreaFix(nd *NetworkDef, fromName string, adds, removes []string) (pktPath string, err error) {
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return "", fmt.Errorf("invalid local address %q", nd.Address)
	}
	uplink := nd.UplinkAddr()
	if uplink == (Addr{}) {
		return "", fmt.Errorf("no uplink configured")
	}

	var body strings.Builder
	if nd.AreaFixPassword != "" {
		fmt.Fprintf(&body, "%s\r\n", nd.AreaFixPassword)
	}
	for _, tag := range adds {
		fmt.Fprintf(&body, "+%s\r\n", strings.ToUpper(strings.TrimSpace(tag)))
	}
	for _, tag := range removes {
		fmt.Fprintf(&body, "-%s\r\n", strings.ToUpper(strings.TrimSpace(tag)))
	}

	msg := &NetmailMsg{
		FromName: fromName,
		FromAddr: our.String(),
		ToName:   AreaFixRobotName,
		ToAddr:   uplink.String(),
		Subject:  "AreaFix",
		Body:     body.String(),
	}

	outDir := OutboundDir(nd.OutboundDir, uplink, uplink, false)
	return WritePKT(our, uplink, nd.Password, outDir, []*NetmailMsg{msg}, nd.Name)
}
