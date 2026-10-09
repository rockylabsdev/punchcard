package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/j-dsmith/punchcard/internal/core"
)

// Changes made elsewhere (another terminal, the mod, an agent) reach the
// TUI by polling, not watching (ADR 0002): each tick stats state.json
// and, only when its stamp moved, re-reads it and diffs the World it
// showed against the new one. A changed row carries a tag for tagFor,
// and the status line logs the change with who made it.
//
// The check is a read: an untouched TUI is never presence and never
// writes. The TUI's own changes come back as the World the store
// returned, so the re-read that follows them finds nothing to tag.

// tagFor is how long a row says who changed it.
const tagFor = 6 * time.Second

// tag is a row's "paused by agent", from at.
type tag struct {
	text string
	at   time.Time
}

// change is one row's transition found by the diff.
type change struct {
	key    string
	what   string // started, resumed, paused, stopped
	origin core.Origin
}

// watch is the tick's stat check, sent through the same gate as every
// store request so it never overlaps one; a busy tick skips it (the
// next tick looks again). Before the first read there is nothing to
// compare against, so it waits for that.
func (m Model) watch() (Model, tea.Cmd) {
	if m.inflight || len(m.queue) > 0 {
		return m, nil
	}
	m.inflight = true
	s, seen := m.store, m.seen
	return m, func() tea.Msg {
		if st, err := s.Stamp(); err != nil || st == seen {
			return resultMsg{unchanged: true} // a failed stat: look again next tick
		}
		return read(s)
	}
}

// read stats then reads the store. The stamp is taken first, so a write
// landing between the two only costs one more read.
func read(s Store) resultMsg {
	st, serr := s.Stamp()
	w, notices, err := s.Read()
	return resultMsg{world: w, notices: notices, err: err, read: true, stamp: st, stamped: serr == nil}
}

// elsewhere takes in a re-read: tags what changed since the World shown
// and logs it. The first read has nothing to compare against.
func (m Model) elsewhere(r resultMsg) Model {
	if r.stamped && r.err == nil {
		m.seen = r.stamp
	}
	if m.world.State.SchemaVersion == 0 || r.world.State.SchemaVersion == 0 {
		return m
	}
	next, _ := core.Recover(r.world, m.now)
	cs := diff(m.shown().State, next.State)
	if len(cs) == 0 {
		return m
	}
	tags := map[string]tag{}
	for k, t := range m.tags {
		if m.now.Sub(t.at) < tagFor {
			tags[k] = t
		}
	}
	texts := make([]string, len(cs))
	for i, c := range cs {
		tags[c.key] = tag{text: c.what + " by " + string(c.origin), at: m.now}
		texts[i] = c.what + " " + c.key
	}
	m.tags = tags
	text := strings.Join(texts, " · ")
	m = m.happened(m.now, cs[0].origin, text)
	return m.notify(who(cs[0].origin) + " " + text) // a toast has no origin column
}

// who is an origin as the status line words it.
func who(o core.Origin) string {
	if o == core.OriginHand {
		return "you"
	}
	return string(o)
}

// tagged is the row's tag while it is young, else "".
func (m Model) tagged(key string) string {
	if t, ok := m.tags[key]; ok && m.now.Sub(t.at) < tagFor {
		return t.text
	}
	return ""
}

// diff finds the rows whose timer moved between two Worlds, starts
// first (they are what the user most needs to see), then pauses and
// stops. Who comes from the entry: its origin names who made its last
// change. Adds, removes, verifications and syncs are not tagged.
func diff(old, cur core.State) []change {
	var starts, rest []change
	for _, t := range cur.TrackedTickets {
		var was core.TrackedTicket
		for _, o := range old.TrackedTickets {
			if o.Key == t.Key {
				was = o
			}
		}
		for _, e := range t.Entries {
			if oe, ok := was.Entry(e.ID); ok && e.Stopped() && !oe.Stopped() {
				rest = append(rest, change{t.Key, "stopped", e.Origin})
			}
		}
		e, ok := openEntry(t)
		if !ok {
			continue
		}
		oe, had := was.Entry(e.ID)
		had = had && !oe.Stopped()
		switch {
		case e.Running() && !had:
			starts = append(starts, change{t.Key, "started", e.Origin})
		case e.Running() && !oe.Running():
			starts = append(starts, change{t.Key, "resumed", e.Origin})
		case e.Paused() && had && oe.Running():
			rest = append(rest, change{t.Key, "paused", e.Origin})
		}
	}
	return append(starts, rest...)
}
