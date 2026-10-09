package setup

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
)

func TestFreshSetup(t *testing.T) {
	r := newRig(t)
	code := r.run("acme", email, tok, "", "")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, r.transcript())
	}
	r.has(
		"? Jira site", "echo → https://acme.atlassian.net",
		"id.atlassian.com/manage-profile/security/api-tokens", "classic",
		"Connected as Jamey Smith",
		"? Save these credentials? (Y/n) = y",
		"Token saved in the macOS Keychain",
		"claude plugin marketplace add j-dsmith/punchcard",
		"claude plugin install punchcard@punchcard -s user",
		`"Bash(punchcard stop *--yes*)"`,
	)
	if c, ok := r.config(); !ok || c.SiteURL != site || c.Email != email {
		t.Errorf("config = %+v (ok %v)", c, ok)
	}
	if diff := cmp.Diff([]string{"deleteAll", "write " + email + " " + tok}, r.kc.calls); diff != "" {
		t.Errorf("Keychain calls (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{site + " " + email + " " + tok}, r.jira.calls); diff != "" {
		t.Errorf("Test Connection calls (-want +got):\n%s", diff)
	}
	if _, ok := r.tokenFile(); ok {
		t.Errorf("a token file was written beside a Keychain item")
	}
	if s, ok := r.settings(); !ok || !strings.Contains(s, AskRule) {
		t.Errorf("settings.json = %q", s)
	}
}

func TestTestConnectionWalksBack(t *testing.T) {
	t.Run("a bad site returns to the site step", func(t *testing.T) {
		r := newRig(t)
		code := r.run("nope.example.com", email, tok, // test fails: site
			"acme", "", "", // site again; email and token keep what was typed
			"", "n")
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		r.has("✗ https://nope.example.com is not a reachable Jira Cloud site", "Connected as Jamey Smith")
		if got := strings.Count(r.transcript(), "? Jira site"); got != 2 {
			t.Errorf("site asked %d times, want 2:\n%s", got, r.transcript())
		}
		if c, _ := r.config(); c.SiteURL != site {
			t.Errorf("saved site %q", c.SiteURL)
		}
	})
	t.Run("a rejected token returns to the token step with the scoped-token copy", func(t *testing.T) {
		r := newRig(t)
		code := r.run("acme", email, "tok-scoped", // 401
			tok, // token again only
			"", "n")
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		r.has("✗ Jira rejected these credentials", "scoped", "Connected as Jamey Smith")
		if got := strings.Count(r.transcript(), "? Jira site"); got != 1 {
			t.Errorf("site asked %d times, want 1", got)
		}
		if got := strings.Count(r.transcript(), "? API token"); got != 2 {
			t.Errorf("token asked %d times, want 2", got)
		}
		if r.kc.items[email] != tok {
			t.Errorf("Keychain = %v", r.kc.items)
		}
	})
	t.Run("an empty or broken site never reaches the test", func(t *testing.T) {
		r := newRig(t)
		r.run("", "https://", "acme", email, tok, "n")
		r.has("! enter your Jira site")
		if len(r.jira.calls) != 1 {
			t.Errorf("Test Connection calls %v", r.jira.calls)
		}
	})
	t.Run("a blank token with nothing stored is refused at the field", func(t *testing.T) {
		r := newRig(t)
		r.run("acme", email, "", tok, "n")
		r.has("! paste an API token")
	})
}

func TestNothingSaved(t *testing.T) {
	steps := []struct {
		name    string
		answers []string
	}{
		{"esc at the site", []string{esc}},
		{"esc at the email", []string{"acme", esc}},
		{"esc at the token", []string{"acme", email, esc}},
		{"esc after a failed test", []string{"acme", email, "bad", esc}},
		{"esc at save", []string{"acme", email, tok, esc}},
		{"no at save", []string{"acme", email, tok, "n"}},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			r := newRig(t)
			r.configured("https://old.atlassian.net", "old@acme.test")
			r.kc.items = map[string]string{"old@acme.test": "tok-old"}
			code := r.run(append([]string{"y"}, s.answers...)...) // change credentials? y
			if code != 0 {
				t.Errorf("exit %d", code)
			}
			r.has("Nothing saved")
			if c, _ := r.config(); c.Email != "old@acme.test" {
				t.Errorf("config changed: %+v", c)
			}
			for _, c := range r.kc.calls {
				if !strings.HasPrefix(c, "read ") {
					t.Errorf("Keychain touched: %v", r.kc.calls)
				}
			}
			if _, ok := r.settings(); ok {
				t.Errorf("settings.json written after a cancel")
			}
			if len(r.claude.calls) > 0 {
				t.Errorf("claude run after a cancel: %v", r.claude.calls)
			}
		})
	}
}

func TestKeychainWriteFails(t *testing.T) {
	r := newRig(t)
	r.kc.writeErr = errors.New("security add-generic-password: User interaction is not allowed.")
	if code := r.run("acme", email, tok, "", "n"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	r.has("Could not write to the macOS Keychain (security add-generic-password: User interaction is not allowed.)",
		"saved the token in "+r.tokenPath()+" (readable only by you) instead")
	if v, ok := r.tokenFile(); !ok || v != tok {
		t.Errorf("token file = %q, %v", v, ok)
	}
	if fi, _ := os.Stat(r.tokenPath()); fi.Mode().Perm() != 0o600 {
		t.Errorf("token file mode %v", fi.Mode().Perm())
	}
}

func TestKeychainWriteRemovesTheFile(t *testing.T) {
	r := newRig(t)
	if err := config.WriteTokenFile(r.getenv, "tok-old-file"); err != nil {
		t.Fatal(err)
	}
	r.run("acme", email, tok, "", "n")
	if _, ok := r.tokenFile(); ok {
		t.Errorf("token file left behind after a Keychain write: status would call it stray")
	}
}

func TestLinuxWritesTheFile(t *testing.T) {
	r := newRig(t)
	r.linux = true
	r.run("acme", email, tok, "", "n")
	if v, ok := r.tokenFile(); !ok || v != tok {
		t.Errorf("token file = %q, %v", v, ok)
	}
	r.has("Token saved in " + r.tokenPath())
	r.lacks("Keychain")
}

func TestRerun(t *testing.T) {
	t.Run("keeping the credentials skips to the plugin and ask rule", func(t *testing.T) {
		r := newRig(t)
		r.configured(site, email)
		r.kc.items = map[string]string{email: tok}
		if code := r.run("", "n"); code != 0 {
			t.Fatalf("exit %d", code)
		}
		r.has("Connected to https://acme.atlassian.net as me@acme.test (token in Keychain)",
			"? Change credentials? (y/N) = n")
		r.lacks("? Jira site", "Nothing saved")
		if len(r.jira.calls) > 0 {
			t.Errorf("tested without changing anything")
		}
	})
	t.Run("a rejected token defaults to yes and says when", func(t *testing.T) {
		r := newRig(t)
		r.configured(site, email)
		r.kc.items = map[string]string{email: "tok-revoked"}
		if _, err := r.store.Apply(core.RecordAuthFailure{}); err != nil {
			t.Fatal(err)
		}
		r.now = r.now.Add(3 * 60 * 60e9)
		r.run("", "", "", tok, "", "n") // yes to change; keep site and email
		r.has("? Change credentials? (Y/n) [Jira rejected the token at 14:30 on Fri 9 Oct]")
		w, _, _ := r.store.Read()
		if !w.State.AuthFailedAt.IsZero() {
			t.Errorf("authFailedAt still set after a fresh setup")
		}
	})
	t.Run("the token field starts blank and names where it is stored", func(t *testing.T) {
		r := newRig(t)
		r.configured(site, email)
		r.kc.items = map[string]string{email: tok}
		r.run("y", "", "", "", "", "n")
		r.has("[Stored in Keychain — leave blank to keep]", "Connected as Jamey Smith")
		if r.kc.items[email] != tok {
			t.Errorf("Keychain = %v", r.kc.items)
		}
	})
	t.Run("a token in the file is named as the file", func(t *testing.T) {
		r := newRig(t)
		r.configured(site, email)
		if err := config.WriteTokenFile(r.getenv, tok); err != nil {
			t.Fatal(err)
		}
		r.run("n", "n")
		r.has("(token in " + r.tokenPath() + ")")
	})
	t.Run("a newer config is refused", func(t *testing.T) {
		r := newRig(t)
		if err := config.WriteFileAtomic(r.configPath(), []byte(`{"schemaVersion": 9}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := r.run(); code != 65 {
			t.Errorf("exit %d, want 65", code)
		}
		r.has("newer punchcard")
	})
}

func TestChangingOnlyTheEmailMovesTheItem(t *testing.T) {
	r := newRig(t)
	r.configured(site, "old@acme.test")
	r.kc.items = map[string]string{"old@acme.test": tok}
	r.run("y", "", email, "", "", "n") // change; keep site; new email; blank token
	want := []string{"read old@acme.test", "deleteAll", "write " + email + " " + tok}
	if diff := cmp.Diff(want, r.kc.calls); diff != "" {
		t.Errorf("Keychain calls (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{site + " " + email + " " + tok}, r.jira.calls); diff != "" {
		t.Errorf("tested with (-want +got):\n%s", diff)
	}
}

func TestEnvToken(t *testing.T) {
	t.Run("a typed token is tested, saved, and the override named", func(t *testing.T) {
		r := newRig(t)
		r.env["PUNCHCARD_TOKEN"] = "tok-env"
		r.run("acme", email, tok, "", "n")
		r.has("PUNCHCARD_TOKEN is set and overrides the saved token")
		if r.kc.items[email] != tok {
			t.Errorf("Keychain = %v", r.kc.items)
		}
	})
	t.Run("a blank token tests with the env token and says so", func(t *testing.T) {
		r := newRig(t)
		r.env["PUNCHCARD_TOKEN"] = tok
		r.run("acme", email, "", "", "n")
		r.has("[PUNCHCARD_TOKEN is set — leave blank to test with it]",
			"Testing with PUNCHCARD_TOKEN", "Connected as Jamey Smith",
			"No token saved: PUNCHCARD_TOKEN supplies it")
		if len(r.kc.calls) > 0 {
			t.Errorf("Keychain touched: %v", r.kc.calls)
		}
		if c, ok := r.config(); !ok || c.Email != email {
			t.Errorf("config not saved: %+v", c)
		}
	})
}

func TestFreshSetupClearsAuthFailure(t *testing.T) {
	r := newRig(t)
	if _, err := r.store.Apply(core.RecordAuthFailure{}); err != nil {
		t.Fatal(err)
	}
	r.run("acme", email, tok, "", "n")
	w, _, _ := r.store.Read()
	if !w.State.AuthFailedAt.IsZero() {
		t.Errorf("authFailedAt survived a fresh setup")
	}
}

func TestFreshSetupCreatesNoStateFile(t *testing.T) {
	r := newRig(t)
	r.run("acme", email, tok, "", "n")
	if _, err := os.Stat(r.store.Dir() + "/state.json"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("setup created state.json (%v); it is the first-run marker", err)
	}
}
