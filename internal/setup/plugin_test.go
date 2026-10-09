package setup

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDecidePlugin(t *testing.T) {
	cases := []struct {
		name             string
		found, installed bool
		have, binary     string
		want             pluginAction
	}{
		{"no claude", false, false, "", "v0.1.0", pluginNoClaude},
		{"not installed", true, false, "", "v0.1.0", pluginAbsent},
		{"same version", true, true, "0.1.0", "v0.1.0", pluginCurrent},
		{"older plugin", true, true, "0.1.0", "v0.2.0", pluginOutdated},
		{"dev binary never offers an update", true, true, "0.1.0", "dev", pluginCurrent},
		{"unknown plugin version", true, true, "unknown", "v0.2.0", pluginCurrent},
	}
	for _, c := range cases {
		if got := decidePlugin(c.found, c.installed, c.have, c.binary); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestInstalledVersion(t *testing.T) {
	list := `[{"id":"other@x","version":"9.9.9","scope":"user","enabled":true},
	{"id":"punchcard@punchcard","version":"0.1.0","scope":"user","enabled":true,"installPath":"/p"}]`
	v, ok, err := installedVersion([]byte(list))
	if err != nil || !ok || v != "0.1.0" {
		t.Errorf("got %q, %v, %v", v, ok, err)
	}
	if _, ok, err := installedVersion([]byte("[]")); ok || err != nil {
		t.Errorf("empty list: %v, %v", ok, err)
	}
	if _, _, err := installedVersion([]byte("No plugins installed.")); err == nil {
		t.Errorf("non-JSON output gave no error")
	}
}

// pluginRig is a re-run that keeps its credentials, so the transcript
// starts at the plugin step.
func pluginRig(t *testing.T) *rig {
	r := newRig(t)
	r.configured(site, email)
	r.kc.items = map[string]string{email: tok}
	r.claude.found = true
	r.claude.list = "[]"
	return r
}

func TestPluginStep(t *testing.T) {
	t.Run("install runs marketplace add then install, output in the gutter", func(t *testing.T) {
		r := pluginRig(t)
		r.run("", "n", "", "n")
		r.has("? Install the Claude Code plugin? (Y/n)",
			"$ claude plugin marketplace add j-dsmith/punchcard", "$ claude plugin install punchcard@punchcard -s user")
		want := []string{"plugin list --json", "plugin marketplace add j-dsmith/punchcard", "plugin install punchcard@punchcard -s user"}
		if diff := cmp.Diff(want, r.claude.calls); diff != "" {
			t.Errorf("claude calls (-want +got):\n%s", diff)
		}
		if !strings.Contains(r.ui.out.String(), "ran plugin install") {
			t.Errorf("command output not shown: %q", r.ui.out.String())
		}
	})
	t.Run("declined runs nothing", func(t *testing.T) {
		r := pluginRig(t)
		r.run("", "n", "n", "n")
		if diff := cmp.Diff([]string{"plugin list --json"}, r.claude.calls); diff != "" {
			t.Errorf("claude calls (-want +got):\n%s", diff)
		}
	})
	t.Run("installed at this version is a tick, no question", func(t *testing.T) {
		r := pluginRig(t)
		r.claude.list = `[{"id":"punchcard@punchcard","version":"0.1.0"}]`
		r.run("", "n", "n")
		r.has("✓ Claude Code plugin installed (0.1.0)")
		r.lacks("? Install", "? Update")
	})
	t.Run("a version mismatch offers claude plugin update", func(t *testing.T) {
		r := pluginRig(t)
		r.claude.list = `[{"id":"punchcard@punchcard","version":"0.0.9"}]`
		r.run("", "n", "", "n")
		r.has("? Update the Claude Code plugin? (Y/n) [The plugin is 0.0.9; this punchcard is v0.1.0] = y",
			"$ claude plugin update punchcard@punchcard")
	})
	t.Run("a failure says so, skips install and goes on to the ask rule", func(t *testing.T) {
		r := pluginRig(t)
		r.claude.fail = map[string]error{"plugin marketplace add j-dsmith/punchcard": errors.New("exit status 1")}
		if code := r.run("", "y", "", "", "", "", "", "n"); code != 0 { // change creds, keep all, save, install, no ask rule
			t.Fatalf("exit %d", code)
		}
		r.has("✗ claude plugin marketplace add j-dsmith/punchcard failed: exit status 1", "? Add the ask rule")
		if c, ok := r.config(); !ok || c.Email != email {
			t.Errorf("credentials undone: %+v", c)
		}
		if r.kc.items[email] != tok {
			t.Errorf("Keychain = %v", r.kc.items)
		}
		for _, c := range r.claude.calls {
			if strings.HasPrefix(c, "plugin install") {
				t.Errorf("install ran after the marketplace add failed")
			}
		}
	})
	t.Run("no claude prints the commands", func(t *testing.T) {
		r := pluginRig(t)
		r.claude.found = false
		r.run("", "n", "n")
		r.has("claude plugin marketplace add j-dsmith/punchcard", "claude plugin install punchcard@punchcard -s user")
		if len(r.claude.calls) > 0 {
			t.Errorf("claude called: %v", r.claude.calls)
		}
	})
	t.Run("esc at the plugin question keeps saved credentials", func(t *testing.T) {
		r := pluginRig(t)
		r.run("", "y", "", "", "", "", esc) // change, keep all, save, esc
		r.has("Stopped. Your Jira credentials are saved.")
		r.lacks("Nothing saved")
		if r.kc.items[email] != tok {
			t.Errorf("Keychain = %v", r.kc.items)
		}
	})
}

func TestAskRuleStep(t *testing.T) {
	t.Run("present is a tick", func(t *testing.T) {
		r := pluginRig(t)
		r.claude.found = false
		writeSettings(t, r, `{"permissions":{"ask":["Bash(punchcard stop *--yes*)"]}}`)
		r.run("", "n")
		r.has("✓ Ask rule in place")
	})
	t.Run("unparseable is left alone and the line printed", func(t *testing.T) {
		r := pluginRig(t)
		r.claude.found = false
		writeSettings(t, r, `{"permissions": {`)
		r.run("", "n")
		r.has("Could not parse", `  "Bash(punchcard stop *--yes*)"`)
		if s, _ := r.settings(); s != `{"permissions": {` {
			t.Errorf("file changed: %q", s)
		}
	})
	t.Run("declined writes nothing", func(t *testing.T) {
		r := pluginRig(t)
		r.claude.found = false
		r.run("", "n", "n")
		if _, ok := r.settings(); ok {
			t.Errorf("settings.json written")
		}
	})
	t.Run("added to an existing file with a .bak", func(t *testing.T) {
		r := pluginRig(t)
		r.claude.found = false
		writeSettings(t, r, `{"model":"opus"}`)
		r.run("", "n", "")
		r.has("old file kept as " + SettingsPath(r.getenv) + ".bak")
		if s, _ := r.settings(); !strings.Contains(s, `"model": "opus"`) || !strings.Contains(s, AskRule) {
			t.Errorf("settings.json = %s", s)
		}
	})
}

func writeSettings(t *testing.T, r *rig, body string) {
	t.Helper()
	p := SettingsPath(r.getenv)
	if err := os.MkdirAll(r.env["CLAUDE_CONFIG_DIR"], 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
