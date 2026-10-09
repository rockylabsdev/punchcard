// Package config reads which tracker is chosen and, for Jira, where it is
// and who to authenticate as: the config file at
// $XDG_CONFIG_HOME/punchcard/config.json (else
// ~/.config/punchcard/config.json) and the API token. The token is read
// only when the tracker is Jira and a command needs it.
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
	SchemaVersion int `json:"schemaVersion"`
	// Tracker is the tracker chosen in setup; see Chosen for an absent
	// field.
	Tracker Tracker `json:"tracker,omitempty"`
	SiteURL string  `json:"siteUrl,omitempty"`
	Email   string  `json:"email,omitempty"`
}

// Tracker names the tracker worklogs are sent to, if any.
type Tracker string

const (
	TrackerNone Tracker = "none"
	TrackerJira Tracker = "jira"
)

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

// Chosen is the tracker in effect: config's tracker field. No config
// file (or no config directory) is TrackerNone; a file without the field
// is TrackerJira when it names a site and an email (a config written
// before there was a choice), else TrackerNone. A file punchcard cannot
// read, a newer schemaVersion or an unknown tracker is an error.
func Chosen(getenv func(string) string) (Tracker, error) {
	c, err := read(getenv)
	switch {
	case errors.Is(err, ErrNotConfigured):
		return TrackerNone, nil
	case err != nil:
		return "", err
	}
	switch c.Tracker {
	case TrackerNone, TrackerJira:
		return c.Tracker, nil
	case "":
		if c.SiteURL != "" && c.Email != "" {
			return TrackerJira, nil
		}
		return TrackerNone, nil
	}
	return "", fmt.Errorf("config.json: unknown tracker %q (want none or jira)", c.Tracker)
}

// Load reads and checks the config file. Missing file or fields (or no
// config directory at all) is ErrNotConfigured; a newer schemaVersion is
// ErrNewerSchema; anything else (unreadable, malformed) is returned as is.
func Load(getenv func(string) string) (Config, error) {
	c, err := read(getenv)
	if err != nil {
		return Config{}, err
	}
	if c.SiteURL == "" || c.Email == "" {
		p, _ := Path(getenv)
		return Config{}, fmt.Errorf("%w: %s needs siteUrl and email (run punchcard setup)", ErrNotConfigured, p)
	}
	return c, nil
}

// read is the file, checked for its schemaVersion only. No file (or no
// config directory) is ErrNotConfigured.
func read(getenv func(string) string) (Config, error) {
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
	return c, nil
}
