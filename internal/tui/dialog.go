package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/j-dsmith/punchcard/internal/core"
)

// dialog is a modal that takes over [2]: frame() lays its pane over the
// Ticket pane with the compositor, at exactly that pane's size, so
// opening one never resizes anything. Dialogs are values, like Model:
// key returns the Model with m.dlg set to the dialog's next value, or to
// nil to close it. A dialog never touches the store or Jira itself; it
// enqueues an intent (or asks m.fetch) as the list keys do.
type dialog interface {
	title() string
	// body is the pane's lines at inner width w.
	body(m Model, w int) []string
	// bindings replace the key bar while the dialog is open.
	bindings() []binding
	key(m Model, k tea.KeyPressMsg) (Model, tea.Cmd)
	// tick follows the clock (a live preview); most return themselves.
	tick(m Model) dialog
}

// newInput is a one-line text field with a steady cursor (no blink
// timer, so frames are deterministic).
func newInput(placeholder string) textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = placeholder
	st := in.Styles()
	st.Cursor.Blink = false
	in.SetStyles(st)
	return in
}

// field is a labelled input: the focused one's label is bold.
func field(label string, in textinput.Model, focused bool, w int) []string {
	l := sDim.Render(label)
	if focused {
		l = sBold.Render(label)
	}
	in.SetWidth(max(1, w-3))
	return []string{l, "  " + in.View()}
}

// stopDialog is x: review the duration, add a comment, enter stops the
// entry and sends its worklog; esc leaves the timer alone. It is pinned
// to the entry it opened on, so core refuses if that entry was stopped
// meanwhile.
type stopDialog struct {
	ticket, entryID string
	summary         string
	recovered       bool // recovery capped it: the user must type a duration
	edited          bool // the duration was typed; it stops following the clock
	dur, comment    textinput.Model
	onComment       bool
	err             string
}

func (m Model) openStop() Model {
	t, ok := m.selected()
	if !ok {
		return m
	}
	e, open := openEntry(t)
	if !open {
		m.status = "nothing to stop on " + t.Key + " — space starts it"
		return m
	}
	d := stopDialog{ticket: t.Key, entryID: e.ID, summary: t.Summary, recovered: e.Recovered,
		dur: newInput("e.g. 1h 20m"), comment: newInput("optional")}
	d.dur.Focus()
	d = d.follow(m)
	m.dlg = d
	return m
}

// measured is the open entry's elapsed as shown now.
func (d stopDialog) measured(m Model) (core.TimeEntry, time.Duration) {
	t, _ := m.tracked(d.ticket)
	for _, e := range t.Entries {
		if e.ID == d.entryID {
			return e, core.Elapsed(e, m.now)
		}
	}
	return core.TimeEntry{}, 0
}

// follow prefills the duration with the measured time, truncated to the
// minute, until the user types one. A recovered entry is never
// prefilled: its cap is only a guess.
func (d stopDialog) follow(m Model) stopDialog {
	if d.edited || d.recovered {
		return d
	}
	_, el := d.measured(m)
	v := ""
	if el >= time.Minute {
		v = hm(el.Truncate(time.Minute))
	}
	if v != d.dur.Value() {
		d.dur.SetValue(v)
		d.dur.CursorEnd()
	}
	return d
}

func (d stopDialog) tick(m Model) dialog { return d.follow(m) }

func (d stopDialog) title() string { return "Stop " + d.ticket }

func (d stopDialog) bindings() []binding {
	return []binding{{"enter", "stop & send"}, {"tab", "field"}, {"esc", "cancel — keep timer"}}
}

func (d stopDialog) key(m Model, k tea.KeyPressMsg) (Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.dlg = nil
		return m, nil
	case "tab", "shift+tab", "up", "down":
		d.onComment = !d.onComment
		if d.onComment {
			d.dur.Blur()
			d.comment.Focus()
		} else {
			d.comment.Blur()
			d.dur.Focus()
		}
		m.dlg = d
		return m, nil
	case "enter":
		dur, ok := parseDuration(d.dur.Value())
		if !ok {
			d.err = "duration? try 1h 20m, 80m or 1:20 (at least 1m)"
			m.dlg = d
			return m, nil
		}
		m.dlg = nil
		m.status = "sending " + hm(dur) + " for " + d.ticket + " to Jira…"
		return m.enqueue(intent{kind: intentStop, stop: core.Stop{
			Key: d.ticket, EntryID: d.entryID, Duration: dur,
			Comment: strings.TrimSpace(d.comment.Value()), Origin: core.OriginHand,
		}})
	}
	var cmd tea.Cmd
	if d.onComment {
		d.comment, cmd = d.comment.Update(k)
	} else {
		before := d.dur.Value()
		d.dur, cmd = d.dur.Update(k)
		d.edited = d.edited || d.dur.Value() != before
	}
	d.err = ""
	m.dlg = d
	return m, cmd
}

func (d stopDialog) body(m Model, w int) []string {
	e, el := d.measured(m)
	n := len(e.Segments)
	lines := []string{fit(sBold.Render(d.ticket)+" "+sDim.Render(d.summary), w)}
	lines = append(lines, wrap(sDim.Render(fmt.Sprintf("measured %s across %d %s", clock(el), n, plural(n, "stretch", "stretches"))), w)...)
	if d.recovered {
		capped := ""
		if n > 0 {
			capped = " at " + e.Segments[n-1].EndedAt.In(m.now.Location()).Format("15:04")
		}
		lines = append(lines, wrap(sRecov.Render(gRecovered+" recovered — capped"+capped+", your last presence; how long did you work?"), w)...)
	}
	lines = append(lines, "")
	lines = append(lines, field("Duration", d.dur, !d.onComment, w)...)
	lines = append(lines, field("Comment", d.comment, d.onComment, w)...)
	lines = append(lines, "")
	dur, ok := parseDuration(d.dur.Value())
	switch {
	case ok:
		lines = append(lines, wrap("→ sends "+sSync.Bold(true).Render(hm(dur))+" to Jira as one worklog", w)...)
	case d.recovered && strings.TrimSpace(d.dur.Value()) == "":
		lines = append(lines, wrap(sRecov.Render("type the time you actually worked"), w)...)
	case strings.TrimSpace(d.dur.Value()) == "":
		lines = append(lines, wrap(sFail.Render(gFailed+" under a minute: type a duration"), w)...)
	default:
		lines = append(lines, wrap(sFail.Render(gFailed+" can't read that duration"), w)...)
	}
	if d.err != "" {
		lines = append(lines, wrap(sFail.Render(d.err), w)...)
	}
	return lines
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

var (
	hmPattern    = regexp.MustCompile(`^(?:([0-9]+)h)?(?:([0-9]+)m)?$`)
	clockPattern = regexp.MustCompile(`^([0-9]+):([0-5][0-9])$`)
)

// parseDuration reads what the stop dialog's field holds: 1h 20m, 1h20m,
// 1h, 80m, 80 (minutes) or 1:20, from 1m to 24h. Spaces are ignored.
func parseDuration(s string) (time.Duration, bool) {
	s = strings.ToLower(strings.ReplaceAll(s, " ", ""))
	var h, mins int
	if n, err := strconv.Atoi(s); err == nil {
		mins = n
	} else if p := clockPattern.FindStringSubmatch(s); p != nil {
		h, _ = strconv.Atoi(p[1])
		mins, _ = strconv.Atoi(p[2])
	} else if p := hmPattern.FindStringSubmatch(s); p != nil && s != "" {
		h, _ = strconv.Atoi(p[1])
		mins, _ = strconv.Atoi(p[2])
	} else {
		return 0, false
	}
	d := time.Duration(h)*time.Hour + time.Duration(mins)*time.Minute
	if d < time.Minute || d > 24*time.Hour {
		return 0, false
	}
	return d, true
}

// removeDialog is d: y removes the ticket, any other key keeps it. When
// the ticket holds time not yet in Jira it says how much would be lost,
// and only then does the remove force past core's refusal.
type removeDialog struct {
	ticket, summary string
	unsent          time.Duration // open entry plus unsent worklogs, when opened
	owes            bool
}

func (m Model) openRemove() Model {
	t, ok := m.selected()
	if !ok {
		return m
	}
	d := removeDialog{ticket: t.Key, summary: summaryText(t)}
	for _, e := range t.Entries {
		switch {
		case e.Worklog == nil:
			d.unsent += core.Elapsed(e, m.now)
			d.owes = true
		case e.Worklog.State != core.WorklogSynced:
			d.unsent += time.Duration(e.Worklog.TimeSpentSeconds) * time.Second
			d.owes = true
		}
	}
	m.dlg = d
	return m
}

func (d removeDialog) title() string { return "Remove " + d.ticket + "?" }

func (d removeDialog) bindings() []binding {
	return []binding{{"y", "remove"}, {"any key", "keep it"}}
}

func (d removeDialog) tick(Model) dialog { return d }

func (d removeDialog) key(m Model, k tea.KeyPressMsg) (Model, tea.Cmd) {
	m.dlg = nil
	if k.String() != "y" {
		return m, nil
	}
	return m.enqueue(intent{kind: intentRemove, key: d.ticket, force: d.owes, say: "removed " + d.ticket})
}

func (d removeDialog) body(m Model, w int) []string {
	lines := []string{fit(sBold.Render(d.ticket)+" "+sDim.Render(d.summary), w), ""}
	if d.owes {
		lines = append(lines, wrap(sFail.Render(gFailed+" its unsent "+hm(d.unsent)+" would be lost: it never reaches Jira"), w)...)
		lines = append(lines, "")
	}
	lines = append(lines, wrap(sBold.Render("y")+sDim.Render(" removes it from your list · any other key keeps it"), w)...)
	return lines
}
