// Package config reads where Jira is and who to authenticate as: the
// config file at $XDG_CONFIG_HOME/punchcard/config.json (else
// ~/.config/punchcard/config.json) and the API token. Only commands that
// call Jira read either; reads and the presence path never do.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// SchemaVersion is the config file version this binary writes and reads.
const SchemaVersion = 1

// Config is config.json. It holds no secret.
type Config struct {
	SchemaVersion int    `json:"schemaVersion"`
	SiteURL       string `json:"siteUrl"`
	Email         string `json:"email"`
}

var (
	// ErrNotConfigured: no config file, a field missing, or no token.
	// Being unconfigured is treated like being offline.
	ErrNotConfigured = errors.New("not connected to Jira")
	// ErrNewerSchema: the file was written by a newer punchcard.
	ErrNewerSchema = errors.New("config written by a newer punchcard")
	// ErrNoConfigDir: neither XDG_CONFIG_HOME nor HOME is set. The path
	// is never guessed relative to the working directory.
	ErrNoConfigDir = errors.New("neither XDG_CONFIG_HOME nor HOME is set")
)

// Path is the config file's path, or ErrNoConfigDir.
func Path(getenv func(string) string) (string, error) {
	if d := getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "punchcard", "config.json"), nil
	}
	if h := getenv("HOME"); h != "" {
		return filepath.Join(h, ".config", "punchcard", "config.json"), nil
	}
	return "", ErrNoConfigDir
}

// Load reads and checks the config file. Missing file or fields (or no
// config directory at all) is ErrNotConfigured; a newer schemaVersion is
// ErrNewerSchema; anything else (unreadable, malformed) is returned as is.
func Load(getenv func(string) string) (Config, error) {
	p, err := Path(getenv)
	if err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrNotConfigured, err)
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("%w: no %s (run punchcard setup)", ErrNotConfigured, p)
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", p, err)
	}
	if c.SchemaVersion > SchemaVersion {
		return Config{}, fmt.Errorf("%w: %s has schemaVersion %d", ErrNewerSchema, p, c.SchemaVersion)
	}
	if c.SiteURL == "" || c.Email == "" {
		return Config{}, fmt.Errorf("%w: %s needs siteUrl and email (run punchcard setup)", ErrNotConfigured, p)
	}
	return c, nil
}
