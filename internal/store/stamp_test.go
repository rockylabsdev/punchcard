package store_test

import (
	"testing"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/store"
)

// Stamp is the TUI's cheap "did state.json change?" check: it moves on
// every write, by this process or any other, and on nothing else.
func TestStampMovesOnlyWhenStateIsWritten(t *testing.T) {
	c := newClock()
	s, _ := open(t, c)
	first, err := s.Stamp()
	if err != nil {
		t.Fatalf("Stamp with no file: %v", err)
	}
	if first != (store.Stamp{}) {
		t.Errorf("Stamp with no file = %+v, want zero", first)
	}

	mustApply(t, s, start("AA-1"))
	written, err := s.Stamp()
	if err != nil {
		t.Fatal(err)
	}
	if written == first {
		t.Error("Stamp did not move after a write")
	}

	if _, _, err := s.Read(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(core.Resume{Key: "", Origin: core.OriginHand}); err != nil {
		t.Fatal(err) // already running: nothing to write
	}
	if got, _ := s.Stamp(); got != written {
		t.Error("Stamp moved without a write to state.json")
	}

	// A second store on the same directory is another process.
	other := store.New(s.Dir(), c.now)
	mustApply(t, other, core.Pause{Origin: core.OriginHand})
	if got, _ := s.Stamp(); got == written {
		t.Error("Stamp did not move after another writer's change")
	}
}

func mustApply(t *testing.T, s *store.Store, c core.Command) {
	t.Helper()
	if _, err := s.Apply(c); err != nil {
		t.Fatalf("%T: %v", c, err)
	}
}
