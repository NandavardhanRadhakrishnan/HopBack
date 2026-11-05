package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestWriteFileAtomic_ReplacesContent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.json")
	must(t, WriteFileAtomic(p, []byte("a"), 0o644))
	must(t, WriteFileAtomic(p, []byte("b"), 0o644))
	got, _ := os.ReadFile(p)
	if string(got) != "b" {
		t.Fatalf("got %q", got)
	}
	if m, _ := filepath.Glob(p + ".tmp-*"); len(m) != 0 {
		t.Fatalf("leftover temp files %v", m)
	}
}

func TestWriteFileAtomic_RetriesRename(t *testing.T) {
	calls := 0
	renameFunc = func(a, b string) error {
		calls++
		if calls < 3 {
			return errors.New("sharing violation")
		}
		return os.Rename(a, b)
	}
	defer func() { renameFunc = os.Rename }()
	must(t, WriteFileAtomic(filepath.Join(t.TempDir(), "f"), []byte("x"), 0o644))
	if calls != 3 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestWriteFileAtomic_GivesUpAndCleansTemp(t *testing.T) {
	renameFunc = func(a, b string) error { return errors.New("nope") }
	defer func() { renameFunc = os.Rename }()
	dir := t.TempDir()
	if err := WriteFileAtomic(filepath.Join(dir, "f"), []byte("x"), 0o644); err == nil {
		t.Fatal("want error")
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*")); len(m) != 0 {
		t.Fatalf("temp not removed: %v", m)
	}
}

func TestTryLock_Exclusive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	l1, ok1, _ := TryLock(p)
	l2, ok2, _ := TryLock(p)
	if !ok1 || ok2 {
		t.Fatalf("ok1=%v ok2=%v", ok1, ok2)
	}
	_ = l2
	must(t, l1.Release())
	l3, ok3, _ := TryLock(p)
	if !ok3 {
		t.Fatal("lock not reacquirable after release")
	}
	must(t, l3.Release())
}

func TestAcquireLock_Timeout(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	l, _, _ := TryLock(p)
	defer l.Release()
	if _, err := AcquireLock(p, 50*time.Millisecond); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("err=%v", err)
	}
}
