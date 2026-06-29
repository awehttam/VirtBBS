package fido

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRestoreLocalNodeEntriesAddsAKAs(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE fido_nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		network TEXT NOT NULL,
		zone INTEGER NOT NULL,
		net INTEGER NOT NULL,
		node_num INTEGER NOT NULL,
		point INTEGER NOT NULL DEFAULT 0,
		name TEXT NOT NULL DEFAULT '',
		location TEXT NOT NULL DEFAULT '',
		sysop TEXT NOT NULL DEFAULT '',
		phone TEXT NOT NULL DEFAULT '',
		baud INTEGER NOT NULL DEFAULT 0,
		flags TEXT NOT NULL DEFAULT '',
		node_type TEXT NOT NULL DEFAULT 'Node',
		is_active INTEGER NOT NULL DEFAULT 1,
		UNIQUE(network, zone, net, node_num, point)
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE fido_nodelist_versions (
		network TEXT PRIMARY KEY,
		imported_at TEXT NOT NULL,
		node_count INTEGER NOT NULL DEFAULT 0,
		source TEXT NOT NULL DEFAULT 'import'
	)`); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	nodelist := filepath.Join(dir, "NODELIST.001")
	const body = `; test
Zone,227,Test_Zone,Internet,Sysop,-Unpublished-,33600
Host,1,Other_Host,Internet,Other,-Unpublished-,33600
,17,Other_Node,Internet,Other,-Unpublished-,33600
`
	if err := os.WriteFile(nodelist, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	nd := &NetworkDef{
		Name:    "LovlyNet",
		Address: "227:1/17",
		AKAs:    []string{"227:1/0"},
		Enabled: true,
		BinkpHost: "bbs.example.com",
	}

	if _, err := ImportFile(db, nodelist, "LovlyNet"); err != nil {
		t.Fatalf("ImportFile: %v", err)
	}

	if err := RestoreLocalNodeEntries(db, nd, "MyBBS", "John", "Internet", 23); err != nil {
		t.Fatalf("RestoreLocalNodeEntries: %v", err)
	}

	ndb := OpenNodelistDB(db)
	primary, err := ndb.LookupAddr("LovlyNet", Addr{Zone: 227, Net: 1, Node: 17})
	if err != nil || primary == nil {
		t.Fatalf("primary lookup: %v", err)
	}
	if primary.Name != "MyBBS" || primary.Sysop != "John" {
		t.Fatalf("primary = %+v, want MyBBS/John", primary)
	}
	if !strings.Contains(primary.Flags, "IBN:bbs.example.com") {
		t.Fatalf("primary flags = %q, want IBN host", primary.Flags)
	}

	host, err := ndb.LookupAddr("LovlyNet", Addr{Zone: 227, Net: 1, Node: 0})
	if err != nil || host == nil {
		t.Fatalf("host AKA lookup: %v", err)
	}
	if host.Type != "Host" || host.Name != "MyBBS" {
		t.Fatalf("host = %+v", host)
	}

	linked := []*NodeEntry{primary, host}
	LinkHostAKAsPtrs(linked)
	LinkConfiguredAKAs(linked, nd)
	if linked[0].AKA != "227:1/0" {
		t.Fatalf("primary AKA = %q, want 227:1/0", linked[0].AKA)
	}
	if linked[1].AKA != "227:1/17" {
		t.Fatalf("host AKA = %q, want 227:1/17", linked[1].AKA)
	}
}
