// Package cli is the punchcard command line: the cobra command tree, the
// --json envelope and exit codes (ADR 0004). Every change goes through
// store.Apply with a core.Command; the CLI never saves a World itself.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"
	"time"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/exitcode"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/j-dsmith/punchcard/internal/tracker"
	"github.com/spf13/cobra"
)

// ContractVersion is `v` in every --json envelope. Additive changes keep it.
const ContractVersion = 1

// Exit codes from ADR 0004, from the one table in package exitcode
// (setup uses it too). Exit 2 is never used: it blocks a Claude Code hook.
const (
	ExitOK                = exitcode.OK
	ExitInternal          = exitcode.Internal
	ExitNotConfigured     = exitcode.NotConfigured
	ExitNoTimer           = exitcode.NoTimer
	ExitTicketNotFound    = exitcode.TicketNotFound
	ExitNeedsConfirmation = exitcode.NeedsConfirmation
	ExitSyncPending       = exitcode.SyncPending
	ExitAuthFailed        = exitcode.AuthFailed
	ExitUsage             = exitcode.Usage
	ExitStateUnreadable   = exitcode.StateUnreadable
	ExitBusy              = exitcode.Busy
)

// outcomes names each exit code; it is `outcome` in the envelope.
var outcomes = map[int]string{
	ExitOK:                "ok",
	ExitInternal:          "internal",
	ExitNotConfigured:     "not_configured",
	ExitNoTimer:           "no_timer",
	ExitTicketNotFound:    "ticket_not_found",
	ExitNeedsConfirmation: "needs_confirmation",
	ExitSyncPending:       "sync_pending",
	ExitAuthFailed:        "auth_failed",
	ExitUsage:             "usage",
	ExitStateUnreadable:   "state_unreadable",
	ExitBusy:              "busy",
}

// version is set at release time with
// -ldflags "-X github.com/j-dsmith/punchcard/internal/cli.version=v1.2.3".
var version string

// BuildVersion is this binary's version: the ldflags value, else the
// module version from `go install`, else "dev".
func BuildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// Env is everything a command touches outside its arguments. Only the
// writers and Getenv are required; every other field has a default.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	// Now is the clock; nil means time.Now.
	Now     func() time.Time
	Version string
	// TTY: stdin and stdout are both a terminal. Prompts, the picker,
	// colour and the TUI need it. JSON never depends on it.
	TTY bool
	// Store replaces store.Open(Getenv, Now); tests inject one.
	Store *store.Store
	// NewID makes entry ids; nil means core.NewEntryID.
	NewID func() string
	// Pick asks the user to choose one of choices (recents first) on a
	// TTY. It returns "" when the user cancels. Nil means a numbered
	// line prompt on Stdin/Stderr; the TUI slice can replace it.
	Pick func(choices []Choice) (string, error)
	// TUI runs the full-screen app for bare `punchcard` on a TTY; main
	// wires it to tui.Run. Nil (tests) prints status instead.
	TUI func(Env) int
	// Tracker is the tracker in effect. Empty means config's answer
	// (config.Chosen): no config file is none. Tests set it.
	Tracker config.Tracker
	// OpenTracker opens the tracker's client; only commands that call
	// the tracker call it. After defaults it answers
	// tracker.ErrNoTracker with no tracker; with Jira, nil means
	// jira.Open(Getenv): the config file plus the token lookup. Tests
	// inject a *tracker.Fake.
	OpenTracker func() (tracker.Client, error)
	// Keychain is the token lookup's Keychain read (config.Keychain).
	// Nil means config.SystemKeychain(): /usr/bin/security on macOS, no
	// Keychain on Linux. Tests always inject a fake.
	Keychain config.Keychain
	// Setup runs the setup wizard on the terminal and returns its exit
	// code. Nil means the real one (Huh on Stdin/Stdout, the macOS
	// Keychain through security, the claude CLI, ~/.claude/settings.json);
	// tests always inject one.
	Setup func(Env) int
}

// Choice is one ticket offered by the picker.
type Choice struct {
	Key     string
	Summary string
}

func (e Env) withDefaults() Env {
	if e.Now == nil {
		e.Now = time.Now
	}
	if e.Stdin == nil {
		e.Stdin = os.Stdin
	}
	if e.NewID == nil {
		e.NewID = core.NewEntryID
	}
	if e.Store == nil {
		e.Store = store.Open(e.Getenv, e.Now)
	}
	if e.Keychain == nil {
		e.Keychain = config.SystemKeychain()
	}
	if e.Setup == nil {
		e.Setup = runWizard
	}
	if e.Tracker == "" {
		e.Tracker = chosenTracker(e.Getenv)
	}
	e.OpenTracker = e.openTracker(e.OpenTracker)
	return e
}

// Run executes one invocation and returns its exit code.
func Run(args []string, env Env) int {
	env = env.withDefaults()
	a := &app{env: env}
	a.out = output{env: env, json: wantJSON(args, env.Getenv), color: env.TTY && env.Getenv("NO_COLOR") == "",
		noticed: new([]string)}
	root := a.root()
	root.SetArgs(append([]string{}, args...)) // never nil: cobra would read os.Args
	root.SetIn(env.Stdin)
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)
	cmd, err := root.ExecuteC()
	if err != nil {
		name := ""
		if cmd != nil && cmd != root {
			name = cmd.Name()
		}
		return a.out.usage(name, err.Error())
	}
	return a.code
}

// wantJSON: --json anywhere before "--", or PUNCHCARD_JSON=1. It is read
// before parsing so that a usage error is an envelope too. A non-TTY
// stdout never turns it on.
func wantJSON(args []string, getenv func(string) string) bool {
	if getenv("PUNCHCARD_JSON") == "1" {
		return true
	}
	if i := slices.Index(args, "--"); i >= 0 {
		args = args[:i]
	}
	return slices.Contains(args, "--json")
}

// app is one invocation: its environment, output mode and result.
type app struct {
	env    Env
	out    output
	origin core.Origin
	code   int
}

// usageError is returned from a command's RunE; Run reports it as exit 64.
type usageError string

func (e usageError) Error() string { return string(e) }

// originFrom: agent inside Claude Code or another agent, else hand; the
// hidden --origin flag overrides it (ADR 0004).
func originFrom(flag string, getenv func(string) string) (core.Origin, error) {
	switch o := core.Origin(flag); o {
	case core.OriginHand, core.OriginPrompt, core.OriginAgent:
		return o, nil
	case "":
	default:
		return "", usageError(`--origin must be hand, prompt or agent`)
	}
	if getenv("CLAUDECODE") != "" || getenv("AI_AGENT") != "" {
		return core.OriginAgent, nil
	}
	return core.OriginHand, nil
}

// root builds the command tree. Cobra parses; Punchcard's own code sets
// every exit code (a.code), and any error cobra returns is usage (64).
func (a *app) root() *cobra.Command {
	var originFlag string
	var jsonFlag bool
	root := &cobra.Command{
		Use:           "punchcard",
		Short:         "Time work against tickets",
		Long:          "Punchcard times work against tickets and logs the time as worklogs, sent to a tracker when one is set up.\nBare `punchcard` opens the TUI on a terminal and prints status otherwise.",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			o, err := originFrom(originFlag, a.env.Getenv)
			a.origin = o
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Without a data directory the TUI has nothing to show: status
			// reports why (exit 1).
			if a.env.TTY && a.env.TUI != nil && !a.out.json && a.env.Store.Err() == nil {
				a.code = a.env.TUI(a.env)
				return nil
			}
			return a.status(false, "")
		},
	}
	root.PersistentFlags().BoolVar(&jsonFlag, "json", false, "print one JSON envelope on stdout (also PUNCHCARD_JSON=1)")
	root.PersistentFlags().StringVar(&originFlag, "origin", "", "who caused this change: hand, prompt or agent")
	_ = root.PersistentFlags().MarkHidden("origin")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err.Error()) })
	root.AddCommand(
		a.startCmd(), a.pauseCmd(), a.resumeCmd(), a.stopCmd(), a.statusCmd(), a.lsCmd(),
		a.addCmd(), a.rmCmd(), a.syncCmd("sync", false), a.syncCmd("retry", true), a.setupCmd(),
		a.versionCmd(), a.touchCmd(), a.logCmd(),
	)
	return root
}

// failure maps a core or store error to its exit code (ADR 0004) and
// reports it. A timer set by hand is the no_timer family (exit 4) with
// its own error code, "by_hand".
func (a *app) failure(cmd string, err error) int {
	var amb *core.AmbiguousError
	var nf ticketNotFound
	switch {
	case errors.As(err, &nf):
		return a.out.fail(cmd, ExitTicketNotFound, failure{code: "ticket_not_found",
			msg:  fmt.Sprintf("Jira has no ticket %s that this account can see", nf.key),
			hint: "check the key; if it is right, the ticket may exist but be hidden from this account. Nothing changed."})
	case errors.As(err, &amb):
		return a.out.fail(cmd, ExitNoTimer, failure{
			code: "no_timer", msg: err.Error(), hint: "name one: punchcard " + cmd + " KEY",
			data: CandidatesData{Candidates: amb.Keys},
		})
	case errors.Is(err, core.ErrTooShort):
		return a.out.fail(cmd, ExitUsage, failure{code: "usage", msg: "a worklog under one minute is never logged", hint: "pass --duration"})
	case errors.Is(err, core.ErrUsage):
		return a.out.fail(cmd, ExitUsage, failure{code: "usage", msg: err.Error()})
	case errors.Is(err, core.ErrUnlogged):
		// With no tracker only an open entry is unlogged: stopping logs it.
		hint := "stop and sync it first, or pass --force to drop the unsent time"
		if !a.env.HasTracker() {
			hint = "stop it first, or pass --force to drop its time"
		}
		return a.out.fail(cmd, ExitNoTimer, failure{code: "no_timer", msg: err.Error(), hint: hint})
	case errors.Is(err, core.ErrByHand):
		return a.out.fail(cmd, ExitNoTimer, failure{code: "by_hand", msg: err.Error(),
			hint: "the user set it by hand; only they can move or pause it"})
	case errors.Is(err, core.ErrNoTimer), errors.Is(err, core.ErrNotTracked):
		return a.out.fail(cmd, ExitNoTimer, failure{code: "no_timer", msg: err.Error()})
	case errors.Is(err, store.ErrNewerSchema):
		return a.out.fail(cmd, ExitStateUnreadable, failure{code: "state_unreadable", msg: err.Error(), hint: "upgrade punchcard"})
	case errors.Is(err, store.ErrBusy):
		return a.out.fail(cmd, ExitBusy, failure{code: "busy", msg: err.Error(), hint: "try again"})
	default:
		return a.out.fail(cmd, ExitInternal, failure{code: "internal", msg: err.Error()})
	}
}

// apply sends one command through the store seam and reports notices.
func (a *app) apply(c core.Command) (store.Result, error) {
	res, err := a.env.Store.Apply(c)
	a.out.notices(res.Notices)
	return res, err
}

// read is the lock-free, write-free read used by status, ls and stop's
// preview.
func (a *app) read() (core.World, time.Time, error) {
	now := a.env.Now()
	w, notices, err := a.env.Store.Read()
	a.out.notices(notices)
	return w, now, err
}

// done reports a successful change: the envelope or the human text, then
// (human only) the unsent-worklog warning.
func (a *app) done(cmd string, res store.Result, data any, human string) {
	a.code = a.out.ok(cmd, data, res.Events, human)
	a.out.unsentWarning(res.World.State)
}
