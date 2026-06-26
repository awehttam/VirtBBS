package files

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	LocalFileDirID   int64 = 1
	LocalFileZipName       = "LOCALFIL.ZIP"
	localFileTextName      = "LOCALFIL.TXT"
	localFileDizName       = "FILE_ID.DIZ"
)

// BuildLocalFile creates LOCALFIL.ZIP in file directory 1 (General) containing
// an SLDIR-style text listing of all files and a FILE_ID.DIZ description.
func (s *Store) BuildLocalFile(bbsName string) error {
	if bbsName == "" {
		bbsName = "VirtBBS"
	}
	if _, err := s.GetDir(LocalFileDirID); err != nil {
		return fmt.Errorf("local file directory %d: %w", LocalFileDirID, err)
	}
	if err := s.EnsureDirPath(LocalFileDirID); err != nil {
		return err
	}

	dirs, err := s.ListDirs()
	if err != nil {
		return err
	}

	now := time.Now()
	listing := buildSLDIRListing(bbsName, now, dirs, func(dirID int64) ([]*File, error) {
		return s.listFilesRaw(dirID)
	})

	diz := fmt.Sprintf("List of all files on %s on %s and %s",
		bbsName, now.Format("01-02-2006"), now.Format("3:04 PM"))

	zipPath := s.AbsPath(LocalFileDirID, LocalFileZipName)
	if err := writeLocalFileZip(zipPath, listing, diz); err != nil {
		return err
	}
	return s.RegisterUpload(LocalFileDirID, LocalFileZipName, diz, "Sysop")
}

func buildSLDIRListing(bbsName string, now time.Time, dirs []*Dir, listFn func(int64) ([]*File, error)) string {
	var sb strings.Builder
	headerDate := now.Format("1-02-2006")
	fmt.Fprintf(&sb, "%s Directory Listing for %-21s [SLDIR ? for Help]\r\n\r\n", bbsName, headerDate)
	sb.WriteString(" Filename   St  Size  D/Ls  Date      Description\r\n")
	sb.WriteString("------------------------------------------------------------------------------\r\n")

	for _, d := range dirs {
		files, err := listFn(d.ID)
		if err != nil || len(files) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "\r\nDirectory: %s\r\n\r\n", d.Name)
		for _, f := range files {
			if strings.EqualFold(f.Filename, LocalFileZipName) {
				continue
			}
			sb.WriteString(formatSLDIRLine(f))
			sb.WriteString("\r\n")
		}
	}
	return sb.String()
}

func formatSLDIRLine(f *File) string {
	name := f.Filename
	if len(name) > 16 {
		name = name[:16]
	}
	desc := f.Description
	if desc == "" {
		desc = defaultScanDesc
	}
	return fmt.Sprintf("%-16s%7s%4d  %-10s %s",
		name,
		formatSLDIRSize(f.Size),
		f.Downloads,
		formatSLDIRDate(f.UploadDate),
		desc,
	)
}

func formatSLDIRSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%6.1fM", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%6.1fK", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%6d ", bytes)
	}
}

func formatSLDIRDate(uploadDate string) string {
	t, err := time.Parse("2006-01-02", uploadDate)
	if err != nil {
		return uploadDate
	}
	return t.Format("1-02-2006")
}

func writeLocalFileZip(path, listing, diz string) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	txt, err := zw.Create(localFileTextName)
	if err != nil {
		return err
	}
	if _, err := txt.Write([]byte(listing)); err != nil {
		return err
	}

	dizf, err := zw.Create(localFileDizName)
	if err != nil {
		return err
	}
	if _, err := dizf.Write([]byte(diz)); err != nil {
		return err
	}

	if err := zw.Close(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}