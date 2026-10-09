package tui

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/jira"
)

// changes drops presence touches: what the model asked the store to change.
func changes(cmds []core.Command) []core.Command {
	out := []core.Command{}
	for _, c := range cmds {
		if _, ok := c.(core.Touch); !ok {
			out = append(out, c)
		}
	}
	return out
}

func selectedKey(hs *harness) string {
	t, _ := hs.m.selected()
	return t.Key
}

func listed(hs *harness) *harness {
	hs.setup("07:00:00", add("AA-1", "BB-2"))
	hs.setup("08:00:00", start("BB-2", "E"), pause)
	return hs
}

func TestAddFetchesTheSummaryLikeStart(t *testing.T) {
	hs := listed(newHarness(t))
	hs.jira = &jira.Fake{Summaries: map[string]string{"PUN-7": "Recents ordering"}}
	hs.open("09:00:00", 120, 32)

	hs.key("a")
	frameHas(t, hs, "Add ticket", "Ticket key", "recent", "BB-2 Summary of BB-2", "AA-1 Summary of AA-1")
	hs.typeText("pun-7")
	hs.key("enter")

	want := []core.Command{core.Add{Tickets: []core.NewTicket{{Key: "PUN-7", Summary: "Recents ordering"}}, Origin: core.OriginHand}}
	if d := cmp.Diff(want, changes(hs.st.commands())); d != "" {
		t.Errorf("changes (-want +got)\n%s", d)
	}
	if d := cmp.Diff([]string{"summary PUN-7"}, hs.jira.Calls()); d != "" {
		t.Errorf("jira calls (-want +got)\n%s", d)
	}
	frameLacks(t, hs, "Add ticket")
	frameHas(t, hs, "added PUN-7")
	if got := selectedKey(hs); got != "PUN-7" {
		t.Errorf("cursor on %q, want PUN-7", got)
	}
}

func TestAddNotFoundChangesNothing(t *testing.T) {
	hs := listed(newHarness(t))
	hs.jira = &jira.Fake{}
	hs.open("09:00:00", 120, 32)

	hs.key("a")
	hs.typeText("PUN-404")
	hs.key("enter")

	if got := changes(hs.st.commands()); len(got) != 0 {
		t.Errorf("changed %v", got)
	}
	frameHas(t, hs, "Add ticket", "PUN-404 not found — or not visible to this")
}

func TestAddWithoutJiraAddsItUnverified(t *testing.T) {
	cases := []struct {
		name string
		jira *jira.Fake
		want []core.Command
	}{
		{"not configured", nil, nil},
		{"unreachable", &jira.Fake{Err: jira.ErrUnreachable}, nil},
		{"token rejected", &jira.Fake{Err: jira.ErrUnauthorized}, []core.Command{core.RecordAuthFailure{}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hs := listed(newHarness(t))
			hs.jira = c.jira
			hs.open("09:00:00", 120, 32)
			hs.key("a")
			hs.typeText("PUN-7")
			hs.key("enter")

			want := append(c.want, core.Add{Tickets: []core.NewTicket{{Key: "PUN-7", Unverified: true}}, Origin: core.OriginHand})
			if d := cmp.Diff(want, changes(hs.st.commands())); d != "" {
				t.Errorf("changes (-want +got)\n%s", d)
			}
			frameHas(t, hs, "added PUN-7 (not verified with Jira yet)")
		})
	}
}

func TestAddPicksARecentTicket(t *testing.T) {
	hs := listed(newHarness(t))
	hs.jira = &jira.Fake{}
	hs.open("09:00:00", 120, 32)

	hs.key("a", "down", "down", "enter") // BB-2, then AA-1

	if got := changes(hs.st.commands()); len(got) != 0 {
		t.Errorf("a ticket already listed changed %v", got)
	}
	if got := hs.jira.Calls(); len(got) != 0 {
		t.Errorf("a confirmed ticket asked Jira: %v", got)
	}
	frameLacks(t, hs, "Add ticket")
	if got := selectedKey(hs); got != "AA-1" {
		t.Errorf("cursor on %q, want AA-1", got)
	}
}

func TestAddFiltersRecentsByWhatIsTyped(t *testing.T) {
	hs := listed(newHarness(t))
	hs.open("09:00:00", 120, 32)
	hs.key("a")
	hs.typeText("aa")

	pane := paneText(t, hs.frame(), 120)
	if !containsLine(pane, "AA-1 Summary of AA-1") || containsLine(pane, "BB-2 Summary of BB-2") {
		t.Errorf("recents for \"aa\" should be AA-1 only:\n%s", hs.frame())
	}
}

func TestAddRefusesWhatIsNotAKey(t *testing.T) {
	hs := listed(newHarness(t))
	hs.jira = &jira.Fake{}
	hs.open("09:00:00", 120, 32)
	hs.key("a")
	hs.typeText("hello")
	hs.key("enter")

	if got := hs.jira.Calls(); len(got) != 0 {
		t.Errorf("asked Jira about a non-key: %v", got)
	}
	frameHas(t, hs, "Add ticket", "HELLO is not a ticket key")
	hs.key("esc")
	frameLacks(t, hs, "Add ticket")
}

func TestStartConfirmsAnUnverifiedTicket(t *testing.T) {
	hs := newHarness(t)
	hs.setup("07:00:00", core.Add{Tickets: []core.NewTicket{{Key: "DATA-9", Unverified: true}}, Origin: core.OriginHand})
	hs.jira = &jira.Fake{Summaries: map[string]string{"DATA-9": "Backfill"}}
	hs.open("09:00:00", 120, 32)

	hs.key("space")

	want := []core.Command{
		core.Start{Key: "DATA-9", Unverified: true, Origin: core.OriginHand, NewEntryID: "ID-1"},
		core.Verify{Key: "DATA-9", Summary: "Backfill"},
	}
	if d := cmp.Diff(want, changes(hs.st.commands())); d != "" {
		t.Errorf("changes (-want +got)\n%s", d)
	}
	frameHas(t, hs, "Backfill")
}
