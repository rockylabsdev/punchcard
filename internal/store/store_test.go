package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func inode(t *testing.T, path string) uint64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Sys().(*syscall.Stat_t).Ino
}

func TestSaveReplacesTheFileByRename(t *testing.T) {
	c := newClock()
	s, dir := open(t, c)
	if _, err := s.Apply(start("PUNCH-12")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, store.StateFile)
	before := inode(t, path)
	c.t = c.t.Add(time.Minute)
	if _, err := s.Apply(core.Pause{Origin: core.OriginHand}); err != nil {
		t.Fatal(err)
	}
	if inode(t, path) == before {
		t.Error("state.json kept its inode: it was rewritten in place, not replaced")
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if d := cmp.Diff([]string{store.HeartbeatFile, store.StateFile, store.LockFile}, names); d != "" {
		t.Errorf("files in the data dir (-want +got):\n%s", d)
	}
}

func TestARefusedCommandWritesNothing(t *testing.T) {
	c := newClock()
	s, dir := open(t, c)
	_, err := s.Apply(core.Pause{Origin: core.OriginHand})
	if !errors.Is(err, core.ErrNoTimer) {
		t.Fatalf("err = %v, want ErrNoTimer", err)
	}
	for _, name := range []string{store.StateFile, store.HeartbeatFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s exists after a refused command", name)
		}
	}
}

func TestAStaleTimerIsRecoveredBeforeTheCommandApplies(t *testing.T) {
	c := newClock()
	s, dir := open(t, c)
	if _, err := s.Apply(start("PUNCH-12")); err != nil {
		t.Fatal(err)
	}
	lastSeen := c.t.Add(30 * time.Minute)
	write(t, filepath.Join(dir, store.HeartbeatFile),
		`{"schemaVersion": 1, "lastSeen": "`+lastSeen.Format(core.TimeLayout)+`"}`)

	c.t = lastSeen.Add(3 * time.Hour)
	res, err := s.Apply(start("PUNCH-13"))
	if err != nil {
		t.Fatal(err)
	}
	want := []core.Event{
		{Kind: core.EventRecovered, Key: "PUNCH-12", At: core.At(lastSeen)},
		{Kind: core.EventAdded, Key: "PUNCH-13"},
	}
	if d := cmp.Diff(want, res.Events); d != "" {
		t.Errorf("events (-want +got):\n%s", d)
	}
	w, _, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	e := w.State.TrackedTickets[0].Entries[0]
	if !e.Recovered || !e.Segments[0].EndedAt.Equal(lastSeen) {
		t.Errorf("PUNCH-12 on disk was not capped at lastSeen: %+v", e)
	}
	if !w.Heartbeat.LastSeen.Equal(c.t) {
		t.Errorf("lastSeen on disk = %v, want now %v", w.Heartbeat.LastSeen, c.t)
	}
}

func TestFirstRunIsAMissingOrEmptyFile(t *testing.T) {
	for name, content := range map[string]*string{"missing": nil, "zero-byte": new(string)} {
		t.Run(name, func(t *testing.T) {
			c := newClock()
			s, dir := open(t, c)
			if content != nil {
				write(t, filepath.Join(dir, store.StateFile), *content)
				write(t, filepath.Join(dir, store.HeartbeatFile), *content)
			}
			w, notices, err := s.Read()
			if err != nil || len(notices) > 0 {
				t.Fatalf("Read: %v %v", err, notices)
			}
			if d := cmp.Diff(core.World{State: core.NewState()}, w); d != "" {
				t.Errorf("first run (-want +got):\n%s", d)
			}
		})
	}
}

func TestACorruptFileIsMovedAsideWithANotice(t *testing.T) {
	for _, name := range []string{store.StateFile, store.HeartbeatFile} {
		t.Run(name, func(t *testing.T) {
			c := newClock()
			s, dir := open(t, c)
			write(t, filepath.Join(dir, name), "{not json")

			res, err := s.Apply(start("PUNCH-12"))
			if err != nil {
				t.Fatal(err)
			}
			backup := filepath.Join(dir, name+".corrupt-20260809T090241.000")
			if got := read(t, backup); got != "{not json" {
				t.Errorf("backup holds %q", got)
			}
			if len(res.Notices) != 1 || !strings.Contains(res.Notices[0], backup) {
				t.Errorf("notices %q do not name %s", res.Notices, backup)
			}
			if !strings.Contains(read(t, filepath.Join(dir, name)), `"schemaVersion": 1`) {
				t.Errorf("%s was not rewritten fresh", name)
			}
		})
	}
}

func TestReadNeverMovesACorruptFile(t *testing.T) {
	c := newClock()
	s, dir := open(t, c)
	path := filepath.Join(dir, store.StateFile)
	write(t, path, "{not json")
	_, notices, err := s.Read()
	if err != nil || len(notices) != 1 {
		t.Fatalf("Read: %v %q", err, notices)
	}
	if read(t, path) != "{not json" {
		t.Error("Read changed the corrupt file")
	}
}

func TestANewerSchemaIsRefusedAndNothingIsWritten(t *testing.T) {
	for _, name := range []string{store.StateFile, store.HeartbeatFile} {
		t.Run(name, func(t *testing.T) {
			c := newClock()
			s, dir := open(t, c)
			newer := `{"schemaVersion": 2, "somethingNew": true}`
			write(t, filepath.Join(dir, name), newer)

			_, err := s.Apply(start("PUNCH-12"))
			if !errors.Is(err, store.ErrNewerSchema) {
				t.Fatalf("Apply err = %v, want ErrNewerSchema", err)
			}
			if _, _, err := s.Read(); !errors.Is(err, store.ErrNewerSchema) {
				t.Errorf("Read err = %v, want ErrNewerSchema", err)
			}
			if got := read(t, filepath.Join(dir, name)); got != newer {
				t.Errorf("%s was changed: %s", name, got)
			}
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if e.Name() != name && e.Name() != store.LockFile {
					t.Errorf("unexpected file written: %s", e.Name())
				}
			}
		})
	}
}
