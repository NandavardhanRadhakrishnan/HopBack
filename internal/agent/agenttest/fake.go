// Package agenttest provides a fake agent adapter for tests. Its sessions run
// the test binary itself as a helper process that plays a scripted scenario
// against the real store and session manager.
//
// A test package that launches sessions must start its TestMain with:
//
//	if agenttest.IsHelper() { agenttest.RunHelper(); return }
package agenttest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/agent"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/fsutil"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/session"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/store"
)

const Handoff = "## Objective\nx\n## Done\nx\n## Stopped at\nx\n## Unfinished\nnone\n## Gotchas\nnone\n## Next step\nx\n"

// Fake records what it was asked to launch. Each launch plays the next
// scenario: "exit", "park", "next" or "checklock" (default "exit").
type Fake struct {
	Scenarios   []string
	Specs       []agent.LaunchSpec
	Unavailable error
	Installs    []string
}

func (f *Fake) Name() string        { return "fake-agent" }
func (f *Fake) DisplayName() string { return "Fake Agent" }
func (f *Fake) Available() error    { return f.Unavailable }

func (f *Fake) Command(spec agent.LaunchSpec) *exec.Cmd {
	f.Specs = append(f.Specs, spec)
	s := "exit"
	if len(f.Scenarios) > 0 {
		s, f.Scenarios = f.Scenarios[0], f.Scenarios[1:]
	}
	native := spec.NativeID
	if spec.ResumeNativeID != "" {
		native = spec.ResumeNativeID
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Dir = spec.ProjectDir
	cmd.Env = append(append(os.Environ(), spec.Env...),
		"HOPBACK_AGENTTEST_HELPER=1", "HOPBACK_AGENTTEST_SCENARIO="+s, "FAKE_AGENT_SESSION_ID="+native)
	return cmd
}

func (f *Fake) DetectSession(env func(string) string) (string, bool) { return Detect(env) }

func (f *Fake) Install(root string) error {
	f.Installs = append(f.Installs, root)
	return nil
}

// Detect is the fake agent's session detector.
func Detect(env func(string) string) (string, bool) {
	v := env("FAKE_AGENT_SESSION_ID")
	return v, v != ""
}

func IsHelper() bool { return os.Getenv("HOPBACK_AGENTTEST_HELPER") == "1" }

func fail(err error) {
	fmt.Fprintln(os.Stderr, "agenttest helper:", err)
	os.Exit(2)
}

// RunHelper plays the scenario named in the environment, then exits.
func RunHelper() {
	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	st, err := store.Open(root)
	if err != nil {
		fail(err)
	}
	m := session.NewManager(st, os.Getenv, session.Detector{Agent: "fake-agent", Detect: Detect})
	attr, cur, err := m.Current()
	if err != nil {
		fail(err)
	}
	switch s := os.Getenv("HOPBACK_AGENTTEST_SCENARIO"); s {
	case "exit":
	case "park":
		snap, err := st.Load()
		if err != nil {
			fail(err)
		}
		in := store.StateInput{Mode: "between_features", BottomLine: "Parked by fake agent."}
		if _, err := st.Apply(attr, store.StateSet{ExpectRevision: snap.State.Revision, State: in}); err != nil {
			fail(err)
		}
		if cur == nil {
			fail(fmt.Errorf("no current session"))
		}
		if err := m.Park(cur.ID, []byte(Handoff)); err != nil {
			fail(err)
		}
	case "next":
		if _, err := st.Apply(attr, store.NextSet{TaskID: "T-1", Reason: "fake pick"}); err != nil {
			fail(err)
		}
	case "checklock":
		res := "free"
		l, ok, err := fsutil.TryLock(session.RunLockPath(root, os.Getenv("HOPBACK_SESSION")))
		if err != nil {
			fail(err)
		}
		if ok {
			l.Release()
		} else {
			res = "held"
		}
		if err := os.WriteFile(filepath.Join(root, "lockcheck"), []byte(res), 0o644); err != nil {
			fail(err)
		}
	default:
		fail(fmt.Errorf("unknown scenario %q", s))
	}
	os.Exit(0)
}

var _ agent.Adapter = (*Fake)(nil)
