package users

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/virtbbs/virtbbs/internal/db"
	"github.com/virtbbs/virtbbs/internal/messages"
)

func openUserMessageStores(t *testing.T) (*Store, *messages.Store) {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	userStore, err := Open(sqlDB)
	if err != nil {
		t.Fatalf("users.Open: %v", err)
	}
	msgStore, err := messages.Open(sqlDB)
	if err != nil {
		t.Fatalf("messages.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return userStore, msgStore
}

func TestClampLastReadAfterDelete(t *testing.T) {
	userStore, msgStore := openUserMessageStores(t)
	const confID = 1
	const userID int64 = 1

	if err := userStore.SetLastRead(userID, confID, 72); err != nil {
		t.Fatalf("SetLastRead: %v", err)
	}
	m := &messages.Message{
		ConferenceID: confID,
		MsgNumber:    72,
		FromName:     "Sysop",
		Subject:      "last",
		DatePosted:   time.Now(),
		Body:         "body",
	}
	if err := msgStore.PostWithNumber(m); err != nil {
		t.Fatalf("PostWithNumber: %v", err)
	}
	if _, err := msgStore.Delete(m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	high, err := msgStore.HighMsgNumber(confID)
	if err != nil {
		t.Fatalf("HighMsgNumber: %v", err)
	}
	if high != 0 {
		t.Fatalf("HighMsgNumber = %d, want 0", high)
	}
	if err := userStore.ClampLastReadForConference(confID, high); err != nil {
		t.Fatalf("ClampLastReadForConference: %v", err)
	}
	if got := userStore.GetLastRead(userID, confID); got != 0 {
		t.Fatalf("GetLastRead after clamp = %d, want 0", got)
	}
	counts, err := userStore.NewMessageCounts(userID)
	if err != nil {
		t.Fatalf("NewMessageCounts: %v", err)
	}
	if counts[confID] != 0 {
		t.Fatalf("NewMessageCounts = %d, want 0", counts[confID])
	}
}

func TestNewMessageCountsCountsActiveOnly(t *testing.T) {
	userStore, msgStore := openUserMessageStores(t)
	const confID = 1
	const userID int64 = 1

	for n := 1; n <= 3; n++ {
		if err := msgStore.PostWithNumber(&messages.Message{
			ConferenceID: confID,
			MsgNumber:    n,
			FromName:     "Sysop",
			Subject:      "msg",
			DatePosted:   time.Now(),
			Body:         "body",
		}); err != nil {
			t.Fatalf("PostWithNumber %d: %v", n, err)
		}
	}
	if err := userStore.SetLastRead(userID, confID, 2); err != nil {
		t.Fatalf("SetLastRead: %v", err)
	}
	counts, err := userStore.NewMessageCounts(userID)
	if err != nil {
		t.Fatalf("NewMessageCounts: %v", err)
	}
	if counts[confID] != 1 {
		t.Fatalf("NewMessageCounts = %d, want 1", counts[confID])
	}
}
