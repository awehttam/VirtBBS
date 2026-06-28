package fido

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/virtbbs/virtbbs/internal/messages"
)

func TestScanNetmailQueue_writesPKTAndMarksSent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store, err := messages.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	_ = store

	outDir := t.TempDir()
	nd := &NetworkDef{
		Name:        "TestNet",
		Address:     "1:234/1",
		Uplink:      "1:234/2",
		OutboundDir: outDir,
	}

	ndb := OpenNetmailDB(db)
	id, err := ndb.Enqueue(&NetmailMsg{
		Network:  "TestNet",
		FromName: "Alice",
		FromAddr: nd.Address,
		ToName:   "Bob",
		ToAddr:   "1:234/3",
		Subject:  "Hello",
		Body:     "Test body",
	})
	if err != nil {
		t.Fatal(err)
	}

	result := ScanNetmailQueue(nd, db)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if result.Exported != 1 {
		t.Fatalf("exported %d, want 1", result.Exported)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || filepath.Ext(entries[0].Name()) != ".pkt" {
		t.Fatalf("expected one .pkt in outbound, got %v", entries)
	}

	msgs, ids, err := ndb.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 || len(ids) != 0 {
		t.Fatalf("queue still has pending rows after scan")
	}

	// Idempotent: nothing left to export.
	again := ScanNetmailQueue(nd, db)
	if again.Exported != 0 {
		t.Fatalf("second scan exported %d, want 0", again.Exported)
	}
	_ = id
}
