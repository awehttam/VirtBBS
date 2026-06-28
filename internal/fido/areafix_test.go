package fido

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/messages"
)

func TestParseAreaFixAddLine(t *testing.T) {
	tests := []struct {
		line    string
		tag     string
		rescan  int
		wantOK  bool
	}{
		{"+GENERAL", "GENERAL", -1, true},
		{"=GENERAL,R=50", "GENERAL", 50, true},
		{"+FOO,R", "FOO", 0, true},
		{"+BAR,R=0", "BAR", 0, true},
		{"+", "", -1, false},
	}
	for _, tc := range tests {
		got, ok := parseAreaFixAddLine(tc.line)
		if ok != tc.wantOK {
			t.Fatalf("%q: ok=%v want %v", tc.line, ok, tc.wantOK)
		}
		if !ok {
			continue
		}
		if got.tag != tc.tag || got.rescanMax != tc.rescan {
			t.Fatalf("%q: got %+v want tag=%q rescan=%d", tc.line, got, tc.tag, tc.rescan)
		}
	}
}

func TestParseAreaFixRescanLine(t *testing.T) {
	tag, ok := parseAreaFixRescanLine("%RESCAN")
	if !ok || tag != "" {
		t.Fatalf("bare %%RESCAN: tag=%q ok=%v", tag, ok)
	}
	tag, ok = parseAreaFixRescanLine("%RESCAN GENERAL")
	if !ok || tag != "GENERAL" {
		t.Fatalf("%%RESCAN TAG: tag=%q ok=%v", tag, ok)
	}
}

func setupAreaFixTest(t *testing.T) (*messages.Store, *conferences.Store, *NetworkDef, Addr) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	msgStore, err := messages.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	confStore, err := conferences.Open(db)
	if err != nil {
		t.Fatal(err)
	}

	conf := &conferences.Conference{
		Name: "General", Echo: true, EchoTag: "GENERAL", Network: "TestNet", Public: true,
	}
	if err := confStore.Create(conf); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	dlAddr := Addr{Zone: 1, Net: 234, Node: 3}
	nd := &NetworkDef{
		Name:        "TestNet",
		Address:     "1:234/1",
		Uplink:      "1:234/2",
		OutboundDir: outDir,
		Downlinks: []Downlink{
			{Name: "Downlink", Address: dlAddr.String(), Password: "secret"},
		},
	}

	return msgStore, confStore, nd, dlAddr
}

func postEchoMessages(t *testing.T, store *messages.Store, confID int, bodies ...string) {
	t.Helper()
	for i, body := range bodies {
		m := &messages.Message{
			ConferenceID: confID,
			FromName:     "Alice",
			ToName:       "All",
			Subject:      "Test",
			Echo:         true,
			Body:         body,
			DatePosted:   time.Now().Add(time.Duration(i) * time.Minute),
		}
		if err := store.Post(m); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRescanEchoToDownlink_includesExportedMessages(t *testing.T) {
	msgStore, confStore, nd, dlAddr := setupAreaFixTest(t)

	postEchoMessages(t, msgStore, 1, "one", "two", "three")
	msgs, err := msgStore.ListEcho(1, 10, 0)
	if err != nil || len(msgs) != 3 {
		t.Fatalf("ListEcho: %d msgs err=%v", len(msgs), err)
	}
	if err := msgStore.MarkExported(msgs[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := msgStore.MarkExported(msgs[1].ID); err != nil {
		t.Fatal(err)
	}

	areafixDB := OpenAreaFixDB(msgStore.DB())
	if err := areafixDB.Subscribe("TestNet", dlAddr.String(), "GENERAL"); err != nil {
		t.Fatal(err)
	}

	res, err := RescanEchoToDownlink(nd, msgStore, confStore, "TestBBS", dlAddr.String(), []string{"GENERAL"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Messages != 3 {
		t.Fatalf("rescan messages=%d want 3", res.Messages)
	}
	if res.PKTPath == "" {
		t.Fatal("expected rescan pkt path")
	}
	if !strings.Contains(filepath.Base(res.PKTPath), "_rescan_") {
		t.Fatalf("unexpected pkt name %s", res.PKTPath)
	}

	// Export state unchanged — still only 2 marked exported from before.
	again, err := msgStore.ListEcho(1, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 {
		t.Fatalf("ListEcho after rescan=%d want 1 unexported", len(again))
	}
}

func TestRescanEchoToDownlink_respectsMaxMsgs(t *testing.T) {
	msgStore, confStore, nd, dlAddr := setupAreaFixTest(t)
	postEchoMessages(t, msgStore, 1, "a", "b", "c", "d", "e")

	areafixDB := OpenAreaFixDB(msgStore.DB())
	_ = areafixDB.Subscribe("TestNet", dlAddr.String(), "GENERAL")

	res, err := RescanEchoToDownlink(nd, msgStore, confStore, "TestBBS", dlAddr.String(), []string{"GENERAL"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Messages != 2 {
		t.Fatalf("rescan messages=%d want 2", res.Messages)
	}
}

func TestProcessAreaFixRequest_rescanOnSubscribe(t *testing.T) {
	msgStore, confStore, nd, dlAddr := setupAreaFixTest(t)
	postEchoMessages(t, msgStore, 1, "hello")

	pm := &Message{
		OrigAddr: dlAddr,
		FromName: "Sysop",
		ToName:   AreaFixRobotName,
		Subject:  "AreaFix",
		Body:     "secret\r\n+GENERAL,R=1\r\n",
	}

	if err := ProcessAreaFixRequest(nd, msgStore, confStore, "TestNet", "TestBBS", pm); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(nd.OutboundDir)
	if err != nil {
		t.Fatal(err)
	}
	var rescanPKT bool
	for _, e := range entries {
		if strings.Contains(e.Name(), "_rescan_") && strings.HasSuffix(e.Name(), ".pkt") {
			rescanPKT = true
		}
	}
	if !rescanPKT {
		t.Fatalf("expected rescan pkt in %v", entries)
	}
}
