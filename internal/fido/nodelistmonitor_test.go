package fido

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/virtbbs/virtbbs/internal/db"
	"github.com/virtbbs/virtbbs/internal/messages"
)

func TestApplyNodelistDiffFile(t *testing.T) {
	sqlDB, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if _, err := messages.Open(sqlDB); err != nil {
		t.Fatal(err)
	}

	if _, err := sqlDB.Exec(`INSERT INTO fido_nodes
		(network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active)
		VALUES ('TestNet', 1, 105, 17, 0, 'Old', 'City', 'Sysop', '', 300, 'CM', 'Node', 1)`); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	diff := filepath.Join(dir, "NODEDIFF.Z77")
	body := "; diff\n,18,New_Node,City,Sysop,-Unpublished-,33600,CM\n-1:105/17\n"
	if err := os.WriteFile(diff, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	nd := &NetworkDef{Name: "TestNet", Address: "1:105/18"}
	if err := ApplyNodelistDiffFile(sqlDB, "TestNet", diff, nd); err != nil {
		t.Fatalf("ApplyNodelistDiffFile: %v", err)
	}

	ndb := OpenNodelistDB(sqlDB)
	removed, _ := ndb.LookupAddr("TestNet", Addr{Zone: 1, Net: 105, Node: 17})
	if removed != nil {
		t.Fatal("node 17 should be removed")
	}
	added, _ := ndb.LookupAddr("TestNet", Addr{Zone: 1, Net: 105, Node: 18})
	if added == nil || added.Name != "New Node" {
		t.Fatalf("node 18 = %+v", added)
	}
}

func TestMonitorSkipsOlderFileAreaNodelist(t *testing.T) {
	sqlDB, err := db.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if _, err := messages.Open(sqlDB); err != nil {
		t.Fatal(err)
	}
	if err := RecordNodelistVersion(sqlDB, "TestNet", 5); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	oldTime := "2020-01-01T00:00:00Z"
	_, err = sqlDB.Exec(`UPDATE fido_nodelist_versions SET imported_at=? WHERE network='TestNet'`, oldTime)
	if err != nil {
		t.Fatal(err)
	}

	nodelist := filepath.Join(dir, "NODELIST.Z01")
	if err := os.WriteFile(nodelist, []byte("Zone,1,Zone,City,Sysop,-Unpublished-,33600\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(nodelist, mustParseTime(oldTime), mustParseTime(oldTime))

	stub := &monitorFileStub{dir: dir, files: []AreaFile{{
		Filename: "NODELIST.Z01",
		FullPath: nodelist,
		ModTime:  mustParseTime(oldTime),
	}}}
	nd := &NetworkDef{Name: "TestNet", Address: "1:105/1", Enabled: true}
	if err := monitorNodelistFileArea(sqlDB, stub, nd, "", "", 0); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM fido_nodelist_applied`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected skip older file, applied count = %d", count)
	}
}

type monitorFileStub struct {
	dir   string
	files []AreaFile
}

func (m *monitorFileStub) EnsureDir(name, description string) (int64, string, error) {
	return 1, m.dir, nil
}
func (m *monitorFileStub) RegisterUpload(dirID int64, filename, description, uploader string) error {
	return nil
}
func (m *monitorFileStub) UploadDir(dirID int64) string { return m.dir }
func (m *monitorFileStub) InstallFile(dirID int64, srcPath, destName, description, uploader string) error {
	return nil
}
func (m *monitorFileStub) ListAreaFiles(dirID int64) ([]AreaFile, error) { return m.files, nil }

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
