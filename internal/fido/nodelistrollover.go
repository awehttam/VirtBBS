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
//   v0.13.0 2026-06-27  VirtNet: day-rollover orchestration — generates the
//                        full nodelist + diff, snapshots fido_members for
//                        tomorrow's diff, posts both into the auto-created
//                        "<NetworkName> Nodelists" echo conference (so
//                        scan.go's existing downlink fan-out distributes
//                        them to every member), and registers the files —
//                        plus NodeChgs.txt and the node diagrams — into the
//                        auto-created "<NetworkName> Nodelist Files" area.
// ============================================================================

// Package fido — nodelistrollover.go
package fido

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/messages"
)

// writeZipAndRegister/writeMultiZipAndRegister build a zip in memory (one
// payload + FILE_ID.DIZ, or several payloads + a shared FILE_ID.DIZ) and
// register it via the FileArea interface. This duplicates the small amount
// of archive/zip logic internal/files/localfile.go's WriteZipWithDiz
// already has, rather than importing that package directly — see ensure.go
// for why internal/fido can't import internal/files.
func writeZipAndRegister(dirPath string, dirID int64, fileArea FileArea, zipName, payloadName string, payload []byte, diz string) error {
	return writeMultiZipAndRegister(dirPath, dirID, fileArea, zipName, map[string][]byte{payloadName: payload}, diz)
}

func writeMultiZipAndRegister(dirPath string, dirID int64, fileArea FileArea, zipName string, payloads map[string][]byte, diz string) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range payloads {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			return err
		}
	}
	d, err := zw.Create("FILE_ID.DIZ")
	if err != nil {
		return err
	}
	if _, err := d.Write([]byte(diz)); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := os.WriteFile(dirPath+"/"+zipName, buf.Bytes(), 0644); err != nil {
		return err
	}
	return fileArea.RegisterUpload(dirID, zipName, diz, "VirtBBS")
}

// RunDayRollover regenerates everything a hub network publishes once per day:
// on Fridays a full NODELIST.Z## plus any changes as NODEDIFF.Z##; on other
// days NODEDIFF.Z## only when members changed. Always snapshots members,
// refreshes fido_nodes, NodeChgs.txt, and diagrams. When publish is true,
// posts nodelist/diff into the echo conference for downlink fan-out.
func RunDayRollover(nd *NetworkDef, db *sql.DB, confStore *conferences.Store, msgStore *messages.Store,
	fileArea FileArea, hubBBSName, hubSysopName string, publish bool) []string {
	var warnings []string
	warn := func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }

	var nlConfID int
	if publish {
		nlConf, err := EnsureEchoConference(confStore, nd.Name+" Nodelists", nd.Name, nd.EffectiveNodelistEchoTag())
		if err != nil {
			warn("ensure Nodelists conference: %v", err)
			return warnings
		}
		nlConfID = nlConf.ID
	}

	fullData, fullName, err := GenerateNodelist(db, nd, hubBBSName, hubSysopName)
	if err != nil {
		warn("generate nodelist: %v", err)
		return warnings
	}
	diffData, diffName, err := GenerateNodelistDiff(db, nd)
	if err != nil {
		warn("generate nodelist diff: %v", err)
	}
	if err := SnapshotMembers(db, nd.Name); err != nil {
		warn("snapshot members: %v", err)
	}

	nodelistSuffix := func(filename string) string {
		if i := strings.LastIndex(filename, ".Z"); i >= 0 {
			return filename[i+1:]
		}
		return NodelistDaySuffix(time.Now())
	}

	if publish && fullName != "" {
		if err := msgStore.Post(&messages.Message{
			ConferenceID: nlConfID,
			FromName:     "VirtBBS NodeAnnounce",
			ToName:       "All",
			Subject:      fmt.Sprintf("%s Nodelist %s", nd.Name, nodelistSuffix(fullName)),
			Status:       "A",
			Echo:         true,
			Body:         string(fullData),
		}); err != nil {
			warn("post nodelist message: %v", err)
		}
	}
	if publish && diffData != nil {
		if err := msgStore.Post(&messages.Message{
			ConferenceID: nlConfID,
			FromName:     "VirtBBS NodeAnnounce",
			ToName:       "All",
			Subject:      fmt.Sprintf("%s Nodelist Diff %s", nd.Name, nodelistSuffix(diffName)),
			Status:       "A",
			Echo:         true,
			Body:         string(diffData),
		}); err != nil {
			warn("post nodelist diff message: %v", err)
		}
	}

	dirID, dirPath, err := fileArea.EnsureDir(nd.Name+" Nodelist Files", nd.Name+" Nodelist Files (auto-created)")
	if err != nil {
		warn("ensure Nodelist Files area: %v", err)
		return warnings
	}
	writeAndRegister := func(filename string, data []byte) {
		if err := os.WriteFile(dirPath+"/"+filename, data, 0644); err != nil {
			warn("write %s: %v", filename, err)
			return
		}
		if err := fileArea.RegisterUpload(dirID, filename, "VirtNet nodelist", "VirtBBS"); err != nil {
			warn("register %s: %v", filename, err)
		}
	}
	if fullName != "" {
		writeAndRegister(fullName, fullData)
	}
	if diffData != nil {
		writeAndRegister(diffName, diffData)
	}
	if chgsText, err := BuildNodeChgsText(db, nd.Name); err != nil {
		warn("build NodeChgs.txt: %v", err)
	} else if err := writeZipAndRegister(dirPath, dirID, fileArea, "NodeChgs.zip", "NodeChgs.txt",
		[]byte(chgsText), "Nodelist Changes"); err != nil {
		warn("zip NodeChgs.txt: %v", err)
	}

	nodes, _ := OpenNodelistDB(db).ListAll(nd.Name)
	diagCount, diagWarnings := rebuildNetworkDiagramZip(nd, nodes, fileArea, dirID, dirPath, hubBBSName, hubSysopName)
	_ = diagCount
	for _, w := range diagWarnings {
		warn("%s", w)
	}

	return warnings
}

// RebuildNetworkDiagrams regenerates <Network>_diags.zip for network nd from
// current fido_nodes and registers it in "<NetworkName> Nodelist Files".
// Returns the number of PNG diagrams written and any non-fatal warnings
// (e.g. graphviz dot missing).
func RebuildNetworkDiagrams(nd *NetworkDef, db *sql.DB, fileArea FileArea, bbsName, sysopName string) (int, []string) {
	if nd == nil || !nd.Enabled {
		return 0, []string{"network disabled"}
	}
	if fileArea == nil {
		return 0, []string{"file area store not available"}
	}
	if db == nil {
		return 0, []string{"database not available"}
	}
	nodes, err := OpenNodelistDB(db).ListAll(nd.Name)
	if err != nil {
		return 0, []string{err.Error()}
	}
	if len(nodes) == 0 {
		return 0, []string{"no nodelist nodes — import or generate a nodelist first"}
	}
	dirID, dirPath, err := fileArea.EnsureDir(nd.Name+" Nodelist Files", nd.Name+" Nodelist Files (auto-created)")
	if err != nil {
		return 0, []string{err.Error()}
	}
	return rebuildNetworkDiagramZip(nd, nodes, fileArea, dirID, dirPath, bbsName, sysopName)
}

func rebuildNetworkDiagramZip(nd *NetworkDef, nodes []NodeEntry, fileArea FileArea, dirID int64, dirPath, bbsName, sysopName string) (int, []string) {
	pngs, warnings := GenerateDiagramsFromNodes(nd.Name, nd, bbsName, sysopName, nodes)
	if len(pngs) == 0 {
		if len(warnings) == 0 {
			warnings = append(warnings, "no diagrams generated")
		}
		return 0, warnings
	}
	zipName := NetworkDiagZipName(nd.Name)
	diz := fmt.Sprintf("Node diagrams for %s", nd.Name)
	if err := writeMultiZipAndRegister(dirPath, dirID, fileArea, zipName, pngs, diz); err != nil {
		return len(pngs), append(warnings, fmt.Sprintf("zip %s: %v", zipName, err))
	}
	return len(pngs), warnings
}
