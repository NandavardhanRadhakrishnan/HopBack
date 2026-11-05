// Package fsutil provides the filesystem primitives every Hopback write uses:
// atomic file replacement and cross-platform file locks.
package fsutil

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// renameFunc is a test hook.
var renameFunc = os.Rename

const (
	renameAttempts = 10
	renameDelay    = 20 * time.Millisecond
)

// WriteFileAtomic writes data to a temp file next to path, fsyncs it, then
// renames it over path. The rename is retried briefly because on Windows it
// can fail while another process has the target open.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	tmp := path + ".tmp-" + hex.EncodeToString(suffix)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	for i := 0; ; i++ {
		err = renameFunc(tmp, path)
		if err == nil {
			return nil
		}
		if i == renameAttempts-1 {
			break
		}
		time.Sleep(renameDelay)
	}
	os.Remove(tmp)
	return fmt.Errorf("replace %s: %w", path, err)
}

// WriteJSON writes v as indented JSON with a trailing newline, atomically.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(b, '\n'), 0o644)
}
