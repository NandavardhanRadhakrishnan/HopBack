package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

var ErrLockTimeout = errors.New("timed out waiting for the project lock")

// Lock is a held file lock.
type Lock struct{ f *flock.Flock }

// Release unlocks and closes the lock file. It does not remove the file.
func (l *Lock) Release() error { return l.f.Unlock() }

// TryLock attempts to take the lock at path without blocking. The bool reports
// whether the lock was acquired.
func TryLock(path string) (*Lock, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	f := flock.New(path)
	ok, err := f.TryLock()
	if err != nil || !ok {
		return nil, false, err
	}
	return &Lock{f: f}, true, nil
}

// AcquireLock retries TryLock every 10 ms until it succeeds or timeout passes.
func AcquireLock(path string, timeout time.Duration) (*Lock, error) {
	deadline := time.Now().Add(timeout)
	for {
		l, ok, err := TryLock(path)
		if err != nil {
			return nil, err
		}
		if ok {
			return l, nil
		}
		if time.Now().After(deadline) {
			return nil, ErrLockTimeout
		}
		time.Sleep(10 * time.Millisecond)
	}
}
