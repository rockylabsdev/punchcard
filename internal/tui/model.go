package tui

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/store"
)

// Store is the seam to the data directory: *store.Store in the binary, a
// fake in tests. The model never saves a World; it asks for a transition
// and shows the World the store hands back.
type Store interface {
	Read() (core.World, []string, error)
	Apply(core.Command) (store.Result, error)
	TryApply(core.Command) (store.Result, error)
}

// Model is the whole TUI. Update is pure apart from the commands it
// returns: it never reads the clock (now arrives on tickMsg) and never
// touches the store itself (requests run as tea.Cmds and come back as
// resultMsg).
type Model struct {
	store Store
	newID func() string

	w, h  int
	now   time.Time
	world core.World // the last World the store handed back

	// sel is the cursor's row; selKey is the ticket under it, so the
	// cursor follows a ticket when the recents order changes.
	sel    int
	selKey string

	// Store requests run one at a time. A key's intent is queued and
	// resolved into a core.Command only when it is sent, against the
	// newest World, so two quick presses never race or act on a stale
	// view.
	queue    []intent
	inflight bool
	// lastPresence is when a presence-bearing request was last sent;
	// plain keys (movement) send presence at most once per throttle.
	lastPresence time.Time

	// status is the status line's slot: the last refusal or notice.
	// The banner, toasts and last event replace this in a later slice.
	status string
}

// tickMsg is the 1 s clock. It is the only way time reaches Update.
type tickMsg time.Time

// resultMsg is a store request coming back.
type resultMsg struct {
	world    core.World
	now      time.Time // the instant the store applied it; zero for a read
	notices  []string
	err      error
	presence bool // a presence-only touch: ErrBusy is expected and silent
}

// intentKind is what a key asked the store for.
type intentKind int

const (
	intentToggle   intentKind = iota // start, pause or resume key
	intentPause                      // pause whatever runs
	intentPresence                   // a keystroke that changes nothing
)

type intent struct {
	kind intentKind
	key  string
}

// New is the model before the first window size and the first read
// (which Init starts). now seeds the clock until the first tick.
func New(s Store, now time.Time, newID func() string) Model {
	return Model{store: s, newID: newID, now: now, inflight: true}
}

// Init loads the store and starts the clock.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// load is the lock-free read the model starts with.
func (m Model) load() tea.Cmd {
	s := m.store
	return func() tea.Msg {
		w, notices, err := s.Read()
		return resultMsg{world: w, notices: notices, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		// Later: stat state.json here and re-read on change.
		m.now = time.Time(msg)
		return m, tick()
	case resultMsg:
		m.inflight = false
		m = m.accept(msg)
		return m.pump()
	case tea.KeyPressMsg:
		return m.key(msg.String())
	}
	return m, nil
}

func (m Model) key(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit // timers keep running: they are on disk
	case "space", "enter":
		if t, ok := m.selected(); ok {
			return m.enqueue(intent{kind: intentToggle, key: t.Key})
		}
	case "p":
		return m.enqueue(intent{kind: intentPause})
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
	// Any other keystroke the TUI acts on is still presence.
	if m.inflight || len(m.queue) > 0 {
		return m, nil // what is queued records presence itself
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
		c := m.command(i)
		if c == nil {
			continue
		}
		m.inflight = true
		m.lastPresence = m.now
		s, presence := m.store, i.kind == intentPresence
		return m, func() tea.Msg {
			apply := s.Apply
			if presence {
				apply = s.TryApply
			}
			res, err := apply(c)
			return resultMsg{world: res.World, now: res.Now, notices: res.Notices, err: err, presence: presence}
		}
	}
	return m, nil
}

// command resolves an intent against what the user sees now. Every TUI
// change is by hand.
func (m Model) command(i intent) core.Command {
	switch i.kind {
	case intentPause:
		return core.Pause{Origin: core.OriginHand}
	case intentPresence:
		return core.Touch{Origin: core.OriginHand}
	}
	shown := m.shown()
	for _, t := range shown.State.TrackedTickets {
		if t.Key != i.key {
			continue
		}
		if r, _, ok := core.Running(shown.State); ok && r.Key == t.Key {
			return core.Pause{Origin: core.OriginHand}
		}
		return core.Start{Key: t.Key, Summary: t.Summary, Unverified: t.Unverified, Origin: core.OriginHand, NewEntryID: m.newID()}
	}
	return nil // the ticket left the list meanwhile
}

// accept takes in what the store handed back.
func (m Model) accept(r resultMsg) Model {
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
		m.status = describe(r.err)
	case len(r.notices) > 0:
		m.status = r.notices[len(r.notices)-1]
	case !r.presence:
		m.status = ""
	}
	return m.follow()
}

// describe is a refusal in the status line's words.
func describe(err error) string {
	switch {
	case errors.Is(err, core.ErrNoTimer):
		return "nothing is running"
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
		v.SetContent(m.frame())
	}
	return v
}
