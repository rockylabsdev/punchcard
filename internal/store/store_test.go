package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/store"
)

var zone = time.FixedZone("UTC-5", -5*60*60)

// clock is a settable now for a store under test.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newClock() *clock { return &clock{time.Date(2026, 8, 9, 9, 2, 41, 0, zone)} }

// open points XDG_DATA_HOME at a temp dir and opens the store there.
func open(t *testing.T, c *clock) (*store.Store, string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := store.Dir(os.Getenv)
	return store.New(dir, c.now), dir
}

func start(key string) core.Start {
	return core.Start{Key: key, Origin: core.OriginHand, NewEntryID: "1E4C9A02-7F3B-4D18-9C51-A0B7E6D24F88"}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDirFollowsXDGDataHome(t *testing.T) {
	env := map[string]string{"XDG_DATA_HOME": "/x/data", "HOME": "/home/u"}
	if got := store.Dir(func(k string) string { return env[k] }); got != "/x/data/punchcard" {
		t.Errorf("Dir = %q", got)
	}
	delete(env, "XDG_DATA_HOME")
	if got := store.Dir(func(k string) string { return env[k] }); got != "/home/u/.local/share/punchcard" {
		t.Errorf("Dir without XDG_DATA_HOME = %q", got)
	}
}

func TestApplyRoundTripsThroughTheFiles(t *testing.T) {
	c := newClock()
	s, dir := open(t, c)
	res, err := s.Apply(start("PUNCH-12"))
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff([]core.Event{{Kind: core.EventAdded, Key: "PUNCH-12"}}, res.Events); d != "" {
		t.Errorf("events (-want +got):\n%s", d)
	}

	// A second, separate store sees exactly what the first wrote.
	got, notices, err := store.New(dir, c.now).Read()
	if err != nil || len(notices) > 0 {
		t.Fatalf("Read: %v %v", err, notices)
	}
	if d := cmp.Diff(res.World, got); d != "" {
		t.Errorf("world read back (-wrote +read):\n%s", d)
	}

	state := read(t, filepath.Join(dir, "state.json"))
	for _, want := range []string{`"schemaVersion": 1`, `"startedAt": "2026-08-09T09:02:41.000-0500"`, `"trackedTickets"`} {
		if !strings.Contains(state, want) {
			t.Errorf("state.json lacks %s:\n%s", want, state)
		}
	}
	for _, absent := range []string{"null", "endedAt", "worklog"} {
		if strings.Contains(state, absent) {
			t.Errorf("state.json carries %q (nil optionals are omitted):\n%s", absent, state)
		}
	}
	hb := read(t, filepath.Join(dir, "heartbeat.json"))
	if !strings.Contains(hb, `"lastSeen": "2026-08-09T09:02:41.000-0500"`) {
		t.Errorf("heartbeat.json:\n%s", hb)
	}
}
