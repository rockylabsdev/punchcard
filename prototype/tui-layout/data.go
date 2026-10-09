// PROTOTYPE — throwaway. In-memory fake of the Punchcard domain, just enough
// to drive the three layout variants. Nothing here is the real core.
package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type syncState int

const (
	syncNone syncState = iota
	syncPending
	syncSynced
	syncFailed
)

type segment struct {
	start, end time.Time // zero end = the open (running) segment
}

type entry struct {
	segs      []segment
	recovered bool
	cappedAt  time.Time
	stopped   bool
	sendDur   time.Duration // truncated, possibly edited, at stop
	comment   string
	sync      syncState
	syncErr   string
	stoppedAt time.Time
	sentAt    time.Time
}

func (e *entry) elapsed(now time.Time) time.Duration {
	var d time.Duration
	for _, s := range e.segs {
		end := s.end
		if end.IsZero() {
			end = now
		}
		d += end.Sub(s.start)
	}
	return d
}

func (e *entry) open() bool {
	return len(e.segs) > 0 && e.segs[len(e.segs)-1].end.IsZero()
}

type ticket struct {
	key, summary string
	unverified   bool
	entries      []*entry
	activity     time.Time
	changedAt    time.Time // set when a change arrives from elsewhere
	changedBy    string    // origin of that change
	changedWhat  string
}

type tstate int

const (
	stIdle tstate = iota
	stRunning
	stPaused
)

// current = the entry not yet stopped, if any
func (t *ticket) current() *entry {
	for _, e := range t.entries {
		if !e.stopped {
			return e
		}
	}
	return nil
}

func (t *ticket) state() tstate {
	c := t.current()
	switch {
	case c == nil:
		return stIdle
	case c.open():
		return stRunning
	default:
		return stPaused
	}
}

func (t *ticket) total(now time.Time) time.Duration {
	var d time.Duration
	for _, e := range t.entries {
		d += e.elapsed(now)
	}
	return d
}

func (t *ticket) recovered() bool {
	c := t.current()
	return c != nil && c.recovered
}

func (t *ticket) counts() (pending, synced, failed int) {
	for _, e := range t.entries {
		switch e.sync {
		case syncPending:
			pending++
		case syncSynced:
			synced++
		case syncFailed:
			failed++
		}
	}
	return
}

type event struct {
	at     time.Time
	origin string // hand | prompt | agent
	text   string
}

type store struct {
	tickets []*ticket
	events  []event
	recents []recent
}

type recent struct{ key, summary string }

func (s *store) sorted() []*ticket {
	out := append([]*ticket(nil), s.tickets...)
	// recents order: most recent activity first (stable insertion sort, tiny n)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].activity.After(out[j-1].activity); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func (s *store) running() *ticket {
	for _, t := range s.tickets {
		if t.state() == stRunning {
			return t
		}
	}
	return nil
}

func (s *store) find(key string) *ticket {
	for _, t := range s.tickets {
		if t.key == key {
			return t
		}
	}
	return nil
}

func (s *store) log(now time.Time, origin, format string, a ...any) {
	s.events = append(s.events, event{now, origin, fmt.Sprintf(format, a...)})
}

func (s *store) pauseRunning(now time.Time) *ticket {
	r := s.running()
	if r != nil {
		c := r.current()
		c.segs[len(c.segs)-1].end = now
	}
	return r
}

// start or resume t; pauses whatever else runs (start B pauses A)
func (s *store) start(now time.Time, t *ticket) (paused *ticket) {
	if t.state() == stRunning {
		return nil
	}
	paused = s.pauseRunning(now)
	c := t.current()
	if c == nil {
		c = &entry{}
		t.entries = append(t.entries, c)
	}
	c.segs = append(c.segs, segment{start: now})
	t.activity = now
	return paused
}

func (s *store) stop(now time.Time, t *ticket, d time.Duration, comment string) {
	c := t.current()
	if c == nil {
		return
	}
	if c.open() {
		c.segs[len(c.segs)-1].end = now
	}
	c.stopped, c.sendDur, c.comment, c.sync, c.stoppedAt = true, d, comment, syncPending, now
	c.recovered = false // the marker clears only at stop
	t.activity = now
}

func (s *store) remove(t *ticket) {
	for i, x := range s.tickets {
		if x == t {
			s.tickets = append(s.tickets[:i], s.tickets[i+1:]...)
			s.recents = append([]recent{{t.key, t.summary}}, s.recents...)
			return
		}
	}
}

func (s *store) worklogs() (out []struct {
	t *ticket
	e *entry
}) {
	for _, t := range s.sorted() {
		for _, e := range t.entries {
			if e.stopped {
				out = append(out, struct {
					t *ticket
					e *entry
				}{t, e})
			}
		}
	}
	return
}

func (s *store) owed() (pending, failed int) {
	for _, t := range s.tickets {
		p, _, f := t.counts()
		pending += p
		failed += f
	}
	return
}

func (s *store) today(now time.Time) time.Duration {
	var d time.Duration
	for _, t := range s.tickets {
		d += t.total(now)
	}
	return d
}

// ---- seeds ---------------------------------------------------------------

func ago(now time.Time, m float64) time.Time {
	return now.Add(-time.Duration(m * float64(time.Minute)))
}

func seed(now time.Time, scenario int) *store {
	s := &store{}
	if scenario == scEmpty {
		s.recents = []recent{{"PUN-142", "Wire flock on the side state.lock"}, {"OPS-77", "Rotate staging certs"}}
		return s
	}
	pun142 := &ticket{key: "PUN-142", summary: "Wire flock on the side state.lock", activity: ago(now, 47)}
	pun142.entries = []*entry{{segs: []segment{{ago(now, 140), ago(now, 115)}, {ago(now, 47), time.Time{}}}}}

	pun138 := &ticket{key: "PUN-138", summary: "Stop dialog: duration parsing and truncation", activity: ago(now, 95)}
	pun138.entries = []*entry{{segs: []segment{{ago(now, 260), ago(now, 188)}}}}

	ops77 := &ticket{key: "OPS-77", summary: "Rotate staging certs", activity: ago(now, 200)}
	ops77.entries = []*entry{
		{segs: []segment{{ago(now, 330), ago(now, 300)}}, stopped: true, sendDur: 30 * time.Minute, sync: syncSynced, comment: "renewed *.staging", stoppedAt: ago(now, 300), sentAt: ago(now, 299)},
		{segs: []segment{{ago(now, 215), ago(now, 200)}}, stopped: true, sendDur: 15 * time.Minute, sync: syncPending, stoppedAt: ago(now, 200)},
	}
	pun120 := &ticket{key: "PUN-120", summary: "Spike: Bubble Tea v2 layout at 80×24", activity: ago(now, 400)}
	pun120.entries = []*entry{{segs: []segment{{ago(now, 480), ago(now, 400)}}, stopped: true, sendDur: 80 * time.Minute, sync: syncFailed, syncErr: "Jira 503 Service Unavailable", stoppedAt: ago(now, 400)}}

	data9 := &ticket{key: "DATA-9", unverified: true, activity: ago(now, 500)}
	pun151 := &ticket{key: "PUN-151", summary: "Recents ordering in ls", activity: ago(now, 600)}
	eng := &ticket{key: "ENG-4410", summary: "Investigate intermittent 409s from the worklog endpoint when two sessions stop within the same second", activity: ago(now, 700)}

	s.tickets = []*ticket{pun142, pun138, ops77, pun120, data9, pun151, eng}
	s.recents = []recent{{"OPS-80", "Bump base image"}, {"PUN-99", "README for setup"}, {"ENG-4398", "Flaky e2e on CI"}}

	if scenario == scRecovered {
		// PUN-138 was running overnight; first presence capped it at lastSeen.
		pun138.entries[0] = &entry{segs: []segment{{ago(now, 400), ago(now, 330)}, {ago(now, 300), ago(now, 240)}}, recovered: true, cappedAt: ago(now, 240)}
		pun138.activity = ago(now, 30)
	}
	return s
}

// ---- formatting ------------------------------------------------------------

func clock(d time.Duration) string {
	d = d.Truncate(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	sec := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
}

func hm(d time.Duration) string {
	d = d.Truncate(time.Minute)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh %02dm", h, m)
	}
}

// parseDur accepts "1h 12m", "72m", "1:12", "1.5h", "90".
func parseDur(s string) (time.Duration, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, false
	}
	if h, m, ok := strings.Cut(s, ":"); ok {
		hi, e1 := strconv.Atoi(h)
		mi, e2 := strconv.Atoi(m)
		if e1 != nil || e2 != nil {
			return 0, false
		}
		return time.Duration(hi)*time.Hour + time.Duration(mi)*time.Minute, true
	}
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Minute, true
	}
	if d, err := time.ParseDuration(strings.ReplaceAll(s, " ", "")); err == nil {
		return d.Truncate(time.Minute), true
	}
	if f, err := strconv.ParseFloat(strings.TrimSuffix(s, "h"), 64); err == nil && strings.HasSuffix(s, "h") {
		return time.Duration(math.Round(f*60)) * time.Minute, true
	}
	return 0, false
}
