package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/config"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func writeConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "punchcard"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "punchcard", "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	cases := []struct {
		name    string
		body    string // "" = no file
		want    config.Config
		wantErr error
	}{
		{"hand-written", `{"schemaVersion": 1, "siteUrl": "https://example.atlassian.net", "email": "user@example.com"}`,
			config.Config{SchemaVersion: 1, SiteURL: "https://example.atlassian.net", Email: "user@example.com"}, nil},
		{"no file", "", config.Config{}, config.ErrNotConfigured},
		{"no email", `{"schemaVersion": 1, "siteUrl": "https://example.atlassian.net"}`, config.Config{}, config.ErrNotConfigured},
		{"no site", `{"schemaVersion": 1, "email": "user@example.com"}`, config.Config{}, config.ErrNotConfigured},
		{"newer", `{"schemaVersion": 2, "siteUrl": "https://x", "email": "e"}`, config.Config{}, config.ErrNewerSchema},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if c.body != "" {
				writeConfig(t, dir, c.body)
			}
			got, err := config.Load(env(map[string]string{"XDG_CONFIG_HOME": dir}))
			if !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if d := cmp.Diff(c.want, got); d != "" {
				t.Error(d)
			}
		})
	}
}

func TestLoadMalformedIsNotSilentlyUnconfigured(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `{"siteUrl": `)
	_, err := config.Load(env(map[string]string{"XDG_CONFIG_HOME": dir}))
	if err == nil || errors.Is(err, config.ErrNotConfigured) {
		t.Errorf("err = %v, want a decode error", err)
	}
}

func TestPath(t *testing.T) {
	cases := []struct {
		env     map[string]string
		want    string
		wantErr error
	}{
		{map[string]string{"XDG_CONFIG_HOME": "/x", "HOME": "/h"}, "/x/punchcard/config.json", nil},
		{map[string]string{"HOME": "/h"}, "/h/.config/punchcard/config.json", nil},
		{map[string]string{}, "", config.ErrNoConfigDir}, // never a path relative to the working directory
	}
	for _, c := range cases {
		got, err := config.Path(env(c.env))
		if got != c.want || !errors.Is(err, c.wantErr) {
			t.Errorf("Path(%v) = %q, %v; want %q, %v", c.env, got, err, c.want, c.wantErr)
		}
		tok, terr := config.TokenPath(env(c.env))
		if (tok == "") != (c.want == "") || !errors.Is(terr, c.wantErr) {
			t.Errorf("TokenPath(%v) = %q, %v", c.env, tok, terr)
		}
	}
	// Unconfigured, and the reason names the missing variables.
	_, err := config.Load(env(map[string]string{}))
	if !errors.Is(err, config.ErrNotConfigured) || !errors.Is(err, config.ErrNoConfigDir) {
		t.Errorf("no HOME: err = %v", err)
	}
	if err := config.Save(env(map[string]string{}), config.Config{SiteURL: "s", Email: "e"}); !errors.Is(err, config.ErrNoConfigDir) {
		t.Errorf("Save with no HOME: err = %v", err)
	}
}
