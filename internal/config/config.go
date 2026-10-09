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
	ErrNotConfigured = errors.New("Jira is not configured")
	// ErrNewerSchema: the file was written by a newer punchcard.
	ErrNewerSchema = errors.New("config written by a newer punchcard")
)

// Path is the config file's path, or "" when neither XDG_CONFIG_HOME
// nor HOME is set.
func Path(getenv func(string) string) string {
	if d := getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "punchcard", "config.json")
	}
	if h := getenv("HOME"); h != "" {
		return filepath.Join(h, ".config", "punchcard", "config.json")
	}
	return ""
}

// Load reads and checks the config file. Missing file or fields is
// ErrNotConfigured; a newer schemaVersion is ErrNewerSchema; anything
// else (unreadable, malformed) is returned as is.
func Load(getenv func(string) string) (Config, error) {
	p := Path(getenv)
	if p == "" {
		return Config{}, fmt.Errorf("%w: neither XDG_CONFIG_HOME nor HOME is set", ErrNotConfigured)
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
		return Config{}, fmt.Errorf("%w: %s needs siteUrl and email", ErrNotConfigured, p)
	}
	return c, nil
}

// Source names where a token came from; status reports it.
type Source string

const SourceEnv Source = "env" // PUNCHCARD_TOKEN

// Secret is the API token and where it was found.
type Secret struct {
	Value  string
	Source Source
}

// Token finds the API token. For now only PUNCHCARD_TOKEN; slice 11
// extends this to env → Keychain → file without changing callers.
func Token(getenv func(string) string) (Secret, error) {
	if t := getenv("PUNCHCARD_TOKEN"); t != "" {
		return Secret{Value: t, Source: SourceEnv}, nil
	}
	return Secret{}, fmt.Errorf("%w: no API token (set PUNCHCARD_TOKEN)", ErrNotConfigured)
}
