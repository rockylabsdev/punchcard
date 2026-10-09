package config

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"
)

// TestSecurityLive round-trips a throwaway Keychain item through the real
// /usr/bin/security. It never runs in CI or by default: set
// PUNCHCARD_LIVE_KEYCHAIN=1 on a Mac to run it. It never touches the
// service "punchcard", and deletes its own item.
func TestSecurityLive(t *testing.T) {
	if os.Getenv("PUNCHCARD_LIVE_KEYCHAIN") != "1" {
		t.Skip("set PUNCHCARD_LIVE_KEYCHAIN=1 to touch a throwaway Keychain item")
	}
	s := Security{Bin: "/usr/bin/security", Service: fmt.Sprintf("punchcard-test-%d", rand.Int64())}
	if s.Service == System.Service {
		t.Fatal("never the real service")
	}
	t.Cleanup(func() { s.DeleteAll() })

	const account, token = "live@example.com", "ATATT3xFfGF0-a_b=c+d/e 1"
	if _, err := s.Read(account); !errors.Is(err, ErrNoKeychainItem) {
		t.Fatalf("before: err = %v", err)
	}
	if err := s.Write(account, token); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(account, token+"2"); err != nil { // -U replaces
		t.Fatal(err)
	}
	if got, err := s.Read(account); err != nil || got != token+"2" {
		t.Fatalf("Read = %q, %v", got, err)
	}
	if err := s.Write("other@example.com", token); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAll(); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{account, "other@example.com"} {
		if _, err := s.Read(a); !errors.Is(err, ErrNoKeychainItem) {
			t.Errorf("after DeleteAll %s: err = %v", a, err)
		}
	}
}
