package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/j-dsmith/punchcard/internal/tracker"
	"github.com/j-dsmith/punchcard/internal/worklog"
	"github.com/spf13/cobra"
)

// stop and sync, the only commands that POST (through package worklog),
// and the setup stub.

// stopFlags are stop's switches.
type stopFlags struct {
	noSync, yes, dryRun bool
}

func (a *app) stopCmd() *cobra.Command {
	var durationFlag, message string
	var f stopFlags
	c := &cobra.Command{
		Use:   "stop [KEY]",
		Short: "Stop an entry and log its worklog, sending it to the tracker if one is set up (no KEY: the running entry, else the single paused one)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := core.Stop{Comment: message, Origin: a.origin}
			if len(args) == 1 {
				s.Key = a.key(args[0])
			}
			if cmd.Flags().Changed("duration") {
				d, err := parseDuration(durationFlag)
				if err != nil {
					return err
				}
				s.Duration = d
			}
			a.stop(s, f)
			return nil
		},
	}
	c.Flags().StringVar(&durationFlag, "duration", "", "time to log instead of the measured elapsed: 1h30m, 1h, 90m or minutes (1m to 24h)")
	c.Flags().StringVarP(&message, "message", "m", "", "worklog comment")
	c.Flags().BoolVar(&f.noSync, "no-sync", false, "record the worklog without sending it to the tracker (pending)")
	c.Flags().BoolVar(&f.yes, "yes", false, "stop without prompting (an agent's stop needs it)")
	c.Flags().BoolVar(&f.dryRun, "dry-run", false, "show the exact worklog a stop would record, and change nothing")
	return c
}

// sender is the worklog seam over this invocation's store and tracker.
func (a *app) sender() worklog.Sender {
	return worklog.Sender{Store: a.env.Store, Tracker: a.env.InEffect()}
}

// stop previews the worklog from a fresh read, applies the CLI's rules
// (dry run, an agent's proposal, the recovered rule, the TTY prompt),
// then stops the previewed entry and sends its worklog. Core re-checks
// everything under the lock.
func (a *app) stop(s core.Stop, f stopFlags) {
	// With no tracker the worklog is local: the preview, --no-sync and
	// the send all record it so.
	s = a.sender().Prepare(s)
	w, now, err := a.read()
	if err != nil {
		a.code = a.failure("stop", err)
		return
	}
	promptable := a.env.TTY && !a.out.json && !f.yes && !f.dryRun && a.origin != core.OriginAgent
	p, err := core.PreviewStop(w, s, now)
	measured := true
	if errors.Is(err, core.ErrTooShort) && promptable && s.Duration == 0 {
		// The prompt can still take a duration; preview with a stand-in.
		probe := s
		probe.Duration = time.Minute
		p, err = core.PreviewStop(w, probe, now)
		measured = false
	}
	if err != nil {
		a.code = a.failure("stop", err)
		return
	}
	pd := PreviewData{Preview: Preview{Key: p.Key, EntryID: p.EntryID, Worklog: p.Worklog, Recovered: p.Recovered}}
	switch {
	case f.dryRun:
		a.code = a.out.ok("stop", pd, nil, previewText(p, now.Location()))
		return
	case a.origin == core.OriginAgent && !f.yes:
		a.code = a.out.fail("stop", ExitNeedsConfirmation, failure{
			code: "needs_confirmation",
			msg:  "an agent's stop is a proposal: " + strings.TrimSuffix(previewText(p, now.Location()), "\n"),
			hint: "show the user this worklog; only `punchcard stop --yes` sends it",
			data: pd,
		})
		return
	case p.Recovered && s.Duration == 0 && !promptable:
		a.code = a.out.fail("stop", ExitNeedsConfirmation, failure{
			code: "needs_confirmation",
			msg:  fmt.Sprintf("%s was recovered: its time stops at the last presence, so it needs a duration", p.Key),
			hint: "rerun with --duration, e.g. --duration 1h30m",
			data: pd,
		})
		return
	}
	if promptable {
		def := 0
		if measured {
			def = p.Worklog.TimeSpentSeconds
		}
		var ok bool
		if s, ok = a.promptStop(s, p, def); !ok {
			fmt.Fprintln(a.env.Stderr, "punchcard: not stopped; the timer is untouched")
			a.code = ExitOK
			return
		}
	}
	s.Key, s.EntryID = p.Key, p.EntryID
	if f.noSync {
		res, err := a.apply(s)
		if err != nil {
			a.code = a.failure("stop", err)
			return
		}
		e, _ := res.World.State.Entry(s.Key, s.EntryID)
		wl := e.Worklog
		d := StopData{Key: s.Key, EntryID: s.EntryID, Worklog: *wl}
		a.done("stop", res, d, a.eventLines(res)+stoppedLine(s.Key, *wl))
		return
	}
	r, err := a.sender().Stop(context.Background(), s)
	a.out.notices(r.Notices)
	if err != nil {
		a.code = a.failure("stop", err)
		return
	}
	d := StopData{Key: r.Key, EntryID: r.EntryID, Worklog: r.Worklog}
	res := store.Result{World: r.World, Events: r.Events, Now: r.Now}
	human := a.timerEventLines(res) + stoppedLine(r.Key, r.Worklog)
	if r.SyncErr == nil {
		a.done("stop", res, d, human)
		return
	}
	code, msg := sendProblem(r.SyncErr)
	a.code = a.out.partial("stop", d, r.Events, human, failure{
		code: code,
		msg:  "the worklog is saved but not sent: " + msg,
		hint: "run `punchcard sync` later; never stop again",
	})
}

// timerEventLines is eventLines without the sync outcomes, which the
// stop line and the error already report.
func (a *app) timerEventLines(res store.Result) string {
	var b strings.Builder
	for _, e := range res.Events {
		if e.Kind != core.EventSynced && e.Kind != core.EventSyncFailed {
			b.WriteString(a.out.describe(e, res.World, res.Now.Location()))
		}
	}
	return b.String()
}

func stoppedLine(key string, wl core.Worklog) string {
	outcome := "worklog " + string(wl.State)
	switch wl.State {
	case core.WorklogSynced:
		outcome = "sent to Jira"
	case core.WorklogLocal:
		outcome = "logged"
	}
	return fmt.Sprintf("■ %s stopped: %s, %s\n", key, worklogDuration(wl.TimeSpentSeconds), outcome)
}

// previewText is the worklog a stop would send (or, with no tracker,
// log), in words.
func previewText(p core.Preview, loc *time.Location) string {
	verb := "send"
	if p.Worklog.State == core.WorklogLocal {
		verb = "log"
	}
	s := fmt.Sprintf("%s: would %s %s, started %s", p.Key, verb, worklogDuration(p.Worklog.TimeSpentSeconds),
		p.Worklog.StartedAt.In(loc).Format("2006-01-02 15:04"))
	if p.Worklog.Comment != "" {
		s += fmt.Sprintf(", comment %q", p.Worklog.Comment)
	}
	if p.Recovered {
		s += " (recovered: capped at the last presence)"
	}
	return s + "\n"
}

// promptStop asks on the terminal for the duration (prefilled with the
// --duration value, else def seconds of measured time; no default under
// a minute) and an optional comment (prefilled with -m). It returns
// false when stdin ends before a duration is given.
func (a *app) promptStop(s core.Stop, p core.Preview, def int) (core.Stop, bool) {
	in := bufio.NewReader(a.env.Stdin)
	if s.Duration != 0 {
		def = int(s.Duration / time.Second)
	}
	if p.Recovered {
		fmt.Fprintf(a.env.Stderr, "%s was recovered: its time stops at the last presence. Correct the duration if needed.\n", p.Key)
	}
	label := "Duration: "
	if def > 0 {
		label = fmt.Sprintf("Duration [%s]: ", worklogDuration(def))
	}
	for {
		fmt.Fprintf(a.env.Stderr, "%s %s", p.Key, label)
		line, err := in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" && errors.Is(err, io.EOF) {
			return s, false
		}
		if line == "" && def > 0 {
			s.Duration = time.Duration(def) * time.Second
			break
		}
		if line == "" {
			continue
		}
		d, perr := parseDuration(line)
		if perr == nil {
			s.Duration = d
			break
		}
		fmt.Fprintf(a.env.Stderr, "  %s\n", strings.TrimPrefix(perr.Error(), "--duration "))
		if errors.Is(err, io.EOF) {
			return s, false
		}
	}
	label = "Comment (optional): "
	if s.Comment != "" {
		label = fmt.Sprintf("Comment [%s]: ", s.Comment)
	}
	fmt.Fprint(a.env.Stderr, label)
	if line, _ := in.ReadString('\n'); strings.TrimSpace(line) != "" {
		s.Comment = strings.TrimSpace(line)
	}
	return s, true
}

// sendProblem names why a worklog was not sent: the envelope's
// error.code (worklog.Why) and a message. A not-configured error already
// says to run setup where setup is the fix.
func sendProblem(err error) (code, msg string) {
	p := worklog.Why(err)
	if p == worklog.ProblemAuthFailed {
		return string(p), "Jira rejected the credentials (a scoped API token is refused too)"
	}
	return string(p), err.Error()
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

// syncCmd sends every pending and failed worklog (KEY's only, with a
// key), one at a time: 0 all sent, 7 some remain, 8 on a 401, 3
// unconfigured or no tracker at all, 75 while another punchcard is
// sending.
func (a *app) syncCmd(name string, hidden bool) *cobra.Command {
	return &cobra.Command{
		Use:    name + " [KEY]",
		Short:  "Send every pending and failed worklog to the tracker",
		Hidden: hidden,
		Args:   cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := ""
			if len(args) == 1 {
				key = a.key(args[0])
				if !a.env.Grammar().Valid(key) {
					return usageError(fmt.Sprintf("%q is not a ticket key", key))
				}
			}
			a.sync(name, key)
			return nil
		},
	}
}

func (a *app) sync(name, key string) {
	if !a.env.HasTracker() {
		a.syncWithoutTracker(name, key)
		return
	}
	rep, err := a.sender().Sync(context.Background(), key)
	a.out.notices(rep.Notices)
	d := SyncData{Sent: unsentData(rep.Sent), Remaining: unsentData(rep.Remaining)}
	human := syncText(d)
	var refused *tracker.Refusal
	switch {
	case err == nil && len(d.Remaining) == 0:
		a.code = a.out.ok(name, d, rep.Events, human)
	case errors.Is(err, config.ErrNotConfigured):
		a.code = a.out.fail(name, ExitNotConfigured, failure{code: "not_configured", msg: err.Error(),
			hint: "run `punchcard setup`", data: d})
	case errors.Is(err, tracker.ErrUnauthorized):
		a.code = a.out.fail(name, ExitAuthFailed, failure{code: "auth_failed",
			msg:  "Jira rejected the credentials; the worklogs stay pending",
			hint: "check the email and API token with `punchcard setup`; a scoped token is refused too", data: d})
	case errors.Is(err, store.ErrSyncBusy):
		a.code = a.out.fail(name, ExitBusy, failure{code: "busy", msg: err.Error(),
			hint: "it may be sending these; try again in a moment", data: d})
	case err == nil, errors.Is(err, tracker.ErrUnreachable), errors.As(err, &refused):
		code, msg := "sync_pending", fmt.Sprintf("%d worklog(s) still owed", len(d.Remaining))
		if err != nil {
			code, msg = sendProblem(err)
		}
		a.code = a.out.partial(name, d, rep.Events, human, failure{code: code, msg: msg,
			hint: "run `punchcard sync` again later"})
	default:
		a.code = a.failure(name, err)
	}
}

// syncWithoutTracker refuses, even with nothing owed: a sync that
// quietly did nothing would let a user believe the time went somewhere.
// Remaining lists what is still owed from when a tracker was set.
func (a *app) syncWithoutTracker(name, key string) {
	w, _, err := a.read()
	if err != nil {
		a.code = a.failure(name, err)
		return
	}
	var owed []core.UnsentWorklog
	for _, u := range core.Unsent(w.State) {
		if key == "" || u.Key == key {
			owed = append(owed, u)
		}
	}
	a.code = a.out.fail(name, ExitNotConfigured, failure{code: "not_configured",
		msg:  "there is no tracker to send to",
		hint: "worklogs are logged here; `punchcard setup` connects one",
		data: SyncData{Sent: []UnsentWorklog{}, Remaining: unsentData(owed)}})
}

func unsentData(us []core.UnsentWorklog) []UnsentWorklog {
	out := []UnsentWorklog{}
	for _, u := range us {
		out = append(out, UnsentWorklog{Key: u.Key, EntryID: u.EntryID, Worklog: u.Worklog})
	}
	return out
}

func syncText(d SyncData) string {
	if len(d.Sent) == 0 && len(d.Remaining) == 0 {
		return "Nothing to send.\n"
	}
	var b strings.Builder
	for _, u := range d.Sent {
		fmt.Fprintf(&b, "✓ %s %s sent (%s)\n", u.Key, worklogDuration(u.Worklog.TimeSpentSeconds), u.Worklog.StartedAt.Format("2006-01-02"))
	}
	for _, u := range d.Remaining {
		switch u.Worklog.State {
		case core.WorklogFailed:
			fmt.Fprintf(&b, "✗ %s %s failed: %s\n", u.Key, worklogDuration(u.Worklog.TimeSpentSeconds), u.Worklog.Message)
		default:
			fmt.Fprintf(&b, "○ %s %s not sent yet\n", u.Key, worklogDuration(u.Worklog.TimeSpentSeconds))
		}
	}
	return b.String()
}

// setupCmd is a stub until the setup slice. It needs a terminal.
