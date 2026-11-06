// Package launcher runs agent sessions as attached child processes and
// records how they ended.
package launcher

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/agent"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/brief"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/fsutil"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/protocol"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/session"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/store"
)

type Launcher struct {
	Store    *store.Store
	Sessions *session.Manager
	Adapter  agent.Adapter
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

type Outcome struct {
	Session model.Session
	Parked  bool
	NextSet *model.Pick // set when a concierge recorded a next task
}

// guardInterrupt stops Ctrl+C from killing Hopback while an agent runs: the
// terminal delivers it to the agent and to Hopback alike, and the agent uses
// it to interrupt or exit. Catching (not ignoring) the signal leaves the
// child's own handling at its default. It returns the function that restores it.
var guardInterrupt = func() (release func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	return func() { signal.Stop(ch) }
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Build starts a fresh build session, optionally with an objective task.
func (l *Launcher) Build(objective *model.Task) (Outcome, error) {
	if err := l.Adapter.Available(); err != nil {
		return Outcome{}, err
	}
	s := model.Session{Role: "build", NativeSessionID: model.NewUUID(), Name: "hopback build"}
	if objective != nil {
		s.ObjectiveTaskID = objective.ID
		s.Name = "hopback build: " + objective.ID + " " + truncate(objective.Title, 40)
	}
	return l.run(s, agent.LaunchSpec{RoleInstructions: protocol.BuildInstructions(), OpeningPrompt: protocol.BuildPrompt(objective)})
}

// Reopen resumes the agent's own conversation for a previous build session.
func (l *Launcher) Reopen(prev model.Session) (Outcome, error) {
	if err := l.Adapter.Available(); err != nil {
		return Outcome{}, err
	}
	s := model.Session{Role: "build", NativeSessionID: prev.NativeSessionID, ResumedFrom: prev.ID,
		ObjectiveTaskID: prev.ObjectiveTaskID, Name: "hopback build (reopened)"}
	return l.run(s, agent.LaunchSpec{ResumeNativeID: prev.NativeSessionID, RoleInstructions: protocol.BuildInstructions(),
		OpeningPrompt: protocol.ReopenPrompt()})
}

// Concierge starts a fresh planning session, primed with what changed since the last one.
func (l *Launcher) Concierge() (Outcome, error) {
	if err := l.Adapter.Available(); err != nil {
		return Outcome{}, err
	}
	last, err := l.Sessions.LastConcierge()
	if err != nil {
		return Outcome{}, err
	}
	var changes []string
	if last != nil {
		evs, err := l.Store.Events()
		if err != nil {
			return Outcome{}, err
		}
		since := last.StartedAt
		if last.EndedAt != nil {
			since = *last.EndedAt
		}
		changes = brief.ChangesSince(evs, since, 30)
	}
	s := model.Session{Role: "concierge", NativeSessionID: model.NewUUID(), Name: "hopback concierge: " + l.Store.Project.Name}
	return l.run(s, agent.LaunchSpec{RoleInstructions: protocol.ConciergeInstructions(),
		OpeningPrompt: protocol.ConciergePrompt(changes, last == nil)})
}

func (l *Launcher) run(s model.Session, spec agent.LaunchSpec) (Outcome, error) {
	now := l.Store.Now().UTC()
	s.ID, s.Status, s.Launched, s.Agent, s.StartedAt = model.NewSessionID(now), "active", true, l.Adapter.Name(), now

	// Hold the run lock from before the record exists until after it is
	// finished, so another hopback process never takes this session for dead.
	lockPath := session.RunLockPath(l.Store.Root, s.ID)
	lock, ok, err := fsutil.TryLock(lockPath)
	if err != nil {
		return Outcome{}, err
	}
	if !ok {
		return Outcome{}, fmt.Errorf("session %s is already running", s.ID)
	}
	defer func() {
		lock.Release()
		os.Remove(lockPath)
	}()
	if err := l.Sessions.Create(s); err != nil {
		return Outcome{}, err
	}

	spec.Role, spec.ProjectDir, spec.SessionName = s.Role, l.Store.Root, s.Name
	spec.Env = append(spec.Env, "HOPBACK_SESSION="+s.ID)
	if spec.ResumeNativeID == "" {
		spec.NativeID = s.NativeSessionID
	}
	release := guardInterrupt()
	cmd := l.Adapter.Command(spec)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = l.Stdin, l.Stdout, l.Stderr
	runErr := cmd.Run()
	release()
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		runErr = nil // the agent's exit code is not Hopback's concern
	}

	var out Outcome
	final, err := l.Sessions.Update(s.ID, func(cur *model.Session) []model.Event {
		end := l.Store.Now().UTC()
		cur.EndedAt = &end
		attr := model.Attribution{Session: cur.ID, Agent: cur.Agent}
		switch {
		case cur.Role == "concierge":
			cur.Status = "completed"
			return []model.Event{l.Store.NewEvent(model.EvSessionCompleted, attr, nil)}
		case cur.Status == "parked":
			out.Parked = true
			return nil
		default:
			cur.Status = "interrupted"
			return []model.Event{l.Store.NewEvent(model.EvSessionEndedWithoutHandoff, attr, nil)}
		}
	})
	if err != nil {
		return out, err
	}
	out.Session = final
	if s.Role == "concierge" {
		if out.NextSet, err = l.nextSetBy(s.ID); err != nil {
			return out, err
		}
	}
	if runErr != nil {
		return out, fmt.Errorf("could not run %s: %w", l.Adapter.DisplayName(), runErr)
	}
	return out, nil
}

// nextSetBy returns the recommended next task if session id recorded one.
func (l *Launcher) nextSetBy(id string) (*model.Pick, error) {
	evs, err := l.Store.Events()
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		if e.Type == model.EvNextSet && e.Session == id {
			snap, err := l.Store.Load()
			if err != nil {
				return nil, err
			}
			return snap.State.RecommendedNext, nil
		}
	}
	return nil, nil
}
