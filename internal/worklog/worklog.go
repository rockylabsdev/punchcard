// Package worklog sends worklogs to Jira: stop-and-send for one entry,
// and sync for everything owed. It is the only code that POSTs, and the
// CLI and the TUI both call it (only `stop` and `sync` ever send).
//
// Sending follows ADR 0002: take sync.lock without waiting, re-read the
// worklog (never POST one that is synced), POST with no state.lock held,
// then record the outcome with a short store.Apply before releasing
// sync.lock. A failed send never touches a running timer.
package worklog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/jira"
	"github.com/j-dsmith/punchcard/internal/store"
)

// PostBudget bounds one worklog POST.
const PostBudget = 10 * time.Second

// markTries is how many times recording a sent worklog retries a busy
// state.lock (each try waits the store's LockWait). Jira already has
// the worklog, so giving up early would invite a duplicate POST.
const markTries = 5

// Sender sends worklogs through Store, to the Jira that Jira opens.
type Sender struct {
	Store *store.Store
	// Jira opens the client; it is called only when there is something
	// to send. Any error from it is treated as not configured.
	Jira func() (jira.Client, error)
}

// StopResult is a stop that happened, and what became of its worklog.
type StopResult struct {
	Key, EntryID string
	// Worklog is the entry's worklog as it is now on disk: synced, or
	// still pending or failed.
	Worklog core.Worklog
	// World is on disk after the last write; Now is when the stop was
	// applied. Events are the stop's (a recovery) then the send's.
	World   core.World
	Now     time.Time
	Events  []core.Event
	Notices []string
	// SyncErr is nil when Jira took the worklog. Otherwise the worklog is
	// owed, and SyncErr says why (match with errors.Is / errors.As):
	// config.ErrNotConfigured, jira.ErrUnauthorized (authFailedAt is now
	// set; pending), jira.ErrUnreachable (failed), *jira.APIError
	// (failed), store.ErrSyncBusy (another punchcard is sending;
	// pending), or a store error recording the outcome.
	SyncErr error
}

// Stop applies c (pinned to its entry: when c.EntryID is empty it is
// resolved from a fresh read first) and then sends that entry's
// worklog, once. The returned error is only for a stop that did not
// happen (nothing open, too short, busy, ...); then nothing changed and
// nothing was sent. A stop whose send failed returns a nil error and a
// non-nil SyncErr: the caller reports a success with debt, never stops
// again.
func (s Sender) Stop(ctx context.Context, c core.Stop) (StopResult, error) {
	var notices []string
	if c.EntryID == "" {
		w, ns, err := s.Store.Read()
		notices = ns
		if err != nil {
			return StopResult{Notices: notices}, err
		}
		p, err := core.PreviewStop(w, c, s.Store.Now())
		if err != nil {
			return StopResult{Notices: notices}, err
		}
		c.Key, c.EntryID = p.Key, p.EntryID
	}
	res, err := s.Store.Apply(c)
	notices = append(notices, res.Notices...)
	if err != nil {
		return StopResult{Notices: notices}, err
	}
	r := StopResult{Key: c.Key, EntryID: c.EntryID, World: res.World, Now: res.Now, Events: res.Events}
	rep, err := s.send(ctx, func(st core.State) []core.UnsentWorklog { return only(st, c.Key, c.EntryID) })
	r.SyncErr = err
	r.Events = append(r.Events, rep.Events...)
	r.Notices = append(notices, rep.Notices...)
	if rep.World.State.SchemaVersion != 0 {
		r.World = rep.World
	}
	if e, ok := r.World.State.Entry(c.Key, c.EntryID); ok && e.Worklog != nil {
		r.Worklog = *e.Worklog
	}
	return r, nil
}

// Report is what one sync did.
type Report struct {
	// Sent are the worklogs Jira took, as now on disk (synced).
	Sent []core.UnsentWorklog
	// Remaining are the worklogs in scope still owed after the sync.
	Remaining []core.UnsentWorklog
	Events    []core.Event
	// World is on disk after the last write (or as read, if none).
	World   core.World
	Notices []string
}

// Sync sends every pending and failed worklog (only key's when key is
// not empty), one at a time, under one hold of sync.lock. A 401 or an
// unreachable Jira stops the run (the rest would fail the same way);
// any other refusal marks that worklog failed and carries on. The error
// is nil when nothing went wrong, else the reason (as for
// StopResult.SyncErr; for a run with several refusals, the last).
// Nothing owed is a nil error without opening Jira.
func (s Sender) Sync(ctx context.Context, key string) (Report, error) {
	return s.send(ctx, func(st core.State) []core.UnsentWorklog {
		var out []core.UnsentWorklog
		for _, u := range core.Unsent(st) {
			if key == "" || u.Key == key {
				out = append(out, u)
			}
		}
		return out
	})
}

// SendOne is Sync for one stopped entry: the TUI's retry of one row.
func (s Sender) SendOne(ctx context.Context, key, entryID string) (Report, error) {
	return s.send(ctx, func(st core.State) []core.UnsentWorklog { return only(st, key, entryID) })
}

func only(st core.State, key, entryID string) []core.UnsentWorklog {
	for _, u := range core.Unsent(st) {
		if u.Key == key && u.EntryID == entryID {
			return []core.UnsentWorklog{u}
		}
	}
	return nil
}

// send POSTs what scope picks from the state, one worklog at a time.
func (s Sender) send(ctx context.Context, scope func(core.State) []core.UnsentWorklog) (Report, error) {
	rep := Report{Sent: []core.UnsentWorklog{}, Remaining: []core.UnsentWorklog{}}
	w, notices, err := s.Store.Read()
	rep.Notices = notices
	if err != nil {
		return rep, err
	}
	rep.World = w
	err = s.sendAll(ctx, scope, &rep)
	rep.Remaining = append(rep.Remaining, scope(rep.World.State)...)
	return rep, err
}

func (s Sender) sendAll(ctx context.Context, scope func(core.State) []core.UnsentWorklog, rep *Report) error {
	targets := scope(rep.World.State)
	if len(targets) == 0 {
		return nil
	}
	client, err := s.Jira()
	if err != nil {
		if !errors.Is(err, config.ErrNotConfigured) {
			err = fmt.Errorf("%w: %w", config.ErrNotConfigured, err)
		}
		return err
	}
	release, err := s.Store.LockSync()
	if err != nil {
		return err
	}
	defer release()

	var last error
	for _, t := range targets {
		err := s.sendOne(ctx, client, t, rep)
		if err == nil {
			continue
		}
		last = err
		var api *jira.APIError
		if !errors.As(err, &api) || errors.Is(err, jira.ErrUnauthorized) {
			break // 401, unreachable, or a store error: the rest would fail too
		}
	}
	return last
}

// sendOne re-reads t under sync.lock, POSTs it unless it is gone or
// synced, and records the outcome.
func (s Sender) sendOne(ctx context.Context, client jira.Client, t core.UnsentWorklog, rep *Report) error {
	w, notices, err := s.Store.Read()
	rep.Notices = append(rep.Notices, notices...)
	if err != nil {
		return err
	}
	e, ok := w.State.Entry(t.Key, t.EntryID)
	if !ok || e.Worklog == nil || e.Worklog.State == core.WorklogSynced {
		return nil // removed, or someone else sent it: never POST twice
	}
	wl := *e.Worklog
	pctx, cancel := context.WithTimeout(ctx, PostBudget)
	id, perr := client.PostWorklog(pctx, t.Key, jira.Worklog{
		Started: wl.StartedAt.Time, TimeSpentSeconds: wl.TimeSpentSeconds, Comment: wl.Comment,
	})
	cancel()
	switch {
	case perr == nil:
		var res store.Result
		for range markTries {
			res, err = s.Store.Apply(core.MarkSynced{Key: t.Key, EntryID: t.EntryID, WorklogID: id})
			if !errors.Is(err, store.ErrBusy) {
				break
			}
		}
		s.record(rep, res)
		if err != nil {
			return fmt.Errorf("jira took worklog %s for %s but it could not be recorded: %w", id, t.Key, err)
		}
		if e, ok := res.World.State.Entry(t.Key, t.EntryID); ok && e.Worklog != nil {
			rep.Sent = append(rep.Sent, core.UnsentWorklog{Key: t.Key, EntryID: t.EntryID, Worklog: *e.Worklog})
		}
		return nil
	case errors.Is(perr, jira.ErrUnauthorized):
		// A fact about Jira, not about the worklog: it stays pending.
		res, err := s.Store.Apply(core.RecordAuthFailure{})
		s.record(rep, res)
		return errors.Join(perr, err)
	default:
		res, err := s.Store.Apply(core.MarkFailed{Key: t.Key, EntryID: t.EntryID, Message: perr.Error()})
		s.record(rep, res)
		return errors.Join(perr, err)
	}
}

func (s Sender) record(rep *Report, res store.Result) {
	rep.Notices = append(rep.Notices, res.Notices...)
	rep.Events = append(rep.Events, res.Events...)
	if res.World.State.SchemaVersion != 0 {
		rep.World = res.World
	}
}
