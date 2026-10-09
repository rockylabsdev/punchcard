package core_test

import (
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

func TestApplySyncOutcomes(t *testing.T) {
	stoppedAs := func(wl core.Worklog) core.TrackedTicket {
		return ticket("AA-1", withWorklog(entry("EA", hand, seg("09:00", "09:30")), wl))
	}
	base := core.Worklog{StartedAt: at("09:00"), TimeSpentSeconds: 1800}
	with := func(state core.WorklogState, f func(*core.Worklog)) core.Worklog {
		wl := base
		wl.State = state
		if f != nil {
			f(&wl)
		}
		return wl
	}
	synced := with(core.WorklogSynced, func(w *core.Worklog) { w.WorklogID = "110001"; w.SyncedAt = at("10:00") })
	failed := with(core.WorklogFailed, func(w *core.Worklog) { w.Message = "timeout"; w.FailedAt = at("10:00") })
	local := with(core.WorklogLocal, nil)
	authFailed := func(s core.State) core.State { s.AuthFailedAt = at("09:45"); return s }

	runApply(t, []applyCase{
		{
			name:   "mark synced records the worklog id and clears the auth failure",
			w:      core.World{State: authFailed(state(stoppedAs(with(core.WorklogPending, nil))))},
			cmd:    core.MarkSynced{Key: "AA-1", EntryID: "EA", WorklogID: "110001"},
			now:    "10:00",
			want:   state(stoppedAs(synced)),
			events: []core.Event{{Kind: core.EventSynced, Key: "AA-1"}},
		},
		{
			name:   "a retry that succeeds clears the failure",
			w:      world("", stoppedAs(failed)),
			cmd:    core.MarkSynced{Key: "AA-1", EntryID: "EA", WorklogID: "110001"},
			now:    "10:00",
			want:   state(stoppedAs(synced)),
			events: []core.Event{{Kind: core.EventSynced, Key: "AA-1"}},
		},
		{
			name:   "mark failed keeps it retryable with the reason",
			w:      world("", stoppedAs(with(core.WorklogPending, nil))),
			cmd:    core.MarkFailed{Key: "AA-1", EntryID: "EA", Message: "timeout"},
			now:    "10:00",
			want:   state(stoppedAs(failed)),
			events: []core.Event{{Kind: core.EventSyncFailed, Key: "AA-1", Message: "timeout"}},
		},
		{
			name: "a refusal keeps the tracker's status",
			w:    world("", stoppedAs(with(core.WorklogPending, nil))),
			cmd:  core.MarkFailed{Key: "AA-1", EntryID: "EA", Message: "Service Unavailable", Status: 503},
			now:  "10:00",
			want: state(stoppedAs(with(core.WorklogFailed, func(w *core.Worklog) {
				w.Message, w.RefusedStatus, w.FailedAt = "Service Unavailable", 503, at("10:00")
			}))),
			events: []core.Event{{Kind: core.EventSyncFailed, Key: "AA-1", Message: "Service Unavailable"}},
		},
		{
			name: "a retry that succeeds after a refusal drops its status",
			w: world("", stoppedAs(with(core.WorklogFailed, func(w *core.Worklog) {
				w.Message, w.RefusedStatus, w.FailedAt = "Service Unavailable", 503, at("09:45")
			}))),
			cmd:    core.MarkSynced{Key: "AA-1", EntryID: "EA", WorklogID: "110001"},
			now:    "10:00",
			want:   state(stoppedAs(synced)),
			events: []core.Event{{Kind: core.EventSynced, Key: "AA-1"}},
		},
		{
			name: "never mark a synced worklog synced again",
			w:    world("", stoppedAs(synced)),
			cmd:  core.MarkSynced{Key: "AA-1", EntryID: "EA", WorklogID: "999"},
			now:  "11:00", err: core.ErrAlreadySynced,
		},
		{
			name: "never downgrade a synced worklog to failed",
			w:    world("", stoppedAs(synced)),
			cmd:  core.MarkFailed{Key: "AA-1", EntryID: "EA", Message: "late timeout"},
			now:  "11:00", err: core.ErrAlreadySynced,
		},
		{
			name: "never mark a local worklog synced: there is no tracker to have told",
			w:    world("", stoppedAs(local)),
			cmd:  core.MarkSynced{Key: "AA-1", EntryID: "EA", WorklogID: "999"},
			now:  "11:00", err: core.ErrLocal,
		},
		{
			name: "never mark a local worklog failed",
			w:    world("", stoppedAs(local)),
			cmd:  core.MarkFailed{Key: "AA-1", EntryID: "EA", Message: "timeout"},
			now:  "11:00", err: core.ErrLocal,
		},
		{
			name: "an entry that is not stopped has no worklog to mark",
			w:    world("", ticket("AA-1", entry("EA", hand, seg("09:00", "")))),
			cmd:  core.MarkSynced{Key: "AA-1", EntryID: "EA", WorklogID: "1"},
			now:  "10:00", err: core.ErrNoTimer,
		},
		{
			name: "record auth failure sets authFailedAt",
			w:    world("", stoppedAs(with(core.WorklogPending, nil))),
			cmd:  core.RecordAuthFailure{},
			now:  "09:45",
			want: authFailed(state(stoppedAs(with(core.WorklogPending, nil)))),
		},
		{
			name: "a successful authenticated call clears the auth failure",
			w:    core.World{State: authFailed(state(stoppedAs(with(core.WorklogPending, nil))))},
			cmd:  core.ClearAuthFailure{},
			now:  "10:00",
			want: state(stoppedAs(with(core.WorklogPending, nil))),
		},
		{
			name: "verify is Jira answering, so it clears the auth failure",
			w:    core.World{State: authFailed(state(ticket("AA-2")))},
			cmd:  core.Verify{Key: "AA-2", Summary: "Fix"},
			now:  "10:00",
			want: state(func() core.TrackedTicket { t := ticket("AA-2"); t.Summary = "Fix"; return t }()),
		},
	})
}

func TestUnsentNeverOffersALoggedWorklog(t *testing.T) {
	wl := func(id string, s core.WorklogState, secs int) core.TimeEntry {
		return withWorklog(entry(id, hand, seg("09:00", "09:30")), core.Worklog{State: s, StartedAt: at("09:00"), TimeSpentSeconds: secs})
	}
	s := state(
		ticket("AA-1", wl("E1", core.WorklogSynced, 600), wl("E2", core.WorklogPending, 1200)),
		ticket("BB-2", wl("E3", core.WorklogFailed, 1800), entry("E4", hand, seg("10:00", ""))),
		ticket("payroll", wl("E5", core.WorklogLocal, 2400)),
	)
	var got []string
	for _, u := range core.Unsent(s) {
		got = append(got, u.Key+"/"+u.EntryID)
	}
	diff(t, "unsent", []string{"AA-1/E2", "BB-2/E3"}, got)
	count, total := core.Owed(s)
	diff(t, "owed count", 2, count)
	diff(t, "owed total seconds", 3000, int(total.Seconds()))
}
