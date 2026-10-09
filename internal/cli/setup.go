package cli

import (
	"os"
	"runtime"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/setup"
	"github.com/spf13/cobra"
)

// setupCmd is `punchcard setup`: interactive only, so a token never
// lands in shell history or an agent's reach.
func (a *app) setupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Set up a tracker and the Claude Code plugin (needs a terminal)",
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
