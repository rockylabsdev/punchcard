package tui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/tracker"
)

// plain renders a frame through a colour-profile writer forced to NoTTY,
// which strips every escape, so a golden file reads as the screen does in
// a no-colour terminal. Refresh with: go test ./internal/tui -update
func plain(frame string) string {
	var buf bytes.Buffer
	w := &colorprofile.Writer{Forward: &buf, Profile: colorprofile.NoTTY}
	if _, err := w.WriteString(frame); err != nil {
		panic(err)
	}
	return buf.String()
}

// scenarios are the named states the goldens pin, each built through the
// store the way the CLI would build it.
// Jira is configured (a tracker.Fake that takes everything) unless the
// build sets hs.unconfigured (Jira without credentials) or hs.noTracker
// (no tracker at all).
var scenarios = []struct {
	name  string
	build func(hs *harness)
	keys  []string // pressed once the TUI is open (see play)
}{
	{"empty", func(hs *harness) {}, nil},
	{"running", runningDay, nil},
	{"paused", pausedDay, nil},
	{"recovered", recoveredDay, nil},
	{"stop", runningDay, []string{"x"}},
	{"stop_recovered", recoveredDay, []string{"x"}},
	{"add", runningDay, []string{"a", "down", "down"}},
	{"remove", runningDay, []string{"d"}},
	// r against a Jira that cannot be reached: once the toast has gone,
	// the banner holds the status line.
	{"sync_failure", func(hs *harness) {
		runningDay(hs)
		hs.jira = &tracker.Fake{Err: tracker.ErrUnreachable}
	}, []string{"r", "wait"}},
	{"not_configured", func(hs *harness) {
		runningDay(hs)
		hs.unconfigured = true
	}, nil},
	{"help", runningDay, []string{"?"}},
	// An agent starts a ticket while the TUI is open: the next tick
	// tags the row and the status line says who.
	{"changed_elsewhere", pausedDay, []string{"agent starts OPS-77"}},
	// No tracker: the empty list says so once, and nothing about Jira
	// shows anywhere.
	{"no_tracker_empty", func(hs *harness) { hs.noTracker = true }, nil},
	// No tracker, one worklog stopped today: it is logged, nothing owed.
	{"no_tracker_logged", localDay, nil},
	// No tracker: the stop dialog logs, it does not send.
	{"no_tracker_stop", localDay, []string{"x"}},
	// No tracker: the add dialog takes a summary after the key, since
	// nothing will look one up.
	{"no_tracker_add", localDay, []string{"a", "type:standup", "tab", "type:Daily standup"}},
}

// localDay is a morning with no tracker: payroll stopped (a local
// worklog) and invoices running.
func localDay(hs *harness) {
	hs.noTracker = true
	hs.setup("07:00:00", core.Add{Origin: core.OriginHand, Tickets: []core.NewTicket{{Key: "payroll"}, {Key: "invoices"}}})
	hs.setup("07:05:00", start("payroll", "E-p"))
	hs.setup("07:45:00", core.Stop{Origin: core.OriginHand, Local: true})
	hs.setup("08:30:00", start("invoices", "E-i"))
}

// outside are changes another process makes, by name, for play.
var outside = map[string]core.Command{
	"agent starts OPS-77": core.Start{Key: "OPS-77", Origin: core.OriginAgent, NewEntryID: "E-77b"},
}

func pausedDay(hs *harness) {
	day(hs)
	hs.setup("08:20:00", start("PUN-142", "E-142b"))
	hs.setup("08:41:30", pause)
}

// configure gives a scenario its Jira: a Fake that takes everything,
// unless the build chose one or chose none.
func (hs *harness) configure() {
	if hs.jira == nil && !hs.unconfigured && !hs.noTracker {
		hs.jira = &tracker.Fake{}
	}
}

// play presses keys; "wait" lets the toast time out instead, "type:TEXT"
// types TEXT, and a name from outside is that change made elsewhere,
// then a tick.
func (hs *harness) play(keys ...string) {
	hs.t.Helper()
	for _, k := range keys {
		if k == "wait" {
			hs.tick(hs.now.Add(toastFor + time.Second).Format("15:04:05"))
			continue
		}
		if text, ok := strings.CutPrefix(k, "type:"); ok {
			hs.typeText(text)
			continue
		}
		if c, ok := outside[k]; ok {
			next := hs.now.Add(time.Second).Format("15:04:05")
			hs.setup(next, c)
			hs.tick(next)
			continue
		}
		hs.key(k)
	}
}

func runningDay(hs *harness) {
	day(hs)
	hs.setup("08:58:00", start("PUN-142", "E-142b"))
}

// recoveredDay ran PUN-142 from 06:00 with a pause, and nobody was
// present after 06:50: at 09:12 the timer is stale and shows recovered,
// capped at 06:50.
func recoveredDay(hs *harness) {
	hs.setup("05:00:00", core.Add{Origin: core.OriginHand, Tickets: []core.NewTicket{
		{Key: "PUN-142", Summary: "Wire flock on the side state.lock"},
		{Key: "PUN-120", Summary: "Spike: Bubble Tea v2 layout"},
		{Key: "DATA-9", Unverified: true},
	}})
	hs.setup("05:05:00", start("PUN-120", "E-120"))
	hs.setup("05:45:00", core.Stop{Origin: core.OriginHand})
	hs.setup("05:46:00", core.MarkSynced{Key: "PUN-120", EntryID: "E-120", WorklogID: "W1"})
	hs.setup("06:00:00", start("PUN-142", "E-142"))
	hs.setup("06:15:00", pause)
	hs.setup("06:20:00", start("PUN-142", "E-142"))
	hs.setup("06:50:00", core.Touch{Origin: core.OriginHand})
}

// day is a morning's list: two stopped worklogs (one failed, one
// pending), a paused entry, an idle ticket, an unverified one and a key
// longer than the column.
func day(hs *harness) {
	hs.setup("07:00:00", core.Add{Origin: core.OriginHand, Tickets: []core.NewTicket{
		{Key: "PUN-142", Summary: "Wire flock on the side state.lock"},
		{Key: "PUN-138", Summary: "Stop dialog: duration parsing and the live preview"},
		{Key: "OPS-77", Summary: "Rotate staging certs"},
		{Key: "PUN-120", Summary: "Spike: Bubble Tea v2 layout"},
		{Key: "DATA-9", Unverified: true},
		{Key: "PLATFORM-4410", Summary: "Investigate intermittent CI timeouts"},
	}})
	hs.setup("07:05:00", start("PUN-120", "E-120"))
	hs.setup("07:45:00", core.Stop{Origin: core.OriginHand})
	hs.setup("07:45:01", core.MarkFailed{Key: "PUN-120", EntryID: "E-120", Message: "503"})
	hs.setup("07:46:00", start("OPS-77", "E-77"))
	hs.setup("08:01:00", core.Stop{Origin: core.OriginHand})
	hs.setup("08:02:00", start("PUN-138", "E-138"))
	hs.setup("08:14:00", start("PUN-142", "E-142"))
	hs.setup("08:15:00", pause)
}

func TestFrames(t *testing.T) {
	sizes := []struct{ w, h int }{{80, 24}, {120, 32}}
	for _, sc := range scenarios {
		for _, sz := range sizes {
			t.Run(fmt.Sprintf("%s_%dx%d", sc.name, sz.w, sz.h), func(t *testing.T) {
				hs := newHarness(t)
				sc.build(hs)
				hs.configure()
				hs.open("09:00:00", sz.w, sz.h)
				hs.tick("09:12:34")
				hs.play(sc.keys...)
				golden.RequireEqual(t, plain(hs.m.View().Content))
			})
		}
	}
}

// The running row ticks: a tick carries now and the frame shows it.
func TestRunningRowTicks(t *testing.T) {
	hs := newHarness(t)
	hs.setup("08:00:00", add("AA-1"), start("AA-1", "E"))
	hs.open("09:00:00", 80, 24)

	hs.tick("09:00:05")
	if !bytes.Contains([]byte(plain(hs.m.View().Content)), []byte("1:00:05")) {
		t.Errorf("frame at 09:00:05 does not show 1:00:05:\n%s", plain(hs.m.View().Content))
	}
	hs.now = at("09:30:00") // the clock moves but no tick arrives
	if !bytes.Contains([]byte(plain(hs.m.View().Content)), []byte("1:00:05")) {
		t.Errorf("the frame moved without a tick")
	}
}
