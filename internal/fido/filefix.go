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
//   v0.4.0  2026-06-25  Initial implementation — FileFix responder (for downlink
//                        file-area subscription requests) and request generator,
//                        mirroring areafix.go's structure for echomail areas
//   v0.5.0  2026-06-28  %RESCAN backlog TIC export, +TAG,R=N subscribe-with-rescan
// ============================================================================

package fido

// Package fido — filefix.go
//
// Implements FileFix, the FidoNet convention (analogous to AreaFix) for
// managing FILE ECHO area subscriptions by netmail. Outbound distribution
// is handled by filescan.go / ticprocess.go (FTS-5006 TIC tickets).
//
// Command syntax mirrors AreaFix, but tags refer to file areas
// (mapped to internal/files.Dir IDs via [fido.file_areas] /
// [fido.networks.file_areas]) instead of echomail conferences:
//
//	<password>
//	+FILE_TAG       subscribe to FILE_TAG
//	-FILE_TAG       unsubscribe from FILE_TAG
//	%LIST           list all file areas available to subscribe to
//	%QUERY          list file areas currently subscribed to
//	%RESCAN         rescan subscribed file areas (or set rescan mode for +TAG)
//	%RESCAN TAG     rescan backlog for subscribed file area TAG
//	%HELP           show this command summary

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// FileFixRobotName is the netmail ToName that triggers the responder.
const FileFixRobotName = "FileFix"

// IsFileFixRequest reports whether toName addresses the FileFix robot.
func IsFileFixRequest(toName string) bool {
	return strings.EqualFold(strings.TrimSpace(toName), FileFixRobotName)
}

// FileFixDB manages FileFix subscription state in SQLite.
type FileFixDB struct{ db *sql.DB }

// OpenFileFixDB returns a FileFixDB using the shared database connection.
func OpenFileFixDB(db *sql.DB) *FileFixDB { return &FileFixDB{db: db} }

// Subscribe records that downlinkAddr (zone:net/node) receives fileTag.
func (a *FileFixDB) Subscribe(network, downlinkAddr, fileTag string) error {
	_, err := a.db.Exec(`INSERT OR IGNORE INTO fido_filefix_subs (network, downlink_addr, file_tag)
		VALUES (?,?,?)`, network, downlinkAddr, fileTag)
	return err
}

// Unsubscribe removes a downlink's subscription to fileTag.
func (a *FileFixDB) Unsubscribe(network, downlinkAddr, fileTag string) error {
	_, err := a.db.Exec(`DELETE FROM fido_filefix_subs WHERE network=? AND downlink_addr=? AND file_tag=?`,
		network, downlinkAddr, fileTag)
	return err
}

// SubscriptionsFor returns the file tags downlinkAddr currently subscribes to.
func (a *FileFixDB) SubscriptionsFor(network, downlinkAddr string) ([]string, error) {
	rows, err := a.db.Query(`SELECT file_tag FROM fido_filefix_subs
		WHERE network=? AND downlink_addr=? ORDER BY file_tag`, network, downlinkAddr)
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
// fileTag. Used by filescan.go to fan TIC distribution out to downlinks.
func (a *FileFixDB) SubscribedDownlinks(network, fileTag string) ([]string, error) {
	rows, err := a.db.Query(`SELECT downlink_addr FROM fido_filefix_subs
		WHERE network=? AND file_tag=? ORDER BY downlink_addr`, network, fileTag)
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

// ProcessFileFixRequest handles an inbound netmail addressed to "FileFix".
// It validates the sender against the network's configured Downlinks list
// and the password supplied as the first non-blank body line, applies any
// +TAG/-TAG/%LIST/%QUERY/%RESCAN/%HELP commands found in the remaining lines,
// and writes an immediate netmail reply summarising the result. When
// filesRoot is non-empty, rescan commands queue backlog TIC files for the
// downlink.
func ProcessFileFixRequest(nd *NetworkDef, db *sql.DB, filesRoot string, pm *Message) error {
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return fmt.Errorf("filefix: invalid local address %q", nd.Address)
	}

	dl := nd.DownlinkByAddr(pm.OrigAddr)
	if dl == nil {
		return replyFileFix(nd, our, pm, "Unknown system — you are not configured as a downlink.\r\n")
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
				if dl.Password == "" {
					passwordOK = true
					cmdLines = append(cmdLines, line)
				} else {
					return replyFileFix(nd, our, pm, "Invalid password.\r\n")
				}
			}
			continue
		}
		cmdLines = append(cmdLines, line)
	}
	if !passwordOK {
		return replyFileFix(nd, our, pm, "Invalid password.\r\n")
	}

	filefixDB := OpenFileFixDB(db)
	downlinkAddr := pm.OrigAddr.String()

	var out strings.Builder
	fmt.Fprintf(&out, "FileFix response for %s (%s)\r\n\r\n", dl.Name, downlinkAddr)

	if len(cmdLines) == 0 {
		writeFileFixHelp(&out)
	}

	rescanMode := false

	flushRescan := func(tags []string, maxFiles int, prefix string) {
		if filesRoot == "" || len(tags) == 0 {
			if filesRoot == "" {
				fmt.Fprintf(&out, "  %srescan ERROR: files path not configured\r\n", prefix)
			}
			return
		}
		res, err := RescanFilesToDownlink(nd, db, filesRoot, downlinkAddr, tags, maxFiles)
		if err != nil {
			fmt.Fprintf(&out, "  %srescan ERROR: %v\r\n", prefix, err)
			return
		}
		for _, e := range res.Errors {
			fmt.Fprintf(&out, "  %srescan WARNING: %s\r\n", prefix, e)
		}
		if res.Files == 0 {
			fmt.Fprintf(&out, "  %srescan — no files to send\r\n", prefix)
		} else {
			fmt.Fprintf(&out, "  %srescan — %d file(s) queued in %d TIC ticket(s)\r\n", prefix, res.Files, res.TICFiles)
		}
	}

	subscribed := func(tag string) bool {
		tags, err := filefixDB.SubscriptionsFor(nd.Name, downlinkAddr)
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
			writeFileFixList(&out, nd)
		case upper == "%QUERY" || upper == "QUERY":
			writeFileFixQuery(&out, filefixDB, nd.Name, downlinkAddr)
		case upper == "%HELP" || upper == "HELP" || upper == "?":
			writeFileFixHelp(&out)
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
				tags, err := filefixDB.SubscriptionsFor(nd.Name, downlinkAddr)
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
			if !fileFixTagExists(nd, tag) {
				fmt.Fprintf(&out, "  +%-30s UNKNOWN FILE AREA — not added\r\n", tag)
				continue
			}
			if err := filefixDB.Subscribe(nd.Name, downlinkAddr, tag); err != nil {
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
			if err := filefixDB.Unsubscribe(nd.Name, downlinkAddr, tag); err != nil {
				fmt.Fprintf(&out, "  -%-30s ERROR: %v\r\n", tag, err)
				continue
			}
			fmt.Fprintf(&out, "  -%-30s unsubscribed\r\n", tag)
		default:
			fmt.Fprintf(&out, "  Unrecognised command: %q\r\n", line)
		}
	}

	out.WriteString("\r\n")
	writeFileFixQuery(&out, filefixDB, nd.Name, downlinkAddr)

	return replyFileFix(nd, our, pm, out.String())
}

// fileFixTagExists reports whether tag is a configured file area
// (present in nd.FileAreas, mapping to an internal/files.Dir ID).
func fileFixTagExists(nd *NetworkDef, tag string) bool {
	_, ok := nd.FileAreas[strings.ToUpper(tag)]
	return ok
}

func writeFileFixList(out *strings.Builder, nd *NetworkDef) {
	out.WriteString("File areas available:\r\n")
	if len(nd.FileAreas) == 0 {
		out.WriteString("  (none configured)\r\n")
		return
	}
	tags := make([]string, 0, len(nd.FileAreas))
	for tag := range nd.FileAreas {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, t := range tags {
		fmt.Fprintf(out, "  %s\r\n", t)
	}
}

func writeFileFixQuery(out *strings.Builder, filefixDB *FileFixDB, networkName, downlinkAddr string) {
	tags, err := filefixDB.SubscriptionsFor(networkName, downlinkAddr)
	out.WriteString("Currently subscribed:\r\n")
	if err != nil || len(tags) == 0 {
		out.WriteString("  (none)\r\n")
		return
	}
	for _, t := range tags {
		fmt.Fprintf(out, "  %s\r\n", t)
	}
}

func writeFileFixHelp(out *strings.Builder) {
	out.WriteString("Commands (one per line, after your password):\r\n")
	out.WriteString("  +TAG         subscribe to file area TAG\r\n")
	out.WriteString("  +TAG,R=N     subscribe and rescan N oldest files\r\n")
	out.WriteString("  +TAG,R       subscribe and rescan full file backlog\r\n")
	out.WriteString("  -TAG         unsubscribe from file area TAG\r\n")
	out.WriteString("  %LIST        list all file areas available\r\n")
	out.WriteString("  %QUERY       list your current subscriptions\r\n")
	out.WriteString("  %RESCAN      rescan all subscribed file areas (+ sets rescan mode)\r\n")
	out.WriteString("  %RESCAN TAG  rescan one subscribed file area\r\n")
	out.WriteString("  %HELP        show this help\r\n\r\n")
}

// replyFileFix writes an immediate netmail reply from the FileFix robot
// back to the requester, routed via the network's configured uplink.
func replyFileFix(nd *NetworkDef, our Addr, pm *Message, body string) error {
	uplink := nd.UplinkAddr()
	if uplink == (Addr{}) {
		return fmt.Errorf("filefix: no uplink configured to route reply")
	}
	reply := &NetmailMsg{
		FromName: FileFixRobotName,
		FromAddr: our.String(),
		ToName:   pm.FromName,
		ToAddr:   pm.OrigAddr.String(),
		Subject:  "FileFix response",
		Body:     body,
	}
	outDir := OutboundDir(nd.OutboundDir, uplink, uplink, false)
	_, err := WritePKT(our, uplink, nd.Password, outDir, []*NetmailMsg{reply}, nd.Name)
	return err
}

// ── Requester (us subscribing to our own uplink's FileFix) ─────────────────

// RequestFileFix composes and writes a netmail to "FileFix" at nd's own
// uplink, requesting subscription changes (adds/removes are file-area tags,
// without +/- prefixes). Used when VirtBBS itself is a downlink of nd.
func RequestFileFix(nd *NetworkDef, fromName string, adds, removes []string) (pktPath string, err error) {
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return "", fmt.Errorf("invalid local address %q", nd.Address)
	}
	uplink := nd.UplinkAddr()
	if uplink == (Addr{}) {
		return "", fmt.Errorf("no uplink configured")
	}

	var body strings.Builder
	if nd.FileFixPassword != "" {
		fmt.Fprintf(&body, "%s\r\n", nd.FileFixPassword)
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
		ToName:   FileFixRobotName,
		ToAddr:   uplink.String(),
		Subject:  "FileFix",
		Body:     body.String(),
	}

	outDir := OutboundDir(nd.OutboundDir, uplink, uplink, false)
	return WritePKT(our, uplink, nd.Password, outDir, []*NetmailMsg{msg}, nd.Name)
}
