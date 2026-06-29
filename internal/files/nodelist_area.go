package files

import (
	"os"
	"time"

	"github.com/virtbbs/virtbbs/internal/fido"
)

// ListAreaFiles returns registered files in dirID with on-disk modification times.
func (s *Store) ListAreaFiles(dirID int64) ([]fido.AreaFile, error) {
	files, err := s.ListFiles(dirID)
	if err != nil {
		return nil, err
	}
	var out []fido.AreaFile
	for _, f := range files {
		path := s.AbsPath(dirID, f.Filename)
		mod := time.Time{}
		if info, err := os.Stat(path); err == nil {
			mod = info.ModTime()
		}
		out = append(out, fido.AreaFile{
			Filename: f.Filename,
			FullPath: path,
			ModTime:  mod,
			Uploader: f.Uploader,
		})
	}
	return out, nil
}
