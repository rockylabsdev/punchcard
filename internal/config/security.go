package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Security is the macOS Keychain reached only through Apple's signed
// `security` binary (ADR 0006): an item it writes, it reads back for any
// caller with no prompt, even from a hook with no TTY. The secret never
// goes in argv: it comes back on stdout and is written over stdin.
type Security struct {
	Bin     string        // /usr/bin/security
	Service string        // punchcard
	Timeout time.Duration // per call; zero is KeychainTimeout
}

// System is the real Keychain item: generic password, service punchcard.
// Only a hand check may touch it; tests use a throwaway Service.
var System = Security{Bin: "/usr/bin/security", Service: "punchcard"}

// errSecItemNotFound is security's exit status for "no such item".
const errSecItemNotFound = 44

// Read is the Keychain seam: the token stored for account email, or
// ErrNoKeychainItem, ErrKeychainTimeout or the failure security named.
func (s Security) Read(email string) (string, error) {
	out, err := s.run(nil, "find-generic-password", "-s", s.Service, "-a", email, "-w")
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(out, "\n"), nil
}

// Write stores token for account email, replacing an item with the same
// account. The command goes over stdin to `security -i`, so the token is
// never in argv. Setup deletes every item first (DeleteAll), so there is
// one credential at a time.
func (s Security) Write(email, token string) error {
	if strings.ContainsAny(email+token, "\"\\\n\r") {
		return errors.New("the email or token has a character the Keychain command cannot carry")
	}
	line := fmt.Sprintf("add-generic-password -U -s %q -a %q -w %q\n", s.Service, email, token)
	out, err := s.run(strings.NewReader(line), "-i")
	if err != nil {
		return err
	}
	// security -i exits 0 even when the command inside it failed; a
	// failure is said on stderr, which run folds into out.
	if msg := strings.TrimSpace(out); msg != "" {
		return fmt.Errorf("security add-generic-password: %s", msg)
	}
	return nil
}

// DeleteAll removes every item for the service, whatever its account.
// None left is success.
func (s Security) DeleteAll() error {
	for range 50 {
		_, err := s.run(nil, "delete-generic-password", "-s", s.Service)
		if errors.Is(err, ErrNoKeychainItem) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return errors.New("security delete-generic-password: items keep coming back")
}

// run calls security with args (never a secret) and stdin; it returns
// stdout, or for `-i` stdout plus stderr.
func (s Security) run(stdin *strings.Reader, args ...string) (string, error) {
	d := s.Timeout
	if d == 0 {
		d = KeychainTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Bin, args...)
	cmd.WaitDelay = 100 * time.Millisecond
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if stdin != nil {
		cmd.Stdin = stdin
	}
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", ErrKeychainTimeout
	}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == errSecItemNotFound:
		return "", ErrNoKeychainItem
	case errors.As(err, &exit):
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = "exit status " + strconv.Itoa(exit.ExitCode())
		}
		return "", fmt.Errorf("security %s: %s", args[0], msg)
	case err != nil:
		return "", err
	}
	if len(args) == 1 && args[0] == "-i" {
		return stdout.String() + stderr.String(), nil
	}
	return stdout.String(), nil
}
