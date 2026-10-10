package worklog_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/store"
	"github.com/j-dsmith/punchcard/internal/tracker"
	"github.com/j-dsmith/punchcard/internal/worklog"
)

var zone = time.FixedZone("UTC-5", -5*60*60)

func at(hhmm string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", "2026-08-09 "+hhmm, zone)
	if err != nil {
		panic(err)
	}
	return t
}

// fixture is a store on a settable clock and a Sender over a Jira.
type fixture struct {
	t    *testing.T
	now  time.Time
	st   *store.Store
	jira tracker.Client // nil: unconfigured
	s    worklog.Sender
	ids  int
}

func newFixture(t *testing.T, j tracker.Client) *fixture {
	f := &fixture{t: t, now: at("09:00"), jira: j}
	f.st = store.New(t.TempDir(), func() time.Time { return f.now })
	f.st.LockWait = 50 * time.Millisecond
	f.s = worklog.Sender{Store: f.st, Tracker: tracker.InEffect{Open: func() (tracker.Client, error) {
		if f.jira == nil {
			return nil, config.ErrNotConfigured
		}
		return f.jira, nil
	}}}
	return f
}

// apply runs a setup command at a wall-clock time.
func (f *fixture) apply(hhmm string, c core.Command) {
	f.t.Helper()
	f.now = at(hhmm)
	if _, err := f.st.Apply(c); err != nil {
		f.t.Fatalf("setup %#v: %v", c, err)
	}
}

func (f *fixture) start(hhmm, key string) {
	f.ids++
	f.apply(hhmm, core.Start{Key: key, Origin: core.OriginHand, NewEntryID: "E" + key})
}

func (f *fixture) world() core.World {
	f.t.Helper()
	w, _, err := f.st.Read()
	if err != nil {
		f.t.Fatal(err)
	}
	return w
}

func (f *fixture) worklogOf(key string) core.Worklog {
	f.t.Helper()
	for _, tk := range f.world().State.TrackedTickets {
		if tk.Key == key {
			for _, e := range tk.Entries {
				if e.Worklog != nil {
					return *e.Worklog
				}
			}
		}
	}
	f.t.Fatalf("%s has no worklog", key)
	return core.Worklog{}
}

func ts(hhmm string) core.Timestamp { return core.At(at(hhmm)) }

func kinds(evs []core.Event) []string {
	out := []string{}
	for _, e := range evs {
		out = append(out, string(e.Kind)+" "+e.Key)
	}
	return out
}

func TestStopSendsTheWorklogAndMarksItSynced(t *testing.T) {
	fake := &tracker.Fake{}
	f := newFixture(t, fake)
	f.start("09:00", "AA-1")
	f.now = at("09:30")

	r, err := f.s.Stop(context.Background(), core.Stop{Comment: "did it", Origin: core.OriginHand})
	if err != nil {
		t.Fatal(err)
	}
	if r.SyncErr != nil {
		t.Errorf("SyncErr = %v, want nil", r.SyncErr)
	}
	want := core.Worklog{State: core.WorklogSynced, StartedAt: ts("09:00"), TimeSpentSeconds: 1800, Comment: "did it",
		WorklogID: "W1", SyncedAt: ts("09:30")}
	if d := cmp.Diff(want, r.Worklog); d != "" {
		t.Errorf("result worklog (-want +got)\n%s", d)
	}
	if d := cmp.Diff(want, f.worklogOf("AA-1")); d != "" {
		t.Errorf("worklog on disk (-want +got)\n%s", d)
	}
	if r.Key != "AA-1" || r.EntryID != "EAA-1" {
		t.Errorf("stopped %s/%s", r.Key, r.EntryID)
	}
	posted := []tracker.PostedWorklog{{Key: "AA-1", ID: "W1", Worklog: tracker.Worklog{Started: at("09:00"), TimeSpentSeconds: 1800, Comment: "did it"}}}
	if d := cmp.Diff(posted, fake.Worklogs(), cmp.Comparer(func(a, b time.Time) bool { return a.Equal(b) })); d != "" {
		t.Errorf("posted (-want +got)\n%s", d)
	}
	if d := cmp.Diff([]string{"synced AA-1"}, kinds(r.Events)); d != "" {
		t.Errorf("events (-want +got)\n%s", d)
	}
}

func TestStopWithNoTrackerLogsLocallyAndNeverOpensOne(t *testing.T) {
	f := newFixture(t, nil)
	opened := 0
	f.s.Tracker = tracker.InEffect{None: true, Open: func() (tracker.Client, error) { opened++; return nil, tracker.ErrNoTracker }}
	f.start("09:00", "payroll")
	f.now = at("09:30")

	r, err := f.s.Stop(context.Background(), core.Stop{Origin: core.OriginHand})
	if err != nil {
		t.Fatal(err)
	}
	if r.SyncErr != nil {
		t.Errorf("SyncErr = %v, want nil: a local worklog is not owed", r.SyncErr)
	}
	want := core.Worklog{State: core.WorklogLocal, StartedAt: ts("09:00"), TimeSpentSeconds: 1800}
	if d := cmp.Diff(want, r.Worklog); d != "" {
		t.Errorf("result worklog (-want +got)\n%s", d)
	}
	if d := cmp.Diff(want, f.worklogOf("payroll")); d != "" {
		t.Errorf("worklog on disk (-want +got)\n%s", d)
	}
	if opened != 0 {
		t.Errorf("opened the tracker %d times, want 0", opened)
	}
}

func TestStopThatCannotSendStillStops(t *testing.T) {
	cases := []struct {
		name       string
		jira       tracker.Client
		holdSync   bool
		wantErr    error
		wantState  core.WorklogState
		wantEvents []string
		authFailed bool
		posts      int
	}{
		{"unreachable marks it failed", &tracker.Fake{Err: tracker.ErrUnreachable}, false,
			tracker.ErrUnreachable, core.WorklogFailed, []string{"syncFailed AA-1"}, false, 1},
		{"a 503 marks it failed", &tracker.Fake{Err: &tracker.Refusal{Status: 503}}, false,
			&tracker.Refusal{Status: 503}, core.WorklogFailed, []string{"syncFailed AA-1"}, false, 1},
		{"a 401 leaves it pending and records the auth failure", &tracker.Fake{Err: tracker.ErrUnauthorized}, false,
			tracker.ErrUnauthorized, core.WorklogPending, []string{}, true, 1},
		{"unconfigured leaves it pending and sends nothing", nil, false,
			config.ErrNotConfigured, core.WorklogPending, []string{}, false, 0},
		{"another sender holding sync.lock leaves it pending", &tracker.Fake{}, true,
			store.ErrSyncBusy, core.WorklogPending, []string{}, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t, c.jira)
			f.start("09:00", "AA-1")
			f.now = at("09:30")
			if c.holdSync {
				release, err := f.st.LockSync()
				if err != nil {
					t.Fatal(err)
				}
				defer release()
			}
			r, err := f.s.Stop(context.Background(), core.Stop{Origin: core.OriginHand})
			if err != nil {
				t.Fatalf("the stop itself failed: %v", err)
			}
			if !errors.Is(r.SyncErr, c.wantErr) && !(c.wantErr != nil && errors.As(r.SyncErr, new(*tracker.Refusal))) {
				t.Errorf("SyncErr = %v, want %v", r.SyncErr, c.wantErr)
			}
			wl := f.worklogOf("AA-1")
			if wl.State != c.wantState || wl.TimeSpentSeconds != 1800 {
				t.Errorf("worklog on disk = %+v, want %s with 1800 s", wl, c.wantState)
			}
			if c.wantState == core.WorklogFailed && (wl.Message == "" || !wl.FailedAt.Equal(at("09:30"))) {
				t.Errorf("failed worklog lacks message or failedAt: %+v", wl)
			}
			var refused *tracker.Refusal
			if errors.As(c.wantErr, &refused) && wl.RefusedStatus != refused.Status {
				t.Errorf("refusedStatus = %d, want the tracker's %d", wl.RefusedStatus, refused.Status)
			}
			if got := !f.world().State.AuthFailedAt.IsZero(); got != c.authFailed {
				t.Errorf("authFailedAt set = %t, want %t", got, c.authFailed)
			}
			if d := cmp.Diff(c.wantEvents, kinds(r.Events)); d != "" {
				t.Errorf("events (-want +got)\n%s", d)
			}
			if fk, ok := c.jira.(*tracker.Fake); ok && len(fk.Calls()) != c.posts {
				t.Errorf("calls %v, want %d POST(s)", fk.Calls(), c.posts)
			}
		})
	}
}

func TestStopRefusedSendsNothing(t *testing.T) {
	fake := &tracker.Fake{}
	f := newFixture(t, fake)
	_, err := f.s.Stop(context.Background(), core.Stop{Origin: core.OriginHand})
	if !errors.Is(err, core.ErrNoTimer) {
		t.Errorf("err = %v, want ErrNoTimer", err)
	}
	if len(fake.Calls()) != 0 {
		t.Errorf("calls %v, want none", fake.Calls())
	}
}

// probe is a Jira that checks, during the POST, that sync.lock is held
// and state.lock is not.
type probe struct {
	tracker.Fake
	st           *store.Store
	syncHeld     bool
	stateLockErr error
}

func (p *probe) PostWorklog(ctx context.Context, key string, w tracker.Worklog) (string, error) {
	if _, err := p.st.LockSync(); errors.Is(err, store.ErrSyncBusy) {
		p.syncHeld = true
	}
	_, p.stateLockErr = p.st.Apply(core.Add{Tickets: []core.NewTicket{{Key: "ZZ-9"}}})
	return p.Fake.PostWorklog(ctx, key, w)
}

func TestThePOSTHoldsSyncLockAndNeverStateLock(t *testing.T) {
	f := newFixture(t, nil)
	p := &probe{st: f.st}
	f.jira = p
	f.start("09:00", "AA-1")
	f.now = at("09:30")
	if _, err := f.s.Stop(context.Background(), core.Stop{Origin: core.OriginHand}); err != nil {
		t.Fatal(err)
	}
	if !p.syncHeld {
		t.Error("sync.lock was free during the POST")
	}
	if p.stateLockErr != nil {
		t.Errorf("state.lock was held during the POST: %v", p.stateLockErr)
	}
}

// stopped puts a pending worklog of the given minutes on key.
func (f *fixture) stopped(key string, from, to string) {
	f.start(from, key)
	f.apply(to, core.Stop{Key: key, Origin: core.OriginHand})
}

func TestSyncSendsEveryPendingAndFailedWorklogOneAtATime(t *testing.T) {
	fake := &tracker.Fake{PostErrs: map[string]error{"AA-2": &tracker.Refusal{Status: 503, Message: "down"}}}
	f := newFixture(t, fake)
	f.stopped("AA-1", "08:00", "08:30")
	f.stopped("AA-2", "08:30", "09:00")
	f.stopped("AA-3", "09:00", "09:15")
	f.apply("09:16", core.MarkFailed{Key: "AA-3", EntryID: "EAA-3", Message: "earlier"})
	f.now = at("10:00")

	rep, err := f.s.Sync(context.Background(), "")
	var api *tracker.Refusal
	if !errors.As(err, &api) || api.Status != 503 {
		t.Errorf("err = %v, want the 503", err)
	}
	if d := cmp.Diff([]string{"worklog AA-1", "worklog AA-2", "worklog AA-3"}, fake.Calls()); d != "" {
		t.Errorf("calls (-want +got)\n%s", d)
	}
	var sent, left []string
	for _, u := range rep.Sent {
		sent = append(sent, u.Key+" "+string(u.Worklog.State))
	}
	for _, u := range rep.Remaining {
		left = append(left, u.Key+" "+string(u.Worklog.State)+" "+u.Worklog.Message)
	}
	if d := cmp.Diff([]string{"AA-1 synced", "AA-3 synced"}, sent); d != "" {
		t.Errorf("sent (-want +got)\n%s", d)
	}
	if d := cmp.Diff([]string{"AA-2 failed the tracker answered 503: down"}, left); d != "" {
		t.Errorf("remaining (-want +got)\n%s", d)
	}
	if d := cmp.Diff([]string{"synced AA-1", "syncFailed AA-2", "synced AA-3"}, kinds(rep.Events)); d != "" {
		t.Errorf("events (-want +got)\n%s", d)
	}

	// A second sync never POSTs what is synced.
	fake.PostErrs = nil
	if _, err := f.s.Sync(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff([]string{"worklog AA-1", "worklog AA-2", "worklog AA-3", "worklog AA-2"}, fake.Calls()); d != "" {
		t.Errorf("calls after the second sync (-want +got)\n%s", d)
	}
}

func TestSyncStopsAtA401AndASuccessClearsIt(t *testing.T) {
	fake := &tracker.Fake{Err: tracker.ErrUnauthorized}
	f := newFixture(t, fake)
	f.stopped("AA-1", "08:00", "08:30")
	f.stopped("AA-2", "08:30", "09:00")
	f.now = at("10:00")

	rep, err := f.s.Sync(context.Background(), "")
	if !errors.Is(err, tracker.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if len(fake.Calls()) != 1 || len(rep.Remaining) != 2 || len(rep.Sent) != 0 {
		t.Errorf("calls %v, sent %d, remaining %d: want one POST and both left", fake.Calls(), len(rep.Sent), len(rep.Remaining))
	}
	w := f.world()
	if !w.State.AuthFailedAt.Equal(at("10:00")) {
		t.Errorf("authFailedAt = %v", w.State.AuthFailedAt)
	}
	if f.worklogOf("AA-1").State != core.WorklogPending {
		t.Errorf("a 401 must leave the worklog pending, got %s", f.worklogOf("AA-1").State)
	}

	fake.Err = nil
	f.now = at("10:05")
	if _, err := f.s.Sync(context.Background(), "AA-1"); err != nil {
		t.Fatal(err)
	}
	if !f.world().State.AuthFailedAt.IsZero() {
		t.Error("a successful POST must clear authFailedAt")
	}
	if f.worklogOf("AA-2").State != core.WorklogPending {
		t.Error("sync AA-1 sent AA-2")
	}
}

func TestSyncStopsWhenJiraIsUnreachable(t *testing.T) {
	fake := &tracker.Fake{Err: tracker.ErrUnreachable}
	f := newFixture(t, fake)
	f.stopped("AA-1", "08:00", "08:30")
	f.stopped("AA-2", "08:30", "09:00")
	rep, err := f.s.Sync(context.Background(), "")
	if !errors.Is(err, tracker.ErrUnreachable) || len(fake.Calls()) != 1 || len(rep.Remaining) != 2 {
		t.Errorf("err %v calls %v remaining %d: want one try, then stop", err, fake.Calls(), len(rep.Remaining))
	}
}

func TestSyncWithNothingOwedNeverOpensJira(t *testing.T) {
	f := newFixture(t, nil) // unconfigured
	rep, err := f.s.Sync(context.Background(), "")
	if err != nil || len(rep.Sent) != 0 || len(rep.Remaining) != 0 {
		t.Errorf("rep %+v err %v, want an empty success", rep, err)
	}
	f.stopped("AA-1", "08:00", "08:30")
	if _, err := f.s.Sync(context.Background(), ""); !errors.Is(err, config.ErrNotConfigured) {
		t.Errorf("err = %v, want ErrNotConfigured", err)
	}
}

// unverifiedStopped puts one pending worklog per span on key, a ticket
// added while the tracker could not be reached.
func (f *fixture) unverifiedStopped(key string, spans ...[2]string) {
	f.t.Helper()
	for i, sp := range spans {
		f.apply(sp[0], core.Start{Key: key, Unverified: true, Origin: core.OriginHand, NewEntryID: fmt.Sprintf("E%s-%d", key, i)})
		f.apply(sp[1], core.Stop{Key: key, Origin: core.OriginHand})
	}
}

func (f *fixture) ticket(key string) core.TrackedTicket {
	f.t.Helper()
	for _, tk := range f.world().State.TrackedTickets {
		if tk.Key == key {
			return tk
		}
	}
	f.t.Fatalf("%s is not tracked", key)
	return core.TrackedTicket{}
}

// #42: an accepted worklog confirms an unverified ticket, and the
// sender fetches its summary once per ticket per run.
func TestASyncedWorklogConfirmsAnUnverifiedTicket(t *testing.T) {
	t.Run("with its summary, fetched once for two worklogs", func(t *testing.T) {
		fake := &tracker.Fake{Summaries: map[string]string{"NEW-1": "Fix it"}}
		f := newFixture(t, fake)
		f.unverifiedStopped("NEW-1", [2]string{"08:00", "08:30"}, [2]string{"08:30", "09:00"})
		f.now = at("10:00")
		if _, err := f.s.Sync(context.Background(), ""); err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff([]string{"worklog NEW-1", "summary NEW-1", "worklog NEW-1"}, fake.Calls()); d != "" {
			t.Errorf("calls (-want +got)\n%s", d)
		}
		if tk := f.ticket("NEW-1"); tk.Unverified || tk.Summary != "Fix it" {
			t.Errorf("ticket unverified %v summary %q, want confirmed with Fix it", tk.Unverified, tk.Summary)
		}
	})
	t.Run("a failed summary fetch is silent", func(t *testing.T) {
		fake := &tracker.Fake{} // every Summary is a 404
		f := newFixture(t, fake)
		f.unverifiedStopped("NEW-1", [2]string{"08:00", "08:30"})
		f.now = at("10:00")
		rep, err := f.s.Sync(context.Background(), "")
		if err != nil {
			t.Fatalf("sync reported %v", err)
		}
		if len(rep.Sent) != 1 || len(rep.Remaining) != 0 {
			t.Errorf("sent %v remaining %v", rep.Sent, rep.Remaining)
		}
		if f.worklogOf("NEW-1").State != core.WorklogSynced {
			t.Error("worklog not synced")
		}
		if tk := f.ticket("NEW-1"); tk.Unverified || tk.Summary != "" {
			t.Errorf("ticket unverified %v summary %q, want confirmed with none", tk.Unverified, tk.Summary)
		}
	})
	t.Run("a verified ticket asks for no summary", func(t *testing.T) {
		fake := &tracker.Fake{Summaries: map[string]string{"AA-1": "Known"}}
		f := newFixture(t, fake)
		f.stopped("AA-1", "08:00", "08:30")
		f.now = at("10:00")
		if _, err := f.s.Sync(context.Background(), ""); err != nil {
			t.Fatal(err)
		}
		if d := cmp.Diff([]string{"worklog AA-1"}, fake.Calls()); d != "" {
			t.Errorf("calls (-want +got)\n%s", d)
		}
	})
}
