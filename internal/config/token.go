package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Source names where a token came from; status reports it.
type Source string

const (
	SourceEnv      Source = "env"      // PUNCHCARD_TOKEN
	SourceKeychain Source = "keychain" // the macOS Keychain, through /usr/bin/security
	SourceFile     Source = "file"     // $XDG_CONFIG_HOME/punchcard/token
)

// Secret is the API token and where it was found. StrayFile: the
// Keychain won while a token file also exists (status notes it).
type Secret struct {
	Value     string
	Source    Source
	StrayFile bool
}

// Keychain reads the token stored for account email: the seam over
// `security find-generic-password`. No item is ErrNoKeychainItem; any
// other error is the Keychain failing. Tests inject a fake; nothing in
// CI may reach a real Keychain.
type Keychain func(email string) (string, error)

// KeychainTimeout bounds a Keychain read (a healthy one takes ~30 ms).
const KeychainTimeout = time.Second

var (
	// ErrNoKeychainItem: the Keychain answered, and holds no token for
	// this account. The lookup moves on to the file.
	ErrNoKeychainItem = errors.New("no punchcard item in the Keychain")
	// ErrKeychainTimeout: the Keychain did not answer in time.
	ErrKeychainTimeout = errors.New("timed out reading the token")
)

// KeychainError: the Keychain could not be read (an error or a timeout)
// and there was no token file to fall back on. It is deliberately not
// ErrNotConfigured and never an auth failure; callers that treat any
// open error as not configured wrap it, and match it with errors.As to
// name the cause.
type KeychainError struct{ Err error }

func (e *KeychainError) Error() string { return "macOS Keychain: " + e.Err.Error() }
func (e *KeychainError) Unwrap() error { return e.Err }

// KeychainFor is the Keychain seam for an OS: the real `security` reader
// on darwin, nil (no Keychain in the chain) elsewhere.
func KeychainFor(goos string) Keychain {
	if goos == "darwin" {
		return System.Read
	}
	return nil
}

// SystemKeychain is KeychainFor(runtime.GOOS).
func SystemKeychain() Keychain { return KeychainFor(runtime.GOOS) }

// TokenPath is the 0600 token file beside config.json, or
// ErrNoConfigDir.
func TokenPath(getenv func(string) string) (string, error) {
	p, err := Path(getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "token"), nil
}

// Tokens is the token lookup: PUNCHCARD_TOKEN, then the Keychain (when
// Keychain is non-nil, i.e. macOS), then the token file.
type Tokens struct {
	Getenv   func(string) string
	Keychain Keychain      // nil: skip the Keychain (Linux)
	Timeout  time.Duration // for the Keychain read; zero is KeychainTimeout
}

// Lookup finds the token for account email. Nothing anywhere is
// ErrNotConfigured; a failing Keychain with no file is *KeychainError.
func (t Tokens) Lookup(email string) (Secret, error) {
	if v := t.Getenv("PUNCHCARD_TOKEN"); v != "" {
		return Secret{Value: v, Source: SourceEnv}, nil
	}
	tokPath, _ := TokenPath(t.Getenv) // no config dir: no token file
	file, ferr := readTokenFile(tokPath)
	var kcErr error
	if t.Keychain != nil {
		v, err := t.readKeychain(email)
		switch {
		case err == nil && v != "":
			return Secret{Value: v, Source: SourceKeychain, StrayFile: file != ""}, nil
		case err != nil && !errors.Is(err, ErrNoKeychainItem):
			kcErr = &KeychainError{Err: err}
		}
	}
	if file != "" {
		return Secret{Value: file, Source: SourceFile}, nil
	}
	if kcErr != nil {
		return Secret{}, kcErr
	}
	if ferr != nil {
		return Secret{}, ferr
	}
	return Secret{}, fmt.Errorf("%w: no API token (run punchcard setup, or set PUNCHCARD_TOKEN)", ErrNotConfigured)
}

// readKeychain calls the seam, giving up after the timeout. A seam that
// hangs is left to finish on its own (the real one kills `security`).
func (t Tokens) readKeychain(email string) (string, error) {
	type answer struct {
		v   string
		err error
	}
	ch := make(chan answer, 1)
	go func() {
		v, err := t.Keychain(email)
		ch <- answer{v, err}
	}()
	d := t.Timeout
	if d == 0 {
		d = KeychainTimeout
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case a := <-ch:
		return a.v, a.err
	case <-timer.C:
		return "", ErrKeychainTimeout
	}
}

// readTokenFile is the file's token, trimmed; "" when there is no file
// (or no path). An unreadable file is an error.
func readTokenFile(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// Credentials is the config file plus the token for its email: what a
// Jira-calling command needs, and what status reports. With no usable
// config it never reaches for a token.
func Credentials(getenv func(string) string, kc Keychain) (Config, Secret, error) {
	cfg, err := Load(getenv)
	if err != nil {
		return Config{}, Secret{}, err
	}
	sec, err := Tokens{Getenv: getenv, Keychain: kc}.Lookup(cfg.Email)
	if err != nil {
		return Config{}, Secret{}, err
	}
	return cfg, sec, nil
}
