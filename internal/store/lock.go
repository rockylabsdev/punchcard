package store

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// ErrBusy: another writer held state.lock for the whole wait (or, for
// TryApply, at the moment of asking). Nothing was changed.
var ErrBusy = errors.New("state is locked by another punchcard")

// lockPoll is how often a waiting writer retries the lock.
const lockPoll = 20 * time.Millisecond

// fileLock is an exclusive flock on its own open of state.lock. The
// kernel drops it when the descriptor closes or the process dies.
type fileLock struct{ f *os.File }

// lock takes the exclusive flock on path, retrying for up to wait. A
// wait of zero tries once.
func lock(path string, wait time.Duration) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return &fileLock{f}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EINTR) {
			f.Close()
			return nil, err
		}
		if !time.Now().Before(deadline) {
			f.Close()
			return nil, ErrBusy
		}
		time.Sleep(lockPoll)
	}
}

// unlock releases the flock by closing the descriptor.
func (l *fileLock) unlock() { l.f.Close() }

// SyncLockFile is the second lock: whoever holds it is the only one
// POSTing worklogs (ADR 0002).
const SyncLockFile = "sync.lock"

// ErrSyncBusy: another punchcard holds sync.lock, so it is sending
// worklogs right now. Nothing was sent.
var ErrSyncBusy = errors.New("another punchcard is sending worklogs")

// LockSync takes sync.lock without waiting and returns its release.
// Hold it across a POST and the re-read that decides to send, never
// state.lock: Apply still works while sync.lock is held, so mark the
// outcome with Apply before releasing. A held lock is ErrSyncBusy.
func (s *Store) LockSync() (release func(), err error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, err
	}
	l, err := lock(filepath.Join(s.dir, SyncLockFile), 0)
	if errors.Is(err, ErrBusy) {
		return nil, ErrSyncBusy
	}
	if err != nil {
		return nil, err
	}
	return l.unlock, nil
}

// Locked reports whether another punchcard holds state.lock right now.
// It never creates the lock file and never waits: a doctor's probe.
func (s *Store) Locked() bool {
	if s.err != nil {
		return false
	}
	f, err := os.OpenFile(filepath.Join(s.dir, LockFile), os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	return errors.Is(unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB), unix.EWOULDBLOCK)
}
