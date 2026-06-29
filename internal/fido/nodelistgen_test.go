package fido

import (
	"os"
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

func TestShouldPreserveImportedNodelist(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if _, err := messages.Open(sqlDB); err != nil {
		t.Fatal(err)
	}

	nd := &NetworkDef{Name: "VirtNet", Uplink: ""}
	if ShouldPreserveImportedNodelist(sqlDB, nd) {
		t.Fatal("expected false before any import")
	}

	f, err := os.CreateTemp(t.TempDir(), "nl-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("Zone,90,VirtNet,Internet,Sysop,-Unpublished-,33600\nHost,1,Hub,Internet,Sysop,-Unpublished-,33600\n,17,Node,Internet,Sysop,-Unpublished-,33600\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportFile(sqlDB, f.Name(), "VirtNet"); err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	if !ShouldPreserveImportedNodelist(sqlDB, nd) {
		t.Fatal("expected true after manual import")
	}

	before, _ := sqlDB.Query(`SELECT COUNT(*) FROM fido_nodes WHERE network='VirtNet'`)
	var countBefore int
	if before.Next() {
		_ = before.Scan(&countBefore)
	}
	before.Close()

	if err := RebuildHubNodelistDB(sqlDB, nd, "VirtBBS", "Sysop"); err != nil {
		t.Fatal(err)
	}
	if ShouldPreserveImportedNodelist(sqlDB, nd) {
		t.Fatal("expected false after member rebuild overwrote import source")
	}
	_ = countBefore
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

func TestRebuildNetworkDiagrams_noNodes(t *testing.T) {
	nd := &NetworkDef{Name: "FidoNet", Uplink: "1:105/1", Enabled: true}
	count, warns := RebuildNetworkDiagrams(nd, nil, nil, "BBS", "Sysop")
	if count != 0 || len(warns) == 0 {
		t.Fatalf("no nodes: got count=%d warns=%v", count, warns)
	}
}

func TestNetworkDiagZipName(t *testing.T) {
	if got := NetworkDiagZipName("VirtNet"); got != "VirtNet_diags.zip" {
		t.Fatalf("VirtNet: got %q", got)
	}
	if got := NetworkDiagZipName("Fido Net"); got != "Fido_Net_diags.zip" {
		t.Fatalf("Fido Net: got %q", got)
	}
}
