package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
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

// Claude is the `claude` CLI on PATH. Tests use a fake: setup never
// runs the real one outside a terminal session.
type Claude interface {
	// Found: `claude` is on PATH.
	Found() bool
	// Run runs claude with args, its output going to out.
	Run(ctx context.Context, out io.Writer, args ...string) error
	// Output runs claude with args and returns its stdout.
	Output(ctx context.Context, args ...string) ([]byte, error)
}

// SystemClaude is the real `claude` on PATH.
type SystemClaude struct{}

func (SystemClaude) Found() bool { _, err := exec.LookPath("claude"); return err == nil }

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

func comparable(v string) bool {
	v = strings.TrimPrefix(v, "v")
	return v != "" && v != "dev" && v != "unknown" && v[0] >= '0' && v[0] <= '9'
}

func command(args []string) string { return "claude " + strings.Join(args, " ") }

// plugin is the Claude Code plugin step. A failure is reported and the
// wizard goes on: it never undoes saved credentials.
func (w wizard) plugin() error {
	c := w.Claude
	found := c != nil && c.Found()
	var have string
	var installed bool
	if found {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		out, err := c.Output(ctx, pluginList...)
		cancel()
		if err == nil {
			have, installed, err = installedVersion(out)
		}
		if err != nil {
			w.UI.Say("⚠ Could not ask Claude Code which plugins are installed: " + err.Error())
		}
	}
	switch decidePlugin(found, installed, have, w.Version) {
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
	err := w.Claude.Run(ctx, out, args...)
	if f, ok := out.(interface{ Flush() }); ok {
		f.Flush() // a last line with no newline
	}
	if err != nil {
		w.UI.Say("✗ "+command(args)+" failed: "+err.Error(),
			"  Your Jira credentials are unaffected; run it again by hand when you are ready.")
		return false
	}
	return true
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
	w.UI.Say("Claude Code asks you before an agent's punchcard stop --yes reaches Jira",
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
