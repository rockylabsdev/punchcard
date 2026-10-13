package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/j-dsmith/punchcard/internal/core"
)

// awayDialog is -: cut time the user was away out of the selected open
// entry. From is when they left (a pause at that time), To when they came
// back (a resume at that time); To empty leaves the entry paused from
// From. On a paused entry From starts at the pause, so editing it moves
// the pause. Both take core.ParseAt's forms. The last line previews the
// cut, or says why core would refuse it: the preview is the same Apply.
type awayDialog struct {
	ticket, entryID string
	summary         string
	since           string // "running since 08:58" or "paused at 10:30"
	// pausedAt is a paused entry's pause, and prefill From's first value
	// (its HH:MM): From left as it was means the pause itself.
	pausedAt core.Timestamp
	prefill  string
	from, to textinput.Model
	onTo     bool
}

func (m Model) openAway() Model {
	t, ok := m.selected()
	if !ok {
		return m
	}
	e, open := openEntry(t)
	if !open {
		m = m.notify("nothing to cut on " + t.Key + " — space starts it")
		return m
	}
	hhmm := func(ts core.Timestamp) string { return ts.In(m.now.Location()).Format("15:04") }
	last := e.Segments[len(e.Segments)-1]
	d := awayDialog{ticket: t.Key, entryID: e.ID, summary: t.Summary,
		from: newInput("10:00 or -45m"), to: newInput("now")}
	if last.Open() {
		d.since = "running since " + hhmm(last.StartedAt)
	} else {
		d.since = "paused at " + hhmm(last.EndedAt)
		d.pausedAt, d.prefill = last.EndedAt, hhmm(last.EndedAt)
		d.from.SetValue(d.prefill)
		d.from.CursorEnd()
	}
	d.from.Focus()
	m.dlg = d
	return m
}

func (d awayDialog) title() string     { return "Away from " + d.ticket }
func (d awayDialog) tick(Model) dialog { return d }

func (d awayDialog) bindings() []binding {
	return []binding{{"enter", "cut"}, {"tab", "field"}, {"esc", "cancel"}}
}

// commands are the changes the fields ask for: a pause at From, then,
// if To is set, a start at To. An error says what cannot be read.
func (d awayDialog) commands(m Model) ([]core.Command, error) {
	var cmds []core.Command
	if d.prefill == "" || strings.TrimSpace(d.from.Value()) != d.prefill {
		from, err := core.ParseAt(d.from.Value(), m.now)
		if err != nil {
			return nil, fmt.Errorf("from? try 10:00 or -45m")
		}
		cmds = append(cmds, core.Pause{Key: d.ticket, At: from, Origin: core.OriginHand})
	}
	if strings.TrimSpace(d.to.Value()) == "" {
		return cmds, nil
	}
	to, err := core.ParseAt(d.to.Value(), m.now)
	if err != nil {
		return nil, fmt.Errorf("to? try 10:45 or -5m, or leave it empty for now")
	}
	t, _ := m.tracked(d.ticket)
	return append(cmds, core.Start{Key: d.ticket, Summary: t.Summary, Unverified: t.Unverified,
		Origin: core.OriginHand, At: to, Grammar: m.grammar}), nil
}

// preview applies the commands to what is shown, as the store would, and
// returns the entry's elapsed before and after.
func (d awayDialog) preview(m Model) (before, after time.Duration, err error) {
	cmds, err := d.commands(m)
	if err != nil {
		return 0, 0, err
	}
	w := m.shown()
	before = entryElapsed(w, d.ticket, d.entryID, m.now)
	for _, c := range cmds {
		if w, _, err = core.Apply(w, c, m.now); err != nil {
			return 0, 0, err
		}
	}
	return before, entryElapsed(w, d.ticket, d.entryID, m.now), nil
}

func entryElapsed(w core.World, key, id string, now time.Time) time.Duration {
	for _, t := range w.State.TrackedTickets {
		if t.Key != key {
			continue
		}
		for _, e := range t.Entries {
			if e.ID == id {
				return core.Elapsed(e, now)
			}
		}
	}
	return 0
}

func (d awayDialog) key(m Model, k tea.KeyPressMsg) (Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.dlg = nil
		return m, nil
	case "tab", "shift+tab", "up", "down":
		d.onTo = !d.onTo
		if d.onTo {
			d.from.Blur()
			d.to.Focus()
		} else {
			d.to.Blur()
			d.from.Focus()
		}
		m.dlg = d
		return m, nil
	case "enter":
		if _, _, err := d.preview(m); err != nil {
			m.dlg = d // the preview line already says why
			return m, nil
		}
		cmds, _ := d.commands(m)
		m.dlg = nil // nothing asked (a pause left where it was): just close
		var batch []tea.Cmd
		for i, c := range cmds {
			if s, ok := c.(core.Start); ok {
				s.NewEntryID = m.newID() // unused: the entry is open
				c = s
			}
			say := ""
			if i == len(cmds)-1 {
				say = "cut time away from " + d.ticket
			}
			var cmd tea.Cmd
			m, cmd = m.enqueue(intent{kind: intentCommand, cmd: c, say: say})
			batch = append(batch, cmd)
		}
		return m, tea.Batch(batch...)
	}
	var cmd tea.Cmd
	if d.onTo {
		d.to, cmd = d.to.Update(k)
	} else {
		d.from, cmd = d.from.Update(k)
	}
	m.dlg = d
	return m, cmd
}

func (d awayDialog) body(m Model, w int) []string {
	lines := []string{fit(sBold.Render(d.ticket)+" "+sDim.Render(d.summary), w)}
	lines = append(lines, wrap(sDim.Render(d.since), w)...)
	lines = append(lines, "")
	lines = append(lines, field("From", d.from, !d.onTo, w)...)
	lines = append(lines, field("To", d.to, d.onTo, w)...)
	lines = append(lines, "")
	if strings.TrimSpace(d.from.Value()) == "" {
		return append(lines, wrap(sDim.Render("when did you leave? 10:00 or -45m"), w)...)
	}
	before, after, err := d.preview(m)
	if err != nil {
		msg := strings.TrimPrefix(err.Error(), core.ErrUsage.Error()+": ")
		return append(lines, wrap(sFail.Render(gFailed+" "+msg), w)...)
	}
	cmds, _ := d.commands(m)
	if len(cmds) == 0 {
		return append(lines, wrap(sDim.Render("change From to move the pause, or set To to resume"), w)...)
	}
	verb, diff := "cuts", before-after
	if diff < 0 {
		verb, diff = "adds", -diff
	}
	becomes := fmt.Sprintf("%d:%02d", int(after.Hours()), int(after.Minutes())%60)
	return append(lines, wrap("→ "+verb+" "+sPaused.Bold(true).Render(hm(diff))+" · entry becomes "+becomes, w)...)
}
