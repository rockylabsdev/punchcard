// Package scripts holds the maintainer scripts; this test runs release.sh
// in a throwaway git repository.
package scripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const pluginJSON = `{
  "name": "punchcard",
  "version": "0.1.0",
  "description": "x"
}
`

const marketJSON = `{
  "name": "punchcard",
  "plugins": [
    {
      "name": "punchcard",
      "source": { "source": "git-subdir", "url": "https://github.com/j-dsmith/punchcard.git", "path": "plugin", "ref": "v0.1.0" }
    }
  ]
}
`

// repo is a git repository on main holding the two manifests and the
// script, committed and clean.
func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	script, err := os.ReadFile("release.sh")
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "plugin/.claude-plugin/plugin.json", pluginJSON, 0o644)
	write(t, dir, ".claude-plugin/marketplace.json", marketJSON, 0o644)
	write(t, dir, "scripts/release.sh", string(script), 0o755)
	git(t, dir, "init", "-q", "-b", "main", ".")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func write(t *testing.T, dir, name, body string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

// gitEnv keeps the user's git config and hooks out of the test.
func gitEnv(dir string) []string {
	return append(os.Environ(),
		"HOME="+dir, "XDG_CONFIG_HOME="+dir, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, gitEnv(dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func release(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(filepath.Join(dir, "scripts/release.sh"), args...)
	cmd.Dir, cmd.Env = dir, gitEnv(dir)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReleaseBumpsBothCommitsThenTags(t *testing.T) {
	dir := repo(t)
	out, code := release(t, dir, "0.2.0")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if d := cmp.Diff(strings.Replace(pluginJSON, `"0.1.0"`, `"0.2.0"`, 1), read(t, dir, "plugin/.claude-plugin/plugin.json")); d != "" {
		t.Errorf("plugin.json (-want +got):\n%s", d)
	}
	if d := cmp.Diff(strings.Replace(marketJSON, `"v0.1.0"`, `"v0.2.0"`, 1), read(t, dir, ".claude-plugin/marketplace.json")); d != "" {
		t.Errorf("marketplace.json (-want +got):\n%s", d)
	}
	if got := git(t, dir, "log", "-1", "--format=%s"); got != "Release v0.2.0" {
		t.Errorf("commit subject = %q", got)
	}
	if got := git(t, dir, "status", "--porcelain"); got != "" {
		t.Errorf("tree left dirty:\n%s", got)
	}
	if tagged, head := git(t, dir, "rev-list", "-n1", "v0.2.0"), git(t, dir, "rev-parse", "HEAD"); tagged != head {
		t.Errorf("v0.2.0 points at %s, HEAD is %s", tagged, head)
	}
	if got := git(t, dir, "cat-file", "-t", "v0.2.0"); got != "tag" {
		t.Errorf("v0.2.0 is a %s, want an annotated tag", got)
	}
	if !strings.Contains(out, "git push --atomic origin main v0.2.0") {
		t.Errorf("no push command printed:\n%s", out)
	}
}

func TestReleaseAlreadyPinnedTagsWithoutCommit(t *testing.T) {
	dir := repo(t)
	before := git(t, dir, "rev-parse", "HEAD")
	out, code := release(t, dir, "v0.1.0")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if got := git(t, dir, "rev-parse", "HEAD"); got != before {
		t.Errorf("made a commit with nothing to bump")
	}
	if got := git(t, dir, "rev-list", "-n1", "v0.1.0"); got != before {
		t.Errorf("v0.1.0 not at HEAD")
	}
}

// The repository's own manifests must agree: the marketplace ref is the tag
// of the plugin's version (ADR 0007).
func TestRepoManifestsAgree(t *testing.T) {
	var plugin struct {
		Version string `json:"version"`
	}
	var market struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source struct {
				URL string `json:"url"`
				Ref string `json:"ref"`
			} `json:"source"`
		} `json:"plugins"`
	}
	for name, v := range map[string]any{"../plugin/.claude-plugin/plugin.json": &plugin, "../.claude-plugin/marketplace.json": &market} {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, v); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if len(market.Plugins) != 1 || market.Plugins[0].Name != "punchcard" {
		t.Fatalf("marketplace plugins = %+v, want one punchcard entry", market.Plugins)
	}
	if got, want := market.Plugins[0].Source.Ref, "v"+plugin.Version; got != want {
		t.Errorf("marketplace ref %q, plugin.json version %q: want ref %q", got, plugin.Version, want)
	}
	// Over HTTPS a private clone uses the credential helper that
	// `gh auth setup-git` installs; the owner/repo shorthand cloned over
	// SSH (#58, ADR 0007).
	if u := market.Plugins[0].Source.URL; !strings.HasPrefix(u, "https://") {
		t.Errorf("marketplace url %q, want an https:// URL", u)
	}
}

func TestReleaseRefuses(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(t *testing.T, dir string)
		args    []string
		want    string
	}{
		{"no version", nil, nil, "usage"},
		{"not X.Y.Z", nil, []string{"1.2"}, "must be X.Y.Z"},
		{"untracked file", func(t *testing.T, dir string) { write(t, dir, "x", "", 0o644) }, []string{"0.2.0"}, "not clean"},
		{"modified file", func(t *testing.T, dir string) { write(t, dir, "plugin/.claude-plugin/plugin.json", "{}", 0o644) }, []string{"0.2.0"}, "not clean"},
		{"not on main", func(t *testing.T, dir string) { git(t, dir, "checkout", "-q", "-b", "feat") }, []string{"0.2.0"}, "release from main"},
		{"tag exists", func(t *testing.T, dir string) { git(t, dir, "tag", "v0.2.0") }, []string{"0.2.0"}, "already exists"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := repo(t)
			if c.prepare != nil {
				c.prepare(t, dir)
			}
			head := git(t, dir, "rev-parse", "HEAD")
			out, code := release(t, dir, c.args...)
			if code == 0 || !strings.Contains(out, c.want) {
				t.Errorf("exit %d, output %q; want a refusal saying %q", code, out, c.want)
			}
			if got := git(t, dir, "rev-parse", "HEAD"); got != head {
				t.Errorf("refusal moved HEAD")
			}
			if c.name != "tag exists" {
				if tags := git(t, dir, "tag"); tags != "" {
					t.Errorf("refusal left tags: %s", tags)
				}
			}
		})
	}
}
