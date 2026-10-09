package setup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSettingsPath(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"HOME": "/h"}, "/h/.claude/settings.json"},
		{map[string]string{"HOME": "/h", "CLAUDE_CONFIG_DIR": "/cc"}, "/cc/settings.json"},
		{map[string]string{}, ""},
	}
	for _, c := range cases {
		if got := SettingsPath(func(k string) string { return c.env[k] }); got != c.want {
			t.Errorf("SettingsPath(%v) = %q, want %q", c.env, got, c.want)
		}
	}
}

func TestAskRule(t *testing.T) {
	cases := []struct {
		name   string
		before string // "" = no file
		state  askState
		after  string // the file once written; "" = not written
	}{
		{
			name:  "missing file is created with just the rule",
			state: askMissing,
			after: `{
  "permissions": {
    "ask": [
      "Bash(punchcard stop *--yes*)"
    ]
  }
}
`,
		},
		{
			name: "appended, every other key kept in its order",
			before: `{"model": "opus", "permissions": {"allow": ["Bash(ls *)"], "ask": ["Bash(rm *)"], "deny": []},
  "hooks": {"Stop": [{"matcher": "", "hooks": [{"type": "command", "command": "echo <hi> & bye"}]}]}}`,
			state: askMissing,
			after: `{
  "model": "opus",
  "permissions": {
    "allow": [
      "Bash(ls *)"
    ],
    "ask": [
      "Bash(rm *)",
      "Bash(punchcard stop *--yes*)"
    ],
    "deny": []
  },
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "echo <hi> & bye"
          }
        ]
      }
    ]
  }
}
`,
		},
		{
			name:   "permissions without ask",
			before: `{"permissions": {"defaultMode": "plan"}, "z": 1}`,
			state:  askMissing,
			after: `{
  "permissions": {
    "defaultMode": "plan",
    "ask": [
      "Bash(punchcard stop *--yes*)"
    ]
  },
  "z": 1
}
`,
		},
		{
			name:   "no permissions key",
			before: `{"theme": "dark"}`,
			state:  askMissing,
			after: `{
  "theme": "dark",
  "permissions": {
    "ask": [
      "Bash(punchcard stop *--yes*)"
    ]
  }
}
`,
		},
		{name: "empty file", before: " \n", state: askMissing, after: "{\n  \"permissions\": {\n    \"ask\": [\n      \"Bash(punchcard stop *--yes*)\"\n    ]\n  }\n}\n"},
		{name: "already there", before: `{"permissions": {"ask": ["Bash(punchcard stop *--yes*)"]}}`, state: askPresent},
		{name: "unparseable", before: `{"permissions": {"ask": [}`, state: askUnparseable},
		{name: "comments are unparseable", before: "// mine\n{}", state: askUnparseable},
		{name: "top level not an object", before: `[]`, state: askUnparseable},
		{name: "ask not a list", before: `{"permissions": {"ask": "Bash(x)"}}`, state: askUnparseable},
		{name: "permissions not an object", before: `{"permissions": true}`, state: askUnparseable},
		{name: "trailing garbage", before: `{} {}`, state: askUnparseable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "claude", "settings.json")
			if c.before != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(c.before), 0o640); err != nil {
					t.Fatal(err)
				}
			}
			plan, err := readAskRule(path)
			if err != nil {
				t.Fatal(err)
			}
			if plan.state != c.state {
				t.Fatalf("state = %v, want %v", plan.state, c.state)
			}
			if c.after == "" {
				return
			}
			if err := writeAskRule(path, plan); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(path)
			if diff := cmp.Diff(c.after, string(got)); diff != "" {
				t.Errorf("settings.json (-want +got):\n%s", diff)
			}
			bak, err := os.ReadFile(path + ".bak")
			switch {
			case c.before == "" && !errors.Is(err, fs.ErrNotExist):
				t.Errorf("a new file left a .bak: %v", err)
			case c.before != "" && string(bak) != c.before:
				t.Errorf(".bak = %q, want the old file %q", bak, c.before)
			}
			if c.before != "" {
				if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o640 {
					t.Errorf("mode %v, want the old file's 0640", fi.Mode().Perm())
				}
			}
			again, _ := readAskRule(path)
			if again.state != askPresent {
				t.Errorf("after writing, state = %v, want present", again.state)
			}
			left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".settings.json.tmp-*"))
			if len(left) > 0 {
				t.Errorf("temp files left: %v", left)
			}
		})
	}
}

// A dotfiles symlink stays a symlink: the file it points at is replaced.
func TestAskRuleThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "settings.json")
	if err := os.MkdirAll(filepath.Dir(real), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	plan, err := readAskRule(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAskRule(link, plan); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("the symlink was replaced by a file")
	}
	if p, _ := readAskRule(real); p.state != askPresent {
		t.Errorf("the target file was not updated")
	}
}
