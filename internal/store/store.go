// Package store persists the core's World in state.json and
// heartbeat.json under the XDG data directory (ADR 0002, ADR 0006).
//
// Every change goes through Apply (or TryApply): one critical section
// that takes the flock on state.lock, reads both files fresh, backs up a
// corrupt file or refuses a newer one, runs core.Apply (which recovers
// before it applies), writes what changed by temp-then-rename, and
// unlocks. Nothing outside this package holds a World it means to save.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
)

// File names inside the data directory.
const (
	StateFile     = "state.json"
	HeartbeatFile = "heartbeat.json"
	LockFile      = "state.lock"
)

// DefaultLockWait is how long a writer waits for state.lock before it
// gives up with ErrBusy (the CLI's exit 75).
const DefaultLockWait = 2 * time.Second

// Dir is the data directory: $XDG_DATA_HOME/punchcard, else
// ~/.local/share/punchcard.
func Dir(getenv func(string) string) string {
	if d := getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "punchcard")
	}
	return filepath.Join(getenv("HOME"), ".local", "share", "punchcard")
}

// Store is the data directory. It holds no state of its own between
// calls: every call reads the files fresh.
type Store struct {
	dir string
	now func() time.Time
	// LockWait is how long Apply waits for the lock. Zero means
	// DefaultLockWait.
	LockWait time.Duration
}

// New returns the store in dir, using now for every transition.
func New(dir string, now func() time.Time) *Store {
	return &Store{dir: dir, now: now}
}

// Dir is the directory the store reads and writes.
func (s *Store) Dir() string { return s.dir }

// Result is what one critical section produced.
type Result struct {
	// World is what is now on disk (unchanged on a refusal).
	World core.World
	// Events are the command's side effects, recovery included.
	Events []core.Event
	// Now is the instant the transition was applied at.
	Now time.Time
	// Notices are things the user must be told that are not events,
	// such as a corrupt file moved aside. They are set even on error.
	Notices []string
}

// Apply runs cmd as one critical section, waiting up to LockWait for
// the lock. A refused command writes nothing and returns the core's
// error; a lock that stays held returns ErrBusy.
func (s *Store) Apply(cmd core.Command) (Result, error) {
	wait := s.LockWait
	if wait == 0 {
		wait = DefaultLockWait
	}
	return s.transact(cmd, wait)
}

func (s *Store) transact(cmd core.Command, wait time.Duration) (Result, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return Result{}, err
	}
	l, err := lock(filepath.Join(s.dir, LockFile), wait)
	if err != nil {
		return Result{}, err
	}
	defer l.unlock()

	var res Result
	in, err := s.load(&res.Notices)
	if err != nil {
		return res, err
	}
	res.Now = s.now()
	out, events, err := core.Apply(in, cmd, res.Now)
	if err != nil {
		res.World = in
		return res, err
	}
	if err := s.save(in, out); err != nil {
		res.World = in
		return res, err
	}
	res.World, res.Events = out, events
	return res, nil
}

// Read returns the world on disk without taking the lock and without
// writing anything (reads are pure). Atomic saves mean it never sees a
// half-written file.
func (s *Store) Read() (core.World, []string, error) {
	var notices []string
	w, err := s.load(&notices)
	return w, notices, err
}

func (s *Store) load(notices *[]string) (core.World, error) {
	w := core.World{State: core.NewState()}
	if err := s.loadFile(StateFile, &w.State); err != nil {
		return core.World{}, err
	}
	if err := s.loadFile(HeartbeatFile, &w.Heartbeat); err != nil {
		return core.World{}, err
	}
	if w.State.TrackedTickets == nil {
		w.State.TrackedTickets = []core.TrackedTicket{}
	}
	return w, nil
}

// loadFile decodes name into v. A missing or empty file leaves v alone.
func (s *Store) loadFile(name string, v any) error {
	b, err := os.ReadFile(filepath.Join(s.dir, name))
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(bytes.TrimSpace(b)) == 0) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// save writes the files that changed between in and out: state first,
// so a failed heartbeat write never loses a recovery.
func (s *Store) save(in, out core.World) error {
	before, err := encode(in.State)
	if err != nil {
		return err
	}
	after, err := encode(out.State)
	if err != nil {
		return err
	}
	if !bytes.Equal(before, after) {
		if err := s.replace(StateFile, after); err != nil {
			return err
		}
	}
	if out.Heartbeat != in.Heartbeat {
		b, err := encode(out.Heartbeat)
		if err != nil {
			return err
		}
		if err := s.replace(HeartbeatFile, b); err != nil {
			return err
		}
	}
	return nil
}

func encode(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// replace writes b to name atomically: a temp file in the same
// directory, synced, then renamed over the old file.
func (s *Store) replace(name string, b []byte) error {
	f, err := os.CreateTemp(s.dir, "."+name+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(s.dir, name))
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", name, err)
	}
	return nil
}
