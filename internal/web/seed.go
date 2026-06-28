package web

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed defaults/*
var defaultWWW embed.FS

// SeedWWW copies bundled default templates and static assets into root
// (typically cfg.Paths.WWW) when files are missing. Existing customisations
// are never overwritten.
func SeedWWW(root string) error {
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	return fs.WalkDir(defaultWWW, "defaults", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("defaults", path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dest := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		if _, err := os.Stat(dest); err == nil {
			return nil // already present — do not overwrite
		} else if !os.IsNotExist(err) {
			return err
		}
		data, err := defaultWWW.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0644)
	})
}
