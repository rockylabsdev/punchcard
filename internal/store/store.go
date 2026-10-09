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
	"reflect"
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

// ErrNoDataDir: neither XDG_DATA_HOME nor HOME is set, so there is no
// data directory. It is never guessed relative to the working directory.
var ErrNoDataDir = errors.New("neither XDG_DATA_HOME nor HOME is set, so punchcard has no data directory; set HOME")

// Dir is the data directory: $XDG_DATA_HOME/punchcard, else
// ~/.local/share/punchcard, else ErrNoDataDir.
func Dir(getenv func(string) string) (string, error) {
	if d := getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "punchcard"), nil
	}
	if h := getenv("HOME"); h != "" {
		return filepath.Join(h, ".local", "share", "punchcard"), nil
	}
	return "", ErrNoDataDir
}

// Store is the data directory. It holds no state of its own between
// calls: every call reads the files fresh.
type Store struct {
	dir string
	now func() time.Time
	// err is why there is no directory (Open with no HOME); every call
	// returns it.
	err error
	// LockWait is how long Apply waits for the lock. Zero means
	// DefaultLockWait.
	LockWait time.Duration
}

// New returns the store in dir, using now for every transition.
func New(dir string, now func() time.Time) *Store {
	return &Store{dir: dir, now: now}
}

// Open is the store at Dir(getenv). Without a data directory it still
// returns a store, whose every call fails with ErrNoDataDir, so commands
// that never touch the store (version, setup) keep working.
func Open(getenv func(string) string, now func() time.Time) *Store {
	dir, err := Dir(getenv)
	return &Store{dir: dir, now: now, err: err}
}

// lockWait is LockWait, or DefaultLockWait when it is zero.
func (s *Store) lockWait() time.Duration {
	if s.LockWait == 0 {
		return DefaultLockWait
	}
	return s.LockWait
}

// Err is why the store has no directory (ErrNoDataDir), or nil.
func (s *Store) Err() error { return s.err }

// Dir is the directory the store reads and writes.
func (s *Store) Dir() string { return s.dir }

// Now is the store's clock: what the next transition will be applied at.
func (s *Store) Now() time.Time { return s.now() }

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
	return s.transact(cmd, s.lockWait())
}

// TryApply is Apply for presence writes: it tries the lock once and
// returns ErrBusy at once if another writer holds it. Skipping is safe
// because the holder is a transition that records presence itself, or
// another presence write.
func (s *Store) TryApply(cmd core.Command) (Result, error) {
	return s.transact(cmd, 0)
}

func (s *Store) transact(cmd core.Command, wait time.Duration) (Result, error) {
	if s.err != nil {
		return Result{}, s.err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return Result{}, err
	}
	l, err := lock(filepath.Join(s.dir, LockFile), wait)
	if err != nil {
		return Result{}, err
	}
	defer l.unlock()

	res := Result{Now: s.now()}
	in, err := s.load(res.Now, true, &res.Notices)
	if err != nil {
		return res, err
	}
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
	if s.err != nil {
		return core.World{}, nil, s.err
	}
	var notices []string
	w, err := s.load(s.now(), false, &notices)
	return w, notices, err
}

// ErrNewerSchema: a file was written by a newer punchcard. It is
// refused, never reset, and nothing is written (the CLI's exit 65).
var ErrNewerSchema = errors.New("written by a newer punchcard")

// load reads both files. A missing or empty file is a first run. An
// undecodable one is reported in notices and treated as a first run;
// with backup it is also renamed to a timestamped .corrupt- file so
// the next save does not destroy it. A newer schema is ErrNewerSchema.
func (s *Store) load(now time.Time, backup bool, notices *[]string) (core.World, error) {
	w := core.World{State: core.NewState()}
	if err := s.loadFile(StateFile, &w.State, now, backup, notices); err != nil {
		return core.World{}, err
	}
	if err := s.loadFile(HeartbeatFile, &w.Heartbeat, now, backup, notices); err != nil {
		return core.World{}, err
	}
	if w.State.TrackedTickets == nil {
		w.State.TrackedTickets = []core.TrackedTicket{}
	}
	return w, nil
}

// loadFile decodes name into v, leaving v alone on a first run.
func (s *Store) loadFile(name string, v any, now time.Time, backup bool, notices *[]string) error {
	path := filepath.Join(s.dir, name)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(bytes.TrimSpace(b)) == 0) {
		return nil
	}
	if err != nil {
		return err
	}
	var head struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if json.Unmarshal(b, &head) == nil && head.SchemaVersion > core.SchemaVersion {
		return fmt.Errorf("%s has schemaVersion %d, this build reads %d: %w", path, head.SchemaVersion, core.SchemaVersion, ErrNewerSchema)
	}
	// Decode into a fresh value so a half-decoded file never leaks into v.
	fresh := reflect.New(reflect.TypeOf(v).Elem())
	derr := json.Unmarshal(b, fresh.Interface())
	if derr == nil {
		reflect.ValueOf(v).Elem().Set(fresh.Elem())
		return nil
	}
	if !backup {
		*notices = append(*notices, fmt.Sprintf("%s is unreadable (%v); the next change will move it aside and start fresh", path, derr))
		return nil
	}
	dest := path + ".corrupt-" + now.Format("20060102T150405.000")
	if err := os.Rename(path, dest); err != nil {
		return fmt.Errorf("moving aside unreadable %s: %w", path, err)
	}
	*notices = append(*notices, fmt.Sprintf("%s was unreadable (%v); moved it to %s and started fresh", path, derr, dest))
	return nil
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

// Introduce writes a first-run state.json if there is none. Its
// existence is the "already introduced" marker: once the first-run
// setup has been offered, bare punchcard never offers it again.
func (s *Store) Introduce() error {
	if s.err != nil {
		return s.err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	l, err := lock(filepath.Join(s.dir, LockFile), s.lockWait())
	if err != nil {
		return err
	}
	defer l.unlock()
	if _, err := os.Stat(filepath.Join(s.dir, StateFile)); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	b, err := encode(core.NewState())
	if err != nil {
		return err
	}
	return s.replace(StateFile, b)
}
