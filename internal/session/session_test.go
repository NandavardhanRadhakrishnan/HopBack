package session

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/fsutil"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/project"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/store"
)

var fixed = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

const goodHandoff = "## Objective\nx\n## Done\nx\n## Stopped at\nx\n## Unfinished\nnone\n## Gotchas\nnone\n## Next step\nx\n"

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mapEnv(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

func newManager(t *testing.T, env map[string]string) *Manager {
	t.Helper()
	root := t.TempDir()
	if _, _, err := project.Init(root, project.InitOptions{Name: "Docslet"}, fixed); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(root)
	must(t, err)
	st.Now = func() time.Time { return fixed }
	return NewManager(st, mapEnv(env), Detector{Agent: "fake-agent", Detect: func(e func(string) string) (string, bool) {
		v := e("FAKE_AGENT_SESSION_ID")
		return v, v != ""
	}})
}

func lastEventType(m *Manager) string {
	evs, _ := m.Store.Events()
	if len(evs) == 0 {
		return ""
	}
	return evs[len(evs)-1].Type
}

func TestCreateAndList(t *testing.T) {
	m := newManager(t, nil)
	must(t, m.Create(model.Session{ID: "b", Role: "build", StartedAt: fixed.Add(time.Hour)}))
	must(t, m.Create(model.Session{ID: "a", Role: "build", StartedAt: fixed}))
	l, _ := m.List()
	if len(l) != 2 || l[0].ID != "a" {
		t.Fatal(l)
	}
}

func TestCurrent_HopbackSessionEnv(t *testing.T) {
	m := newManager(t, map[string]string{"HOPBACK_SESSION": "s1"})
	must(t, m.Create(model.Session{ID: "s1", Role: "build", Agent: "fake-agent", Status: "active", StartedAt: fixed}))
	if a, s, _ := m.Current(); a.Session != "s1" || a.Agent != "fake-agent" || s.ID != "s1" {
		t.Fatal(a)
	}
}

func TestCurrent_StaleHopbackSessionFallsBack(t *testing.T) {
	m := newManager(t, map[string]string{"HOPBACK_SESSION": "bogus", "FAKE_AGENT_SESSION_ID": "n1"})
	a, s, err := m.Current()
	if err != nil || s == nil || s.NativeSessionID != "n1" || s.Launched || s.Role != "build" || s.Status != "active" || a.Agent != "fake-agent" {
		t.Fatal(a, s, err)
	}
}

func TestCurrent_NativeMatchesLatest(t *testing.T) {
	m := newManager(t, map[string]string{"FAKE_AGENT_SESSION_ID": "n1"})
	must(t, m.Create(model.Session{ID: "old", NativeSessionID: "n1", Role: "build", Status: "parked", StartedAt: fixed}))
	must(t, m.Create(model.Session{ID: "new", NativeSessionID: "n1", Role: "build", Status: "active", StartedAt: fixed.Add(time.Hour)}))
	if a, _, _ := m.Current(); a.Session != "new" {
		t.Fatal(a)
	}
}

func TestCurrent_NoSignals(t *testing.T) {
	m := newManager(t, nil)
	a, s, err := m.Current()
	l, _ := m.List()
	if err != nil || s != nil || a.Agent != "unknown" || len(l) != 0 {
		t.Fatal(a, s, err, l)
	}
}

func TestCurrent_RevivesInterrupted(t *testing.T) {
	m := newManager(t, map[string]string{"HOPBACK_SESSION": "s1"})
	must(t, m.Create(model.Session{ID: "s1", Role: "build", Status: "interrupted", StartedAt: fixed}))
	_, s, _ := m.Current()
	if s.Status != "active" || lastEventType(m) != "session_revived" {
		t.Fatal(s.Status)
	}
	if got, _ := m.Get("s1"); got.Status != "active" {
		t.Fatal("revival not saved")
	}
}

func TestPark_Success(t *testing.T) {
	m := newManager(t, nil)
	must(t, m.Create(model.Session{ID: "s1", Role: "build", Status: "active", StartedAt: fixed}))
	if _, err := m.Store.Apply(model.Attribution{Session: "s1", Agent: "fake-agent"}, store.StateSet{State: store.StateInput{Mode: "between_features", BottomLine: "Done."}}); err != nil {
		t.Fatal(err)
	}
	must(t, m.Park("s1", []byte(goodHandoff)))
	s, _ := m.Get("s1")
	md, _ := os.ReadFile(m.Store.Path("sessions/s1.md"))
	if s.Status != "parked" || s.ParkedAt == nil || s.Handoff != "sessions/s1.md" || string(md) != goodHandoff || lastEventType(m) != "session_parked" {
		t.Fatal(s)
	}
}

func TestPark_ReportsEverythingMissing(t *testing.T) {
	m := newManager(t, nil)
	must(t, m.Create(model.Session{ID: "s1", Role: "build", Status: "active", StartedAt: fixed}))
	err := m.Park("s1", []byte(strings.Replace(goodHandoff, "## Gotchas\nnone\n", "", 1)))
	var pe *ParkError
	if !errors.As(err, &pe) || !reflect.DeepEqual(pe.Missing, []string{"handoff section: Gotchas", "project state not updated in this session (run `hopback state set`)"}) {
		t.Fatal(err)
	}
	if s, _ := m.Get("s1"); s.Status != "active" {
		t.Fatal("status changed on failed park")
	}
	if _, err := os.Stat(m.Store.Path("sessions/s1.md")); !os.IsNotExist(err) {
		t.Fatal("handoff written on failed park")
	}
}

func TestPark_AcceptsCRLFAndCase(t *testing.T) {
	m := newManager(t, nil)
	must(t, m.Create(model.Session{ID: "s1", Role: "build", Status: "active", StartedAt: fixed}))
	if _, err := m.Store.Apply(model.Attribution{Session: "s1"}, store.StateSet{State: store.StateInput{Mode: "between_features", BottomLine: "x"}}); err != nil {
		t.Fatal(err)
	}
	h := strings.ReplaceAll(strings.Replace(goodHandoff, "## Stopped at", "### stopped AT  ", 1), "\n", "\r\n")
	must(t, m.Park("s1", []byte(h)))
}

func TestPark_RejectsConcierge(t *testing.T) {
	m := newManager(t, nil)
	must(t, m.Create(model.Session{ID: "c1", Role: "concierge", Status: "active", StartedAt: fixed}))
	if err := m.Park("c1", []byte(goodHandoff)); err == nil || !strings.Contains(err.Error(), "concierge sessions don't park") {
		t.Fatal(err)
	}
}

func TestReconcile(t *testing.T) {
	m := newManager(t, nil)
	root := m.Store.Root
	must(t, m.Create(model.Session{ID: "dead", Role: "build", Status: "active", Launched: true, StartedAt: fixed}))
	must(t, m.Create(model.Session{ID: "live", Role: "build", Status: "active", Launched: true, StartedAt: fixed}))
	must(t, m.Create(model.Session{ID: "unl", Role: "build", Status: "active", Launched: false, StartedAt: fixed}))
	l, _, _ := fsutil.TryLock(RunLockPath(root, "live"))
	defer l.Release()
	got, _ := m.Reconcile()
	dead, _ := m.Get("dead")
	live, _ := m.Get("live")
	unl, _ := m.Get("unl")
	if dead.Status != "interrupted" || dead.EndedAt == nil || live.Status != "active" || unl.Status != "interrupted" || len(got) != 2 {
		t.Fatal(dead, live, unl)
	}
}

func TestLatestBuild_IgnoresConcierge(t *testing.T) {
	m := newManager(t, nil)
	must(t, m.Create(model.Session{ID: "b", Role: "build", StartedAt: fixed}))
	must(t, m.Create(model.Session{ID: "c", Role: "concierge", StartedAt: fixed.Add(time.Hour)}))
	if s, _ := m.LatestBuild(); s.ID != "b" {
		t.Fatal(s.ID)
	}
}
