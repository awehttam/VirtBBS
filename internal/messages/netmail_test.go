package messages

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestListNetmail_filtersByRecipient(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}

	post := func(toName, origin string) {
		t.Helper()
		if err := store.Post(&Message{
			ConferenceID: 0,
			FromName:     "Remote",
			ToName:       toName,
			Subject:      "Test",
			DatePosted:   time.Now(),
			Status:       "A",
			Body:         "body",
			FidoOrigin:   origin,
		}); err != nil {
			t.Fatal(err)
		}
	}

	post("Alice", "1:234/567")
	post("Bob", "1:234/568")
	post("All", "1:234/569")

	msgs, err := store.ListNetmail("Alice", false, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("Alice got %d messages, want 2 (Alice + All)", len(msgs))
	}

	all, err := store.ListNetmail("Bob", true, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("sysop got %d messages, want 3", len(all))
	}
}

func TestCountNetmailUnread_andGetNetmail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := store.Post(&Message{
			ConferenceID: 0,
			FromName:     "Remote",
			ToName:       "Alice",
			Subject:      "Test",
			DatePosted:   time.Now(),
			Status:       "A",
			Body:         "body",
			FidoOrigin:   "1:234/567",
		}); err != nil {
			t.Fatal(err)
		}
	}
	unread, err := store.CountNetmailUnread("Alice", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if unread != 3 {
		t.Fatalf("unread = %d, want 3", unread)
	}
	unread, err = store.CountNetmailUnread("Alice", false, 2)
	if err != nil {
		t.Fatal(err)
	}
	if unread != 1 {
		t.Fatalf("unread after 2 = %d, want 1", unread)
	}
	m, err := store.GetNetmail("Alice", false, 2)
	if err != nil {
		t.Fatal(err)
	}
	if m.MsgNumber != 2 {
		t.Fatalf("GetNetmail msg_number = %d, want 2", m.MsgNumber)
	}
}
