package store_test

import (
	"os"
	"path/filepath"
	"testing"
)

// Introduce writes a first-run state.json (the "already introduced"
// marker) and never touches one that exists.
func TestIntroduce(t *testing.T) {
	c := newClock()
	s, dir := open(t, c)
	if err := s.Introduce(); err != nil {
		t.Fatal(err)
	}
	w, _, err := s.Read()
	if err != nil || w.State.SchemaVersion != 1 || len(w.State.TrackedTickets) != 0 {
		t.Fatalf("after Introduce: %+v, %v", w.State, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Fatalf("no state.json: %v", err)
	}
	if _, err := s.Apply(start("AA-1")); err != nil {
		t.Fatal(err)
	}
	before := read(t, filepath.Join(dir, "state.json"))
	if err := s.Introduce(); err != nil {
		t.Fatal(err)
	}
	if after := read(t, filepath.Join(dir, "state.json")); after != before {
		t.Errorf("Introduce rewrote an existing state.json")
	}
}
