package setup

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
)

func TestFreshSetup(t *testing.T) {
	r := newRig(t)
	code := r.run("jira", "acme", email, tok, "", "")
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
		code := r.run("jira", "nope.example.com", email, tok, // test fails: site
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
		code := r.run("jira", "acme", email, "tok-scoped", // 401
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
		r.run("jira", "", "https://", "acme", email, tok, "n")
		r.has("! enter your Jira site")
		if len(r.jira.calls) != 1 {
			t.Errorf("Test Connection calls %v", r.jira.calls)
		}
	})
	t.Run("a blank token with nothing stored is refused at the field", func(t *testing.T) {
		r := newRig(t)
		r.run("jira", "acme", email, "", tok, "n")
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
			code := r.run(append([]string{"", "y"}, s.answers...)...) // keep Jira; change credentials? y
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
	if code := r.run("jira", "acme", email, tok, "", "n"); code != 0 {
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
	r.run("jira", "acme", email, tok, "", "n")
	if _, ok := r.tokenFile(); ok {
		t.Errorf("token file left behind after a Keychain write: status would call it stray")
	}
}

func TestLinuxWritesTheFile(t *testing.T) {
	r := newRig(t)
	r.linux = true
	r.run("jira", "acme", email, tok, "", "n")
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
		if code := r.run("", "", "n"); code != 0 {
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
		r.run("", "", "", "", tok, "", "n") // yes to change; keep site and email
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
		r.run("", "y", "", "", "", "", "n")
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
		r.run("", "n", "n")
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
	r.run("", "y", "", email, "", "", "n") // change; keep site; new email; blank token
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
		r.run("jira", "acme", email, tok, "", "n")
		r.has("PUNCHCARD_TOKEN is set and overrides the saved token")
		if r.kc.items[email] != tok {
			t.Errorf("Keychain = %v", r.kc.items)
		}
	})
	t.Run("a blank token tests with the env token and says so", func(t *testing.T) {
		r := newRig(t)
		r.env["PUNCHCARD_TOKEN"] = tok
		r.run("jira", "acme", email, "", "", "n")
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
	r.run("jira", "acme", email, tok, "", "n")
	w, _, _ := r.store.Read()
	if !w.State.AuthFailedAt.IsZero() {
		t.Errorf("authFailedAt survived a fresh setup")
	}
}

func TestFreshSetupCreatesNoStateFile(t *testing.T) {
	r := newRig(t)
	r.run("jira", "acme", email, tok, "", "n")
	if _, err := os.Stat(r.store.Dir() + "/state.json"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("setup created state.json (%v) with nothing to put in it", err)
	}
}

func TestTrackerStep(t *testing.T) {
	t.Run("a fresh run choosing none writes tracker none and no site or email, then offers the plugin", func(t *testing.T) {
		r := newRig(t)
		r.claude.found = true
		r.claude.list = "[]"
		if code := r.run("", "n", "n"); code != 0 { // none (prefilled), no plugin, no ask rule
			t.Fatalf("exit %d\n%s", code, r.transcript())
		}
		r.has("? Tracker {none, jira} = none", "? Install the Claude Code plugin? (Y/n)", "? Add the ask rule")
		r.lacks("? Jira site", "? API token")
		if got := r.file(); got != (config.Config{SchemaVersion: config.SchemaVersion, Tracker: config.TrackerNone}) {
			t.Errorf("config.json = %+v", got)
		}
		if len(r.kc.calls) > 0 || len(r.jira.calls) > 0 {
			t.Errorf("none touched the Keychain %v or Jira %v", r.kc.calls, r.jira.calls)
		}
	})
	t.Run("choosing Jira runs the credentials steps and writes tracker jira", func(t *testing.T) {
		r := newRig(t)
		if code := r.run("jira", "acme", email, tok, "", "n"); code != 0 {
			t.Fatalf("exit %d\n%s", code, r.transcript())
		}
		r.has("? Tracker {none, jira} = jira", "? Jira site", "Connected as Jamey Smith")
		want := config.Config{SchemaVersion: config.SchemaVersion, Tracker: config.TrackerJira, SiteURL: site, Email: email}
		if got := r.file(); got != want {
			t.Errorf("config.json = %+v", got)
		}
	})
	t.Run("a re-run with Jira configured shows Jira selected", func(t *testing.T) {
		r := newRig(t)
		r.configured(site, email)
		r.kc.items = map[string]string{email: tok}
		r.run("", "n", "n")
		r.has("? Tracker {none, jira} = jira", "? Change credentials? (y/N) = n")
	})
	t.Run("keeping a config from before the choice writes tracker jira", func(t *testing.T) {
		r := newRig(t)
		r.configured(site, email)
		r.kc.items = map[string]string{email: tok}
		r.run("", "n", "n")
		if got := r.file().Tracker; got != config.TrackerJira {
			t.Errorf("tracker = %q", got)
		}
	})
	t.Run("a re-run with none configured shows none selected", func(t *testing.T) {
		r := newRig(t)
		if err := config.Save(r.getenv, config.Config{Tracker: config.TrackerNone}); err != nil {
			t.Fatal(err)
		}
		r.run("", "n")
		r.has("? Tracker {none, jira} = none")
	})
	t.Run("esc at the tracker saves nothing", func(t *testing.T) {
		r := newRig(t)
		if code := r.run(esc); code != 0 {
			t.Errorf("exit %d", code)
		}
		r.has("Nothing saved")
		if _, err := os.Stat(r.configPath()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("config.json written after esc (%v)", err)
		}
	})
	t.Run("choosing none keeps the Keychain, and choosing Jira again finds the token", func(t *testing.T) {
		r := newRig(t)
		r.configured(site, email)
		r.kc.items = map[string]string{email: tok}
		r.run("none", "n")
		if len(r.kc.calls) > 0 {
			t.Errorf("choosing none touched the Keychain: %v", r.kc.calls)
		}
		if got := r.file().Tracker; got != config.TrackerNone {
			t.Errorf("tracker = %q", got)
		}
		r.run("jira", "", "", "", "", "n") // keep site, email and the stored token; save
		r.has("? Tracker {none, jira} = jira", "[Stored in Keychain — leave blank to keep]", "Connected as Jamey Smith")
		r.lacks("? Change credentials?")
		want := config.Config{SchemaVersion: config.SchemaVersion, Tracker: config.TrackerJira, SiteURL: site, Email: email}
		if got := r.file(); got != want {
			t.Errorf("config.json = %+v", got)
		}
		if r.kc.items[email] != tok {
			t.Errorf("Keychain = %v", r.kc.items)
		}
	})
}

// owing gives the rig two pending worklogs and one failed, 2h 15m in
// all.
func owing(t *testing.T, r *rig) {
	t.Helper()
	for i, d := range []time.Duration{time.Hour, 45 * time.Minute, 30 * time.Minute} {
		key := []string{"AA-1", "AA-2", "AA-3"}[i]
		cmds := []core.Command{
			core.Start{Key: key, Origin: core.OriginHand, NewEntryID: "E" + key},
			core.Stop{Key: key, Duration: d, Origin: core.OriginHand},
		}
		if i == 2 {
			cmds = append(cmds, core.MarkFailed{Key: key, EntryID: "E" + key, Message: "Service Unavailable", Status: 503})
		}
		for _, c := range cmds {
			if _, err := r.store.Apply(c); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestLeavingJira(t *testing.T) {
	jiraRig := func(t *testing.T) *rig {
		r := newRig(t)
		r.configured(site, email)
		r.kc.items = map[string]string{email: tok}
		return r
	}
	states := func(r *rig) (out []string) {
		w, _, err := r.store.Read()
		if err != nil {
			r.t.Fatal(err)
		}
		for _, tk := range w.State.TrackedTickets {
			for _, e := range tk.Entries {
				out = append(out, tk.Key+" "+string(e.Worklog.State)+" "+e.Worklog.Message)
			}
		}
		return out
	}
	t.Run("accepted, every owed worklog is local with no failure", func(t *testing.T) {
		r := jiraRig(t)
		owing(t, r)
		if code := r.run("none", "", "n"); code != 0 {
			t.Fatalf("exit %d\n%s", code, r.transcript())
		}
		r.has("? Mark 3 worklogs (2h 15m) as local? (Y/n)", "Marked 3 worklogs local")
		want := []string{"AA-1 local ", "AA-2 local ", "AA-3 local "}
		if diff := cmp.Diff(want, states(r)); diff != "" {
			t.Errorf("worklogs (-want +got):\n%s", diff)
		}
		w, _, _ := r.store.Read()
		if n, _ := core.Owed(w.State); n != 0 {
			t.Errorf("still owed: %d", n)
		}
		if got := r.file().Tracker; got != config.TrackerNone {
			t.Errorf("tracker = %q", got)
		}
	})
	t.Run("declined, they stay pending and failed", func(t *testing.T) {
		r := jiraRig(t)
		owing(t, r)
		r.run("none", "n", "n")
		want := []string{"AA-1 pending ", "AA-2 pending ", "AA-3 failed Service Unavailable"}
		if diff := cmp.Diff(want, states(r)); diff != "" {
			t.Errorf("worklogs (-want +got):\n%s", diff)
		}
		if got := r.file().Tracker; got != config.TrackerNone {
			t.Errorf("tracker = %q", got)
		}
	})
	t.Run("esc at the offer saves nothing", func(t *testing.T) {
		r := jiraRig(t)
		owing(t, r)
		r.run("none", esc)
		r.has("Nothing saved")
		if got := r.file().Tracker; got != "" {
			t.Errorf("tracker = %q", got)
		}
	})
	t.Run("nothing owed, no offer", func(t *testing.T) {
		r := jiraRig(t)
		r.run("none", "n")
		r.lacks("as local?")
	})
	t.Run("already none, no offer", func(t *testing.T) {
		r := newRig(t)
		if err := config.Save(r.getenv, config.Config{Tracker: config.TrackerNone}); err != nil {
			t.Fatal(err)
		}
		owing(t, r)
		r.run("", "n")
		r.lacks("as local?")
	})
}
