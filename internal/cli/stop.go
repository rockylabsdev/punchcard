package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/spf13/cobra"
)

// stop, and the stubs that wait for Jira: sync (and hidden retry) and
// setup.

func (a *app) stopCmd() *cobra.Command {
	var durationFlag, message string
	var noSync bool
	c := &cobra.Command{
		Use:   "stop [KEY]",
		Short: "Stop an entry and record its worklog (no KEY: the running entry, else the single paused one)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := core.Stop{Comment: message, Origin: a.origin}
			if len(args) == 1 {
				s.Key = args[0]
			}
			if cmd.Flags().Changed("duration") {
				d, err := parseDuration(durationFlag)
				if err != nil {
					return err
				}
				s.Duration = d
			}
			a.stop(s, noSync)
			return nil
		},
	}
	c.Flags().StringVar(&durationFlag, "duration", "", "time to log instead of the measured elapsed: 1h30m, 1h, 90m or minutes (1m to 24h)")
	c.Flags().StringVarP(&message, "message", "m", "", "worklog comment")
	c.Flags().BoolVar(&noSync, "no-sync", false, "record the worklog as pending without sending it")
	return c
}

// stop previews the worklog from a fresh read, applies the CLI's own
// rules, then sends core.Stop pinned to the previewed ticket. Core
// re-checks everything under the lock.
func (a *app) stop(s core.Stop, noSync bool) {
	w, now, err := a.read()
	if err != nil {
		a.code = a.failure("stop", err)
		return
	}
	p, err := core.PreviewStop(w, s, now)
	if err != nil {
		a.code = a.failure("stop", err)
		return
	}
	// Slice 5 adds, here: the TTY prompt for duration and comment (never
	// with --json), and origin agent without --yes exiting 6 with this
	// preview. Until the prompt exists, a recovered entry always needs
	// --duration.
	if p.Recovered && s.Duration == 0 {
		a.code = a.out.fail("stop", ExitNeedsConfirmation, failure{
			code: "needs_confirmation",
			msg:  fmt.Sprintf("%s was recovered: its time stops at the last presence, so it needs a duration", p.Key),
			hint: "rerun with --duration, e.g. --duration 1h30m",
			data: PreviewData{Preview: Preview{Key: p.Key, EntryID: p.EntryID, Worklog: p.Worklog, Recovered: true}},
		})
		return
	}
	s.Key = p.Key
	res, err := a.apply(s)
	if err != nil {
		a.code = a.failure("stop", err)
		return
	}
	e := findEntry(res.World.State, p.Key, p.EntryID)
	if e.Worklog == nil { // core stopped some other entry; cannot happen with the key pinned
		a.code = a.failure("stop", fmt.Errorf("entry %s on %s was not stopped", p.EntryID, p.Key))
		return
	}
	d := StopData{Key: p.Key, EntryID: e.ID, Worklog: *e.Worklog}
	human := a.eventLines(res) + fmt.Sprintf("■ %s stopped: %s, worklog pending\n", p.Key, worklogDuration(e.Worklog.TimeSpentSeconds))
	if noSync {
		a.done("stop", res, d, human)
		return
	}
	// Slice 5 POSTs here, inline, and returns 0 when Jira takes it.
	a.code = a.out.partial("stop", d, res.Events, human, failure{
		code: "sync_pending",
		msg:  "the worklog is on disk but not sent: sending to Jira is not built yet",
		hint: "run `punchcard sync` later; never stop again",
	})
}

var durationPattern = regexp.MustCompile(`^(?:([0-9]+)h)?(?:([0-9]+)m)?$`)

// parseDuration accepts 1h30m, 1h, 90m or bare minutes, from 1m to 24h.
// h:mm, seconds and fractions are refused.
func parseDuration(s string) (time.Duration, error) {
	var d time.Duration
	if n, err := strconv.Atoi(s); err == nil {
		d = time.Duration(n) * time.Minute
	} else if m := durationPattern.FindStringSubmatch(s); m != nil && s != "" {
		h, _ := strconv.Atoi(m[1])
		mins, _ := strconv.Atoi(m[2])
		d = time.Duration(h)*time.Hour + time.Duration(mins)*time.Minute
	} else {
		return 0, usageError(fmt.Sprintf("--duration %q: use 1h30m, 1h, 90m or minutes", s))
	}
	if d < time.Minute || d > 24*time.Hour {
		return 0, usageError(fmt.Sprintf("--duration %q: must be from 1m to 24h", s))
	}
	return d, nil
}

// syncCmd is a stub until the worklog slice: it lists what is owed and
// sends nothing. Exit 0 when nothing is owed, else 7.
func (a *app) syncCmd(name string, hidden bool) *cobra.Command {
	return &cobra.Command{
		Use:    name + " [KEY]",
		Short:  "Send every pending and failed worklog to Jira",
		Hidden: hidden,
		Args:   cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := ""
			if len(args) == 1 {
				key = args[0]
				if !core.ValidKey(key) {
					return usageError(fmt.Sprintf("%q is not a ticket key", key))
				}
			}
			w, _, err := a.read()
			if err != nil {
				a.code = a.failure(name, err)
				return nil
			}
			d := SyncData{Sent: []UnsentWorklog{}, Remaining: unsentOf(w.State, key)}
			if len(d.Remaining) == 0 {
				a.code = a.out.ok(name, d, nil, "Nothing to send.\n")
				return nil
			}
			human := ""
			for _, u := range d.Remaining {
				human += fmt.Sprintf("%s %s %s %s\n", u.Key, worklogDuration(u.Worklog.TimeSpentSeconds), u.Worklog.State,
					u.Worklog.StartedAt.Format("2006-01-02"))
			}
			a.code = a.out.partial(name, d, nil, human, failure{
				code: "sync_pending",
				msg:  fmt.Sprintf("%d worklog(s) not sent: sending to Jira is not built yet", len(d.Remaining)),
			})
			return nil
		},
	}
}

// setupCmd is a stub until the setup slice. It needs a terminal.
func (a *app) setupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Configure Jira and the Claude Code plugin (needs a terminal)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !a.env.TTY || a.out.json {
				return usageError("setup needs a terminal (--json never prompts)")
			}
			a.code = a.out.fail("setup", ExitInternal, failure{code: "internal",
				msg:  "setup is not built yet",
				hint: "write $XDG_CONFIG_HOME/punchcard/config.toml by hand for now"})
			return nil
		},
	}
}
