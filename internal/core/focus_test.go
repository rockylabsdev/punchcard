package core_test

import (
	"errors"
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
)

// focus is a prompt's keyed touch; it may need an entry id if it moves.
func focus(key string) core.Touch {
	return core.Touch{Key: key, Origin: prompt, NewEntryID: "F-" + key}
}

// ADR 0009: focus moves a timer only when it changes.
func TestFocusMovesATimerOnlyWhenItChanges(t *testing.T) {
	t.Run("the #41 repro: an agent's start survives the next prompt", func(t *testing.T) {
		w := steps(t, world("09:00", ticket("PUN-1")),
			focus("PUN-1"),
			core.Start{Key: "OPS-77", Origin: agent, NewEntryID: "E1"},
			focus("PUN-1"))
		if got := runningKey(w); got != "OPS-77" {
			t.Errorf("running = %q, want OPS-77", got)
		}
		if got := w.State.LastFocus; got != "PUN-1" {
			t.Errorf("last focus = %q, want PUN-1", got)
		}
	})

	t.Run("a change of focus moves it, and so does changing back", func(t *testing.T) {
		w := lastFocus(world("09:00", ticket("OPS-77", entry("EO", agent, seg("09:00", "")))), "PUN-1")
		w = steps(t, w, focus("ABC-9"))
		if got := runningKey(w); got != "ABC-9" {
			t.Fatalf("after ABC-9: running = %q", got)
		}
		w = steps(t, w, focus("PUN-1"))
		if got := runningKey(w); got != "PUN-1" {
			t.Errorf("after PUN-1: running = %q", got)
		}
	})

	t.Run("a touch while held records the last focus", func(t *testing.T) {
		w := steps(t, world("09:00", ticket("PUN-1")),
			core.Start{Key: "OPS-77", Origin: hand, NewEntryID: "E1"},
			focus("PUN-1"))
		if got := runningKey(w); got != "OPS-77" {
			t.Fatalf("held timer moved to %q", got)
		}
		if got := w.State.LastFocus; got != "PUN-1" {
			t.Fatalf("last focus = %q, want PUN-1", got)
		}
		// Later un-held on the same ticket: the same focus does not move it.
		w = steps(t, w,
			core.Pause{Origin: hand},
			core.Start{Key: "OPS-77", Origin: agent},
			focus("PUN-1"))
		if got := runningKey(w); got != "OPS-77" {
			t.Errorf("un-held timer moved to %q by an unchanged focus", got)
		}
	})

	t.Run("a keyless touch is not a change", func(t *testing.T) {
		w := steps(t, world("09:00", ticket("PUN-1")),
			focus("PUN-1"),
			core.Start{Key: "OPS-77", Origin: agent, NewEntryID: "E1"},
			core.Touch{Origin: prompt},
			focus("PUN-1"))
		if got := runningKey(w); got != "OPS-77" {
			t.Errorf("running = %q, want OPS-77", got)
		}
	})

	t.Run("an empty last focus differs from any key", func(t *testing.T) {
		w := steps(t, world("09:00", ticket("OPS-77", entry("EO", agent, seg("09:00", "")))), focus("PUN-1"))
		if got := runningKey(w); got != "PUN-1" {
			t.Errorf("running = %q, want PUN-1", got)
		}
	})

	t.Run("focus still never starts a timer from idle", func(t *testing.T) {
		w := steps(t, lastFocus(world("09:00", ticket("PUN-1")), "ABC-9"), focus("PUN-1"))
		if got := runningKey(w); got != "" {
			t.Errorf("running = %q, want nothing", got)
		}
	})
}

func TestOnlyAKeyedPresenceTouchWritesTheLastFocus(t *testing.T) {
	base := func() core.World {
		return lastFocus(world("09:00", ticket("AA-1", entry("EA", agent, seg("09:00", ""))), ticket("BB-2")), "PUN-1")
	}
	for _, c := range []struct {
		name string
		cmd  core.Command
	}{
		{"start", core.Start{Key: "BB-2", Origin: hand, NewEntryID: "EB"}},
		{"pause", core.Pause{Origin: hand}},
		{"stop", core.Stop{Origin: hand}},
		{"an agent's touch", core.Touch{Key: "BB-2", Origin: agent, NewEntryID: "EB"}},
		{"a keyless touch", core.Touch{Origin: prompt}},
		{"resume", core.Resume{Origin: hand}},
		{"remove", core.Remove{Key: "BB-2", Origin: hand}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := steps(t, base(), c.cmd).State.LastFocus; got != "PUN-1" {
				t.Errorf("last focus = %q, want PUN-1 unchanged", got)
			}
		})
	}
	t.Run("recovery", func(t *testing.T) {
		w := lastFocus(world("06:00", ticket("AA-1", entry("EA", agent, seg("06:00", "")))), "PUN-1")
		got, events, err := core.Apply(w, core.Touch{Origin: agent}, at("09:30").Time)
		if err != nil || len(events) != 1 || events[0].Kind != core.EventRecovered {
			t.Fatalf("want a recovery: err %v events %v", err, events)
		}
		if got.State.LastFocus != "PUN-1" {
			t.Errorf("last focus = %q, want PUN-1 unchanged", got.State.LastFocus)
		}
	})
	t.Run("a prompt with an invalid key is refused and writes nothing", func(t *testing.T) {
		if _, _, err := core.Apply(base(), core.Touch{Key: "pay roll", Origin: prompt}, at("09:30").Time); !errors.Is(err, core.ErrUsage) {
			t.Errorf("err = %v, want ErrUsage", err)
		}
	})
	t.Run("a keyed touch by hand", func(t *testing.T) {
		w := steps(t, base(), core.Touch{Key: "BB-2", Origin: hand, NewEntryID: "EB"})
		if got := w.State.LastFocus; got != "BB-2" {
			t.Errorf("last focus = %q, want BB-2", got)
		}
		if _, e, _ := core.Running(w.State); e.Held {
			t.Error("a touch by hand held the timer")
		}
	})
	t.Run("a keyed prompt on the running ticket", func(t *testing.T) {
		if got := steps(t, base(), focus("AA-1")).State.LastFocus; got != "AA-1" {
			t.Errorf("last focus = %q, want AA-1", got)
		}
	})
}
