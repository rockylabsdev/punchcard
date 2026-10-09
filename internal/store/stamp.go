package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Stamp identifies one version of state.json without reading it: its
// inode (every save renames a new file into place) and its mtime and
// size. The zero Stamp is "no file".
type Stamp struct {
	Inode   uint64
	ModTime time.Time
	Size    int64
}

// Stamp stats state.json: no lock, no read, no write. When it differs
// from an earlier Stamp, the file has been replaced since; Read it.
func (s *Store) Stamp() (Stamp, error) {
	fi, err := os.Stat(filepath.Join(s.dir, StateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Stamp{}, nil
	}
	if err != nil {
		return Stamp{}, err
	}
	st := Stamp{ModTime: fi.ModTime(), Size: fi.Size()}
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		st.Inode = uint64(sys.Ino)
	}
	return st, nil
}
