package config_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/config"
)

func TestSaveThenLoad(t *testing.T) {
	dir := t.TempDir()
	getenv := env(map[string]string{"XDG_CONFIG_HOME": filepath.Join(dir, "fresh")})
	want := config.Config{SiteURL: "https://acme.atlassian.net", Email: "me@acme.test"}
	if err := config.Save(getenv, want); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(getenv)
	if err != nil {
		t.Fatal(err)
	}
	want.SchemaVersion = config.SchemaVersion
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("round trip (-want +got):\n%s", diff)
	}
	// Overwriting replaces it.
	if err := config.Save(getenv, config.Config{SiteURL: "https://b.atlassian.net", Email: "b@b.test"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := config.Load(getenv); got.Email != "b@b.test" {
		t.Errorf("after second save: %+v", got)
	}
}

func TestTokenFile(t *testing.T) {
	dir := t.TempDir()
	getenv := env(map[string]string{"XDG_CONFIG_HOME": dir})
	if err := config.WriteTokenFile(getenv, "tok-1"); err != nil {
		t.Fatal(err)
	}
	p := config.TokenPath(getenv)
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("token file mode %v, want 0600", fi.Mode().Perm())
	}
	sec, err := config.Tokens{Getenv: getenv}.Lookup("me@acme.test")
	if err != nil || sec.Value != "tok-1" || sec.Source != config.SourceFile {
		t.Errorf("lookup after write: %+v, %v", sec, err)
	}
	if err := config.RemoveTokenFile(getenv); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("token file still there: %v", err)
	}
	if err := config.RemoveTokenFile(getenv); err != nil {
		t.Errorf("removing a missing file: %v", err)
	}
}
