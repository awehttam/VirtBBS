package fido

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/virtbbs/virtbbs/internal/db"
)

func TestImportFileDoesNotBlockOnSingleConnection(t *testing.T) {
	sqlDB, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if _, err := sqlDB.Exec(`CREATE TABLE fido_nodes (
		id INTEGER PRIMARY KEY, network TEXT, zone INT, net INT, node_num INT, point INT,
		name TEXT, location TEXT, sysop TEXT, phone TEXT, baud INT, flags TEXT, node_type TEXT, is_active INT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`CREATE TABLE fido_nodelist_versions (network TEXT PRIMARY KEY, imported_at TEXT, node_count INT, source TEXT NOT NULL DEFAULT 'import')`); err != nil {
		t.Fatal(err)
	}

	f, err := os.CreateTemp(t.TempDir(), "nl-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("Zone,1,Test,City,Sysop,555,9600\nHost,105,Hub,City,Sysop,555,9600\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var importErr error
	go func() {
		_, importErr = ImportFile(sqlDB, f.Name(), "FidoNet")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ImportFile blocked longer than 3s (likely sqlite deadlock with maxOpenConns=1)")
	}
	if importErr != nil {
		t.Fatalf("ImportFile: %v", importErr)
	}
}

func TestImportFileExtractsZipArchive(t *testing.T) {
	sqlDB, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if _, err := sqlDB.Exec(`CREATE TABLE fido_nodes (
		id INTEGER PRIMARY KEY, network TEXT, zone INT, net INT, node_num INT, point INT,
		name TEXT, location TEXT, sysop TEXT, phone TEXT, baud INT, flags TEXT, node_type TEXT, is_active INT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`CREATE TABLE fido_nodelist_versions (network TEXT PRIMARY KEY, imported_at TEXT, node_count INT, source TEXT NOT NULL DEFAULT 'import')`); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join("..", "..", "files", "LovlyNet_Nodelist_Files", "LOVLYNET.Z26")
	if _, err := os.Stat(zipPath); err != nil {
		t.Skip("LOVLYNET.Z26 not present in workspace files area")
	}

	result, err := ImportFile(sqlDB, zipPath, "LovlyNet")
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	if result.Inserted == 0 {
		t.Fatal("expected nodes from ZIP nodelist, got 0")
	}
	var count int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM fido_nodes WHERE network='LovlyNet'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no rows stored after ZIP import")
	}
}
