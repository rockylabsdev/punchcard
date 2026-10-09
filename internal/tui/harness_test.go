package tui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/jira"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/j-dsmith/punchcard/internal/tracker"
	"github.com/j-dsmith/punchcard/internal/worklog"
)

// fakeStore is the store seam in memory: the same core.Apply, no files.
// It records every command it was asked to apply.
type fakeStore struct {
	mu   sync.Mutex
	w    core.World
	now  func() time.Time
	cmds []core.Command
	busy bool // Apply and TryApply refuse with store.ErrBusy
	// writes counts the changes applied: the fake's Stamp.
	writes int
	nreads int
}

func (f *fakeStore) Read() (core.World, []string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nreads++
	return f.w, nil, nil
}

// Stamp moves on every applied change, as state.json's would.
func (f *fakeStore) Stamp() (store.Stamp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return store.Stamp{Inode: uint64(f.writes)}, nil
}

// reads returns how many times the store was read, then forgets it.
func (f *fakeStore) reads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := f.nreads
	f.nreads = 0
	return n
}

func (f *fakeStore) Apply(c core.Command) (store.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, c)
	if f.busy {
		return store.Result{}, store.ErrBusy
	}
	now := f.now()
	out, events, err := core.Apply(f.w, c, now)
	if err != nil {
		return store.Result{World: f.w, Now: now}, err
	}
	f.w = out
	f.writes++
	return store.Result{World: out, Events: events, Now: now}, nil
}

func (f *fakeStore) TryApply(c core.Command) (store.Result, error) { return f.Apply(c) }

// commands returns what the model asked for, then forgets it.
func (f *fakeStore) commands() []core.Command {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.cmds
	f.cmds = nil
	return c
}

// anyGrammar compares commands without their key grammar (the fake
// store's core.Apply checks keys with it; a test names the grammar only
// when the grammar is its subject).
var anyGrammar = cmpopts.IgnoreTypes(tracker.Grammar{})

var zone = time.FixedZone("UTC-5", -5*3600)

// at is a wall-clock time on the test day, in UTC-5.
func at(hhmmss string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", "2026-08-09 "+hhmmss, zone)
	if err != nil {
		panic(err)
	}
	return t
}

// harness drives a Model the way the Bubble Tea loop would: it calls
// Update, runs the returned command and feeds its message back, with a
// fixed clock that only the test moves.
type harness struct {
	t      *testing.T
	st     *fakeStore
	sender *fakeSender
	jira   *tracker.Fake // nil: Jira not configured (set before open)
	// noTracker: none is configured (set before open); the key grammar
	// is then the slug.
	noTracker bool
	// unconfigured: a scenario that wants no Jira (see configure)
	unconfigured bool
	// openErr: opening the Jira client fails so (a broken config file)
	openErr error
	m       Model
	now     time.Time
	ids     int
}

func newHarness(t *testing.T) *harness {
	hs := &harness{t: t, now: at("09:00:00")}
	hs.st = &fakeStore{w: core.World{State: core.NewState()}, now: func() time.Time { return hs.now }}
	hs.sender = &fakeSender{st: hs.st}
	return hs
}

// fakeSender is the worklog seam over the fake store: it applies the
// stop, then marks the worklog synced, or leaves it owed with syncErr.
type fakeSender struct {
	st      *fakeStore
	syncErr error
	jira    *tracker.Fake // Sync's Jira; nil is not configured (open sets it from hs.jira)
	// noTracker: none is configured, as worklog.Sender{NoTracker}: a
	// stop is local and nothing is sent (open sets it from hs.noTracker).
	noTracker bool
	mu        sync.Mutex
	stops     []core.Stop
	syncs     int
}

func (f *fakeSender) Stop(_ context.Context, c core.Stop) (worklog.StopResult, error) {
	f.mu.Lock()
	f.stops = append(f.stops, c)
	f.mu.Unlock()
	c.Local = f.noTracker
	res, err := f.st.Apply(c)
	if err != nil {
		return worklog.StopResult{}, err
	}
	r := worklog.StopResult{Key: c.Key, EntryID: c.EntryID, World: res.World, Now: res.Now, SyncErr: f.syncErr}
	if f.syncErr == nil && !f.noTracker {
		res, err = f.st.Apply(core.MarkSynced{Key: c.Key, EntryID: c.EntryID, WorklogID: "W1"})
		if err != nil {
			return r, err
		}
		r.World = res.World
	}
	for _, t := range r.World.State.TrackedTickets {
		for _, e := range t.Entries {
			if t.Key == c.Key && e.ID == c.EntryID && e.Worklog != nil {
				r.Worklog = *e.Worklog
			}
		}
	}
	return r, nil
}

// Sync sends every owed worklog to jira the way worklog.Sender does:
// unconfigured is config.ErrNotConfigured, a 401 records the failure
// and stops, unreachable marks failed and stops, an API error marks
// failed and carries on.
func (f *fakeSender) Sync(ctx context.Context, key string) (worklog.Report, error) {
	f.mu.Lock()
	f.syncs++
	f.mu.Unlock()
	w, _, _ := f.st.Read()
	rep := worklog.Report{World: w}
	todo := core.Unsent(w.State)
	if len(todo) == 0 {
		return rep, nil
	}
	switch {
	case f.noTracker:
		rep.Remaining = todo
		return rep, fmt.Errorf("%w: %w", config.ErrNotConfigured, tracker.ErrNoTracker)
	case f.jira == nil:
		rep.Remaining = todo
		return rep, fmt.Errorf("%w: no config", config.ErrNotConfigured)
	}
	var last error
	for _, u := range todo {
		id, err := f.jira.PostWorklog(ctx, u.Key, tracker.Worklog{TimeSpentSeconds: u.Worklog.TimeSpentSeconds})
		var c core.Command = core.MarkSynced{Key: u.Key, EntryID: u.EntryID, WorklogID: id}
		var api *tracker.Refusal
		switch {
		case errors.Is(err, tracker.ErrUnauthorized):
			c = core.RecordAuthFailure{}
		case errors.As(err, &api):
			c = core.MarkFailed{Key: u.Key, EntryID: u.EntryID, Message: err.Error(), Status: api.Status}
		case err != nil:
			c = core.MarkFailed{Key: u.Key, EntryID: u.EntryID, Message: err.Error()}
		}
		res, aerr := f.st.Apply(c)
		if aerr != nil {
			return rep, aerr
		}
		rep.World = res.World
		if err == nil {
			rep.Sent = append(rep.Sent, u)
			continue
		}
		last = err
		if !errors.As(err, &api) {
			break
		}
	}
	rep.Remaining = core.Unsent(rep.World.State)
	return rep, last
}

func (f *fakeSender) calls() []core.Stop {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]core.Stop(nil), f.stops...)
}

// frame is the screen as plain text.
func (hs *harness) frame() string { return plain(hs.m.View().Content) }

// typeText types s one key at a time.
func (hs *harness) typeText(s string) {
	hs.t.Helper()
	for _, r := range s {
		hs.key(string(r))
	}
}

// setup applies commands to the store directly, as another process
// would, at the given time.
func (hs *harness) setup(when string, cmds ...core.Command) {
	hs.t.Helper()
	hs.now = at(when)
	for _, c := range cmds {
		if _, err := hs.st.Apply(c); err != nil {
			hs.t.Fatalf("setup %T: %v", c, err)
		}
	}
	hs.st.commands()
}

// open builds the model at time when, sizes it and loads the store, as
// Run does at startup.
func (hs *harness) open(when string, w, h int) {
	hs.t.Helper()
	hs.now = at(when)
	o := Options{
		Store:    hs.st,
		Now:      func() time.Time { return hs.now },
		NewID:    func() string { hs.ids++; return fmt.Sprintf("ID-%d", hs.ids) },
		Tracking: Tracking{Sender: hs.sender, Grammar: jira.Grammar, NoTracker: hs.noTracker},
	}
	if hs.noTracker {
		// As the binary wires it: the seam answers ErrNoTracker.
		o.Grammar = tracker.Slug
		o.Tracker = func() (tracker.Client, error) { return nil, tracker.ErrNoTracker }
	}
	if hs.jira != nil {
		o.Tracker = func() (tracker.Client, error) { return hs.jira, nil }
	}
	if hs.openErr != nil {
		o.Tracker = func() (tracker.Client, error) { return nil, hs.openErr }
	}
	hs.sender.jira, hs.sender.noTracker = hs.jira, hs.noTracker
	hs.m = New(o)
	hs.m.ticker = func() tea.Cmd { return nil } // the test moves the clock
	hs.send(tea.WindowSizeMsg{Width: w, Height: h})
	hs.run(hs.m.startup())
}

// send delivers one message and everything it causes. The next tick is
// never scheduled (open stubs the ticker): only the test moves time.
func (hs *harness) send(msg tea.Msg) {
	hs.t.Helper()
	next, cmd := hs.m.Update(msg)
	hs.m = next.(Model)
	hs.run(cmd)
}

func (hs *harness) run(cmd tea.Cmd) {
	hs.t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			hs.run(c)
		}
		return
	}
	if msg != nil {
		hs.send(msg)
	}
}

// tick moves the clock to when and delivers the tick, as Bubble Tea
// does each second.
func (hs *harness) tick(when string) {
	hs.t.Helper()
	hs.now = at(when)
	hs.send(tickMsg(hs.now))
}

func (hs *harness) key(keys ...string) {
	hs.t.Helper()
	for _, k := range keys {
		hs.send(keyPress(k))
	}
}

// keyPress builds the message Bubble Tea sends for a named key.
func keyPress(k string) tea.KeyPressMsg {
	switch k {
	case "space":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "})
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "up":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
	case "down":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	case "ctrl+c":
		return tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
	case "esc":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	case "tab":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab})
	case "shift+tab":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift})
	case "backspace":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace})
	}
	r := []rune(k)
	if len(r) != 1 {
		panic("unknown key " + k)
	}
	return tea.KeyPressMsg(tea.Key{Code: r[0], Text: k})
}

// running is the key of the running ticket on disk, or "".
func (hs *harness) running() string {
	w, _, _ := hs.st.Read()
	t, _, ok := core.Running(w.State)
	if !ok {
		return ""
	}
	return t.Key
}

func add(keys ...string) core.Command {
	var ts []core.NewTicket
	for _, k := range keys {
		ts = append(ts, core.NewTicket{Key: k, Summary: "Summary of " + k})
	}
	return core.Add{Tickets: ts, Origin: core.OriginHand}
}

func start(key, id string) core.Command {
	return core.Start{Key: key, Origin: core.OriginHand, NewEntryID: id}
}

var pause = core.Pause{Origin: core.OriginHand}
