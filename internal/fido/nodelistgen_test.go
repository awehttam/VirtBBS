package fido

import (
	"path/filepath"
	"testing"

	"github.com/virtbbs/virtbbs/internal/db"
	"github.com/virtbbs/virtbbs/internal/messages"
)

func TestUsesMemberNodelist(t *testing.T) {
	primaryFido := NetworkDef{Name: "FidoNet", IsPrimary: true, Uplink: ""}
	if !primaryFido.NodelistFetchEnabled() {
		t.Fatal("primary FidoNet should have automatic nodelist fetch enabled")
	}
	if primaryFido.UsesMemberNodelist() {
		t.Fatal("primary FidoNet must not use member nodelist even without uplink")
	}

	virtNet := NetworkDef{Name: "VirtNet", Uplink: ""}
	if virtNet.NodelistFetchEnabled() {
		t.Fatal("VirtNet hub should not auto-fetch an imported nodelist")
	}
	if !virtNet.UsesMemberNodelist() {
		t.Fatal("VirtNet hub should publish from fido_members")
	}

	leaf := NetworkDef{Name: "FidoNet", IsPrimary: true, Uplink: "1:105/1"}
	if leaf.UsesMemberNodelist() {
		t.Fatal("downlink FidoNet must not use member nodelist")
	}
}

func TestRebuildHubNodelistDB_preservesImportedFidoNet(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if _, err := messages.Open(sqlDB); err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= 3; i++ {
		_, err := sqlDB.Exec(`INSERT INTO fido_nodes
			(network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active)
			VALUES ('FidoNet', 1, 105, ?, 0, 'Node', 'Internet', 'Sysop', '', 300, 'CM', 'Node', 1)`, i)
		if err != nil {
			t.Fatal(err)
		}
	}

	nd := &NetworkDef{Name: "FidoNet", IsPrimary: true, Uplink: ""}
	if err := RebuildHubNodelistDB(sqlDB, nd, "VirtBBS", "Sysop"); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM fido_nodes WHERE network='FidoNet'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("RebuildHubNodelistDB wiped imported FidoNet rows: got %d nodes, want 3", count)
	}
}

func TestRebuildNetworkDiagrams_nonHub(t *testing.T) {
	nd := &NetworkDef{Name: "FidoNet", Uplink: "1:105/1"}
	count, warns := RebuildNetworkDiagrams(nd, nil, nil, "BBS", "Sysop")
	if count != 0 || len(warns) == 0 {
		t.Fatalf("non-hub: got count=%d warns=%v, want 0 warns", count, warns)
	}
}
