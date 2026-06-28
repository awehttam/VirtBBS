package fido

import (
	"path/filepath"
	"testing"

	"github.com/virtbbs/virtbbs/internal/db"
	"github.com/virtbbs/virtbbs/internal/messages"
)

func TestRouteAddrDefersCrashForHoldNode(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if _, err := messages.Open(sqlDB); err != nil {
		t.Fatal(err)
	}

	const network = "TestNet"
	holdAddr, _ := ParseAddr("1:153/150")
	uplinkAddr, _ := ParseAddr("227:1/1")

	_, err = sqlDB.Exec(`INSERT INTO fido_nodes
		(network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active)
		VALUES (?, ?, ?, ?, 0, 'Hold_Node', 'City', 'Sysop', '', 33600, 'CM,IBN', 'Hold', 1)`,
		network, holdAddr.Zone, holdAddr.Net, holdAddr.Node)
	if err != nil {
		t.Fatal(err)
	}

	nd := &NetworkDef{
		Name:    network,
		Address: "227:1/77",
		Uplink:  uplinkAddr.String(),
	}

	msg := &NetmailMsg{ToAddr: holdAddr.String(), Crash: true}
	hop, err := RouteAddr(sqlDB, msg, nd)
	if err != nil {
		t.Fatal(err)
	}
	if hop != uplinkAddr {
		t.Fatalf("RouteAddr = %v, want uplink %v for Hold crash dest", hop, uplinkAddr)
	}
}

func TestRouteAddrDirectForConfiguredDownlinkHold(t *testing.T) {
	dir := t.TempDir()
	sqlDB, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if _, err := messages.Open(sqlDB); err != nil {
		t.Fatal(err)
	}

	const network = "TestNet"
	holdAddr, _ := ParseAddr("1:153/150")

	_, err = sqlDB.Exec(`INSERT INTO fido_nodes
		(network, zone, net, node_num, point, name, location, sysop, phone, baud, flags, node_type, is_active)
		VALUES (?, ?, ?, ?, 0, 'Hold_Node', 'City', 'Sysop', '', 33600, 'CM,IBN', 'Hold', 1)`,
		network, holdAddr.Zone, holdAddr.Net, holdAddr.Node)
	if err != nil {
		t.Fatal(err)
	}

	nd := &NetworkDef{
		Name:      network,
		Address:   "227:1/77",
		Uplink:    "227:1/1",
		Downlinks: []Downlink{{Name: "Member", Address: holdAddr.String()}},
	}

	msg := &NetmailMsg{ToAddr: holdAddr.String(), Crash: true}
	hop, err := RouteAddr(sqlDB, msg, nd)
	if err != nil {
		t.Fatal(err)
	}
	if hop != holdAddr {
		t.Fatalf("RouteAddr = %v, want direct %v for configured downlink", hop, holdAddr)
	}
}
