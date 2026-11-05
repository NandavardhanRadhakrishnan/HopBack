package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/registry"
)

// seedMidFeature adds task T-1 "Auth" and a mid-feature state on it.
func seedMidFeature(t *testing.T, app *App) {
	t.Helper()
	if _, e, code := run(app, "", "task", "add", "--title", "Auth", "--origin", "user"); code != 0 {
		t.Fatal(e)
	}
	st := `{"mode":"mid_feature","bottom_line":"Auth is half done.","current_feature":{"task_id":"T-1","why":"Users need accounts.","stopped_at":"Login form.","next_step":"Sessions."}}`
	if _, e, code := run(app, st, "state", "set", "--expect-revision", "0", "--file", "-"); code != 0 {
		t.Fatal(e)
	}
}

func TestInitCLI(t *testing.T) {
	app, f, _ := newApp(t, nil)
	dir := t.TempDir()
	app.WorkDir = dir
	out, e, code := run(app, "", "init", "--name", "Smoke")
	if code != 0 || !strings.Contains(out, "Initialized Hopback in") || len(f.Installs) != 1 {
		t.Fatal(out, e)
	}
	if _, err := registry.Lookup(app.Home, "smoke"); err != nil {
		t.Fatal(err)
	}
	if out, _, _ := run(app, "", "init"); !strings.Contains(out, "already initialized") {
		t.Fatal(out)
	}
}

func TestStatusCLI(t *testing.T) {
	app, _, _ := newApp(t, nil)
	run(app, `{"mode":"between_features","bottom_line":"Auth done."}`, "state", "set", "--expect-revision", "0", "--file", "-")
	if out, _, _ := run(app, "", "status"); !strings.Contains(out, "BOTTOM LINE\n  Auth done.") {
		t.Fatal(out)
	}
}

func TestProjectsCLI(t *testing.T) {
	app, _, _ := newApp(t, nil)
	registry.Register(app.Home, registry.Entry{ID: "gone", Name: "Gone", Path: filepath.Join(t.TempDir(), "nope")})
	out, _, _ := run(app, "", "projects")
	if !strings.Contains(out, "Docslet") || !strings.Contains(out, "(missing: ") {
		t.Fatal(out)
	}
}

func TestSessionsCLI(t *testing.T) {
	app, _, root := newApp(t, nil)
	m := managerFor(t, root)
	m.Create(model.Session{ID: "b", Role: "build", Status: "parked", Name: "hopback build: T-1 Auth", StartedAt: time.Now()})
	m.Create(model.Session{ID: "c", Role: "concierge", Status: "completed", Name: "hopback concierge: Docslet", StartedAt: time.Now()})
	b, _, _ := run(app, "", "sessions")
	c, _, _ := run(app, "", "sessions", "--concierge")
	if !strings.Contains(b, "T-1 Auth") || strings.Contains(b, "concierge") || !strings.Contains(c, "concierge") {
		t.Fatal(b, c)
	}
}

func TestInteractive_EmptyDefaultsToConcierge(t *testing.T) {
	app, f, _ := newApp(t, nil)
	f.Scenarios = []string{"exit"}
	run(app, "\n")
	if len(f.Specs) != 1 || f.Specs[0].Role != "concierge" {
		t.Fatal(f.Specs)
	}
}

func TestInteractive_MidFeatureBuildThenDeclineConcierge(t *testing.T) {
	app, f, _ := newApp(t, nil)
	f.Scenarios = []string{"park"}
	seedMidFeature(t, app)
	out, _, _ := run(app, "\nn\n")
	if len(f.Specs) != 1 || f.Specs[0].Role != "build" || !strings.Contains(f.Specs[0].OpeningPrompt, "T-1") ||
		!strings.Contains(out, "Parked. Talk to the concierge about what's next? [Y/n]") {
		t.Fatal(out)
	}
}

func TestInteractive_ParkedThenConciergeThenBuild(t *testing.T) {
	app, f, _ := newApp(t, nil)
	f.Scenarios = []string{"park", "next", "exit"}
	seedMidFeature(t, app)
	out, _, _ := run(app, "\n\n\n")
	if len(f.Specs) != 3 || f.Specs[1].Role != "concierge" || f.Specs[2].Role != "build" || !strings.Contains(out, "Session ended without a handoff") {
		t.Fatal(out)
	}
}

func TestInteractive_Quit(t *testing.T) {
	app, f, _ := newApp(t, nil)
	run(app, "q\n")
	if len(f.Specs) != 0 {
		t.Fatal("launched on quit")
	}
}

func TestInteractive_OutsideProjectPicker(t *testing.T) {
	app, f, _ := newApp(t, nil)
	app.WorkDir = t.TempDir()
	out, _, _ := run(app, "1\nq\n")
	if !strings.Contains(out, "1) Docslet") || !strings.Contains(out, "Docslet · status brief") || len(f.Specs) != 0 {
		t.Fatal(out)
	}
}

func TestResume_ReopenFreshAndFallback(t *testing.T) {
	app, f, root := newApp(t, nil)
	m := managerFor(t, root)
	m.Create(model.Session{ID: "b", Role: "build", Status: "parked", NativeSessionID: "n1", StartedAt: time.Now()})
	run(app, "n\n", "resume")
	run(app, "n\n", "resume", "--fresh")
	if f.Specs[0].ResumeNativeID != "n1" || f.Specs[1].ResumeNativeID != "" || f.Specs[1].NativeID == "" {
		t.Fatal(f.Specs)
	}
	m.Create(model.Session{ID: "u", Role: "build", Status: "parked", StartedAt: time.Now().Add(time.Hour)})
	if out, _, _ := run(app, "n\n", "resume"); !strings.Contains(out, "can't be reopened; starting a fresh build session") {
		t.Fatal(out)
	}
}

func TestStatus_ReconcilesDeadSession(t *testing.T) {
	app, _, root := newApp(t, nil)
	managerFor(t, root).Create(model.Session{ID: "d", Role: "build", Status: "active", Launched: true, Agent: "fake-agent", StartedAt: time.Now()})
	run(app, `{"mode":"between_features","bottom_line":"x"}`, "state", "set", "--expect-revision", "0", "--file", "-")
	if out, _, _ := run(app, "", "status"); !strings.Contains(out, "ended without a handoff") {
		t.Fatal(out)
	}
}
