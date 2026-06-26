package node

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestPurgeInactive_removesOrphans(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO nodes (id, status) VALUES (1, 'main'), (2, 'login'), (3, 'main')`); err != nil {
		t.Fatal(err)
	}
	RegisterControl(2, func() {})

	if err := store.PurgeInactive(); err != nil {
		t.Fatal(err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != 2 {
		t.Fatalf("after purge want node 2 only, got %+v", list)
	}
	UnregisterControl(2)
}