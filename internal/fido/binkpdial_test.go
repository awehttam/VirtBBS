package fido

import (
	"path/filepath"
	"testing"

	"github.com/virtbbs/virtbbs/internal/db"
	"github.com/virtbbs/virtbbs/internal/messages"
)

func TestDialFromNodeFlags(t *testing.T) {
	tests := []struct {
		flags    string
		wantHost string
		wantPort int
	}{
		{"CM,IBN:ferchobbs.ddns.net", "ferchobbs.ddns.net", 0},
		{"CM,IBN:hub.awesomenet.us:24556", "hub.awesomenet.us", 24556},
		{"CM,IBN,INA:bbs.outpostbbs.net,ITN:60177", "bbs.outpostbbs.net", 60177},
		{"CM,INA:ftsc.bnbbbs.net,IBN:24555", "ftsc.bnbbbs.net", 24555},
		{"CM,IBN:24555,INA:phoenix.bnbbbs.net", "phoenix.bnbbbs.net", 24555},
		{"CM,IBN:host.example.com", "host.example.com", 0},
		{"CM,IBN:host.example.com:24556", "host.example.com", 24556},
		{"CM,IBN:some.host.com,ITN:24555", "some.host.com", 24555},
	}
	for _, tc := range tests {
		h, p := dialFromNodeFlags(tc.flags)
		if h != tc.wantHost || p != tc.wantPort {
			t.Errorf("dialFromNodeFlags(%q) = (%q, %d), want (%q, %d)",
				tc.flags, h, p, tc.wantHost, tc.wantPort)
		}
	}
}

func TestResolveBinkpDialTarget_nodelist(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if _, err := messages.Open(sqlDB); err != nil {
		t.Fatal(err)
	}

	_, err = sqlDB.Exec(`INSERT INTO fido_nodes
		(network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active)
		VALUES ('FidoNet', 4, 902, 19, 0, 'Fercho_BBS', 'La_Plata_BA', 'Fernando_Miculan', '', 300, 'CM,IBN:ferchobbs.ddns.net', 'Node', 1)`)
	if err != nil {
		t.Fatal(err)
	}

	host, port, err := ResolveBinkpDialTarget("FidoNet", "4:902/19", 24554, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if host != "ferchobbs.ddns.net" || port != 24554 {
		t.Fatalf("got (%q, %d), want (ferchobbs.ddns.net, 24554)", host, port)
	}
}

func TestResolveBinkpDialTarget_zone2net277(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	if _, err := messages.Open(sqlDB); err != nil {
		t.Fatal(err)
	}

	_, err = sqlDB.Exec(`INSERT INTO fido_nodes
		(network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active)
		VALUES ('FidoNet', 2, 277, 1, 0, 'Example_Hub', 'Internet', 'Sysop', '', 300, 'CM,IBN:host.example.com', 'Node', 1)`)
	if err != nil {
		t.Fatal(err)
	}

	host, port, err := ResolveBinkpDialTarget("FidoNet", "2:277/1", 24554, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if host != "host.example.com" || port != 24554 {
		t.Fatalf("got (%q, %d), want (host.example.com, 24554)", host, port)
	}
}

func TestResolveBinkpDialTarget_atHost(t *testing.T) {
	host, port, err := ResolveBinkpDialTarget("FidoNet", "4:902/19@example.com:24555", 24554, nil)
	if err != nil {
		t.Fatal(err)
	}
	if host != "example.com" || port != 24555 {
		t.Fatalf("got (%q, %d), want (example.com, 24555)", host, port)
	}
}

func TestScanNodes_singleRow(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if _, err := messages.Open(sqlDB); err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`INSERT INTO fido_nodes
		(network, zone, net, node_num, point, name, flags, node_type, is_active)
		VALUES ('FidoNet', 4, 902, 19, 0, 'Test', 'IBN:test.example', 'Node', 1)`)
	if err != nil {
		t.Fatal(err)
	}
	row := sqlDB.QueryRow(`SELECT id, network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active
		FROM fido_nodes WHERE zone=4 AND net=902 AND node_num=19`)
	nodes, err := scanNodes(singleRow{row})
	if err != nil {
		t.Fatalf("scanNodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Name != "Test" {
		t.Fatalf("scanNodes: got %+v", nodes)
	}
}
