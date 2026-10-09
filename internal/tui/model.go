package tui

import (
	"context"
	"errors"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/j-dsmith/punchcard/internal/config"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/jira"
	"github.com/j-dsmith/punchcard/internal/store"
)

// Store is the seam to the data directory: *store.Store in the binary, a
// fake in tests. The model never saves a World; it asks for a transition
// and shows the World the store hands back.
type Store interface {
	Read() (core.World, []string, error)
	Apply(core.Command) (store.Result, error)
	TryApply(core.Command) (store.Result, error)
	// Stamp stats state.json; it moves when another process (or the
	// TUI) replaced the file. It never locks, reads or writes.
	Stamp() (store.Stamp, error)
}

// Model is the whole TUI. Update is pure apart from the commands it
// returns: it never reads the clock (now arrives on tickMsg), never
// touches the store itself (requests run as tea.Cmds and come back as
// resultMsg) and never waits on Jira (lookups come back as fetchMsg).
type Model struct {
	store  Store
	newID  func() string
	sender Sender                      // nil: stops leave the worklog pending
	jira   func() (jira.Client, error) // nil: Jira is not configured

	w, h  int
	now   time.Time
	world core.World // the last World the store handed back

	// sel is the cursor's row; selKey is the ticket under it, so the
	// cursor follows a ticket when the recents order changes. pendingSel
	// is a ticket the cursor should land on once it appears (an add).
	sel        int
	selKey     string
	pendingSel string

	// dlg is the open dialog, laid over [2]; nil is the list. While one
	// is open it gets every key but ctrl+c.
	dlg dialog

	// Store requests run one at a time. A key's intent is queued and
	// resolved into a core.Command only when it is sent, against the
	// newest World, so two quick presses never race or act on a stale
	// view.
	queue    []intent
	inflight bool
	// lastPresence is when a presence-bearing request was last sent;
	// plain keys (movement) send presence at most once per throttle.
	lastPresence time.Time

	// The status line (status.go): a toast, else the banner, else the
	// last event. syncing is true while r's sync is out.
	toast   toast
	last    event
	syncing bool
	// link is whether Jira is configured, as the last probe found.
	link link
	// profile is the terminal's colour profile, for the grey fallback.
	profile colorprofile.Profile

	// Changes made elsewhere (elsewhere.go): seen is state.json's stamp
	// at the last read; tags are rows' "paused by agent" markers.
	seen store.Stamp
	tags map[string]tag
	// ticker schedules the next tick (tests stub it: they move time).
	ticker func() tea.Cmd
	// setup builds the setup command S runs; execProcess suspends the
	// TUI to run it (tea.ExecProcess; tests stub it).
	setup       func() *exec.Cmd
	execProcess func(*exec.Cmd, tea.ExecCallback) tea.Cmd
}

// tickMsg is the 1 s clock. It is the only way time reaches Update.
type tickMsg time.Time

// resultMsg is a store request coming back.
type resultMsg struct {
	world    core.World
	now      time.Time // the instant the store applied it; zero for a read
	notices  []string
	err      error
	presence bool   // a presence-only touch: ErrBusy is expected and silent
	say      string // the status line on success
	// A read (startup, reload, the tick's check): stamp is state.json's
	// stamp taken just before it, if stamped. unchanged: the check found
	// the same stamp and read nothing.
	read, stamped, unchanged bool
	stamp                    store.Stamp
}

// intentKind is what a key asked the store for.
type intentKind int

const (
	intentToggle      intentKind = iota // start, pause or resume key
	intentPause                         // pause whatever runs
	intentPresence                      // a keystroke that changes nothing
	intentStop                          // stop an entry and send its worklog
	intentAdd                           // put ticket on the list
	intentRemove                        // take key off the list
	intentVerify                        // Jira confirmed key: record summary
	intentAuthFailure                   // Jira answered 401
	intentReload                        // re-read the store (after a sync)
	intentClearAuth                     // Jira answered with these credentials
)

type intent struct {
	kind    intentKind
	key     string
	stop    core.Stop      // intentStop
	ticket  core.NewTicket // intentAdd
	summary string         // intentVerify
	force   bool           // intentRemove: the user was warned of unsent time
	say     string         // the status line once it lands
}

// New is the model before the first window size and the first read
// (which Init starts). o.Now is read once, to seed the clock until the
// first tick.
func New(o Options) Model {
	return Model{store: o.Store, newID: o.NewID, sender: o.Sender, jira: o.Jira, now: o.Now(), inflight: true, ticker: tick,
		setup: o.Setup, execProcess: tea.ExecProcess}
}

// Init loads the store, asks whether Jira is set up and starts the
// clock.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.startup(), m.ticker())
}

// startup is Init without the clock: the first read and the probe.
func (m Model) startup() tea.Cmd {
	return tea.Batch(m.load(), m.probe())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// load is the lock-free read the model starts with.
func (m Model) load() tea.Cmd {
	s := m.store
	return func() tea.Msg { return read(s) }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.now = time.Time(msg)
		if m.dlg != nil {
			m.dlg = m.dlg.tick(m)
		}
		m, watch := m.watch()
		return m, tea.Batch(m.ticker(), watch)
	case resultMsg:
		m.inflight = false
		if msg.unchanged {
			return m.pump()
		}
		m = m.accept(msg)
		return m.pump()
	case fetchMsg:
		return m.fetched(msg)
	case syncMsg:
		return m.synced(msg)
	case linkMsg:
		m.link = link{known: true, err: msg.err}
		return m, nil
	case setupDoneMsg:
		return m.setupDone(msg)
	case tea.ColorProfileMsg:
		m.profile = msg.Profile
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.dlg != nil {
			m, cmd := m.dlg.key(m, msg)
			m, presence := m.presence()
			return m, tea.Batch(cmd, presence)
		}
		return m.key(msg.String())
	}
	return m, nil
}

func (m Model) key(k string) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch k {
	case "q":
		return m, tea.Quit // timers keep running: they are on disk
	case "space", "enter":
		if t, ok := m.selected(); ok {
			m, cmd = m.enqueue(intent{kind: intentToggle, key: t.Key})
			if t.Unverified && m.stateOf(t) != core.TimerRunning {
				// Confirm on contact: the start goes ahead now; Jira's
				// answer, if any, lands as its own change.
				cmd = tea.Batch(cmd, m.fetch(t.Key, true, false))
			}
			return m, cmd
		}
	case "p":
		return m.enqueue(intent{kind: intentPause})
	case "x":
		m = m.openStop()
	case "a":
		m, cmd = m.openAdd()
	case "d":
		m = m.openRemove()
	case "r":
		m, cmd = m.sync()
	case "S":
		m, cmd = m.runSetup()
	case "?":
		m = m.openHelp()
	case "j", "down":
		m = m.moveTo(m.sel + 1)
	case "k", "up":
		m = m.moveTo(m.sel - 1)
	case "g", "home":
		m = m.moveTo(0)
	case "G", "end":
		m = m.moveTo(len(m.rows()) - 1)
	default:
		return m, nil
	}
	m, presence := m.presence()
	return m, tea.Batch(cmd, presence)
}

// presence records that a keystroke the TUI acted on happened: at most
// once per throttle, and only when nothing queued records it already.
func (m Model) presence() (Model, tea.Cmd) {
	if m.inflight || len(m.queue) > 0 {
		return m, nil
	}
	if !m.lastPresence.IsZero() && m.now.Sub(m.lastPresence) < core.PresenceThrottle {
		return m, nil
	}
	return m.enqueue(intent{kind: intentPresence})
}

func (m Model) enqueue(i intent) (Model, tea.Cmd) {
	m.queue = append(m.queue, i)
	return m.pump()
}

// pump sends the next queued intent if nothing is in flight.
func (m Model) pump() (Model, tea.Cmd) {
	for !m.inflight && len(m.queue) > 0 {
		i := m.queue[0]
		m.queue = m.queue[1:]
		if i.kind == intentReload {
			m.inflight = true
			return m, m.load()
		}
		c := m.command(i)
		if c == nil {
			continue
		}
		m.inflight = true
		m.lastPresence = m.now
		if stop, ok := c.(core.Stop); ok && m.sender != nil {
			return m, sendStop(m.sender, stop)
		}
		s, presence, say := m.store, i.kind == intentPresence, i.say
		if say == "" {
			say = m.saying(c)
		}
		return m, func() tea.Msg {
			apply := s.Apply
			if presence {
				apply = s.TryApply
			}
			res, err := apply(c)
			return resultMsg{world: res.World, now: res.Now, notices: res.Notices, err: err, presence: presence, say: say}
		}
	}
	return m, nil
}

// sendStop stops the entry and sends its worklog through the worklog
// seam. It may wait on Jira, so it only ever runs as a tea.Cmd.
func sendStop(s Sender, c core.Stop) tea.Cmd {
	return func() tea.Msg {
		r, err := s.Stop(context.Background(), c)
		if err != nil {
			return resultMsg{notices: r.Notices, err: err}
		}
		d := hm(time.Duration(r.Worklog.TimeSpentSeconds) * time.Second)
		say := "stopped " + r.Key + " · logged " + d
		if r.SyncErr != nil {
			say = "stopped " + r.Key + " · " + d + " not sent: " + sendProblem(r.SyncErr) + " · nothing is lost"
		}
		return resultMsg{world: r.World, now: r.Now, notices: r.Notices, say: say}
	}
}

// sendProblem says why a stopped worklog is still owed.
func sendProblem(err error) string {
	var api *jira.APIError
	switch {
	case errors.Is(err, config.ErrNotConfigured):
		return "Jira is not set up"
	case errors.Is(err, jira.ErrUnauthorized):
		return "Jira rejected the token"
	case errors.Is(err, jira.ErrUnreachable):
		return "Jira not reachable"
	case errors.As(err, &api):
		return "Jira answered " + api.Error()
	case errors.Is(err, store.ErrSyncBusy):
		return "another punchcard is sending"
	}
	return err.Error()
}

// command resolves an intent against what the user sees now. Every TUI
// change is by hand. Nil: there is nothing left to do.
func (m Model) command(i intent) core.Command {
	switch i.kind {
	case intentPause:
		return core.Pause{Origin: core.OriginHand}
	case intentPresence:
		return core.Touch{Origin: core.OriginHand}
	case intentStop:
		return i.stop
	case intentAdd:
		return core.Add{Tickets: []core.NewTicket{i.ticket}, Origin: core.OriginHand}
	case intentRemove:
		return core.Remove{Key: i.key, Force: i.force, Origin: core.OriginHand}
	case intentAuthFailure:
		return core.RecordAuthFailure{}
	case intentClearAuth:
		if m.world.State.AuthFailedAt.IsZero() {
			return nil
		}
		return core.ClearAuthFailure{}
	}
	shown := m.shown()
	for _, t := range shown.State.TrackedTickets {
		if t.Key != i.key {
			continue
		}
		if i.kind == intentVerify {
			if !t.Unverified {
				return nil // confirmed meanwhile
			}
			return core.Verify{Key: t.Key, Summary: i.summary}
		}
		if r, _, ok := core.Running(shown.State); ok && r.Key == t.Key {
			return core.Pause{Origin: core.OriginHand}
		}
		return core.Start{Key: t.Key, Summary: t.Summary, Unverified: t.Unverified, Origin: core.OriginHand, NewEntryID: m.newID()}
	}
	return nil // the ticket left the list meanwhile
}

// saying is the last event a start or pause makes, in its own words.
func (m Model) saying(c core.Command) string {
	switch c := c.(type) {
	case core.Start:
		return "started " + c.Key
	case core.Pause:
		if t, _, ok := core.Running(m.shown().State); ok {
			return "paused " + t.Key
		}
	}
	return ""
}

// accept takes in what the store handed back.
func (m Model) accept(r resultMsg) Model {
	if r.read {
		m = m.elsewhere(r) // before the World moves: it diffs against it
	}
	// A World with no schema version means the read itself failed
	// (busy lock, newer schema): keep showing the last good one.
	if r.world.State.SchemaVersion != 0 {
		m.world = r.world
	}
	if r.now.After(m.now) {
		m.now = r.now
	}
	switch {
	case r.err != nil && !(r.presence && errors.Is(r.err, store.ErrBusy)):
		m = m.notify(describe(r.err))
	case r.err != nil:
	case r.say != "":
		m = m.happened(r.now, core.OriginHand, r.say)
	}
	if len(r.notices) > 0 {
		m = m.notify(r.notices[len(r.notices)-1]) // a notice outranks the outcome's toast
	}
	if m.pendingSel != "" {
		for i, t := range m.rows() {
			if t.Key == m.pendingSel {
				m.sel, m.selKey, m.pendingSel = i, t.Key, ""
				return m
			}
		}
	}
	return m.follow()
}

// describe is a refusal in the status line's words.
func describe(err error) string {
	switch {
	case errors.Is(err, core.ErrTooShort):
		return "under a minute: nothing to send"
	case errors.Is(err, core.ErrNoTimer):
		return "nothing is running"
	case errors.Is(err, core.ErrUnlogged):
		return "it has time not yet in Jira; nothing removed"
	case errors.Is(err, store.ErrBusy):
		return "another punchcard holds the state file; try again"
	case errors.Is(err, store.ErrNewerSchema):
		return "state.json was written by a newer punchcard; upgrade to change anything"
	}
	return err.Error()
}

// shown is the World as displayed: a stale timer appears capped, as the
// next change will record it.
func (m Model) shown() core.World {
	w, _ := core.Recover(m.world, m.now)
	return w
}

// rows is the Tickets pane's order: recents, running first.
func (m Model) rows() []core.TrackedTicket {
	return core.ListOrder(m.shown().State, m.now)
}

// tracked is the ticket key as shown, if it is on the list.
func (m Model) tracked(key string) (core.TrackedTicket, bool) {
	for _, t := range m.shown().State.TrackedTickets {
		if t.Key == key {
			return t, true
		}
	}
	return core.TrackedTicket{}, false
}

func (m Model) selected() (core.TrackedTicket, bool) {
	rows := m.rows()
	if m.sel < 0 || m.sel >= len(rows) {
		return core.TrackedTicket{}, false
	}
	return rows[m.sel], true
}

func (m Model) moveTo(i int) Model {
	rows := m.rows()
	m.sel = max(0, min(i, len(rows)-1))
	m.selKey = ""
	if len(rows) > 0 {
		m.selKey = rows[m.sel].Key
	}
	return m
}

// follow keeps the cursor on its ticket after the list changed.
func (m Model) follow() Model {
	for i, t := range m.rows() {
		if t.Key == m.selKey {
			m.sel = i
			return m
		}
	}
	return m.moveTo(m.sel)
}

func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.WindowTitle = "punchcard"
	if m.w > 0 && m.h > 0 {
		f := m.frame()
		if m.profile == colorprofile.ANSI {
			f = greyFallback(f) // the renderer would map greys to white or black
		}
		v.SetContent(f)
	}
	return v
}
