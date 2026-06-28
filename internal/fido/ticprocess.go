package fido

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TICProcessResult summarises inbound TIC processing.
type TICProcessResult struct {
	Processed int
	Skipped   int
	Errors    []string
}

// ProcessInboundTICs scans nd.InboundDir for .tic files, validates them,
// installs payload files into mapped file areas, and moves processed files
// to inbound/.ticdone/.
func ProcessInboundTICs(nd *NetworkDef, db *sql.DB, fileArea FileArea) *TICProcessResult {
	res := &TICProcessResult{}
	if fileArea == nil || nd == nil || !nd.Enabled {
		return res
	}
	if err := os.MkdirAll(nd.InboundDir, 0755); err != nil {
		res.Errors = append(res.Errors, err.Error())
		return res
	}
	doneDir := filepath.Join(nd.InboundDir, ".ticdone")
	_ = os.MkdirAll(doneDir, 0755)

	entries, err := os.ReadDir(nd.InboundDir)
	if err != nil {
		res.Errors = append(res.Errors, err.Error())
		return res
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".tic") {
			continue
		}
		ticPath := filepath.Join(nd.InboundDir, e.Name())
		body, err := os.ReadFile(ticPath)
		if err != nil {
			res.Errors = append(res.Errors, err.Error())
			continue
		}
		ticket, err := ParseTIC(body)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}

		payloadPath := filepath.Join(nd.InboundDir, ticket.File)
		if _, err := os.Stat(payloadPath); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: payload %q missing", e.Name(), ticket.File))
			continue
		}
		data, err := os.ReadFile(payloadPath)
		if err != nil {
			res.Errors = append(res.Errors, err.Error())
			continue
		}
		if ticket.CRC != "" && !strings.EqualFold(TICFileCRC(data), ticket.CRC) {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: CRC mismatch for %s", e.Name(), ticket.File))
			continue
		}
		if ticket.Size > 0 && int64(len(data)) != ticket.Size {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: size mismatch for %s", e.Name(), ticket.File))
			continue
		}

		fromAddr, err := ParseAddr(ticket.From)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: bad From address", e.Name()))
			continue
		}
		if !validateTICPassword(nd, fromAddr, ticket.Password) {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: TIC password rejected from %s", e.Name(), ticket.From))
			continue
		}

		dirID := int64(nd.FileDirForTag(ticket.Area))
		if dirID < 0 {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: unknown area %s", e.Name(), ticket.Area))
			continue
		}

		destName := originalFilenameFromTICFile(ticket.File)

		uploader := fmt.Sprintf("TIC %s", ticket.From)
		if err := fileArea.InstallFile(dirID, payloadPath, destName, ticket.Desc, uploader); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: install: %v", e.Name(), err))
			continue
		}

		_ = os.Rename(ticPath, filepath.Join(doneDir, e.Name()))
		_ = os.Rename(payloadPath, filepath.Join(doneDir, ticket.File))
		res.Processed++
	}
	return res
}

func validateTICPassword(nd *NetworkDef, from Addr, pw string) bool {
	if dl := nd.DownlinkByAddr(from); dl != nil {
		if dl.Password == "" {
			return true
		}
		return pw == dl.Password
	}
	uplink := nd.UplinkAddr()
	if uplink != (Addr{}) && addrsEqualBoss(from, uplink) {
		if nd.TicPassword == "" {
			return true
		}
		return pw == nd.TicPassword
	}
	// Accept from any configured downlink without password if not in list — reject
	return false
}

func addrsEqualBoss(a, b Addr) bool {
	return a.Zone == b.Zone && a.Net == b.Net && a.Node == b.Node
}

func originalFilenameFromTICFile(ticFile string) string {
	base := filepath.Base(ticFile)
	if idx := strings.LastIndex(base, "."); idx > 0 {
		if u := strings.LastIndex(base[:idx], "_"); u >= 0 {
			return sanitizeTICPayloadName(base[u+1:])
		}
	}
	return sanitizeTICPayloadName(base)
}
