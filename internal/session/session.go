// Package session manages session records in .hopback/sessions/: which
// session a write comes from, parking, and marking sessions whose launching
// process has gone away.
package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/fsutil"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/project"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/store"
)

// Detector finds the calling agent's native session ID in the environment.
type Detector struct {
	Agent  string
	Detect func(env func(string) string) (nativeID string, ok bool)
}

type Manager struct {
	Store     *store.Store
	Env       func(string) string
	Detectors []Detector
}

func NewManager(st *store.Store, env func(string) string, detectors ...Detector) *Manager {
	return &Manager{Store: st, Env: env, Detectors: detectors}
}

// RunLockPath is the machine-local lock a launcher holds while its session runs.
func RunLockPath(root, sessionID string) string {
	return filepath.Join(root, project.Dir, "run", sessionID+".lock")
}

func validID(id string) bool {
	return id != "" && !strings.ContainsAny(id, `/\:`) && !strings.Contains(id, "..")
}

func (m *Manager) path(id string) string { return m.Store.Path("sessions/" + id + ".json") }

func (m *Manager) read(path string) (model.Session, error) {
	var s model.Session
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		rel, _ := filepath.Rel(m.Store.Path(""), path)
		return s, &store.FileError{File: filepath.ToSlash(rel), Err: err}
	}
	return s, nil
}

// List returns all sessions sorted by start time, oldest first.
func (m *Manager) List() ([]model.Session, error) {
	paths, err := filepath.Glob(m.Store.Path("sessions/*.json"))
	if err != nil {
		return nil, err
	}
	out := make([]model.Session, 0, len(paths))
	for _, p := range paths {
		s, err := m.read(p)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *Manager) Get(id string) (model.Session, error) {
	if !validID(id) {
		return model.Session{}, &store.ValidationError{Msg: "unknown session " + id}
	}
	s, err := m.read(m.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return s, &store.ValidationError{Msg: "unknown session " + id}
	}
	return s, err
}

// Create writes a new session record and emits session_started.
func (m *Manager) Create(s model.Session) error {
	return m.Store.WithLock(func() error { _, err := m.createLocked(s); return err })
}

func (m *Manager) createLocked(s model.Session) (model.Session, error) {
	now := m.Store.Now().UTC()
	s.SchemaVersion, s.ProjectID = model.SchemaVersion, m.Store.Project.ID
	if s.ID == "" {
		s.ID = model.NewSessionID(now)
	}
	if s.StartedAt.IsZero() {
		s.StartedAt = now
	}
	if s.Status == "" {
		s.Status = "active"
	}
	if err := fsutil.WriteJSON(m.path(s.ID), s); err != nil {
		return s, err
	}
	data := map[string]any{"role": string(s.Role), "launched": s.Launched}
	if s.ObjectiveTaskID != "" {
		data["task_id"] = s.ObjectiveTaskID
	}
	ev := m.Store.NewEvent(model.EvSessionStarted, model.Attribution{Session: s.ID, Agent: s.Agent}, data)
	return s, m.Store.AppendEventsLocked(ev)
}

// Save rewrites an existing session record.
func (m *Manager) Save(s model.Session) error {
	return m.Store.WithLock(func() error { return fsutil.WriteJSON(m.path(s.ID), s) })
}

// Update changes a session record under the project lock. fn edits the record
// and returns any events to append with it.
func (m *Manager) Update(id string, fn func(s *model.Session) []model.Event) (model.Session, error) {
	var s model.Session
	err := m.Store.WithLock(func() error {
		var err error
		if s, err = m.Get(id); err != nil {
			return err
		}
		evs := fn(&s)
		if err := fsutil.WriteJSON(m.path(s.ID), s); err != nil {
			return err
		}
		return m.Store.AppendEventsLocked(evs...)
	})
	return s, err
}

// Current identifies the session a command runs in: HOPBACK_SESSION if it
// names a session in this project, else the newest session matching a
// detector's native ID (created on first sight), else an unknown session.
// It never refuses a write for lack of a session.
func (m *Manager) Current() (model.Attribution, *model.Session, error) {
	var found *model.Session
	err := m.Store.WithLock(func() error {
		if id := m.Env("HOPBACK_SESSION"); id != "" {
			if s, err := m.Get(id); err == nil {
				found = &s
			}
		}
		for _, d := range m.Detectors {
			if found != nil {
				break
			}
			nid, ok := d.Detect(m.Env)
			if !ok {
				continue
			}
			all, err := m.List()
			if err != nil {
				return err
			}
			for i := len(all) - 1; i >= 0; i-- {
				if all[i].NativeSessionID == nid {
					found = &all[i]
					break
				}
			}
			if found == nil {
				s, err := m.createLocked(model.Session{Role: "build", Agent: d.Agent, NativeSessionID: nid, Status: "active"})
				if err != nil {
					return err
				}
				found = &s
			}
		}
		if found != nil && found.Status == "interrupted" {
			found.Status, found.EndedAt = "active", nil
			if err := fsutil.WriteJSON(m.path(found.ID), *found); err != nil {
				return err
			}
			ev := m.Store.NewEvent(model.EvSessionRevived, model.Attribution{Session: found.ID, Agent: found.Agent}, nil)
			return m.Store.AppendEventsLocked(ev)
		}
		return nil
	})
	if err != nil {
		return model.Attribution{Agent: "unknown"}, nil, err
	}
	if found == nil {
		return model.Attribution{Agent: "unknown"}, nil, nil
	}
	agent := found.Agent
	if agent == "" {
		agent = "unknown"
	}
	return model.Attribution{Session: found.ID, Agent: agent}, found, nil
}

// Reconcile ends active sessions that can no longer be running: launched
// sessions whose launcher no longer holds the run lock, and (by the MVP rule)
// sessions Hopback did not launch, which a later write revives. Build
// sessions become interrupted and are returned; concierge sessions complete.
func (m *Manager) Reconcile() ([]model.Session, error) {
	var marked []model.Session
	err := m.Store.WithLock(func() error {
		all, err := m.List()
		if err != nil {
			return err
		}
		now := m.Store.Now().UTC()
		for _, s := range all {
			if s.Status != "active" {
				continue
			}
			if s.Launched {
				lp := RunLockPath(m.Store.Root, s.ID)
				l, ok, err := fsutil.TryLock(lp)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				l.Release()
				os.Remove(lp)
			}
			s.EndedAt = &now
			typ := model.EvSessionEndedWithoutHandoff
			if s.Role == "concierge" {
				s.Status, typ = "completed", model.EvSessionCompleted
			} else {
				s.Status = "interrupted"
			}
			if err := fsutil.WriteJSON(m.path(s.ID), s); err != nil {
				return err
			}
			ev := m.Store.NewEvent(typ, model.Attribution{Session: s.ID, Agent: s.Agent}, nil)
			if err := m.Store.AppendEventsLocked(ev); err != nil {
				return err
			}
			if s.Status == "interrupted" {
				marked = append(marked, s)
			}
		}
		return nil
	})
	return marked, err
}

func (m *Manager) latest(match func(model.Session) bool) (*model.Session, error) {
	all, err := m.List()
	if err != nil {
		return nil, err
	}
	for i := len(all) - 1; i >= 0; i-- {
		if match(all[i]) {
			return &all[i], nil
		}
	}
	return nil, nil
}

// LatestBuild is the most recently started build session, or nil.
func (m *Manager) LatestBuild() (*model.Session, error) {
	return m.latest(func(s model.Session) bool { return s.Role == "build" })
}

// LatestParkedBuild is the most recently started parked build session, or nil.
func (m *Manager) LatestParkedBuild() (*model.Session, error) {
	return m.latest(func(s model.Session) bool { return s.Role == "build" && s.Status == "parked" })
}

// LastConcierge is the most recently started concierge session, or nil.
func (m *Manager) LastConcierge() (*model.Session, error) {
	return m.latest(func(s model.Session) bool { return s.Role == "concierge" })
}
