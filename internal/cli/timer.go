package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/store"
)

// runTimer handles start, pause, resume and status in plain text. The
// full --json contract for them is slice 3's.
func runTimer(o output, cmd string, args []string) int {
	now := o.env.Now
	if now == nil {
		now = time.Now
	}
	st := store.New(store.Dir(o.env.Getenv), now)
	origin := originFrom(o.env.Getenv)

	var c core.Command
	switch cmd {
	case "status":
		if len(args) > 0 {
			return o.usage(cmd, "status takes no arguments")
		}
		return status(o, st, now())
	case "pause":
		if len(args) > 0 {
			return o.usage(cmd, "pause takes no arguments")
		}
		c = core.Pause{Origin: origin}
	case "start":
		if len(args) != 1 {
			return o.usage(cmd, "start needs one ticket key (the picker comes with the TUI)")
		}
		c = core.Start{Key: args[0], Unverified: true, Origin: origin, NewEntryID: core.NewEntryID()}
	case "resume":
		if len(args) > 1 {
			return o.usage(cmd, "resume takes at most one ticket key")
		}
		r := core.Resume{Origin: origin, NewEntryID: core.NewEntryID()}
		if len(args) == 1 {
			r.Key = args[0]
		}
		c = r
	}

	res, err := st.Apply(c)
	notify(o, res.Notices)
	if err != nil {
		return failure(o, cmd, err)
	}
	var b strings.Builder
	for _, e := range res.Events {
		b.WriteString(describe(e, res.Now.Location()))
	}
	switch t, e, ok := core.Running(res.World.State); {
	case ok:
		fmt.Fprintf(&b, "▶ %s running  %s\n", t.Key, clock(core.Elapsed(e, res.Now)))
	case cmd == "pause":
		if t, e, ok := lastPaused(res.World.State); ok {
			fmt.Fprintf(&b, "⏸ %s paused  %s\n", t.Key, clock(core.Elapsed(e, res.Now)))
		}
	}
	return o.ok(cmd, nil, b.String())
}

// originFrom: agent inside Claude Code or another agent, else hand
// (ADR 0004). --origin arrives with slice 3.
func originFrom(getenv func(string) string) core.Origin {
	if getenv("CLAUDECODE") != "" || getenv("AI_AGENT") != "" {
		return core.OriginAgent
	}
	return core.OriginHand
}

func notify(o output, notices []string) {
	for _, n := range notices {
		fmt.Fprintf(o.env.Stderr, "punchcard: %s\n", n)
	}
}

// failure maps a core or store error to its exit code (ADR 0004).
func failure(o output, cmd string, err error) int {
	switch {
	case errors.Is(err, core.ErrUsage), errors.Is(err, core.ErrTooShort):
		return o.fail(cmd, ExitUsage, "usage", err.Error())
	case errors.Is(err, core.ErrNoTimer), errors.Is(err, core.ErrNotTracked), errors.Is(err, core.ErrUnlogged):
		return o.fail(cmd, ExitNoTimer, "no_timer", err.Error())
	case errors.Is(err, core.ErrHeld):
		// The spec has no code for this yet; slice 3 settles it.
		return o.fail(cmd, ExitNoTimer, "held", err.Error())
	case errors.Is(err, store.ErrNewerSchema):
		return o.fail(cmd, ExitStateUnreadable, "state_unreadable", err.Error()+"; upgrade punchcard")
	case errors.Is(err, store.ErrBusy):
		return o.fail(cmd, ExitBusy, "busy", err.Error()+"; try again")
	default:
		return o.fail(cmd, ExitInternal, "internal", err.Error())
	}
}

func describe(e core.Event, loc *time.Location) string {
	switch e.Kind {
	case core.EventAdded:
		return fmt.Sprintf("+ %s added (not verified with Jira yet)\n", e.Key)
	case core.EventPaused:
		return fmt.Sprintf("⏸ %s paused\n", e.Key)
	case core.EventHeld:
		return fmt.Sprintf("%s is now held: only you can move it\n", e.Key)
	case core.EventStarted:
		return fmt.Sprintf("▶ %s started\n", e.Key)
	case core.EventRecovered:
		return fmt.Sprintf("! %s recovered: no presence since %s, so it was capped there; fix the duration at stop\n",
			e.Key, e.At.In(loc).Format("15:04"))
	default:
		return fmt.Sprintf("%s %s\n", e.Kind, e.Key)
	}
}

// status prints the world without writing anything.
func status(o output, st *store.Store, now time.Time) int {
	w, notices, err := st.Read()
	notify(o, notices)
	if err != nil {
		return failure(o, "status", err)
	}
	var b strings.Builder
	if t, e, ok := core.Running(w.State); ok {
		if core.Stale(w, now) {
			r, _ := core.Recover(w, now)
			capped := core.Elapsed(findEntry(r.State, t.Key, e.ID), now)
			seen := w.Heartbeat.LastSeen
			if seen.IsZero() {
				seen = e.Segments[len(e.Segments)-1].StartedAt
			}
			fmt.Fprintf(&b, "! %s stale  %s (capped; %s uncapped)\n", t.Key, clock(capped), clock(core.Elapsed(e, now)))
			fmt.Fprintf(&b, "  last seen %s (%s ago); the next action will recover it\n",
				seen.In(now.Location()).Format("15:04"), age(now.Sub(seen.Time)))
		} else {
			fmt.Fprintf(&b, "▶ %s running  %s\n", t.Key, clock(core.Elapsed(e, now)))
		}
	} else {
		b.WriteString("Nothing running.\n")
	}
	for _, t := range core.ListOrder(w.State, now) {
		for _, e := range t.Entries {
			if e.Paused() {
				fmt.Fprintf(&b, "⏸ %s paused  %s%s\n", t.Key, clock(core.Elapsed(e, now)), recovered(e))
			}
		}
	}
	if n, total := core.Owed(w.State); n > 0 {
		fmt.Fprintf(&b, "%d unsent worklog(s), %s\n", n, clock(total))
	}
	return o.ok("status", nil, b.String())
}

func recovered(e core.TimeEntry) string {
	if e.Recovered {
		return "  (recovered: check the duration at stop)"
	}
	return ""
}

func findEntry(s core.State, key, id string) core.TimeEntry {
	for _, t := range s.TrackedTickets {
		if t.Key == key {
			for _, e := range t.Entries {
				if e.ID == id {
					return e
				}
			}
		}
	}
	return core.TimeEntry{}
}

// lastPaused is the most recently paused entry.
func lastPaused(s core.State) (core.TrackedTicket, core.TimeEntry, bool) {
	var bt core.TrackedTicket
	var be core.TimeEntry
	found := false
	for _, t := range s.TrackedTickets {
		for _, e := range t.Entries {
			if !e.Paused() {
				continue
			}
			end := e.Segments[len(e.Segments)-1].EndedAt
			if !found || end.After(be.Segments[len(be.Segments)-1].EndedAt.Time) {
				bt, be, found = t, e, true
			}
		}
	}
	return bt, be, found
}

// clock formats a duration as h:mm:ss.
func clock(d time.Duration) string {
	d = d.Truncate(time.Second)
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

// age formats how long ago, coarsely: 3h15m, 12m, 40s.
func age(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
