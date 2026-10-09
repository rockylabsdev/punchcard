package cli

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/spf13/cobra"
)

// The reads: status, ls and version. They never call Jira and never
// write; status and ls exit 0 unless the state cannot be read. Status
// (but not --short) looks up the token to report its source.

func (a *app) statusCmd() *cobra.Command {
	var shortForm bool
	var cwd string
	c := &cobra.Command{
		Use:   "status",
		Short: "Show the running ticket, paused tickets and unsent worklogs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.status(shortForm, cwd)
		},
	}
	c.Flags().BoolVar(&shortForm, "short", false, "one line for a shell prompt; nothing when idle; always exits 0")
	c.Flags().StringVar(&cwd, "cwd", "", "also report this directory's key (its .punchcard, else its git branch) as here")
	return c
}

func (a *app) status(shortForm bool, cwd string) error {
	w, now, err := a.read()
	if err != nil {
		if shortForm && !a.out.json {
			return nil // a shell prompt shows nothing and never fails
		}
		a.code = a.failure("status", err)
		if shortForm {
			a.code = ExitOK
		}
		return nil
	}
	d := snapshot(w, now)
	if shortForm {
		// A shell prompt runs this every time: no token lookup.
		a.code = a.out.ok("status", d, nil, a.out.shortLine(d))
		return nil
	}
	rep := a.link(d)
	if cwd != "" {
		here, notices := resolveHere(cwd, a.env.Grammar())
		a.out.notices(notices)
		rep.Here = here
	}
	a.code = a.out.ok("status", rep, nil, a.out.statusText(rep, now))
	return nil
}

// link is d plus the tracker and, with Jira, its link: the config file
// and the token lookup, never a network call. With no tracker there is
// no token to look up.
func (a *app) link(d StatusData) StatusReport {
	rep := StatusReport{StatusData: d, Tracker: a.env.Tracker}
	if !a.env.HasTracker() {
		return rep
	}
	_, sec, err := config.Credentials(a.env.Getenv, a.env.Keychain)
	var kc *config.KeychainError
	configured := err == nil
	rep.Configured = &configured
	switch {
	case err == nil:
		rep.TokenSource = sec.Source
		if sec.StrayFile {
			rep.StrayTokenFile, _ = config.TokenPath(a.env.Getenv) // a stray file implies a config dir
		}
	case errors.As(err, &kc):
		rep.KeychainError = kc.Error()
		rep.NotConfigured = kc.Error()
	default:
		rep.NotConfigured = err.Error()
	}
	return rep
}

func (o output) statusText(rep StatusReport, now time.Time) string {
	d := rep.StatusData
	var b strings.Builder
	if d.Running != nil {
		b.WriteString(o.timerLine(*d.Running, now.Location(), now))
	} else {
		b.WriteString("Nothing running.\n")
	}
	for _, tm := range d.Paused {
		b.WriteString(o.timerLine(tm, now.Location(), now))
	}
	if d.Unsent.Count > 0 {
		fmt.Fprintf(&b, "%d unsent worklog(s), %s\n", d.Unsent.Count, clock(secs(d.Unsent.Seconds)))
	}
	if !d.AuthFailedAt.IsZero() {
		fmt.Fprintf(&b, "%s Jira rejected the token at %s; run `punchcard setup`\n",
			o.paint(red, "!"), d.AuthFailedAt.In(now.Location()).Format("2006-01-02 15:04"))
	}
	switch {
	case rep.Configured == nil: // no tracker: nothing to connect
	case rep.KeychainError != "":
		fmt.Fprintf(&b, "%s Not connected to Jira: %s\n", o.paint(red, "!"), rep.KeychainError)
	case !*rep.Configured:
		fmt.Fprintf(&b, "Not connected to Jira: %s\n", strings.TrimPrefix(rep.NotConfigured, config.ErrNotConfigured.Error()+": "))
	default:
		fmt.Fprintf(&b, "Jira token: %s\n", sourceText(rep.TokenSource))
	}
	if rep.StrayTokenFile != "" {
		fmt.Fprintf(&b, "%s a stray token file at %s is ignored (the Keychain wins); delete it\n", o.paint(yellow, "!"), rep.StrayTokenFile)
	}
	return b.String()
}

func sourceText(s config.Source) string {
	switch s {
	case config.SourceEnv:
		return "PUNCHCARD_TOKEN"
	case config.SourceKeychain:
		return "Keychain"
	}
	return string(s)
}

// shortLine is `status --short`: ▶ ABC-123 1:12 · 2 unsent, ⏸ …, ! … stale.
// Idle with nothing owed prints nothing; idle with worklogs owed prints
// only the unsent count, which status --short always shows.
func (o output) shortLine(d StatusData) string {
	var parts []string
	var tm *Timer
	if d.Running != nil {
		tm = d.Running
	} else if len(d.Paused) > 0 {
		tm = &d.Paused[0]
	}
	if tm != nil {
		switch tm.State {
		case core.TimerRunning:
			parts = append(parts, fmt.Sprintf("%s %s %s", o.paint(green, "▶"), tm.Key, short(secs(tm.ElapsedSeconds))))
		case core.TimerPaused:
			parts = append(parts, fmt.Sprintf("%s %s %s", o.paint(yellow, "⏸"), tm.Key, short(secs(tm.ElapsedSeconds))))
		case core.TimerStale:
			parts = append(parts, fmt.Sprintf("%s %s stale", o.paint(red, "!"), tm.Key))
		}
	}
	if d.Unsent.Count > 0 {
		parts = append(parts, fmt.Sprintf("%d unsent", d.Unsent.Count))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ") + "\n"
}

func (a *app) lsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List tracked tickets, most recently active first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w, now, err := a.read()
			if err != nil {
				a.code = a.failure("ls", err)
				return nil
			}
			d := listData(w, now, nil)
			a.code = a.out.ok("ls", d, nil, a.out.listText(d))
			return nil
		},
	}
}

func (o output) listText(d ListData) string {
	if len(d.Tickets) == 0 {
		return "No tickets yet. Add one with `punchcard add KEY` or `punchcard start KEY`.\n"
	}
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, t := range d.Tickets {
		marker, state, elapsed := " ", "-", ""
		switch t.State {
		case core.TimerRunning:
			marker = o.paint(green, "▶")
		case core.TimerPaused:
			marker = o.paint(yellow, "⏸")
		case core.TimerStale:
			marker = o.paint(red, "!")
		}
		if t.State != core.TimerIdle {
			state, elapsed = string(t.State), clock(secs(t.ElapsedSeconds))
		}
		unsent := ""
		if t.Unsynced > 0 {
			unsent = fmt.Sprintf("%d unsent", t.Unsynced)
		}
		summary := t.Summary
		if t.Unverified {
			summary = "(unverified)"
		}
		fmt.Fprintf(tw, "%s %s\t%s\t%s\t%s\t%s\n", marker, t.Key, state, elapsed, unsent, summary)
	}
	tw.Flush()
	return b.String()
}

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version, contract version and platform",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d := VersionData{
				Version:       a.env.Version,
				Contract:      ContractVersion,
				SchemaVersion: core.SchemaVersion,
				Go:            runtime.Version(),
				Platform:      runtime.GOOS + "/" + runtime.GOARCH,
			}
			a.code = a.out.ok("version", d, nil,
				fmt.Sprintf("punchcard %s (contract v%d, %s %s)\n", d.Version, d.Contract, d.Go, d.Platform))
			return nil
		},
	}
}
