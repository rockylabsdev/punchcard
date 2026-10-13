package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/setup"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/spf13/cobra"
)

// Machine is what doctor reads about the computer it runs on. Nil
// fields mean the real ones; tests inject all three.
type Machine struct {
	// Executable is this binary's path (os.Executable).
	Executable func() (string, error)
	// OSVersion is the OS release: sw_vers on macOS, uname -r on Linux.
	OSVersion func() string
	// TermSize is stdout's size in cells; ok is false off a terminal.
	TermSize func() (w, h int, ok bool)
}

// DoctorData is `data` for doctor: what the testers' guide asks for in
// a bug report, redacted unless --no-redact. Paths under HOME start
// with ~; the site and the email are <redacted>.
type DoctorData struct {
	Punchcard VersionData    `json:"punchcard"`
	Binary    string         `json:"binary"`
	Data      DoctorState    `json:"data"`
	Config    DoctorConfig   `json:"config"`
	Token     string         `json:"token"` // env|keychain|file|none
	Timer     *Timer         `json:"timer"` // the status line's timer; null when idle
	Unsent    Owed           `json:"unsent"`
	Claude    DoctorClaude   `json:"claude"`
	Plugin    DoctorPlugin   `json:"plugin"`
	AskRule   DoctorAskRule  `json:"askRule"`
	Terminal  DoctorTerminal `json:"terminal"`
	OS        DoctorOS       `json:"os"`
}

// DoctorState is the data directory and state.json, in status's words.
type DoctorState struct {
	Dir string `json:"dir"`
	// State is state.json: ok|none|unreadable|newer|error.
	State         string         `json:"state"`
	Problem       string         `json:"problem,omitempty"` // what status would say, when not ok
	SchemaVersion int            `json:"schemaVersion,omitempty"`
	Tickets       int            `json:"tickets"`
	Heartbeat     core.Timestamp `json:"heartbeat"` // null when never seen
	Locked        bool           `json:"locked"`
}

// DoctorConfig is config.json, never the token.
type DoctorConfig struct {
	Path    string         `json:"path"`
	Exists  bool           `json:"exists"`
	Tracker config.Tracker `json:"tracker"`
	Site    string         `json:"site,omitempty"`
	Email   string         `json:"email,omitempty"`
	Problem string         `json:"problem,omitempty"`
}

// DoctorClaude is `claude` on PATH.
type DoctorClaude struct {
	Found   bool   `json:"found"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
}

// DoctorPlugin is the punchcard plugin as Claude Code reports it.
type DoctorPlugin struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
	// Matches: the plugin is this binary's release. False with a dev
	// build, which is never compared.
	Matches bool   `json:"matches"`
	Skewed  bool   `json:"skewed"` // a different release: run punchcard setup
	Problem string `json:"problem,omitempty"`
}

// DoctorAskRule is the permissions.ask rule (ADR 0005).
type DoctorAskRule struct {
	Path  string `json:"path,omitempty"`
	State string `json:"state"` // present|missing|unparseable|unreadable|unknown
}

// DoctorTerminal is what decides the TUI's colours and layout.
type DoctorTerminal struct {
	Term        string `json:"TERM"`
	TermProgram string `json:"TERM_PROGRAM"`
	ColorTerm   string `json:"COLORTERM"`
	NoColor     bool   `json:"NO_COLOR"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

// DoctorOS is the operating system.
type DoctorOS struct {
	GOOS    string `json:"goos"`
	Version string `json:"version,omitempty"`
	Arch    string `json:"arch"`
}

// doctorBudget bounds each `claude` command doctor runs.
const doctorBudget = 10 * time.Second

// doctorCmd is `punchcard doctor`: read-only, exit 0 whatever it
// finds, never the tracker.
func (a *app) doctorCmd() *cobra.Command {
	var noRedact bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Print what a bug report needs, redacted (site, email, home directory)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d := a.doctor(!noRedact)
			a.code = a.out.ok("doctor", d, nil, a.out.doctorText(d))
			return nil
		},
	}
	c.Flags().BoolVar(&noRedact, "no-redact", false, "show the site, the email and the home directory")
	return c
}

func (a *app) doctor(redact bool) DoctorData {
	e := a.env
	m := e.Machine.withDefaults()
	r := redactor{on: redact, home: e.Getenv("HOME")}
	d := DoctorData{
		Punchcard: VersionData{Version: e.Version, Contract: ContractVersion, SchemaVersion: core.SchemaVersion,
			Go: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH},
		OS: DoctorOS{GOOS: runtime.GOOS, Version: m.OSVersion(), Arch: runtime.GOARCH},
	}
	if p, err := m.Executable(); err == nil {
		d.Binary = r.path(p)
	}
	d.Data = a.doctorState(r, &d)
	d.Config = doctorConfig(e.Getenv, r)
	d.Token = "none"
	if e.HasTracker() {
		if rep := a.link(StatusData{}); rep.TokenSource != "" {
			d.Token = string(rep.TokenSource)
		}
	}
	d.Claude, d.Plugin = doctorClaude(e.Claude, e.Version, r)
	d.AskRule = doctorAskRule(e.Getenv, r)
	d.Terminal = DoctorTerminal{Term: e.Getenv("TERM"), TermProgram: e.Getenv("TERM_PROGRAM"),
		ColorTerm: e.Getenv("COLORTERM"), NoColor: e.Getenv("NO_COLOR") != ""}
	if w, h, ok := m.TermSize(); ok {
		d.Terminal.Width, d.Terminal.Height = w, h
	}
	return d
}

// doctorState reads state.json as status would, without moving an
// unreadable file aside, and fills in the timer and unsent count.
func (a *app) doctorState(r redactor, d *DoctorData) DoctorState {
	st := a.env.Store
	if err := st.Err(); err != nil {
		return DoctorState{State: "error", Problem: err.Error()}
	}
	s := DoctorState{Dir: r.path(st.Dir()), Locked: st.Locked()}
	w, notices, err := st.Read()
	switch {
	case errors.Is(err, store.ErrNewerSchema):
		s.State, s.Problem = "newer", store.ErrNewerSchema.Error()
		return s
	case err != nil:
		s.State, s.Problem = "error", r.text(err.Error())
		return s
	case len(notices) > 0:
		s.State, s.Problem = "unreadable", r.text(strings.Join(notices, "; "))
	default:
		s.State = "ok"
		if _, err := os.Stat(filepath.Join(st.Dir(), store.StateFile)); errors.Is(err, fs.ErrNotExist) {
			s.State = "none"
		}
	}
	if s.State == "ok" {
		s.SchemaVersion = w.State.SchemaVersion
	}
	for _, t := range w.State.TrackedTickets {
		if t.Listed() {
			s.Tickets++
		}
	}
	now := a.env.Now()
	if hb := w.Heartbeat.LastSeen; !hb.IsZero() {
		s.Heartbeat = core.At(hb.In(now.Location()))
	}
	snap := snapshot(w, now)
	d.Timer = snap.Running
	if d.Timer == nil && len(snap.Paused) > 0 {
		d.Timer = &snap.Paused[0]
	}
	d.Unsent = snap.Unsent
	return s
}

func doctorConfig(getenv func(string) string, r redactor) DoctorConfig {
	var c DoctorConfig
	p, err := config.Path(getenv)
	if err != nil {
		c.Problem = err.Error()
		c.Tracker = config.TrackerNone
		return c
	}
	c.Path = r.path(p)
	if _, err := os.Stat(p); err == nil {
		c.Exists = true
	}
	c.Tracker, err = config.Chosen(getenv)
	if err != nil {
		c.Problem = r.text(err.Error())
		return c
	}
	if c.Tracker == config.TrackerJira {
		cfg, err := config.Load(getenv)
		if err != nil {
			c.Problem = r.text(err.Error())
		}
		c.Site, c.Email = r.secret(cfg.SiteURL), r.secret(cfg.Email)
	}
	return c
}

// doctorClaude asks claude its version and its plugin list, through
// setup's own check.
func doctorClaude(c setup.Claude, binary string, r redactor) (DoctorClaude, DoctorPlugin) {
	var dc DoctorClaude
	var dp DoctorPlugin
	if c == nil || c.Path() == "" {
		return dc, dp
	}
	dc.Found, dc.Path = true, r.path(c.Path())
	ctx, cancel := context.WithTimeout(context.Background(), doctorBudget)
	out, err := c.Output(ctx, "--version")
	cancel()
	if f := strings.Fields(string(out)); err == nil && len(f) > 0 {
		dc.Version = f[0]
	}
	p := setup.InspectPlugin(c)
	dp.Installed, dp.Version = p.Installed, p.Version
	if p.Err != nil {
		dp.Problem = r.text(p.Err.Error())
	}
	dp.Skewed = p.Skewed(binary)
	dp.Matches = p.Comparable(binary) && !dp.Skewed
	return dc, dp
}

func doctorAskRule(getenv func(string) string, r redactor) DoctorAskRule {
	p := setup.SettingsPath(getenv)
	if p == "" {
		return DoctorAskRule{State: "unknown"}
	}
	s, err := setup.CheckAskRule(p)
	if err != nil {
		s = "unreadable"
	}
	return DoctorAskRule{Path: r.path(p), State: s}
}

func (o output) doctorText(d DoctorData) string {
	var b strings.Builder
	row := func(label, format string, args ...any) {
		fmt.Fprintf(&b, "%-12s"+format+"\n", append([]any{label}, args...)...)
	}
	v := d.Punchcard
	fmt.Fprintf(&b, "punchcard %s (contract v%d, %s %s)\n", v.Version, v.Contract, v.Go, v.Platform)
	row("binary", "%s", or(d.Binary, "unknown"))
	row("data", "%s", d.Data.text())
	row("config", "%s", d.Config.text())
	row("token", "%s", tokenText(d.Token, d.Config.Tracker))
	timer := "none"
	if d.Timer != nil {
		timer = strings.TrimSpace(o.shortLine(StatusData{Running: d.Timer}))
		if d.Timer.ByHand {
			timer += " by hand"
		}
	}
	row("timer", "%s", timer)
	unsent := "0"
	if d.Unsent.Count > 0 {
		unsent = fmt.Sprintf("%d (%s)", d.Unsent.Count, clock(secs(d.Unsent.Seconds)))
	}
	row("unsent", "%s", unsent)
	if d.Claude.Found {
		row("claude", "%s at %s", or(d.Claude.Version, "unknown version"), d.Claude.Path)
	} else {
		row("claude", "not found on PATH")
	}
	row("plugin", "%s", d.pluginText())
	row("ask rule", "%s", d.AskRule.text())
	t := d.Terminal
	term := fmt.Sprintf("TERM=%s  TERM_PROGRAM=%s  COLORTERM=%s  NO_COLOR %s",
		or(t.Term, "unset"), or(t.TermProgram, "unset"), or(t.ColorTerm, "unset"), map[bool]string{true: "set", false: "unset"}[t.NoColor])
	if t.Width > 0 {
		term += fmt.Sprintf("  %dx%d", t.Width, t.Height)
	}
	row("terminal", "%s", term)
	row("os", "%s", strings.Join(strings.Fields(d.OS.GOOS+" "+d.OS.Version+" "+d.OS.Arch), " "))
	return b.String()
}

func (s DoctorState) text() string {
	if s.Dir == "" {
		return "no data directory: " + s.Problem
	}
	var state string
	switch s.State {
	case "ok":
		n := "tickets"
		if s.Tickets == 1 {
			n = "ticket"
		}
		state = fmt.Sprintf("state.json ok (schema %d, %d %s)", s.SchemaVersion, s.Tickets, n)
	case "none":
		state = "state.json none yet"
	case "newer":
		state = "state.json " + s.Problem + " (upgrade punchcard)"
	default:
		state = "state.json " + s.State + ": " + s.Problem
	}
	if s.Locked {
		state += "  " + store.ErrBusy.Error()
	}
	hb := "never"
	if !s.Heartbeat.IsZero() {
		hb = s.Heartbeat.Format("15:04")
	}
	return s.Dir + "  " + state + "  heartbeat " + hb
}

func (c DoctorConfig) text() string {
	if c.Path == "" {
		return "no config directory: " + c.Problem
	}
	parts := []string{c.Path}
	if !c.Exists {
		parts = append(parts, "none yet")
	}
	if c.Problem != "" && c.Exists {
		parts = append(parts, "problem: "+c.Problem)
	}
	parts = append(parts, "tracker "+or(string(c.Tracker), "unknown"))
	if c.Tracker == config.TrackerJira {
		parts = append(parts, "site "+or(c.Site, "unset"), "email "+or(c.Email, "unset"))
	}
	return strings.Join(parts, "  ")
}

func tokenText(source string, t config.Tracker) string {
	switch {
	case t != config.TrackerJira:
		return "none (no tracker)"
	case source == "none":
		return "none"
	}
	return sourceText(config.Source(source))
}

func (d DoctorData) pluginText() string {
	p := d.Plugin
	switch {
	case !d.Claude.Found:
		return "claude not found"
	case p.Problem != "":
		return "could not ask Claude Code: " + p.Problem
	case !p.Installed:
		return "not installed → run punchcard setup"
	}
	s := setup.PluginID + " " + p.Version
	switch {
	case p.Matches:
		return s + "  ✓ matches binary"
	case p.Skewed:
		return s + "  ≠ " + d.Punchcard.Version + " → run punchcard setup"
	}
	return s + "  (this build, " + d.Punchcard.Version + ", is not a release: not compared)"
}

func (a DoctorAskRule) text() string {
	switch a.State {
	case "present":
		return "present in " + a.Path
	case "missing":
		return "absent → punchcard setup adds it"
	case "unparseable":
		return a.Path + " unparseable"
	case "unreadable":
		return a.Path + " unreadable"
	}
	return "unknown: neither CLAUDE_CONFIG_DIR nor HOME is set"
}

func or(s, otherwise string) string {
	if s == "" {
		return otherwise
	}
	return s
}

// redactor hides what a pasted report should not carry: the site, the
// email, and the home directory in paths.
type redactor struct {
	on   bool
	home string
}

func (r redactor) secret(s string) string {
	if r.on && s != "" {
		return "<redacted>"
	}
	return s
}

func (r redactor) path(p string) string {
	if !r.on || len(r.home) < 2 {
		return p
	}
	home := strings.TrimSuffix(r.home, "/")
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+"/"); ok {
		return "~/" + rest
	}
	return p
}

// text redacts the home directory wherever it appears in s (an error
// that names a path).
func (r redactor) text(s string) string {
	if !r.on || len(r.home) < 2 {
		return s
	}
	return strings.ReplaceAll(s, strings.TrimSuffix(r.home, "/")+"/", "~/")
}

func (m Machine) withDefaults() Machine {
	if m.Executable == nil {
		m.Executable = os.Executable
	}
	if m.OSVersion == nil {
		m.OSVersion = osVersion
	}
	if m.TermSize == nil {
		m.TermSize = func() (int, int, bool) {
			w, h, err := term.GetSize(os.Stdout.Fd())
			return w, h, err == nil
		}
	}
	return m
}

// osVersion is the OS release, or "" when it cannot be asked.
func osVersion() string {
	var cmd []string
	switch runtime.GOOS {
	case "darwin":
		cmd = []string{"sw_vers", "-productVersion"}
	case "linux":
		cmd = []string{"uname", "-r"}
	default:
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cmd[0], cmd[1:]...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
