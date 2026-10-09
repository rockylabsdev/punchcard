package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/spf13/cobra"
)

// The timer commands: start, pause, resume and the hidden touch. Each
// builds one core command and sends it through the store seam; data is
// the world after the change (StatusData).

func (a *app) startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start [KEY]",
		Short: "Start the timer on a ticket, pausing any other (no KEY: pick from recents)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var key string
			if len(args) == 1 {
				key = a.key(args[0])
			} else {
				k, err := a.pick("start")
				if err != nil || k == "" {
					return err // "" is a cancelled picker: a.code stays 0
				}
				key = a.key(k)
			}
			n, err := a.newTicket(key)
			if err != nil {
				a.code = a.failure("start", err)
				return nil
			}
			a.change("start", core.Start{Key: key, Summary: n.Summary, Unverified: n.Unverified,
				Origin: a.origin, NewEntryID: a.env.NewID(), Grammar: a.env.Grammar()})
			return nil
		},
	}
}

func (a *app) pauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause",
		Short: "Pause the running timer (the entry stays open)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a.change("pause", core.Pause{Origin: a.origin})
			return nil
		},
	}
}

func (a *app) resumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume [KEY]",
		Short: "Resume the most recently paused entry; resume KEY is start KEY",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := core.Resume{Origin: a.origin, NewEntryID: a.env.NewID()}
			if len(args) == 1 {
				key := a.key(args[0])
				n, err := a.newTicket(key)
				if err != nil {
					a.code = a.failure("resume", err)
					return nil
				}
				a.change("resume", core.Start{Key: key, Summary: n.Summary, Unverified: n.Unverified,
					Origin: a.origin, NewEntryID: c.NewEntryID, Grammar: a.env.Grammar()})
				return nil
			}
			a.change("resume", c)
			return nil
		},
	}
}

// touchCmd is the mod's presence call: presence, and focus on KEY (or
// the key in --cwd's git branch). It never waits for the lock.
func (a *app) touchCmd() *cobra.Command {
	var cwd string
	c := &cobra.Command{
		Use:    "touch [KEY]",
		Short:  "Record presence, and focus on KEY or the --cwd branch's key",
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := ""
			switch {
			case len(args) == 1:
				key = a.key(args[0])
			case cwd != "":
				key = branchKey(cwd)
			}
			// Presence never asks the tracker: a new key with one is unverified.
			t := core.Touch{Key: key, Unverified: a.env.HasTracker(), Origin: a.origin, NewEntryID: a.env.NewID(),
				Grammar: a.env.Grammar()}
			res, err := a.env.Store.TryApply(t)
			a.out.notices(res.Notices)
			switch {
			case errors.Is(err, store.ErrBusy):
				a.code = a.out.ok("touch", nil, nil, "") // presence skips when busy
			case err != nil:
				a.code = a.failure("touch", err)
			default:
				a.done("touch", res, snapshot(res.World, res.Now), a.eventLines(res))
			}
			return nil
		},
	}
	c.Flags().StringVar(&cwd, "cwd", "", "take the key from this directory's git branch")
	return c
}

// change applies a timer command and reports the world after it.
func (a *app) change(name string, c core.Command) {
	res, err := a.apply(c)
	if err != nil {
		a.code = a.failure(name, err)
		return
	}
	d := snapshot(res.World, res.Now)
	human := a.eventLines(res)
	loc := res.Now.Location()
	switch {
	case d.Running != nil:
		human += a.out.timerLine(*d.Running, loc, res.Now)
	case name == "pause":
		if tm, ok := lastPaused(res); ok {
			human += a.out.timerLine(tm, loc, res.Now)
		}
	}
	a.done(name, res, d, human)
}

func (a *app) eventLines(res store.Result) string {
	var b strings.Builder
	for _, e := range res.Events {
		b.WriteString(a.out.describe(e, res.World, res.Now.Location()))
	}
	return b.String()
}

// lastPaused is the most recently paused timer.
func lastPaused(res store.Result) (Timer, bool) {
	t, e, ok := core.LastPaused(res.World.State)
	if !ok {
		return Timer{}, false
	}
	return timerOf(res.World, t, e, res.Now), true
}

// pick asks for a ticket on a TTY, recents (the ls order) first. With no
// TTY, or with --json (which never prompts), a missing key is usage.
func (a *app) pick(cmd string) (string, error) {
	if !a.env.TTY || a.out.json {
		return "", usageError(cmd + " needs a ticket key when not on a terminal (--json never prompts)")
	}
	w, now, err := a.read()
	if err != nil {
		a.code = a.failure(cmd, err)
		return "", nil
	}
	var choices []Choice
	for _, t := range core.ListOrder(w.State, now) {
		choices = append(choices, Choice{Key: t.Key, Summary: t.Summary})
	}
	if len(choices) == 0 {
		return "", usageError("no tickets yet: punchcard " + cmd + " KEY")
	}
	pick := a.env.Pick
	if pick == nil {
		pick = a.linePick
	}
	key, err := pick(choices)
	if err != nil {
		return "", usageError(err.Error())
	}
	if key == "" {
		fmt.Fprintln(a.env.Stderr, "Nothing started.")
	}
	return key, nil
}
