// Package store reads and writes a project's .hopback/ files. Every write is a
// transaction: it runs under the project lock, validates all operations
// against an in-memory snapshot, and only then replaces files atomically and
// appends events.
package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/fsutil"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/project"
)

// ValidationError is a rejected agent-supplied update.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return &ValidationError{Msg: fmt.Sprintf(format, a...)} }

// FileError reports a .hopback file Hopback could not parse. The file is left as is.
type FileError struct {
	File string
	Err  error
}

func (e *FileError) Error() string {
	return fmt.Sprintf("invalid .hopback/%s: %v (Hopback did not modify it)", e.File, e.Err)
}
func (e *FileError) Unwrap() error { return e.Err }

type Store struct {
	Root    string
	Project model.Project
	Now     func() time.Time
}

// Open reads the project at root.
func Open(root string) (*Store, error) {
	s := &Store{Root: root, Now: time.Now}
	if err := s.readJSON("project.json", &s.Project); err != nil {
		return nil, err
	}
	return s, nil
}

// Path returns the path of name inside .hopback/.
func (s *Store) Path(name string) string {
	return filepath.Join(s.Root, project.Dir, filepath.FromSlash(name))
}

func (s *Store) readJSON(name string, v any) error {
	b, err := os.ReadFile(s.Path(name))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return &FileError{File: name, Err: err}
	}
	return nil
}

type Snapshot struct {
	State     model.State
	Backlog   model.Backlog
	Decisions model.Decisions
}

// Load reads state, backlog and decisions without taking the lock.
func (s *Store) Load() (Snapshot, error) {
	var snap Snapshot
	for _, f := range []struct {
		name string
		v    any
	}{{"state.json", &snap.State}, {"backlog.json", &snap.Backlog}, {"decisions.json", &snap.Decisions}} {
		if err := s.readJSON(f.name, f.v); err != nil {
			return snap, err
		}
	}
	return snap, nil
}

// Tx is the in-memory transaction operations run against.
type Tx struct {
	Snapshot
	Attr model.Attribution
	Now  time.Time

	events                               []model.Event
	created                              []string
	backlogDirty, decisDirty, stateDirty bool
}

// Emit records an event to append when the transaction commits and returns its ID.
func (tx *Tx) Emit(typ string, data map[string]any) string {
	ev := model.Event{ID: model.NewSessionID(tx.Now), TS: tx.Now, Type: typ, Session: tx.Attr.Session, Agent: tx.Attr.Agent, Data: data}
	tx.events = append(tx.events, ev)
	return ev.ID
}

// Task returns the backlog item with id, for in-place modification.
func (tx *Tx) Task(id string) (*model.Task, error) {
	for i := range tx.Backlog.Items {
		if tx.Backlog.Items[i].ID == id {
			return &tx.Backlog.Items[i], nil
		}
	}
	return nil, invalid("unknown task %s", id)
}

func (tx *Tx) MarkBacklog()   { tx.backlogDirty = true }
func (tx *Tx) MarkDecisions() { tx.decisDirty = true }
func (tx *Tx) MarkState()     { tx.stateDirty = true }

// Op is one validated change to a project.
type Op interface{ Apply(tx *Tx) error }

type Result struct {
	Events  []model.Event
	Created []string
}

// Apply runs ops in order under the project lock. If any op fails, nothing is written.
func (s *Store) Apply(attr model.Attribution, ops ...Op) (Result, error) {
	var res Result
	err := s.WithLock(func() error {
		snap, err := s.Load()
		if err != nil {
			return err
		}
		tx := &Tx{Snapshot: snap, Attr: attr, Now: s.Now().UTC()}
		for _, op := range ops {
			if err := op.Apply(tx); err != nil {
				return err
			}
		}
		if tx.backlogDirty {
			if tx.Backlog.Items == nil {
				tx.Backlog.Items = []model.Task{}
			}
			if err := fsutil.WriteJSON(s.Path("backlog.json"), tx.Backlog); err != nil {
				return err
			}
		}
		if tx.decisDirty {
			if tx.Decisions.Items == nil {
				tx.Decisions.Items = []model.Decision{}
			}
			if err := fsutil.WriteJSON(s.Path("decisions.json"), tx.Decisions); err != nil {
				return err
			}
		}
		if tx.stateDirty {
			if err := fsutil.WriteJSON(s.Path("state.json"), tx.State); err != nil {
				return err
			}
		}
		if err := s.AppendEventsLocked(tx.events...); err != nil {
			return err
		}
		res = Result{Events: tx.events, Created: tx.created}
		return nil
	})
	return res, err
}

// WithLock runs fn while holding the project lock.
func (s *Store) WithLock(fn func() error) error {
	lock, err := fsutil.AcquireLock(s.Path(project.LockFile), project.LockTimeout)
	if err != nil {
		return err
	}
	defer lock.Release()
	return fn()
}

// AppendEventsLocked appends events to events.jsonl. The caller holds the lock.
func (s *Store) AppendEventsLocked(evs ...model.Event) error {
	if len(evs) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, ev := range evs {
		b, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	f, err := os.OpenFile(s.Path("events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// NewEvent builds an event stamped with the store's clock.
func (s *Store) NewEvent(typ string, attr model.Attribution, data map[string]any) model.Event {
	now := s.Now().UTC()
	return model.Event{ID: model.NewSessionID(now), TS: now, Type: typ, Session: attr.Session, Agent: attr.Agent, Data: data}
}

// Events reads the whole event log, oldest first.
func (s *Store) Events() ([]model.Event, error) {
	f, err := os.Open(s.Path("events.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var evs []model.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev model.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, &FileError{File: "events.jsonl", Err: fmt.Errorf("line %d: %v", n, err)}
		}
		evs = append(evs, ev)
	}
	return evs, sc.Err()
}
