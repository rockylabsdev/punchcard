// Package setup is `punchcard setup`: the one place the tracker is
// chosen and credentials are edited. The first step is the tracker,
// none or Jira. Jira is a linear wizard (site → email → token → Test
// Connection → save) that refuses to save anything it has not just
// tested; none saves the choice, after offering to mark what is still
// owed to Jira local. Then come the optional plugin and ask-rule steps. Every outside effect goes through
// a seam in Deps, so the whole flow runs in tests with no terminal, no
// Keychain, no Jira and no `claude`.
package setup

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// ErrAborted: the user pressed esc or ctrl+c in a prompt.
var ErrAborted = errors.New("cancelled")

// UI is the wizard's face: Huh on a terminal (NewTerminal), a script in
// tests.
type UI interface {
	// Ask shows one text field and returns the answer, or ErrAborted.
	Ask(f Field) (string, error)
	// Confirm asks a yes/no question, or returns ErrAborted.
	Confirm(q Question) (bool, error)
	// Choose asks for one of a few options and returns its Value, or
	// ErrAborted.
	Choose(c Choice) (string, error)
	// Say prints lines in the wizard's gutter.
	Say(lines ...string)
	// Output is where a command's own output goes, shown in the gutter.
	Output() io.Writer
}

// Field is one text prompt.
type Field struct {
	Title       string
	Note        []string // lines shown with the field (links, warnings)
	Placeholder string
	Value       string // prefilled
	Secret      bool   // masked, and never echoed back
	// Echo, if set, is a line shown live under the input as it changes
	// (the normalised site URL).
	Echo     func(string) string
	Validate func(string) error
}

// Question is a yes/no prompt.
type Question struct {
	Title   string
	Note    []string
	Default bool
}

// Choice is a pick-one prompt; Value is the option selected to start.
type Choice struct {
	Title   string
	Note    []string
	Options []Option
	Value   string
}

// Option is one of a Choice's answers: Label is shown, Value returned.
type Option struct{ Label, Value string }

// Keychain is the macOS Keychain item, read and written only through
// /usr/bin/security (config.Security satisfies it). Tests use a fake.
type Keychain interface {
	Read(email string) (string, error)
	Write(email, token string) error
	DeleteAll() error
}

// Connect is Test Connection with typed credentials: serverInfo, then
// myself. Nil in Deps means the real Jira client.
type Connect func(ctx context.Context, site, email, token string) (tracker.Account, error)

// Store is the state file, for clearing authFailedAt on a fresh setup
// and for marking owed worklogs local when Jira is removed.
type Store interface {
	Read() (core.World, []string, error)
	Apply(core.Command) (store.Result, error)
}

// Deps is everything the wizard touches.
type Deps struct {
	UI     UI
	Getenv func(string) string
	// Keychain: nil means no Keychain (Linux); the token goes to the
	// 0600 file and that is no fallback.
	Keychain Keychain
	Connect  Connect // nil: jira.New(...).TestConnection
	Claude   Claude  // nil: `claude` is never found
	Store    Store   // nil: authFailedAt is left alone and nothing is offered as local
	// Version is this binary's; a dev build never offers a plugin update.
	Version string
	Now     func() time.Time // for showing when the token was rejected
}

// ConnectBudget bounds Test Connection (two calls).
const ConnectBudget = 15 * time.Second
