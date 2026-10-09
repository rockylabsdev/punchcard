package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/j-dsmith/punchcard/internal/core"
)

// Envelope is the one --json shape on stdout, success or failure.
type Envelope struct {
	V       int          `json:"v"`
	OK      bool         `json:"ok"`
	Outcome string       `json:"outcome"`
	Command string       `json:"command"`
	Data    any          `json:"data"`
	Events  []core.Event `json:"events"`
	Error   *Error       `json:"error"`
	// Notices are what stderr also said: a corrupt file moved aside, a
	// .punchcard that names no key. Absent when there are none.
	Notices []string `json:"notices,omitempty"`
}

// Error is the envelope's error member.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// failure is a refusal as a command reports it.
type failure struct {
	code, msg, hint string
	data            any
}

// output writes either the envelope (json) or human text. In human mode
// results go to stdout and errors, notices and warnings to stderr.
type output struct {
	env   Env
	json  bool
	color bool
	// noticed collects this invocation's notices for the envelope; every
	// copy of output shares it.
	noticed *[]string
}

// ok reports success (exit 0) with data for the envelope and human text.
func (o output) ok(cmd string, data any, events []core.Event, human string) int {
	if o.json {
		o.envelope(Envelope{OK: true, Outcome: "ok", Command: cmd, Data: data, Events: events})
	} else {
		fmt.Fprint(o.env.Stdout, human)
	}
	return ExitOK
}

// partial reports a success with debt (exit 7): ok is true and the
// change is on disk, but a worklog is still owed to Jira.
func (o output) partial(cmd string, data any, events []core.Event, human string, f failure) int {
	if o.json {
		o.envelope(Envelope{OK: true, Outcome: outcomes[ExitSyncPending], Command: cmd, Data: data, Events: events,
			Error: &Error{Code: f.code, Message: f.msg, Hint: f.hint}})
	} else {
		fmt.Fprint(o.env.Stdout, human)
		o.errLine(f)
	}
	return ExitSyncPending
}

func (o output) usage(cmd, msg string) int {
	return o.fail(cmd, ExitUsage, failure{code: "usage", msg: msg, hint: "see punchcard --help"})
}

// fail reports a refusal or failure: the envelope with --json, else the
// message on stderr.
func (o output) fail(cmd string, exit int, f failure) int {
	if o.json {
		o.envelope(Envelope{Outcome: outcomes[exit], Command: cmd, Data: f.data, Error: &Error{Code: f.code, Message: f.msg, Hint: f.hint}})
	} else {
		o.errLine(f)
	}
	return exit
}

func (o output) errLine(f failure) {
	fmt.Fprintf(o.env.Stderr, "%s %s\n", o.paint(red, "punchcard:"), f.msg)
	if f.hint != "" && !saysAlready(f.msg, f.hint) {
		fmt.Fprintf(o.env.Stderr, "  %s\n", f.hint)
	}
}

// saysAlready reports whether msg already gives the advice in hint, ignoring
// the backticks the hint wraps commands in. Some errors carry their own
// advice ("… (run punchcard setup)"), and stderr shouldn't say it twice.
// The --json envelope keeps the hint either way.
func saysAlready(msg, hint string) bool {
	plain := strings.ReplaceAll(hint, "`", "")
	return strings.Contains(strings.ReplaceAll(msg, "`", ""), plain)
}

func (o output) envelope(e Envelope) {
	e.V = ContractVersion
	if o.noticed != nil {
		e.Notices = *o.noticed
	}
	if e.Events == nil {
		e.Events = []core.Event{}
	}
	b, err := json.Marshal(e)
	if err != nil { // only plain data is ever passed; this cannot happen
		panic(err)
	}
	fmt.Fprintf(o.env.Stdout, "%s\n", b)
}

// notices are messages beside the result (a corrupt file moved aside, a
// .punchcard that names no key): always on stderr, so they never break
// the envelope on stdout, and in the envelope's notices too.
func (o output) notices(ns []string) {
	if o.noticed != nil {
		*o.noticed = append(*o.noticed, ns...)
	}
	for _, n := range ns {
		fmt.Fprintf(o.env.Stderr, "punchcard: %s\n", n)
	}
}

// unsentWarning: every human-mode change warns on stderr while unsent
// worklogs exist.
func (o output) unsentWarning(s core.State) {
	if o.json {
		return
	}
	if n, total := core.Owed(s); n > 0 {
		fmt.Fprintf(o.env.Stderr, "%s %d unsent worklog(s), %s; run `punchcard sync`\n", o.paint(yellow, "punchcard:"), n, clock(total))
	}
}

const (
	red    = "31"
	green  = "32"
	yellow = "33"
)

// paint colours s only on a TTY with NO_COLOR unset.
func (o output) paint(color, s string) string {
	if !o.color {
		return s
	}
	return "\x1b[" + color + "m" + s + "\x1b[0m"
}
