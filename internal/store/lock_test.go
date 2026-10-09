package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/j-dsmith/punchcard/internal/store"
	"golang.org/x/sys/unix"
)

// holdLock takes the flock on its own open of state.lock, as another
// punchcard process would. Closing the returned file releases it.
func holdLock(t *testing.T, dir string) *os.File {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, store.LockFile), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestASecondWriterWaitsForTheLockAndRunsWhenItIsReleased(t *testing.T) {
	c := newClock()
	s, dir := open(t, c)
	held := holdLock(t, dir)

	done := make(chan error, 1)
	go func() {
		_, err := s.Apply(start("PUNCH-12"))
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("Apply finished while the lock was held: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	held.Close() // the kernel releases the flock with the descriptor
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Apply after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Apply still waiting after the lock was released")
	}
	if _, err := os.Stat(filepath.Join(dir, store.StateFile)); err != nil {
		t.Errorf("state.json not written: %v", err)
	}
}

func TestAWriterGivesUpWithErrBusyAfterTheWait(t *testing.T) {
	c := newClock()
	s, dir := open(t, c)
	s.LockWait = 100 * time.Millisecond
	holdLock(t, dir)

	begun := time.Now()
	_, err := s.Apply(start("PUNCH-12"))
	if !errors.Is(err, store.ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
	if waited := time.Since(begun); waited < 100*time.Millisecond {
		t.Errorf("gave up after %v, before the wait", waited)
	}
	if _, err := os.Stat(filepath.Join(dir, store.StateFile)); !errors.Is(err, os.ErrNotExist) {
		t.Error("state.json written without the lock")
	}
}

func TestAPresenceWriteSkipsAtOnceWhenTheLockIsHeld(t *testing.T) {
	c := newClock()
	s, dir := open(t, c)
	held := holdLock(t, dir)

	begun := time.Now()
	_, err := s.TryApply(core.Touch{Origin: core.OriginPrompt})
	if !errors.Is(err, store.ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
	if waited := time.Since(begun); waited > 50*time.Millisecond {
		t.Errorf("presence write waited %v; it must not block", waited)
	}
	if _, err := os.Stat(filepath.Join(dir, store.HeartbeatFile)); !errors.Is(err, os.ErrNotExist) {
		t.Error("heartbeat.json written without the lock")
	}

	held.Close()
	if _, err := s.TryApply(core.Touch{Origin: core.OriginPrompt}); err != nil {
		t.Fatalf("presence write after release: %v", err)
	}
	w, _, _ := s.Read()
	if !w.Heartbeat.LastSeen.Equal(c.t) {
		t.Errorf("lastSeen = %v, want %v", w.Heartbeat.LastSeen, c.t)
	}
}

func TestTwoGoroutinesWritingTogetherLoseNothing(t *testing.T) {
	c := newClock()
	s, dir := open(t, c)
	other := store.New(dir, c.now) // a separate store: its own open of state.lock

	keys := []string{"PUNCH-1", "PUNCH-2"}
	errs := make(chan error, 2)
	for i, st := range []*store.Store{s, other} {
		go func() {
			_, err := st.Apply(core.Add{Tickets: []core.NewTicket{{Key: keys[i]}}, Origin: core.OriginHand})
			errs <- err
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	w, _, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if n := len(w.State.TrackedTickets); n != 2 {
		t.Errorf("tracked %d tickets, want 2: one writer overwrote the other", n)
	}
}

func TestSyncLockIsTakenWithoutWaitingAndIsNotTheStateLock(t *testing.T) {
	c := newClock()
	s, dir := open(t, c)
	release, err := s.LockSync()
	if err != nil {
		t.Fatalf("first LockSync: %v", err)
	}
	t0 := time.Now()
	if _, err := s.LockSync(); !errors.Is(err, store.ErrSyncBusy) {
		t.Errorf("second LockSync: err = %v, want ErrSyncBusy", err)
	}
	if waited := time.Since(t0); waited > 100*time.Millisecond {
		t.Errorf("second LockSync waited %v; it must not wait", waited)
	}
	if _, err := s.Apply(start("PUNCH-12")); err != nil {
		t.Errorf("Apply while sync.lock is held: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, store.SyncLockFile)); err != nil {
		t.Errorf("sync.lock not in the data dir: %v", err)
	}
	release()
	again, err := s.LockSync()
	if err != nil {
		t.Fatalf("LockSync after release: %v", err)
	}
	again()
}
