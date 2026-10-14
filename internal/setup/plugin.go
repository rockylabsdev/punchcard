package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/j-dsmith/punchcard/internal/config"
)

// PluginID is the plugin as Claude Code names it: plugin@marketplace.
const PluginID = "punchcard@punchcard"

// The commands the plugin step runs (slice 06's marketplace layout).
var (
	marketplaceAdd = []string{"plugin", "marketplace", "add", "j-dsmith/punchcard"}
	pluginInstall  = []string{"plugin", "install", PluginID, "-s", "user"}
	pluginUpdate   = []string{"plugin", "update", PluginID}
	pluginList     = []string{"plugin", "list", "--json"}
)

// pluginBudget bounds each `claude plugin` command (they clone from
// GitHub).
const pluginBudget = 3 * time.Minute

// inspectBudget bounds setup's `claude plugin list`.
const inspectBudget = time.Minute

// Claude is the `claude` CLI on PATH. Tests use a fake: setup never
// runs the real one outside a terminal session.
type Claude interface {
	// Path is where `claude` is on PATH; "" when it is not.
	Path() string
	// Run runs claude with args, its output going to out.
	Run(ctx context.Context, out io.Writer, args ...string) error
	// Output runs claude with args and returns its stdout.
	Output(ctx context.Context, args ...string) ([]byte, error)
}

// SystemClaude is the real `claude` on PATH.
type SystemClaude struct{}

func (SystemClaude) Path() string { p, _ := exec.LookPath("claude"); return p }

func (SystemClaude) Run(ctx context.Context, out io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.Stdin = nil
	cmd.Env = os.Environ()
	return cmd.Run()
}

func (SystemClaude) Output(ctx context.Context, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && stderr.Len() > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, err
}

// pluginAction is what the plugin step does, decided from what it found.
type pluginAction int

const (
	pluginNoClaude pluginAction = iota // print the commands to run later
	pluginCurrent                      // ✓, no question
	pluginOutdated                     // offer `claude plugin update`
	pluginAbsent                       // offer marketplace add + install
)

// installedVersion finds the plugin in `claude plugin list --json`
// output: its version, and whether it is installed at all.
func installedVersion(list []byte) (string, bool, error) {
	var plugins []struct {
		ID      string `json:"id"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(list, &plugins); err != nil {
		return "", false, fmt.Errorf("reading claude plugin list: %w", err)
	}
	for _, p := range plugins {
		if p.ID == PluginID {
			return p.Version, true, nil
		}
	}
	return "", false, nil
}

// decidePlugin is the plugin step's choice. A dev build, or a version
// Claude Code cannot name, never counts as a mismatch.
func decidePlugin(found, installed bool, have, binary string) pluginAction {
	switch {
	case !found:
		return pluginNoClaude
	case !installed:
		return pluginAbsent
	case comparable(binary) && comparable(have) && strings.TrimPrefix(have, "v") != strings.TrimPrefix(binary, "v"):
		return pluginOutdated
	}
	return pluginCurrent
}

// comparable reports whether v is a release, so that a difference is
// skew. dev, unknown and empty are not; nor is a Go pseudo-version
// (what `go install ./cmd/punchcard` stamps, e.g.
// v0.0.0-20261009220046-aaacff0d8d07) or a build of a dirty tree (+dirty).
// The plugin's release() in plugin/hooks/logic.mjs applies the same rule.
func comparable(v string) bool {
	v = strings.TrimPrefix(v, "v")
	return v != "" && v[0] >= '0' && v[0] <= '9' &&
		!pseudoVersion.MatchString(v) && !strings.Contains(v, "+dirty")
}

// pseudoVersion is a Go pseudo-version's tail, in each of its three
// forms (-TIMESTAMP-HASH, -pre.0.TIMESTAMP-HASH, -0.TIMESTAMP-HASH).
var pseudoVersion = regexp.MustCompile(`[-.][0-9]{14}-[0-9a-f]{12}(\+|$)`)

// PluginState is the punchcard plugin as Claude Code reports it: what
// setup's plugin step decides from, and what doctor prints.
type PluginState struct {
	Found     bool   // `claude` is on PATH
	Installed bool   // the plugin is in `claude plugin list`
	Version   string // the installed plugin's version
	// Err is why Claude Code could not say (Found, but the list failed).
	Err error
}

// InspectPlugin asks c which plugins are installed, within ctx: the
// caller sets the budget. A nil c, or one not on PATH, is not found and
// is never run.
func InspectPlugin(ctx context.Context, c Claude) PluginState {
	if c == nil || c.Path() == "" {
		return PluginState{}
	}
	p := PluginState{Found: true}
	out, err := c.Output(ctx, pluginList...)
	if err == nil {
		p.Version, p.Installed, err = installedVersion(out)
	}
	p.Err = err
	return p
}

// Comparable reports whether the plugin and binary can be compared:
// both are installed releases (comparable). Only then is Skewed meaningful.
func (p PluginState) Comparable(binary string) bool {
	return p.Installed && comparable(binary) && comparable(p.Version)
}

// Skewed reports whether the installed plugin is a different release
// from binary, which setup offers to update.
func (p PluginState) Skewed(binary string) bool {
	return decidePlugin(p.Found, p.Installed, p.Version, binary) == pluginOutdated
}

func command(args []string) string { return "claude " + strings.Join(args, " ") }

// plugin is the Claude Code plugin step. A failure is reported and the
// wizard goes on: it never undoes saved credentials.
func (w wizard) plugin() error {
	ctx, cancel := context.WithTimeout(context.Background(), inspectBudget)
	p := InspectPlugin(ctx, w.Claude)
	cancel()
	if p.Err != nil {
		w.UI.Say("⚠ Could not ask Claude Code which plugins are installed: " + p.Err.Error())
	}
	have := p.Version
	switch decidePlugin(p.Found, p.Installed, have, w.Version) {
	case pluginNoClaude:
		w.UI.Say("Claude Code (claude) is not on PATH. To add the punchcard plugin later, run:",
			"  "+command(marketplaceAdd), "  "+command(pluginInstall))
		return nil
	case pluginCurrent:
		w.UI.Say("✓ Claude Code plugin installed (" + have + ")")
		return nil
	case pluginOutdated:
		ok, err := w.UI.Confirm(Question{
			Title:   "Update the Claude Code plugin?",
			Note:    []string{fmt.Sprintf("The plugin is %s; this punchcard is %s", have, w.Version)},
			Default: true,
		})
		if err != nil || !ok {
			return err
		}
		w.runClaude(pluginUpdate)
		return nil
	}
	ok, err := w.UI.Confirm(Question{
		Title:   "Install the Claude Code plugin?",
		Note:    []string{"It shows your timer above the prompt and lets Claude start, pause and stop it"},
		Default: true,
	})
	if err != nil || !ok {
		return err
	}
	if w.runClaude(marketplaceAdd) {
		w.runClaude(pluginInstall)
	}
	return nil
}

// runClaude runs one claude command with its output in the gutter and
// says how it went.
func (w wizard) runClaude(args []string) bool {
	w.UI.Say("$ " + command(args))
	ctx, cancel := context.WithTimeout(context.Background(), pluginBudget)
	defer cancel()
	out := w.UI.Output()
	var said bytes.Buffer
	err := w.Claude.Run(ctx, io.MultiWriter(out, &said), args...)
	if f, ok := out.(interface{ Flush() }); ok {
		f.Flush() // a last line with no newline
	}
	if err != nil {
		lines := []string{"✗ " + command(args) + " failed: " + err.Error()}
		jira := w.jira()
		switch why := pluginFailure(said.String()); {
		case why != "" && jira:
			lines = append(lines, "  "+why, "  Your Jira credentials are unaffected.")
		case why != "":
			lines = append(lines, "  "+why)
		case jira:
			lines = append(lines, "  Your Jira credentials are unaffected; run it again by hand when you are ready.")
		default:
			lines = append(lines, "  Run it again by hand when you are ready.")
		}
		w.UI.Say(lines...)
		return false
	}
	return true
}

// jira reports whether Jira is the tracker the tracker step left, so a
// failure mentions Jira credentials only when there are some.
func (w wizard) jira() bool {
	t, err := config.Chosen(w.Getenv)
	return err == nil && t == config.TrackerJira
}

// askRule is the ask-rule step: offered whatever the plugin step did.
func (w wizard) askRule() error {
	path := SettingsPath(w.Getenv)
	if path == "" {
		return nil
	}
	plan, err := readAskRule(path)
	if err != nil {
		w.UI.Say("⚠ Could not read " + path + ": " + err.Error())
		return nil
	}
	line := string(quote(AskRule))
	switch plan.state {
	case askPresent:
		w.UI.Say("✓ Ask rule in place (" + path + ")")
		return nil
	case askUnparseable:
		w.UI.Say("Could not parse "+path+"; left it alone. To gate an agent's stop, add this to permissions.ask by hand:",
			"  "+line)
		return nil
	}
	w.UI.Say("Claude Code asks you before an agent's punchcard stop --yes logs a worklog",
		"if your settings have this rule:",
		`  { "permissions": { "ask": [`+line+`] } }`)
	ok, err := w.UI.Confirm(Question{Title: "Add the ask rule to " + path + "?", Default: true})
	if err != nil || !ok {
		return err
	}
	if err := writeAskRule(path, plan); err != nil {
		w.UI.Say("✗ Could not write " + path + ": " + err.Error())
		return nil
	}
	if plan.exists {
		w.UI.Say("✓ Added the ask rule (old file kept as " + path + ".bak)")
	} else {
		w.UI.Say("✓ Created " + path + " with the ask rule")
	}
	return nil
}

// missingTag is git's answer when the marketplace pins a ref that was
// never pushed: a build whose release is not out yet.
var missingTag = regexp.MustCompile(`Remote branch (\S+) not found`)

// pluginFailure is why a `claude plugin` command failed, from its
// output, for the two failures a tester can fix; "" for anything else.
func pluginFailure(said string) string {
	switch {
	case strings.Contains(said, "Permission denied (publickey)"), strings.Contains(said, "could not read Username"),
		strings.Contains(said, "unable to get password from user"), strings.Contains(said, "terminal prompts disabled"):
		return "Claude Code could not clone the repository. It is private: run `gh auth login` and `gh auth setup-git`, then `punchcard setup` again."
	}
	if m := missingTag.FindStringSubmatch(said); m != nil {
		return "This build is `" + m[1] + "` and no release with that tag exists yet. The plugin can be loaded from a clone for one session: `claude --plugin-dir /path/to/punchcard/plugin`."
	}
	return ""
}
