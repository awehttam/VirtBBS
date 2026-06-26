package files

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	defaultScanDesc = "No Description"
	missingFileDesc = "<Missing File>"
	maxDizLen       = 80
)

// ScanResult summarizes changes made to one file directory.
// AddedFileInfo records a file newly registered by a directory scan.
type AddedFileInfo struct {
	Filename string
	Size     int64
}

type ScanResult struct {
	DirID      int64
	DirName    string
	Added      int
	Missing    int
	Restored   int
	OnDisk     int
	AddedFiles []AddedFileInfo
}

// ScanTotals aggregates a full scan across directories.
type ScanTotals struct {
	Dirs    int
	Added   int
	Missing int
	Results []ScanResult
}

// ScanAll walks every active file directory on disk and synchronises the catalog.
func (s *Store) ScanAll(uploader string) (*ScanTotals, error) {
	dirs, err := s.ListDirs()
	if err != nil {
		return nil, err
	}
	if uploader == "" {
		uploader = "Sysop"
	}

	totals := &ScanTotals{}
	for _, d := range dirs {
		res, err := s.ScanDir(d.ID, uploader)
		if err != nil {
			return totals, fmt.Errorf("scan dir %q: %w", d.Name, err)
		}
		totals.Dirs++
		totals.Added += res.Added
		totals.Missing += res.Missing
		totals.Results = append(totals.Results, *res)
	}
	return totals, nil
}

// ScanDir synchronises one file directory: registers new disk files and marks
// database entries whose files are no longer on disk.
func (s *Store) ScanDir(dirID int64, uploader string) (*ScanResult, error) {
	dir, err := s.GetDir(dirID)
	if err != nil {
		return nil, err
	}
	if uploader == "" {
		uploader = "Sysop"
	}

	dirPath := filepath.Join(s.filesRoot, dir.Path)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, fmt.Errorf("ensure dir path: %w", err)
	}

	onDisk, err := listDirFiles(dirPath)
	if err != nil {
		return nil, err
	}

	catalog, err := s.listFilesRaw(dirID)
	if err != nil {
		return nil, err
	}

	catalogByName := make(map[string]*File, len(catalog))
	for _, f := range catalog {
		catalogByName[strings.ToLower(f.Filename)] = f
	}

	res := &ScanResult{DirID: dirID, DirName: dir.Name, OnDisk: len(onDisk)}

	for nameLower, filename := range onDisk {
		path := filepath.Join(dirPath, filename)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}

		if existing, ok := catalogByName[nameLower]; ok {
			if existing.Description == missingFileDesc {
				desc := describeFile(path)
				if err := s.restoreFromDisk(dirID, filename, info.Size(), desc); err != nil {
					return res, err
				}
				res.Restored++
			}
			continue
		}

		desc := describeFile(path)
		if err := s.insertScanned(dirID, filename, info.Size(), desc, uploader); err != nil {
			return res, err
		}
		res.Added++
		res.AddedFiles = append(res.AddedFiles, AddedFileInfo{
			Filename: filename,
			Size:     info.Size(),
		})
	}

	for nameLower, f := range catalogByName {
		if _, ok := onDisk[nameLower]; ok {
			continue
		}
		if err := s.markMissing(dirID, f.Filename); err != nil {
			return res, err
		}
		res.Missing++
	}

	return res, nil
}

func (s *Store) listFilesRaw(dirID int64) ([]*File, error) {
	rows, err := s.db.Query(`SELECT id, dir_id, filename, size, description, uploader, upload_date, downloads
		FROM files WHERE dir_id=?`, dirID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*File
	for rows.Next() {
		f := &File{}
		if err := rows.Scan(&f.ID, &f.DirID, &f.Filename, &f.Size, &f.Description, &f.Uploader, &f.UploadDate, &f.Downloads); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) insertScanned(dirID int64, filename string, size int64, description, uploader string) error {
	_, err := s.db.Exec(`INSERT INTO files (dir_id, filename, size, description, uploader, upload_date, flagged)
		VALUES (?,?,?,?,?,?,0)`,
		dirID, filename, size, description, uploader, time.Now().Format("2006-01-02"))
	return err
}

func (s *Store) markMissing(dirID int64, filename string) error {
	_, err := s.db.Exec(`UPDATE files SET description=?, size=0, flagged=1 WHERE dir_id=? AND filename=?`,
		missingFileDesc, dirID, filename)
	return err
}

func (s *Store) restoreFromDisk(dirID int64, filename string, size int64, description string) error {
	_, err := s.db.Exec(`UPDATE files SET size=?, description=?, flagged=0 WHERE dir_id=? AND filename=?`,
		size, description, dirID, filename)
	return err
}

// listDirFiles returns a map of lowercase name → actual filename for regular files.
func listDirFiles(dirPath string) (map[string]string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out[strings.ToLower(e.Name())] = e.Name()
	}
	return out, nil
}

func describeFile(path string) string {
	if diz := readArchiveDiz(path); diz != "" {
		return diz
	}
	return defaultScanDesc
}

func readArchiveDiz(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip":
		return readZipDiz(path)
	default:
		return ""
	}
}

func readZipDiz(path string) string {
	r, err := zip.OpenReader(path)
	if err != nil {
		return ""
	}
	defer r.Close()

	for _, f := range r.File {
		base := strings.ToLower(filepath.Base(f.Name))
		if base != "file_id.diz" && base != "file.diz" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, 4096))
		rc.Close()
		if err != nil {
			continue
		}
		if desc := normalizeDiz(data); desc != "" {
			return desc
		}
	}
	return ""
}

func normalizeDiz(data []byte) string {
	s := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' {
			return ' '
		}
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, string(data))

	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)
	if len(s) > maxDizLen {
		s = s[:maxDizLen]
		s = strings.TrimRight(s, " ,.;:-")
	}
	if !hasVisibleText(s) {
		return ""
	}
	return s
}

func hasVisibleText(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}