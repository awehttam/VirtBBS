package fido

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileScanResult summarises a TIC file-scan run.
type FileScanResult struct {
	Files    int // source files exported
	TICFiles int // .tic control files written
	Errors   []string
}

// FileRescanResult summarises a FileFix-triggered backlog rescan to one downlink.
type FileRescanResult struct {
	Files    int
	TICFiles int
	Errors   []string
}

// FileScanAll exports unexported files from every configured file area via TIC.
func FileScanAll(cfg *Config, db *sql.DB, filesRoot string) (*FileScanResult, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("FidoNet is disabled in config")
	}
	total := &FileScanResult{}
	for _, nd := range cfg.AllNetworks() {
		if !nd.Enabled || len(nd.FileAreas) == 0 {
			continue
		}
		r, err := fileScanNetwork(&nd, db, filesRoot)
		if err != nil {
			total.Errors = append(total.Errors, fmt.Sprintf("[%s] %v", nd.Name, err))
			continue
		}
		total.Files += r.Files
		total.TICFiles += r.TICFiles
		total.Errors = append(total.Errors, r.Errors...)
	}
	return total, nil
}

func fileScanNetwork(nd *NetworkDef, db *sql.DB, filesRoot string) (*FileScanResult, error) {
	result := &FileScanResult{}
	if err := os.MkdirAll(nd.OutboundDir, 0755); err != nil {
		return nil, err
	}
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return nil, fmt.Errorf("invalid local address %q", nd.Address)
	}
	uplink := nd.UplinkAddr()
	if uplink == (Addr{}) && !nd.IsHub() {
		return nil, fmt.Errorf("no uplink configured for network %s", nd.Name)
	}

	filefixDB := OpenFileFixDB(db)
	exportDB := OpenTICExportDB(db)

	type destBucket struct {
		addr Addr
		tag  string
	}
	buckets := map[string][]ticOutboundItem{}
	dests := map[string]destBucket{}

	addDest := func(addr Addr) string {
		key := addr.String()
		if _, ok := dests[key]; !ok {
			dests[key] = destBucket{addr: addr, tag: sanitizeAddrForFilename(addr)}
		}
		return key
	}

	if uplink != (Addr{}) {
		addDest(uplink)
	}

	for tag, dirID := range nd.FileAreas {
		dirID64 := int64(dirID)
		files, err := queryCatalogFiles(db, dirID64)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("dir %d: %v", dirID, err))
			continue
		}
		dirRel, err := queryDirPath(db, dirID64)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("dir %d path: %v", dirID, err))
			continue
		}
		srcDir := filepath.Join(filesRoot, dirRel)

		for _, f := range files {
			exported, err := exportDB.IsExported(nd.Name, dirID64, f.Filename)
			if err != nil || exported {
				continue
			}
			srcPath := filepath.Join(srcDir, f.Filename)
			info, err := os.Stat(srcPath)
			if err != nil || info.IsDir() {
				continue
			}
			data, err := os.ReadFile(srcPath)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", f.Filename, err))
				continue
			}

			item := ticOutboundItem{
				areaTag:  strings.ToUpper(tag),
				srcPath:  srcPath,
				filename: f.Filename,
				desc:     f.Description,
				size:     info.Size(),
				crc:      TICFileCRC(data),
				dirID:    dirID64,
			}

			// Uplink bucket
			if uplink != (Addr{}) {
				key := addDest(uplink)
				buckets[key] = append(buckets[key], item)
			}

			// Subscribed downlinks
			downlinks, _ := filefixDB.SubscribedDownlinks(nd.Name, item.areaTag)
			for _, addrStr := range downlinks {
				a, err := ParseAddr(addrStr)
				if err != nil || nd.DownlinkByAddr(a) == nil {
					continue
				}
				key := addDest(a)
				buckets[key] = append(buckets[key], item)
			}

			result.Files++
		}
	}

	tsBase := time.Now().Format("20060102150405.000000")
	for key, items := range buckets {
		if len(items) == 0 {
			continue
		}
		d := dests[key]
		prefix := fmt.Sprintf("%s_%s_%s", nd.Name, d.tag, tsBase)
		tics, errs := writeTICItemsForDest(nd, our, d.addr, uplink, items, prefix)
		result.TICFiles += tics
		result.Errors = append(result.Errors, errs...)
	}

	// Mark exported once at least one bucket succeeded for that file
	seen := map[string]bool{}
	for _, items := range buckets {
		for _, item := range items {
			mk := fmt.Sprintf("%d/%s", item.dirID, item.filename)
			if seen[mk] {
				continue
			}
			seen[mk] = true
			if err := exportDB.MarkExported(nd.Name, item.dirID, item.filename); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("mark exported %s: %v", item.filename, err))
			}
		}
	}

	return result, nil
}

// RescanFilesToDownlink exports files from fileTags to a single downlink via TIC,
// including files already marked exported. Unlike FileScanAll it does not update
// fido_file_exports, so normal file-scan uplink behaviour is unchanged.
// maxFiles 0 sends the full catalog backlog per area (oldest by filename).
func RescanFilesToDownlink(nd *NetworkDef, db *sql.DB, filesRoot, downlinkAddr string, fileTags []string, maxFiles int) (*FileRescanResult, error) {
	result := &FileRescanResult{}
	if filesRoot == "" {
		return result, fmt.Errorf("filefix rescan: files root path required")
	}
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return result, fmt.Errorf("filefix rescan: invalid local address %q", nd.Address)
	}
	dlAddr, err := ParseAddr(downlinkAddr)
	if err != nil {
		return result, fmt.Errorf("filefix rescan: invalid downlink address %q: %w", downlinkAddr, err)
	}
	if nd.DownlinkByAddr(dlAddr) == nil {
		return result, fmt.Errorf("filefix rescan: %s is not a configured downlink", downlinkAddr)
	}
	if err := os.MkdirAll(nd.OutboundDir, 0755); err != nil {
		return result, err
	}

	uplink := nd.UplinkAddr()
	var items []ticOutboundItem
	for _, tag := range fileTags {
		tag = strings.ToUpper(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		dirID, ok := nd.FileAreas[tag]
		if !ok {
			result.Errors = append(result.Errors, fmt.Sprintf("rescan: unknown file area %s", tag))
			continue
		}
		dirID64 := int64(dirID)
		catalog, err := queryCatalogFiles(db, dirID64)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("rescan %s: %v", tag, err))
			continue
		}
		if maxFiles > 0 && len(catalog) > maxFiles {
			catalog = catalog[:maxFiles]
		}
		dirRel, err := queryDirPath(db, dirID64)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("rescan %s path: %v", tag, err))
			continue
		}
		srcDir := filepath.Join(filesRoot, dirRel)
		for _, f := range catalog {
			srcPath := filepath.Join(srcDir, f.Filename)
			info, err := os.Stat(srcPath)
			if err != nil || info.IsDir() {
				continue
			}
			data, err := os.ReadFile(srcPath)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", f.Filename, err))
				continue
			}
			items = append(items, ticOutboundItem{
				areaTag:  tag,
				srcPath:  srcPath,
				filename: f.Filename,
				desc:     f.Description,
				size:     info.Size(),
				crc:      TICFileCRC(data),
				dirID:    dirID64,
			})
		}
	}

	if len(items) == 0 {
		return result, nil
	}

	destTag := sanitizeAddrForFilename(dlAddr)
	tsBase := time.Now().Format("20060102150405.000000")
	prefix := fmt.Sprintf("%s_%s_rescan_%s", nd.Name, destTag, tsBase)
	tics, errs := writeTICItemsForDest(nd, our, dlAddr, uplink, items, prefix)
	result.TICFiles = tics
	result.Files = len(items)
	result.Errors = append(result.Errors, errs...)
	return result, nil
}

func writeTICItemsForDest(nd *NetworkDef, our, dest, uplink Addr, items []ticOutboundItem, batchPrefix string) (ticCount int, errs []string) {
	for i, item := range items {
		batch := fmt.Sprintf("%s_%04d", batchPrefix, i)
		payloadName := batch + "_" + sanitizeTICPayloadName(item.filename)
		payloadPath := filepath.Join(nd.OutboundDir, payloadName)
		if err := copyFile(item.srcPath, payloadPath); err != nil {
			errs = append(errs, fmt.Sprintf("copy %s: %v", item.filename, err))
			continue
		}

		ticket := &TICTicket{
			Area:   item.areaTag,
			Origin: our.String(),
			From:   our.String(),
			File:   payloadName,
			Desc:   item.desc,
			Size:   item.size,
			CRC:    item.crc,
			Path:   fmt.Sprintf("%d/%d", our.Net, our.Node),
			SeenBy: fmt.Sprintf("%d/%d", our.Net, our.Node),
		}
		if nd.TicPassword != "" && uplink != (Addr{}) && dest == uplink {
			ticket.Password = nd.TicPassword
		}
		if dl := nd.DownlinkByAddr(dest); dl != nil && dl.Password != "" {
			ticket.Password = dl.Password
		}

		ticPath := filepath.Join(nd.OutboundDir, batch+".tic")
		if err := os.WriteFile(ticPath, FormatTIC(ticket), 0644); err != nil {
			errs = append(errs, fmt.Sprintf("write tic %s: %v", batch, err))
			_ = os.Remove(payloadPath)
			continue
		}
		ticCount++
	}
	return ticCount, errs
}

type ticOutboundItem struct {
	areaTag  string
	srcPath  string
	filename string
	desc     string
	size     int64
	crc      string
	dirID    int64
}

type catalogFile struct {
	Filename    string
	Description string
}

func queryCatalogFiles(db *sql.DB, dirID int64) ([]catalogFile, error) {
	rows, err := db.Query(`SELECT filename, description FROM files WHERE dir_id=? ORDER BY filename`, dirID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalogFile
	for rows.Next() {
		var f catalogFile
		if err := rows.Scan(&f.Filename, &f.Description); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func queryDirPath(db *sql.DB, dirID int64) (string, error) {
	var p string
	err := db.QueryRow(`SELECT path FROM file_dirs WHERE id=?`, dirID).Scan(&p)
	if err != nil {
		return "", err
	}
	if p == "" {
		p = fmt.Sprintf("dir%d", dirID)
	}
	return p, nil
}

func sanitizeTICPayloadName(name string) string {
	name = filepath.Base(name)
	return strings.NewReplacer(":", "_", "/", "_", "\\", "_").Replace(name)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
