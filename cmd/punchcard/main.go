// Command punchcard times work against Jira tickets: one binary, a CLI
// and (later) a TUI over one core.
package main

import (
	"os"

	"github.com/j-dsmith/punchcard/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.Env{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Getenv:  os.Getenv,
		Version: cli.BuildVersion(),
	}))
}
