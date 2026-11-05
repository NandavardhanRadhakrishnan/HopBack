package launcher

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/agent/agenttest"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/project"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/session"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/store"
)

func TestMain(m *testing.M) {
	if agenttest.IsHelper() {
		agenttest.RunHelper()
		return
	}
	os.Exit(m.Run())
}

func newLauncher(t *testing.T, scenarios ...string) (*Launcher, *agenttest.Fake) {
	t.Helper()
	root := t.TempDir()
	if _, _, err := project.Init(root, project.InitOptions{Name: "Docslet"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Apply(model.Attribution{Agent: "user"}, store.TaskAdd{Title: "Auth", Origin: "user_requested"}); err != nil {
		t.Fatal(err)
	}
	f := &agenttest.Fake{Scenarios: scenarios}
	m := session.NewManager(st, func(string) string { return "" }, session.Detector{Agent: f.Name(), Detect: agenttest.Detect})
	return &Launcher{Store: st, Sessions: m, Adapter: f, Stdout: io.Discard, Stderr: io.Discard}, f
}

func mustTask(t *testing.T, l *Launcher, id string) model.Task {
	t.Helper()
	snap, err := l.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range snap.Backlog.Items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("no task %s", id)
	return model.Task{}
}

func lastEventType(st *store.Store) string {
	evs, _ := st.Events()
	if len(evs) == 0 {
		return ""
	}
	return evs[len(evs)-1].Type
}

func TestBuild_Parked(t *testing.T) {
	l, f := newLauncher(t, "park")
	task := mustTask(t, l, "T-1")
	out, err := l.Build(&task)
	if err != nil || !out.Parked || out.Session.Status != "parked" || out.Session.EndedAt == nil {
		t.Fatal(out, err)
	}
	sp := f.Specs[0]
	if sp.Role != "build" || sp.NativeID == "" || sp.SessionName != "hopback build: T-1 Auth" || !strings.Contains(sp.OpeningPrompt, "T-1: Auth") ||
		!slices.Contains(sp.Env, "HOPBACK_SESSION="+out.Session.ID) {
		t.Fatal(sp)
	}
}

func TestBuild_ExitWithoutPark(t *testing.T) {
	l, _ := newLauncher(t, "exit")
	out, _ := l.Build(nil)
	if out.Parked || out.Session.Status != "interrupted" || lastEventType(l.Store) != "session_ended_without_handoff" {
		t.Fatal(out)
	}
}

func TestConcierge_NextSetAndFirstTime(t *testing.T) {
	l, f := newLauncher(t, "next")
	out, _ := l.Concierge()
	if out.Session.Role != "concierge" || out.Session.Status != "completed" || out.NextSet == nil || out.NextSet.TaskID != "T-1" {
		t.Fatal(out)
	}
	if f.Specs[0].SessionName != "hopback concierge: Docslet" || !strings.Contains(f.Specs[0].OpeningPrompt, "first concierge session") {
		t.Fatal(f.Specs[0])
	}
}

func TestConcierge_ListsChangesSinceLast(t *testing.T) {
	l, f := newLauncher(t, "exit", "exit")
	if _, err := l.Concierge(); err != nil {
		t.Fatal(err)
	}
	l.Store.Now = func() time.Time { return time.Now().Add(time.Minute) }
	if _, err := l.Store.Apply(model.Attribution{Agent: "x"}, store.TaskAdd{Title: "Later", Origin: "user_requested"}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Concierge(); err != nil {
		t.Fatal(err)
	}
	if p := f.Specs[1].OpeningPrompt; !strings.Contains(p, `task_created T-2 "Later"`) || strings.Contains(p, "first concierge session") {
		t.Fatal(p)
	}
}

func TestReopen_UsesNativeID(t *testing.T) {
	l, f := newLauncher(t, "exit")
	prev := model.Session{ID: "p", Role: "build", NativeSessionID: "n1", Status: "parked", StartedAt: time.Now()}
	if err := l.Sessions.Create(prev); err != nil {
		t.Fatal(err)
	}
	out, _ := l.Reopen(prev)
	if f.Specs[0].ResumeNativeID != "n1" || out.Session.NativeSessionID != "n1" || out.Session.ResumedFrom != "p" {
		t.Fatal(f.Specs[0], out)
	}
}

func TestLaunch_AgentUnavailable(t *testing.T) {
	l, f := newLauncher(t)
	f.Unavailable = errors.New("no agent")
	if _, err := l.Build(nil); err == nil {
		t.Fatal("want error")
	}
	if s, _ := l.Sessions.List(); len(s) != 0 {
		t.Fatal("session created despite unavailable agent")
	}
}

func TestRunLockHeldDuringSession(t *testing.T) {
	l, _ := newLauncher(t, "checklock")
	if _, err := l.Build(nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(l.Store.Root, "lockcheck")); string(b) != "held" {
		t.Fatal(string(b))
	}
}
