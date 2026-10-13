package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/spf13/cobra"
)

// The timer commands: start, pause, resume and the hidden touch. Each
// builds one core command and sends it through the store seam; data is
// the world after the change (StatusData).

func (a *app) startCmd() *cobra.Command {
	var atFlag string
	c := &cobra.Command{
		Use:   "start [KEY]",
		Short: "Start the timer on a ticket, pausing any other (no KEY: pick from recents)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			when, err := a.at(atFlag)
			if err != nil {
				return err
			}
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
			a.change("start", core.Start{Key: key, At: when, Summary: n.Summary, Unverified: n.Unverified,
				Origin: a.origin, NewEntryID: a.env.NewID(), Grammar: a.env.Grammar()})
			return nil
		},
	}
	atOption(c, &atFlag, "start then instead of now; a ticket it pauses is paused then too")
	return c
}

func (a *app) pauseCmd() *cobra.Command {
	var atFlag string
	c := &cobra.Command{
		Use:   "pause",
		Short: "Pause the running timer (the entry stays open)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			when, err := a.at(atFlag)
			if err != nil {
				return err
			}
			a.change("pause", core.Pause{At: when, Origin: a.origin})
			return nil
		},
	}
	atOption(c, &atFlag, "pause then instead of now (with nothing running, move the last pause there)")
	return c
}

func (a *app) resumeCmd() *cobra.Command {
	var atFlag string
	c := &cobra.Command{
		Use:   "resume [KEY]",
		Short: "Resume the most recently paused entry; resume KEY is start KEY",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			when, err := a.at(atFlag)
			if err != nil {
				return err
			}
			c := core.Resume{At: when, Origin: a.origin, NewEntryID: a.env.NewID()}
			if len(args) == 1 {
				key := a.key(args[0])
				n, err := a.newTicket(key)
				if err != nil {
					a.code = a.failure("resume", err)
					return nil
				}
				a.change("resume", core.Start{Key: key, At: when, Summary: n.Summary, Unverified: n.Unverified,
					Origin: a.origin, NewEntryID: c.NewEntryID, Grammar: a.env.Grammar()})
				return nil
			}
			a.change("resume", c)
			return nil
		},
	}
	atOption(c, &atFlag, "resume then instead of now")
	return c
}

// atOption is --at TIME on a timer command.
func atOption(c *cobra.Command, v *string, usage string) {
	c.Flags().StringVar(v, "at", "", usage+": HH:MM or -D (-45m: that long ago)")
}

// at reads --at; "" is now (the zero time).
func (a *app) at(flag string) (time.Time, error) {
	if flag == "" {
		return time.Time{}, nil
	}
	t, err := core.ParseAt(flag, a.env.Now())
	if err != nil {
		return time.Time{}, usageError(fmt.Sprintf("--at %q is not a time: use %s", flag, core.AtForms))
	}
	return t, nil
}

// touchCmd is the mod's presence call: presence, and focus on KEY (or
// --cwd's key, as status --cwd reports it). It never waits for the lock.
func (a *app) touchCmd() *cobra.Command {
	var cwd string
	c := &cobra.Command{
		Use:    "touch [KEY]",
		Short:  "Record presence, and focus on KEY or --cwd's key",
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := ""
			switch {
			case len(args) == 1:
				key = a.key(args[0])
			case cwd != "":
				// A plain read for the list the branch rule asks about;
				// it never waits on the lock. Its notices and error are
				// dropped: the apply below reads again and reports both,
				// and a failed read leaves an empty list (capitals only).
				w, _, _ := a.env.Store.Read()
				here, notices := resolveHere(cwd, a.env.Grammar(), trackedKeys(w.State))
				a.out.notices(notices)
				if here != nil {
					key = here.Key
				}
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
	c.Flags().StringVar(&cwd, "cwd", "", "take the key from this directory (its .punchcard, else its git branch)")
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
	// A change at a named time reports itself as an event with At; its
	// line is the timer line, which then says when.
	own := core.EventStarted
	if name == "pause" {
		own = core.EventPaused
	}
	var self *core.Event
	var others []core.Event
	for i, e := range res.Events {
		if e.Kind == own && !e.At.IsZero() && self == nil {
			self = &res.Events[i]
			continue
		}
		others = append(others, e)
	}
	human := a.describeAll(others, res)
	loc := res.Now.Location()
	var line string
	switch {
	case d.Running != nil:
		line = a.out.timerLine(*d.Running, loc, res.Now)
	case name == "pause":
		if self != nil {
			if tm, ok := timerFor(res, self.Key); ok {
				line = a.out.timerLine(tm, loc, res.Now)
			}
		} else if tm, ok := lastPaused(res); ok {
			line = a.out.timerLine(tm, loc, res.Now)
		}
	}
	if self != nil {
		hhmm := core.Clock(self.At.Time, loc)
		line = strings.Replace(line, " running  ", " running since "+hhmm+"  ", 1)
		line = strings.Replace(line, " paused  ", " paused at "+hhmm+"  ", 1)
		if self.Message != "" {
			line += "  " + self.Message + "\n"
		}
	}
	a.done(name, res, d, human+line)
}

func (a *app) eventLines(res store.Result) string { return a.describeAll(res.Events, res) }

func (a *app) describeAll(events []core.Event, res store.Result) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString(a.out.describe(e, res.World, res.Now.Location()))
	}
	return b.String()
}

// timerFor is key's open timer after the change.
func timerFor(res store.Result, key string) (Timer, bool) {
	for _, t := range res.World.State.TrackedTickets {
		if t.Key != key {
			continue
		}
		for _, e := range t.Entries {
			if !e.Stopped() {
				return timerOf(res.World, t, e, res.Now), true
			}
		}
	}
	return Timer{}, false
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
