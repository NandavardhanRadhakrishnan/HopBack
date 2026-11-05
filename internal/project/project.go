// Package project finds and initialises Hopback projects: directories that
// contain a .hopback/ folder.
package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/fsutil"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/protocol"
)

const (
	Dir         = ".hopback"
	LockFile    = ".lock"
	LockTimeout = 10 * time.Second
)

var ErrNotProject = errors.New("not inside a Hopback project (run `hopback init`)")

// Find walks up from start to the first directory containing .hopback/project.json.
func Find(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, Dir, "project.json")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotProject
		}
		dir = parent
	}
}

type InitOptions struct {
	Name       string
	Installers []func(root string) error
}

// Init creates .hopback/ in dir, or refreshes an existing one. The bool
// reports whether the project was newly created. Installer errors are
// returned joined, alongside the project, after every installer has run.
func Init(dir string, opts InitOptions, now time.Time) (model.Project, bool, error) {
	hp := filepath.Join(dir, Dir)
	var p model.Project
	created := false
	if b, err := os.ReadFile(filepath.Join(hp, "project.json")); err == nil {
		if err := json.Unmarshal(b, &p); err != nil {
			return p, false, fmt.Errorf("invalid .hopback/project.json: %v (Hopback did not modify it)", err)
		}
		if err := EnsureTemplates(dir); err != nil {
			return p, false, err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		name := opts.Name
		if name == "" {
			name = filepath.Base(dir)
		}
		p = model.Project{SchemaVersion: model.SchemaVersion, ID: model.NewUUID(), Name: name, CreatedAt: now.UTC()}
		if err := create(hp, p, now.UTC()); err != nil {
			return p, false, err
		}
		created = true
	} else {
		return p, false, err
	}
	if err := UpsertPointerBlock(filepath.Join(dir, "CLAUDE.md"), true); err != nil {
		return p, created, err
	}
	if err := UpsertPointerBlock(filepath.Join(dir, "AGENTS.md"), false); err != nil {
		return p, created, err
	}
	var errs []error
	for _, install := range opts.Installers {
		if err := install(dir); err != nil {
			errs = append(errs, err)
		}
	}
	return p, created, errors.Join(errs...)
}

func create(hp string, p model.Project, now time.Time) error {
	for _, d := range []string{"sessions", "run"} {
		if err := os.MkdirAll(filepath.Join(hp, d), 0o755); err != nil {
			return err
		}
	}
	lock, err := fsutil.AcquireLock(filepath.Join(hp, LockFile), LockTimeout)
	if err != nil {
		return err
	}
	defer lock.Release()
	ev := model.Event{ID: model.NewSessionID(now), TS: now, Type: model.EvProjectInitialized, Agent: "user",
		Data: map[string]any{"project_id": p.ID, "name": p.Name}}
	evLine, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	files := []struct {
		name string
		v    any
	}{
		{"project.json", p},
		{"state.json", model.State{SchemaVersion: model.SchemaVersion, ProjectStatus: "active"}},
		{"backlog.json", model.Backlog{SchemaVersion: model.SchemaVersion, Items: []model.Task{}}},
		{"decisions.json", model.Decisions{SchemaVersion: model.SchemaVersion, Items: []model.Decision{}}},
	}
	for _, f := range files {
		if err := fsutil.WriteJSON(filepath.Join(hp, f.name), f.v); err != nil {
			return err
		}
	}
	if err := fsutil.WriteFileAtomic(filepath.Join(hp, "events.jsonl"), append(evLine, '\n'), 0o644); err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(filepath.Join(hp, ".gitignore"), []byte(".lock\nrun/\n*.tmp-*\n"), 0o644); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(filepath.Join(hp, "HOPBACK.md"), protocol.HopbackMD(), 0o644)
}

// EnsureTemplates rewrites .hopback/HOPBACK.md when it is missing or older
// than the template this Hopback carries. It never downgrades.
func EnsureTemplates(root string) error {
	path := filepath.Join(root, Dir, "HOPBACK.md")
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if v, _ := protocol.ParseMarkerVersion(b); err == nil && v >= protocol.TemplateVersion {
		return nil
	}
	lock, err := fsutil.AcquireLock(filepath.Join(root, Dir, LockFile), LockTimeout)
	if err != nil {
		return err
	}
	defer lock.Release()
	return fsutil.WriteFileAtomic(path, protocol.HopbackMD(), 0o644)
}

// UpsertPointerBlock replaces the marked Hopback block in path, or appends it.
// A missing file is created only when create is true.
func UpsertPointerBlock(path string, create bool) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if !create {
			return nil
		}
		return fsutil.WriteFileAtomic(path, []byte(protocol.PointerBlock()), 0o644)
	}
	if err != nil {
		return err
	}
	s := string(b)
	block := protocol.PointerBlock()
	if i := strings.Index(s, protocol.PointerBegin); i >= 0 {
		if j := strings.Index(s[i:], protocol.PointerEnd); j >= 0 {
			end := i + j + len(protocol.PointerEnd)
			if strings.HasPrefix(s[end:], "\r\n") {
				end += 2
			} else if strings.HasPrefix(s[end:], "\n") {
				end++
			}
			out := s[:i] + block + s[end:]
			if out == s {
				return nil
			}
			return fsutil.WriteFileAtomic(path, []byte(out), 0o644)
		}
	}
	var buf bytes.Buffer
	buf.WriteString(s)
	if len(s) > 0 {
		if !strings.HasSuffix(s, "\n") {
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}
	buf.WriteString(block)
	return fsutil.WriteFileAtomic(path, buf.Bytes(), 0o644)
}
