package files

import (
	"archive/zip"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestScanDir_addsNewAndMarksMissing(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(dirPath, "NEWFILE.TXT"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO files (dir_id, filename, size, description, uploader)
		VALUES (1, 'GONE.ZIP', 100, 'Old file', 'Sysop')`); err != nil {
		t.Fatal(err)
	}

	res, err := store.ScanDir(1, "Sysop")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 || res.Missing != 1 {
		t.Fatalf("ScanDir = %+v, want Added=1 Missing=1", res)
	}

	files, err := store.ListFiles(1)
	if err != nil {
		t.Fatal(err)
	}
	var newFile, gone *File
	for _, f := range files {
		switch f.Filename {
		case "NEWFILE.TXT":
			newFile = f
		case "GONE.ZIP":
			gone = f
		}
	}
	if newFile == nil || newFile.Description != defaultScanDesc {
		t.Fatalf("new file: %+v", newFile)
	}
	if gone == nil || gone.Description != missingFileDesc {
		t.Fatalf("gone file: %+v", gone)
	}
}

func TestReadZipDiz(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("FILE_ID.DIZ")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("Cool BBS ware\r\nSecond line"))
	zw.Close()
	f.Close()

	got := readZipDiz(path)
	want := "Cool BBS ware Second line"
	if got != want {
		t.Fatalf("readZipDiz = %q, want %q", got, want)
	}
}

func TestNormalizeDiz_truncates(t *testing.T) {
	got := normalizeDiz([]byte(strings.Repeat("A", 120)))
	if len(got) > maxDizLen {
		t.Fatalf("normalizeDiz len %d > %d", len(got), maxDizLen)
	}
}