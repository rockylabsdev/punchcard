package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/setup"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/spf13/cobra"
)

// setupCmd is `punchcard setup`: interactive only, so a token never
// lands in shell history or an agent's reach.
func (a *app) setupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Configure Jira and the Claude Code plugin (needs a terminal)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !a.env.TTY || a.out.json {
				return usageError("setup is interactive; run it in a terminal")
			}
			a.code = a.env.Setup(a.env)
			return nil
		},
	}
}

// firstRun: no config file and no state file. The state file's
// existence is the "already introduced" marker; with no config
// directory at all there is nowhere to set up.
func firstRun(e Env) bool {
	p, err := config.Path(e.Getenv)
	if err != nil || e.Store.Err() != nil {
		return false
	}
	return missing(p) && missing(filepath.Join(e.Store.Dir(), store.StateFile))
}

func missing(p string) bool {
	_, err := os.Lstat(p)
	return errors.Is(err, fs.ErrNotExist)
}

// runWizard is the real setup: Huh on the terminal, the Keychain
// through /usr/bin/security on macOS (the token file elsewhere), the
// claude CLI on PATH, Test Connection against the typed site.
func runWizard(e Env) int {
	ui := setup.NewTerminal(e.Stdin, e.Stdout, os.Environ())
	ui.Begin()
	defer ui.End()
	var kc setup.Keychain
	if runtime.GOOS == "darwin" {
		kc = config.System
	}
	return setup.Run(setup.Deps{
		UI: ui, Getenv: e.Getenv, Keychain: kc, Claude: setup.SystemClaude{},
		Store: e.Store, Version: e.Version, Now: e.Now,
	})
}
