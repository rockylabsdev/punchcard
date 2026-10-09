package store

import (
	"errors"
	"os"
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
