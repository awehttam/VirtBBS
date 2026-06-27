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

// RunDayRollover regenerates everything VirtNet publishes once per day for
// a hub network nd: the full nodelist, its diff against yesterday, a
// snapshot for tomorrow's diff, the NodeChgs.txt change log, and the node
// diagrams — posting the nodelist text into the network's echo conference
// (auto-created, auto-wired to nd.EffectiveNodelistEchoTag) and registering
// every generated file into the network's Nodelist Files area (also
// auto-created). Only meaningful for hub networks (nd.IsHub()); returns
// non-fatal warnings rather than failing outright on any one step, so e.g.
// a missing `dot` binary doesn't block the nodelist itself from publishing.
func RunDayRollover(nd *NetworkDef, db *sql.DB, confStore *conferences.Store, msgStore *messages.Store,
	fileArea FileArea, hubBBSName, hubSysopName string) []string {
	var warnings []string
	warn := func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }

	nlConf, err := EnsureEchoConference(confStore, nd.Name+" Nodelists", nd.Name, nd.EffectiveNodelistEchoTag())
	if err != nil {
		warn("ensure Nodelists conference: %v", err)
		return warnings
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

	if err := msgStore.Post(&messages.Message{
		ConferenceID: nlConf.ID,
		FromName:     "VirtBBS NodeAnnounce",
		ToName:       "All",
		Subject:      fmt.Sprintf("VirtNet Nodelist %s", fullName[len("VirtNode."):]),
		Status:       "A",
		Echo:         true,
		Body:         string(fullData),
	}); err != nil {
		warn("post nodelist message: %v", err)
	}
	if diffData != nil {
		if err := msgStore.Post(&messages.Message{
			ConferenceID: nlConf.ID,
			FromName:     "VirtBBS NodeAnnounce",
			ToName:       "All",
			Subject:      fmt.Sprintf("VirtNet Nodelist Diff %s", diffName[len("VirtNode."):]),
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
	writeAndRegister(fullName, fullData)
	if diffData != nil {
		writeAndRegister(diffName, diffData)
	}

	if chgsText, err := BuildNodeChgsText(db, nd.Name); err != nil {
		warn("build NodeChgs.txt: %v", err)
	} else if err := writeZipAndRegister(dirPath, dirID, fileArea, "NodeChgs.zip", "NodeChgs.txt",
		[]byte(chgsText), "Nodelist Changes"); err != nil {
		warn("zip NodeChgs.txt: %v", err)
	}

	mdb := OpenMembersDB(db)
	members, err := mdb.ListMembers(nd.Name)
	if err != nil {
		warn("list members for diagrams: %v", err)
		return warnings
	}
	pngs, diagWarnings := GenerateDiagrams(nd.NodeAddr(), hubBBSName, hubSysopName, members)
	warnings = append(warnings, diagWarnings...)
	if len(pngs) > 0 {
		if err := writeMultiZipAndRegister(dirPath, dirID, fileArea, "VirtDiag.zip", pngs, "Node Diagrams for VirtNet"); err != nil {
			warn("zip VirtDiag.zip: %v", err)
		}
	}

	return warnings
}
