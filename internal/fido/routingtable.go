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
//   v0.13.0 2026-06-27  VirtNet: BinkleyTerm-style plain-text routing-table
//                        import/export, for sysop bulk edits and backup —
//                        in addition to (not instead of) the DB itself
//                        being the live routing table fido_members uses.
// ============================================================================

// Package fido — routingtable.go
//
// A literal plain-text routing-table file, the format BinkleyTerm/
// SyncroNet-style mailers use: one line per node, "<addr> <host:port>
// <password>". Blank lines and ';'-prefixed comments are ignored, mirroring
// the comment convention nodelist.go already uses for FTS-0005.
package fido

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ExportRoutingTable writes one line per active fido_members row for
// network: "<zone:net/node>  <host:port>  <password>", column-aligned.
func ExportRoutingTable(db *sql.DB, network string) ([]byte, error) {
	mdb := OpenMembersDB(db)
	members, err := mdb.ListMembers(network)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "; VirtNet routing table for %q, generated %s\r\n", network, time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "; %-16s %-30s %s\r\n", "Address", "Host:Port", "Password")
	for _, m := range members {
		fmt.Fprintf(&b, "%-16s %-30s %s\r\n", m.Addr4D(), m.BinkpHost, m.Password)
	}
	return []byte(b.String()), nil
}

// RoutingImportResult summarises a routing-table import.
type RoutingImportResult struct {
	Updated int
	Unknown []string // addresses with no matching fido_members row
	Errors  []string
}

// ImportRoutingTable parses data (the format ExportRoutingTable writes) and,
// for every address matching an existing fido_members row for network,
// updates its binkp_host/password. Addresses with no matching member are
// reported as Unknown, not auto-created — this stays a bulk-edit tool for
// already-approved members, not a backdoor around the join/approval flow.
func ImportRoutingTable(db *sql.DB, network string, data []byte) (*RoutingImportResult, error) {
	mdb := OpenMembersDB(db)
	result := &RoutingImportResult{}

	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			result.Errors = append(result.Errors, fmt.Sprintf("malformed line: %q", line))
			continue
		}
		addrStr, hostPort := fields[0], fields[1]
		password := ""
		if len(fields) > 2 {
			password = strings.Join(fields[2:], " ")
		}
		a, err := ParseAddr(addrStr)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", addrStr, err))
			continue
		}
		m, err := mdb.GetMemberByAddr(network, a)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", addrStr, err))
			continue
		}
		if m == nil {
			result.Unknown = append(result.Unknown, addrStr)
			continue
		}
		m.BinkpHost = hostPort
		m.Password = password
		if err := mdb.UpdateMemberInfo(m); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", addrStr, err))
			continue
		}
		result.Updated++
	}
	return result, sc.Err()
}
