package fido

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/messages"
)

const (
	nodelistAppliedSourceFile  = "file"
	nodelistAppliedSourceEcho  = "echo"
	nodelistAppliedSourceQueue = "queue"
)

// MonitorNetworkNodelists scans the pending echo queue, the "<Network> Nodelist
// Files" area, and the "<Network> Nodelists" conference for nodelists and
// diffs newer than the current import, applying them automatically.
func MonitorNetworkNodelists(db *sql.DB, confStore *conferences.Store, msgStore *messages.Store,
	fileArea FileArea, nd *NetworkDef, bbsName, sysopName string, telnetPort int) []string {
	if nd == nil || !nd.Enabled {
		return nil
	}
	var warnings []string
	warn := func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }

	if fileArea != nil {
		for _, w := range processPendingNodelistEchoes(db, fileArea, nd.Name, nd, bbsName, sysopName) {
			warn("%s", w)
		}
	}

	if nd.UsesMemberNodelist() && ShouldPreserveImportedNodelist(db, nd) {
		return warnings
	}

	if fileArea != nil {
		if err := monitorNodelistFileArea(db, fileArea, nd, bbsName, sysopName, telnetPort); err != nil {
			warn("file area: %v", err)
		}
	}
	if confStore != nil && msgStore != nil {
		if err := monitorNodelistConference(db, confStore, msgStore, fileArea, nd, bbsName, sysopName, telnetPort); err != nil {
			warn("conference: %v", err)
		}
	}
	return warnings
}

func monitorNodelistFileArea(db *sql.DB, fileArea FileArea, nd *NetworkDef, bbsName, sysopName string, telnetPort int) error {
	dirName := nd.Name + " Nodelist Files"
	dirID, _, err := fileArea.EnsureDir(dirName, dirName+" (auto-created)")
	if err != nil {
		return err
	}
	files, err := fileArea.ListAreaFiles(dirID)
	if err != nil {
		return err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})

	for _, f := range files {
		if skipOwnHubNodelist(nd, f.Uploader, "") {
			continue
		}
		filename := filepath.Base(f.Filename)
		if !IsFullNodelistFilename(filename) && !IsNodelistDiffFilename(filename) {
			if strings.HasSuffix(strings.ToLower(filename), ".zip") {
				if err := tryApplyNodelistZip(db, nd, f.FullPath, f.ModTime, fileArea); err != nil {
					return err
				}
			}
			continue
		}
		sourceKey := filename
		if nodelistAlreadyApplied(db, nd.Name, nodelistAppliedSourceFile, sourceKey) {
			continue
		}
		zSuffix, _ := ParseNodelistZSuffix(filename)
		if !nodelistCandidateIsNewer(db, nd.Name, f.ModTime, zSuffix) {
			continue
		}
		if err := applyNodelistArtifact(db, fileArea, nd, filename, f.FullPath, bbsName, sysopName, telnetPort); err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		if err := recordNodelistApplied(db, nd.Name, nodelistAppliedSourceFile, sourceKey, filename); err != nil {
			return err
		}
	}
	return nil
}

func tryApplyNodelistZip(db *sql.DB, nd *NetworkDef, zipPath string, modTime time.Time, fileArea FileArea) error {
	sourceKey := filepath.Base(zipPath)
	if nodelistAlreadyApplied(db, nd.Name, nodelistAppliedSourceFile, sourceKey) {
		return nil
	}
	zSuffix, _ := ParseNodelistZSuffix(sourceKey)
	if !nodelistCandidateIsNewer(db, nd.Name, modTime, zSuffix) {
		return nil
	}
	plainPath, cleanup, err := prepareNodelistImportPath(zipPath)
	if err != nil {
		return nil
	}
	defer cleanup()
	if _, err := ImportFile(db, plainPath, nd.Name); err != nil {
		return err
	}
	_ = RestoreLocalNodeEntries(db, nd, "", "", "Internet", 0)
	return recordNodelistApplied(db, nd.Name, nodelistAppliedSourceFile, sourceKey, sourceKey)
}

func monitorNodelistConference(db *sql.DB, confStore *conferences.Store, msgStore *messages.Store,
	fileArea FileArea, nd *NetworkDef, bbsName, sysopName string, telnetPort int) error {
	confName := nd.Name + " Nodelists"
	conf, err := confStore.GetByName(confName)
	if err != nil || conf == nil {
		return err
	}
	msgs, err := msgStore.List(conf.ID, 100, 0)
	if err != nil {
		return err
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		subject := strings.TrimSpace(m.Subject)
		upper := strings.ToUpper(subject)
		if !strings.Contains(upper, "NODELIST") {
			continue
		}
		if skipOwnHubNodelist(nd, "", m.FromName) {
			continue
		}
		sourceKey := fmt.Sprintf("%d", m.ID)
		if nodelistAlreadyApplied(db, nd.Name, nodelistAppliedSourceEcho, sourceKey) {
			continue
		}
		filename := nodelistFilenameFromSubject(subject)
		zSuffix, _ := ParseNodelistZSuffix(filename)
		refTime := m.DatePosted
		if refTime.IsZero() {
			refTime = time.Now()
		}
		if !nodelistCandidateIsNewer(db, nd.Name, refTime, zSuffix) {
			continue
		}
		if fileArea == nil {
			continue
		}
		dirID, dirPath, err := fileArea.EnsureDir(nd.Name+" Nodelist Files", nd.Name+" Nodelist Files (auto-created)")
		if err != nil {
			return err
		}
		fullPath := dirPath + "/" + filename
		if err := os.WriteFile(fullPath, []byte(m.Body), 0644); err != nil {
			return err
		}
		_ = fileArea.RegisterUpload(dirID, filename, nd.Name+" nodelist (conference)", "VirtBBS")
		if err := applyNodelistArtifact(db, fileArea, nd, filename, fullPath, bbsName, sysopName, telnetPort); err != nil {
			return fmt.Errorf("msg %d: %w", m.ID, err)
		}
		if err := recordNodelistApplied(db, nd.Name, nodelistAppliedSourceEcho, sourceKey, filename); err != nil {
			return err
		}
	}
	return nil
}

func applyNodelistArtifact(db *sql.DB, fileArea FileArea, nd *NetworkDef, filename, fullPath, bbsName, sysopName string, telnetPort int) error {
	if IsFullNodelistFilename(filename) {
		if _, err := ImportFile(db, fullPath, nd.Name); err != nil {
			return err
		}
		_ = RestoreLocalNodeEntries(db, nd, bbsName, sysopName, "Internet", telnetPort)
		if fileArea != nil && bbsName != "" {
			if _, warns := RebuildNetworkDiagrams(nd, db, fileArea, bbsName, sysopName); len(warns) > 0 {
				return fmt.Errorf("diagrams: %s", strings.Join(warns, "; "))
			}
		}
		return nil
	}
	if IsNodelistDiffFilename(filename) {
		return ApplyNodelistDiffFile(db, nd.Name, fullPath, nd)
	}
	return nil
}

// markPendingEchoApplied records a processed queue entry so conference/file
// scans do not duplicate work.
func markPendingEchoApplied(db *sql.DB, network, filename string, echoID int64) {
	_ = recordNodelistApplied(db, network, nodelistAppliedSourceQueue, fmt.Sprintf("%d", echoID), filename)
}
