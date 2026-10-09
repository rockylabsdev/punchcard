package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Save writes config.json atomically (setup is its only writer). The
// schemaVersion is always this binary's.
func Save(getenv func(string) string, c Config) error {
	p := Path(getenv)
	if p == "" {
		return errors.New("neither XDG_CONFIG_HOME nor HOME is set")
	}
	c.SchemaVersion = SchemaVersion
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(p, append(b, '\n'), 0o644)
}

// WriteTokenFile writes the 0600 token fallback beside config.json.
func WriteTokenFile(getenv func(string) string, token string) error {
	p := TokenPath(getenv)
	if p == "" {
		return errors.New("neither XDG_CONFIG_HOME nor HOME is set")
	}
	return WriteFileAtomic(p, []byte(token+"\n"), 0o600)
}

// RemoveTokenFile deletes the token fallback; none there is success.
func RemoveTokenFile(getenv func(string) string) error {
	p := TokenPath(getenv)
	if p == "" {
		return nil
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// WriteFileAtomic replaces path with b: a temp file in the same
// directory (created 0700 if missing), chmod'd to perm and synced, then
// renamed over the old file, so a reader never sees half a file.
func WriteFileAtomic(path string, b []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	err = f.Chmod(perm)
	if err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
