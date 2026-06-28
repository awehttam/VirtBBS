package fido

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/virtbbs/virtbbs/internal/messages"
)

func setupFileFixTest(t *testing.T) (*sql.DB, *NetworkDef, Addr, string) {
	t.Helper()
	filesRoot := t.TempDir()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := messages.Open(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS file_dirs (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
		path TEXT NOT NULL, sort_type INTEGER NOT NULL DEFAULT 0, read_sec INTEGER NOT NULL DEFAULT 10,
		upload_sec INTEGER NOT NULL DEFAULT 20, conference_id INTEGER, active INTEGER NOT NULL DEFAULT 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT, dir_id INTEGER NOT NULL, filename TEXT NOT NULL,
		size INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '', uploader TEXT NOT NULL DEFAULT '',
		upload_date TEXT NOT NULL DEFAULT (date('now')), downloads INTEGER NOT NULL DEFAULT 0,
		flagged INTEGER NOT NULL DEFAULT 0, UNIQUE (dir_id, filename))`); err != nil {
		t.Fatal(err)
	}

	dirID := int64(1)
	if _, err := db.Exec(`INSERT INTO file_dirs (id, name, description, path) VALUES (1, 'Games', 'Test', 'games')`); err != nil {
		t.Fatal(err)
	}
	gamesDir := filepath.Join(filesRoot, "games")
	if err := os.MkdirAll(gamesDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha.zip", "beta.zip", "gamma.zip"} {
		if err := os.WriteFile(filepath.Join(gamesDir, name), []byte("data-"+name), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO files (dir_id, filename, size, description, uploader) VALUES (?,?,?,?,?)`,
			dirID, name, int64(len("data-"+name)), "Test "+name, "test"); err != nil {
			t.Fatal(err)
		}
	}

	outDir := t.TempDir()
	dlAddr := Addr{Zone: 1, Net: 234, Node: 3}
	nd := &NetworkDef{
		Name:        "TestNet",
		Address:     "1:234/1",
		Uplink:      "1:234/2",
		OutboundDir: outDir,
		FileAreas:   map[string]int{"GAMES": 1},
		Downlinks: []Downlink{
			{Name: "Downlink", Address: dlAddr.String(), Password: "secret"},
		},
	}

	if err := OpenFileFixDB(db).Subscribe("TestNet", dlAddr.String(), "GAMES"); err != nil {
		t.Fatal(err)
	}
	if err := OpenTICExportDB(db).MarkExported("TestNet", dirID, "alpha.zip"); err != nil {
		t.Fatal(err)
	}
	if err := OpenTICExportDB(db).MarkExported("TestNet", dirID, "beta.zip"); err != nil {
		t.Fatal(err)
	}

	return db, nd, dlAddr, filesRoot
}

func TestRescanFilesToDownlink_includesExportedFiles(t *testing.T) {
	db, nd, dlAddr, filesRoot := setupFileFixTest(t)

	res, err := RescanFilesToDownlink(nd, db, filesRoot, dlAddr.String(), []string{"GAMES"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 3 {
		t.Fatalf("files=%d want 3", res.Files)
	}
	if res.TICFiles != 3 {
		t.Fatalf("tic files=%d want 3", res.TICFiles)
	}

	entries, err := os.ReadDir(nd.OutboundDir)
	if err != nil {
		t.Fatal(err)
	}
	var rescanTIC int
	for _, e := range entries {
		if strings.Contains(e.Name(), "_rescan_") && strings.HasSuffix(strings.ToLower(e.Name()), ".tic") {
			rescanTIC++
		}
	}
	if rescanTIC != 3 {
		t.Fatalf("rescan TIC count=%d want 3", rescanTIC)
	}

	exported, err := OpenTICExportDB(db).IsExported("TestNet", int64(nd.FileAreas["GAMES"]), "alpha.zip")
	if err != nil || !exported {
		t.Fatal("export marker should remain after rescan-only export")
	}
}

func TestRescanFilesToDownlink_respectsMaxFiles(t *testing.T) {
	db, nd, dlAddr, filesRoot := setupFileFixTest(t)

	res, err := RescanFilesToDownlink(nd, db, filesRoot, dlAddr.String(), []string{"GAMES"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 2 {
		t.Fatalf("files=%d want 2 (alpha, beta by filename order)", res.Files)
	}
}

func TestProcessFileFixRequest_rescan(t *testing.T) {
	db, nd, dlAddr, filesRoot := setupFileFixTest(t)

	pm := &Message{
		FromName: "Sysop",
		OrigAddr: dlAddr,
		ToName:   FileFixRobotName,
		Subject:  "FileFix",
		Body:     "secret\r\n%RESCAN GAMES\r\n",
	}
	if err := ProcessFileFixRequest(nd, db, filesRoot, pm); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(nd.OutboundDir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "_rescan_") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected rescan TIC in outbound after %RESCAN GAMES")
	}
}

func TestProcessFileFixRequest_subscribeWithRescanLimit(t *testing.T) {
	db, nd, dlAddr, filesRoot := setupFileFixTest(t)
	_ = OpenFileFixDB(db).Unsubscribe("TestNet", dlAddr.String(), "GAMES")

	pm := &Message{
		FromName: "Sysop",
		OrigAddr: dlAddr,
		ToName:   FileFixRobotName,
		Subject:  "FileFix",
		Body:     "secret\r\n+GAMES,R=1\r\n",
	}
	if err := ProcessFileFixRequest(nd, db, filesRoot, pm); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(nd.OutboundDir)
	ticCount := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), "_rescan_") && strings.HasSuffix(strings.ToLower(e.Name()), ".tic") {
			ticCount++
		}
	}
	if ticCount != 1 {
		t.Fatalf("subscribe R=1 should queue 1 TIC, got %d", ticCount)
	}
}
