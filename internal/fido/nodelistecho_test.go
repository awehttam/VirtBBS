package fido

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/virtbbs/virtbbs/internal/messages"
)

type stubFileArea struct {
	dirPath string
}

func (s *stubFileArea) EnsureDir(name, description string) (int64, string, error) {
	return 1, s.dirPath, nil
}

func (s *stubFileArea) RegisterUpload(dirID int64, filename, description, uploader string) error {
	return nil
}

func (s *stubFileArea) UploadDir(dirID int64) string { return s.dirPath }

func (s *stubFileArea) InstallFile(dirID int64, srcPath, destName, description, uploader string) error {
	return nil
}

func TestProcessPendingNodelistEchoesForNetworkFiltersByNetwork(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := messages.Open(db); err != nil {
		t.Fatal(err)
	}

	dirPath := t.TempDir()
	fileArea := &stubFileArea{dirPath: dirPath}

	for _, tc := range []struct {
		network string
		subject string
	}{
		{"AlphaNet", "VirtNet Nodelist Diff D045"},
		{"BetaNet", "VirtNet Nodelist Diff D045"},
	} {
		if err := QueueNodelistEcho(db, tc.network, tc.subject, "body-"+tc.network); err != nil {
			t.Fatal(err)
		}
	}

	errs := ProcessPendingNodelistEchoesForNetwork(db, fileArea, "AlphaNet")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	alphaPath := filepath.Join(dirPath, "VirtNode.D045")
	if _, err := os.Stat(alphaPath); err != nil {
		t.Fatalf("AlphaNet diff not written: %v", err)
	}

	pending, err := ListPendingNodelistEchoes(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending count = %d, want 1 (BetaNet only)", len(pending))
	}
	if pending[0].Network != "BetaNet" {
		t.Fatalf("remaining pending network = %q, want BetaNet", pending[0].Network)
	}
}
