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
//   v0.0.1  2026-06-24  Initial implementation
// ============================================================================

// Package files manages the VirtBBS file directory system.
package files

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Dir represents a file directory (section).
type Dir struct {
	ID           int64
	Name         string
	Description  string
	Path         string // relative to files root
	SortType     int
	ReadSec      int
	UploadSec    int
	ConferenceID *int
	Active       bool
}

// File represents an entry in a file directory.
type File struct {
	ID          int64
	DirID       int64
	Filename    string
	Size        int64
	Description string
	Uploader    string
	UploadDate  string
	Downloads   int
}

// Store manages file directories in SQLite.
type Store struct {
	db       *sql.DB
	filesRoot string
}

// Open opens the database, applies the schema, and sets the files root path.
func Open(dbPath, filesRoot string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("files schema: %w", err)
	}
	return &Store{db: db, filesRoot: filesRoot}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// ListDirs returns all active file directories.
func (s *Store) ListDirs() ([]*Dir, error) {
	rows, err := s.db.Query(`SELECT id, name, description, path, sort_type, read_sec, upload_sec, active FROM file_dirs WHERE active=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Dir
	for rows.Next() {
		d := &Dir{}
		var active int
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.Path, &d.SortType, &d.ReadSec, &d.UploadSec, &active); err != nil {
			return nil, err
		}
		d.Active = active != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDir fetches a single directory by ID.
func (s *Store) GetDir(id int64) (*Dir, error) {
	row := s.db.QueryRow(`SELECT id, name, description, path, sort_type, read_sec, upload_sec, active FROM file_dirs WHERE id=?`, id)
	d := &Dir{}
	var active int
	if err := row.Scan(&d.ID, &d.Name, &d.Description, &d.Path, &d.SortType, &d.ReadSec, &d.UploadSec, &active); err != nil {
		return nil, err
	}
	d.Active = active != 0
	return d, nil
}

// GetDirByName finds an active directory by its exact name, or nil if none
// exists. See EnsureDir for the find-or-create wrapper around this.
func (s *Store) GetDirByName(name string) (*Dir, error) {
	row := s.db.QueryRow(`SELECT id, name, description, path, sort_type, read_sec, upload_sec, active FROM file_dirs WHERE name=? AND active=1`, name)
	d := &Dir{}
	var active int
	if err := row.Scan(&d.ID, &d.Name, &d.Description, &d.Path, &d.SortType, &d.ReadSec, &d.UploadSec, &active); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	d.Active = active != 0
	return d, nil
}

// CreateDir adds a new file directory. path is relative to the files root
// (see EnsureDirPath to create the on-disk directory afterward).
func (s *Store) CreateDir(name, description, path string, readSec, uploadSec int) (*Dir, error) {
	res, err := s.db.Exec(`INSERT INTO file_dirs (name, description, path, sort_type, read_sec, upload_sec, active)
		VALUES (?,?,?,0,?,?,1)`, name, description, path, readSec, uploadSec)
	if err != nil {
		return nil, fmt.Errorf("create file dir %q: %w", name, err)
	}
	id, _ := res.LastInsertId()
	return &Dir{ID: id, Name: name, Description: description, Path: path, ReadSec: readSec, UploadSec: uploadSec, Active: true}, nil
}

// EnsureDir finds a file area literally named name, creating it (and its
// on-disk directory) with sane defaults if it doesn't exist yet. Returns
// its ID and the absolute on-disk path callers can write payload files
// into directly before RegisterUpload.
func (s *Store) EnsureDir(name, description string) (dirID int64, dirPath string, err error) {
	d, err := s.GetDirByName(name)
	if err != nil {
		return 0, "", err
	}
	if d == nil {
		d, err = s.CreateDir(name, description, sanitizeDirPath(name), 0, 0)
		if err != nil {
			return 0, "", err
		}
	}
	if err := s.EnsureDirPath(d.ID); err != nil {
		return 0, "", err
	}
	return d.ID, filepath.Join(s.filesRoot, s.dirPath(d.ID)), nil
}

func sanitizeDirPath(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

// ListFiles returns files in a directory, sorted per the dir's SortType.
func (s *Store) ListFiles(dirID int64) ([]*File, error) {
	rows, err := s.db.Query(`SELECT id, dir_id, filename, size, description, uploader, upload_date, downloads FROM files WHERE dir_id=? ORDER BY filename`, dirID)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Get the dir's sort type
	dir, _ := s.GetDir(dirID)
	if dir != nil {
		sortFiles(out, dir.SortType)
	}
	return out, nil
}

// Search finds files whose name or description contains query (case-insensitive).
func (s *Store) Search(query string) ([]*File, error) {
	q := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.Query(`SELECT id, dir_id, filename, size, description, uploader, upload_date, downloads
		FROM files WHERE lower(filename) LIKE ? OR lower(description) LIKE ? ORDER BY filename`, q, q)
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

// RegisterUpload adds a file record to the database after a successful upload.
func (s *Store) RegisterUpload(dirID int64, filename, description, uploader string) error {
	path := filepath.Join(s.filesRoot, s.dirPath(dirID), filename)
	info, err := os.Stat(path)
	size := int64(0)
	if err == nil {
		size = info.Size()
	}
	_, err = s.db.Exec(`INSERT OR REPLACE INTO files (dir_id, filename, size, description, uploader, upload_date)
		VALUES (?,?,?,?,?,?)`,
		dirID, filename, size, description, uploader, time.Now().Format("2006-01-02"))
	return err
}

// IncrementDownloads records a download.
func (s *Store) IncrementDownloads(fileID int64) error {
	_, err := s.db.Exec(`UPDATE files SET downloads=downloads+1 WHERE id=?`, fileID)
	return err
}

// AbsPath returns the absolute filesystem path for a file.
func (s *Store) AbsPath(dirID int64, filename string) string {
	return filepath.Join(s.filesRoot, s.dirPath(dirID), filename)
}

// EnsureDirPath creates the directory on disk if it doesn't exist.
func (s *Store) EnsureDirPath(dirID int64) error {
	path := filepath.Join(s.filesRoot, s.dirPath(dirID))
	return os.MkdirAll(path, 0755)
}

// UploadDir returns the path for storing an uploaded file.
func (s *Store) UploadDir(dirID int64) string {
	return filepath.Join(s.filesRoot, s.dirPath(dirID))
}

func (s *Store) dirPath(dirID int64) string {
	row := s.db.QueryRow(`SELECT path FROM file_dirs WHERE id=?`, dirID)
	var p string
	_ = row.Scan(&p)
	if p == "" {
		p = fmt.Sprintf("dir%d", dirID)
	}
	return p
}

func sortFiles(files []*File, sortType int) {
	switch sortType {
	case 1: // name asc
		sort.Slice(files, func(i, j int) bool { return files[i].Filename < files[j].Filename })
	case 2: // date asc
		sort.Slice(files, func(i, j int) bool { return files[i].UploadDate < files[j].UploadDate })
	case 3: // name desc
		sort.Slice(files, func(i, j int) bool { return files[i].Filename > files[j].Filename })
	case 4: // date desc
		sort.Slice(files, func(i, j int) bool { return files[i].UploadDate > files[j].UploadDate })
	}
}

// FormatSize returns a human-readable file size string.
func FormatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%5.1fM", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%5.1fK", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%5d ", bytes)
	}
}
