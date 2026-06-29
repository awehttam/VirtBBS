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
//   v0.13.0 2026-06-27  VirtNet: downstream distribution of the generated
//                        nodelist as ordinary echomail (reusing scan.go's
//                        existing downlink fan-out completely unmodified —
//                        not a new TIC/file-echo pipeline, which doesn't
//                        exist anywhere in this codebase to build on), with
//                        auto-processing on arrival at every downstream
//                        node: import into the local nodelist file area and
//                        feed the existing fido.ImportFile to update that
//                        instance's own fido_nodes rows for the network.
// ============================================================================

// Package fido — nodelistecho.go
package fido

import (
	"database/sql"
	"fmt"
	"os"
)

// QueueNodelistEcho records an arrived "<NetworkName> Nodelists" echomail
// for later processing by ProcessPendingNodelistEchoes — see toss.go's
// inbound dispatch and the file header comment for why this is queued
// rather than processed inline.
func QueueNodelistEcho(db *sql.DB, network, subject, body string) error {
	_, err := db.Exec(`INSERT INTO fido_nodelist_echo_pending (network, subject, body) VALUES (?,?,?)`,
		network, subject, body)
	return err
}

// PendingNodelistEcho is one queued, not-yet-processed nodelist echo.
type PendingNodelistEcho struct {
	ID      int64
	Network string
	Subject string
	Body    string
}

// ListPendingNodelistEchoes returns every queued, unprocessed nodelist echo.
func ListPendingNodelistEchoes(db *sql.DB) ([]*PendingNodelistEcho, error) {
	rows, err := db.Query(`SELECT id, network, subject, body FROM fido_nodelist_echo_pending ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PendingNodelistEcho
	for rows.Next() {
		p := &PendingNodelistEcho{}
		if err := rows.Scan(&p.ID, &p.Network, &p.Subject, &p.Body); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ClearPendingNodelistEcho removes a processed entry from the queue.
func ClearPendingNodelistEcho(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM fido_nodelist_echo_pending WHERE id=?`, id)
	return err
}

// ApplyPendingNodelistEcho writes one queued echo's body to a file, registers
// it into "<NetworkName> Nodelist Files" (creating that area if needed via
// fileArea), and — for a full nodelist (NODELIST.Z*, not NODEDIFF.Z*) —
// calls ImportFile to update this instance's fido_nodes rows for the network.
func ApplyPendingNodelistEcho(db *sql.DB, p *PendingNodelistEcho, fileArea FileArea, nd *NetworkDef, bbsName, sysopName string, telnetPort int) error {
	filename := nodelistFilenameFromSubject(p.Subject)
	dirID, dirPath, err := fileArea.EnsureDir(p.Network+" Nodelist Files", p.Network+" Nodelist Files (auto-created)")
	if err != nil {
		return err
	}
	fullPath := dirPath + "/" + filename
	if err := os.WriteFile(fullPath, []byte(p.Body), 0644); err != nil {
		return err
	}
	if err := fileArea.RegisterUpload(dirID, filename, "VirtNet nodelist", "VirtBBS"); err != nil {
		return err
	}
	if nd == nil {
		nd = &NetworkDef{Name: p.Network}
	}
	if err := applyNodelistArtifact(db, fileArea, nd, filename, fullPath, bbsName, sysopName, telnetPort); err != nil {
		return err
	}
	markPendingEchoApplied(db, p.Network, filename, p.ID)
	return nil
}

// ProcessPendingNodelistEchoes drains the whole queue, applying each entry
// via ApplyPendingNodelistEcho and clearing it on success. Call after every
// toss (TossDir) and from the scheduler's 1-minute echo ticker on hub and
// member networks.
func ProcessPendingNodelistEchoes(db *sql.DB, fileArea FileArea) []string {
	return processPendingNodelistEchoes(db, fileArea, "", nil, "", "")
}

// ProcessPendingNodelistEchoesForNetwork drains queued nodelist echoes for
// one logical network name only. When nd and bbsName/sysopName are set and a
// full nodelist (.Z) is applied, network diagrams are regenerated afterward.
func ProcessPendingNodelistEchoesForNetwork(db *sql.DB, fileArea FileArea, nd *NetworkDef, bbsName, sysopName string) []string {
	network := ""
	if nd != nil {
		network = nd.Name
	}
	return processPendingNodelistEchoes(db, fileArea, network, nd, bbsName, sysopName)
}

func processPendingNodelistEchoes(db *sql.DB, fileArea FileArea, network string, nd *NetworkDef, bbsName, sysopName string) []string {
	if fileArea == nil {
		return []string{"file area store not available"}
	}
	var errs []string
	pending, err := ListPendingNodelistEchoes(db)
	if err != nil {
		return []string{err.Error()}
	}
	nodelistImported := false
	for _, p := range pending {
		if network != "" && p.Network != network {
			continue
		}
		filename := nodelistFilenameFromSubject(p.Subject)
		if err := ApplyPendingNodelistEcho(db, p, fileArea, nd, bbsName, sysopName, 0); err != nil {
			errs = append(errs, fmt.Sprintf("echo %d: %v", p.ID, err))
			continue
		}
		if IsFullNodelistFilename(filename) || IsNodelistDiffFilename(filename) {
			if IsFullNodelistFilename(filename) {
				nodelistImported = true
			}
		}
		if err := ClearPendingNodelistEcho(db, p.ID); err != nil {
			errs = append(errs, fmt.Sprintf("echo %d: clear: %v", p.ID, err))
		}
	}
	if nodelistImported && nd != nil && bbsName != "" {
		if _, warns := RebuildNetworkDiagrams(nd, db, fileArea, bbsName, sysopName); len(warns) > 0 {
			for _, w := range warns {
				errs = append(errs, "diagram: "+w)
			}
		}
	}
	return errs
}
