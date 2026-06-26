package files

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBuildLocalFile_createsZipWithListing(t *testing.T) {
	root := t.TempDir()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	store := &Store{db: db, filesRoot: root}
	dirPath := filepath.Join(root, "general")
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "DEMO.ZIP"), []byte("demo"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO files (dir_id, filename, size, description, uploader, upload_date)
		VALUES (1, 'DEMO.ZIP', 4, 'Demo archive', 'Sysop', '2026-06-26')`); err != nil {
		t.Fatal(err)
	}

	if err := store.BuildLocalFile("Larry's Farm BBS"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dirPath, LocalFileZipName))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	var listing, diz string
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		switch strings.ToUpper(f.Name) {
		case localFileTextName:
			listing = string(body)
		case localFileDizName:
			diz = string(body)
		}
	}
	if !strings.Contains(listing, "Larry's Farm BBS Directory Listing") {
		t.Fatalf("listing missing header: %q", listing[:min(80, len(listing))])
	}
	if !strings.Contains(listing, "DEMO.ZIP") {
		t.Fatalf("listing missing demo file")
	}
	if !strings.Contains(diz, "Larry's Farm BBS") {
		t.Fatalf("diz = %q", diz)
	}
}

func TestFormatSLDIRLine(t *testing.T) {
	line := formatSLDIRLine(&File{
		Filename:    "35MMCAM.ZIP",
		Size:        55296,
		Downloads:   2,
		UploadDate:  "2008-09-01",
		Description: "35mm camera for Lwave",
	})
	if !strings.Contains(line, "35MMCAM.ZIP") || !strings.Contains(line, "54.0K") {
		t.Fatalf("unexpected line: %q", line)
	}
}

func TestGetCatalogStats(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	today := "2026-06-26"
	old := "2026-05-01"
	if _, err := db.Exec(`INSERT INTO files (dir_id, filename, size, description, uploader, upload_date) VALUES
		(1,'A.ZIP',1,'ok','Sysop',?),
		(1,'B.ZIP',1,'ok','Sysop',?),
		(1,'C.ZIP',0,'`+missingFileDesc+`','Sysop',?)`, today, old, today); err != nil {
		t.Fatal(err)
	}
	store := &Store{db: db, filesRoot: t.TempDir()}
	st, err := store.GetCatalogStats()
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 2 || st.Today != 1 || st.LastMonth != 1 {
		t.Fatalf("stats = %+v", st)
	}
}