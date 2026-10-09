// Package cli is the punchcard command line: argument handling, the
// --json envelope and exit codes (ADR 0004). Slice 1 has only `version`;
// slice 3 moves the command tree onto cobra.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"

	"github.com/j-dsmith/punchcard/internal/core"
)

// ContractVersion is `v` in every --json envelope. Additive changes keep it.
const ContractVersion = 1

// Exit codes from ADR 0004. Exit 2 is never used.
const (
	ExitOK    = 0
	ExitUsage = 64
)

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

// Env is everything a command touches outside its arguments.
type Env struct {
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	Version        string
}

// Envelope is the one --json shape on stdout, success or failure.
type Envelope struct {
	V       int          `json:"v"`
	OK      bool         `json:"ok"`
	Outcome string       `json:"outcome"`
	Command string       `json:"command"`
	Data    any          `json:"data"`
	Events  []core.Event `json:"events"`
	Error   *Error       `json:"error"`
}

// Error is the envelope's error member.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// VersionData is `data` for `version --json`.
type VersionData struct {
	Version       string `json:"version"`
	Contract      int    `json:"contract"`
	SchemaVersion int    `json:"schemaVersion"`
	Go            string `json:"go"`
	Platform      string `json:"platform"`
}

// Run executes one invocation and returns its exit code.
func Run(args []string, env Env) int {
	jsonOut := env.Getenv("PUNCHCARD_JSON") == "1"
	var rest []string
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
			continue
		}
		rest = append(rest, a)
	}
	o := output{env: env, json: jsonOut}
	if len(rest) == 0 {
		return o.usage("", "no command given (only `version` exists yet)")
	}
	switch cmd := rest[0]; cmd {
	case "version":
		if len(rest) > 1 {
			return o.usage(cmd, "version takes no arguments")
		}
		d := VersionData{
			Version:       env.Version,
			Contract:      ContractVersion,
			SchemaVersion: core.SchemaVersion,
			Go:            runtime.Version(),
			Platform:      runtime.GOOS + "/" + runtime.GOARCH,
		}
		return o.ok(cmd, d, fmt.Sprintf("punchcard %s (contract v%d, %s %s)\n", d.Version, d.Contract, d.Go, d.Platform))
	default:
		return o.usage(cmd, fmt.Sprintf("unknown command %q", cmd))
	}
}

type output struct {
	env  Env
	json bool
}

func (o output) ok(cmd string, data any, human string) int {
	if o.json {
		o.envelope(Envelope{OK: true, Outcome: "ok", Command: cmd, Data: data})
	} else {
		fmt.Fprint(o.env.Stdout, human)
	}
	return ExitOK
}

func (o output) usage(cmd, msg string) int {
	if o.json {
		o.envelope(Envelope{Outcome: "usage", Command: cmd, Error: &Error{Code: "usage", Message: msg}})
	} else {
		fmt.Fprintf(o.env.Stderr, "punchcard: %s\nusage: punchcard version [--json]\n", msg)
	}
	return ExitUsage
}

func (o output) envelope(e Envelope) {
	e.V = ContractVersion
	if e.Events == nil {
		e.Events = []core.Event{}
	}
	b, err := json.Marshal(e)
	if err != nil { // only plain data is ever passed; this cannot happen
		panic(err)
	}
	fmt.Fprintf(o.env.Stdout, "%s\n", b)
}
