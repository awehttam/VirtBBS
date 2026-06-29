package fido

import (
	"os"
	"path/filepath"
	"testing"
)

type registerFileAreaStub struct {
	dirPath string
	files   map[string]string
}

func (s *registerFileAreaStub) EnsureDir(name, description string) (int64, string, error) {
	return 1, s.dirPath, nil
}

func (s *registerFileAreaStub) RegisterUpload(dirID int64, filename, description, uploader string) error {
	s.files[filename] = description
	return nil
}

func (s *registerFileAreaStub) UploadDir(dirID int64) string { return s.dirPath }

func (s *registerFileAreaStub) InstallFile(dirID int64, srcPath, destName, description, uploader string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.dirPath, destName), data, 0644); err != nil {
		return err
	}
	return s.RegisterUpload(dirID, destName, description, uploader)
}

func (s *registerFileAreaStub) ListAreaFiles(dirID int64) ([]AreaFile, error) { return nil, nil }

func TestRegisterNodelistInFileArea(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "NODELIST.026")
	if err := os.WriteFile(src, []byte(";,test"), 0644); err != nil {
		t.Fatal(err)
	}
	areaDir := filepath.Join(dir, "files")
	if err := os.MkdirAll(areaDir, 0755); err != nil {
		t.Fatal(err)
	}
	stub := &registerFileAreaStub{dirPath: areaDir, files: map[string]string{}}
	nd := &NetworkDef{Name: "LovlyNet"}

	if err := RegisterNodelistInFileArea(stub, nd, src, "fetched nodelist"); err != nil {
		t.Fatalf("RegisterNodelistInFileArea: %v", err)
	}
	if stub.files["NODELIST.026"] != "fetched nodelist" {
		t.Fatalf("catalog = %#v", stub.files)
	}
	if _, err := os.Stat(filepath.Join(areaDir, "NODELIST.026")); err != nil {
		t.Fatalf("file not copied: %v", err)
	}
}
