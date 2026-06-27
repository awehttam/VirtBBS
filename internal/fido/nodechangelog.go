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
//   v0.13.0 2026-06-27  VirtNet: NodeChgs.txt change log, one line per
//                        new/changed node (written by ApplyNodeAnnounceInfo
//                        for both inbound and local announcements), zipped
//                        with a FILE_ID.DIZ on every day-rollover
//                        regeneration into the Nodelist Files area.
// ============================================================================

// Package fido — nodechangelog.go
package fido

import (
	"database/sql"
	"strings"
)

// LogNodeChange appends one line to network's change log
// (fido_node_change_log), dumped into NodeChgs.txt by BuildNodeChgsText.
func LogNodeChange(db *sql.DB, network, line string) error {
	_, err := db.Exec(`INSERT INTO fido_node_change_log (network, line) VALUES (?,?)`, network, line)
	return err
}

// BuildNodeChgsText returns the full NodeChgs.txt contents for network —
// every logged change, oldest first.
func BuildNodeChgsText(db *sql.DB, network string) (string, error) {
	rows, err := db.Query(`SELECT created_at, line FROM fido_node_change_log WHERE network=? ORDER BY id`, network)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	b.WriteString("VirtNet Nodelist Changes\r\n")
	b.WriteString("========================\r\n\r\n")
	for rows.Next() {
		var createdAt, line string
		if err := rows.Scan(&createdAt, &line); err != nil {
			return "", err
		}
		b.WriteString(line)
		b.WriteString("\r\n")
	}
	return b.String(), rows.Err()
}
