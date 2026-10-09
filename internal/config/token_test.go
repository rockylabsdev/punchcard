package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/config"
)

const email = "user@example.com"

// keychain is a fake Keychain seam: it answers with token or err, or
// blocks until the test ends when hang is set. It records the accounts
// it was asked for.
type keychain struct {
	token string
	err   error
	hang  bool
	asked []string
}

func (k *keychain) read(t *testing.T) config.Keychain {
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	return func(account string) (string, error) {
		k.asked = append(k.asked, account)
		if k.hang {
			<-done
		}
		return k.token, k.err
	}
}

func writeToken(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "punchcard"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "punchcard", "token"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The chain: PUNCHCARD_TOKEN, then the Keychain (macOS only: a nil seam
// is Linux), then the 0600 file. A Keychain error with no file is its
// own error, never ErrNotConfigured and never an auth failure.
func TestTokenChain(t *testing.T) {
	broken := errors.New("security: SecKeychainSearchCopyNext: user interaction is not allowed")
	cases := []struct {
		name      string
		envToken  string
		file      string // "" = no file
		kc        *keychain
		linux     bool
		want      config.Secret
		wantErr   error // matched with errors.Is
		wantKCErr bool  // errors.As *config.KeychainError
		asked     []string
	}{
		{name: "env wins over everything", envToken: "e", file: "f\n", kc: &keychain{token: "k"},
			want: config.Secret{Value: "e", Source: config.SourceEnv}},
		{name: "keychain", kc: &keychain{token: "k"},
			want: config.Secret{Value: "k", Source: config.SourceKeychain}, asked: []string{email}},
		{name: "keychain wins and the file is stray", file: "f", kc: &keychain{token: "k"},
			want: config.Secret{Value: "k", Source: config.SourceKeychain, StrayFile: true}, asked: []string{email}},
		{name: "no keychain item falls to the file", file: "  f\n", kc: &keychain{err: config.ErrNoKeychainItem},
			want: config.Secret{Value: "f", Source: config.SourceFile}, asked: []string{email}},
		{name: "a keychain error falls to the file", file: "f", kc: &keychain{err: broken},
			want: config.Secret{Value: "f", Source: config.SourceFile}, asked: []string{email}},
		{name: "a keychain error with no file names the keychain", kc: &keychain{err: broken},
			wantErr: broken, wantKCErr: true, asked: []string{email}},
		{name: "a keychain timeout with no file names the keychain", kc: &keychain{hang: true},
			wantErr: config.ErrKeychainTimeout, wantKCErr: true, asked: []string{email}},
		{name: "a keychain timeout falls to the file", file: "f", kc: &keychain{hang: true},
			want: config.Secret{Value: "f", Source: config.SourceFile}, asked: []string{email}},
		{name: "nothing anywhere", kc: &keychain{err: config.ErrNoKeychainItem},
			wantErr: config.ErrNotConfigured, asked: []string{email}},
		{name: "an empty file is no token", file: "\n", kc: &keychain{err: config.ErrNoKeychainItem},
			wantErr: config.ErrNotConfigured, asked: []string{email}},
		{name: "linux: env", linux: true, envToken: "e", file: "f",
			want: config.Secret{Value: "e", Source: config.SourceEnv}},
		{name: "linux: the file, never a keychain", linux: true, file: "f",
			want: config.Secret{Value: "f", Source: config.SourceFile}},
		{name: "linux: nothing", linux: true, wantErr: config.ErrNotConfigured},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if c.file != "" {
				writeToken(t, dir, c.file)
			}
			vars := map[string]string{"XDG_CONFIG_HOME": dir, "PUNCHCARD_TOKEN": c.envToken}
			tok := config.Tokens{Getenv: env(vars), Timeout: 20 * time.Millisecond}
			if !c.linux {
				tok.Keychain = c.kc.read(t)
			}
			got, err := tok.Lookup(email)
			if c.wantErr == nil && err != nil {
				t.Fatalf("err = %v", err)
			}
			if c.wantErr != nil && !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			var kcErr *config.KeychainError
			if errors.As(err, &kcErr) != c.wantKCErr {
				t.Errorf("KeychainError = %v, want %t", err, c.wantKCErr)
			}
			if c.wantKCErr && errors.Is(err, config.ErrNotConfigured) {
				t.Errorf("a Keychain error is not ErrNotConfigured: %v", err)
			}
			if d := cmp.Diff(c.want, got); d != "" {
				t.Error(d)
			}
			if !c.linux {
				if d := cmp.Diff(c.asked, c.kc.asked); d != "" {
					t.Errorf("keychain asked (-want +got):\n%s", d)
				}
			}
		})
	}
}

// The Keychain error reads as the Keychain and carries only the cause.
func TestKeychainErrorText(t *testing.T) {
	tok := config.Tokens{Getenv: env(map[string]string{"XDG_CONFIG_HOME": t.TempDir()}),
		Keychain: func(string) (string, error) { return "", errors.New("exit status 51") }}
	_, err := tok.Lookup(email)
	if err == nil || err.Error() != "macOS Keychain: exit status 51" {
		t.Errorf("err = %v", err)
	}
}

// The platform picks the seam: the real `security` reader on macOS, none
// elsewhere.
func TestKeychainFor(t *testing.T) {
	if config.KeychainFor("linux") != nil {
		t.Error("linux has a Keychain")
	}
	if config.KeychainFor("darwin") == nil {
		t.Error("darwin has no Keychain")
	}
}

// Credentials is config plus the token for the configured email; with no
// config it never reaches for a token.
func TestCredentials(t *testing.T) {
	dir := t.TempDir()
	kc := &keychain{token: "k"}
	get := env(map[string]string{"XDG_CONFIG_HOME": dir})
	if _, _, err := config.Credentials(get, kc.read(t)); !errors.Is(err, config.ErrNotConfigured) || len(kc.asked) != 0 {
		t.Fatalf("no config: err = %v, asked %v", err, kc.asked)
	}
	writeConfig(t, dir, `{"schemaVersion": 1, "siteUrl": "https://example.atlassian.net", "email": "`+email+`"}`)
	cfg, sec, err := config.Credentials(get, kc.read(t))
	if err != nil || cfg.Email != email || sec != (config.Secret{Value: "k", Source: config.SourceKeychain}) {
		t.Errorf("Credentials = %+v, %+v, %v", cfg, sec, err)
	}
}

func TestTokenPath(t *testing.T) {
	if got := config.TokenPath(env(map[string]string{"XDG_CONFIG_HOME": "/x", "HOME": "/h"})); got != "/x/punchcard/token" {
		t.Errorf("TokenPath = %q", got)
	}
	if got := config.TokenPath(env(map[string]string{"HOME": "/h"})); got != "/h/.config/punchcard/token" {
		t.Errorf("TokenPath = %q", got)
	}
	if got := config.TokenPath(env(map[string]string{})); got != "" {
		t.Errorf("TokenPath = %q", got)
	}
}
