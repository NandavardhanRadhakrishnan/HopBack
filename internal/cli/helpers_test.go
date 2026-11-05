package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/agent/agenttest"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/project"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/registry"
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

const goodHandoff = "## Objective\nx\n## Done\nx\n## Stopped at\nx\n## Unfinished\nnone\n## Gotchas\nnone\n## Next step\nx\n"

// newApp inits project "Docslet" in a temp dir, registers it in a temp home
// and wires the fake agent. It uses the real clock because the fake agent
// runs in a separate process.
func newApp(t *testing.T, env map[string]string) (*App, *agenttest.Fake, string) {
	t.Helper()
	root, home := t.TempDir(), t.TempDir()
	p, _, err := project.Init(root, project.InitOptions{Name: "Docslet"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(home, registry.Entry{ID: p.ID, Name: p.Name, Path: root, RegisteredAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	f := &agenttest.Fake{}
	app := &App{WorkDir: root, Home: home, Env: func(k string) string { return env[k] }, Adapter: f, Now: time.Now}
	return app, f, root
}

// run executes one hopback command line in-process.
func run(app *App, stdin string, args ...string) (stdout, stderr string, code int) {
	var out, errb bytes.Buffer
	app.In, app.Out, app.Err = strings.NewReader(stdin), &out, &errb
	code = app.Run(args)
	return out.String(), errb.String(), code
}

func readJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func loadTask(t *testing.T, root, id string) model.Task {
	t.Helper()
	var b model.Backlog
	readJSONFile(t, filepath.Join(root, project.Dir, "backlog.json"), &b)
	for _, it := range b.Items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("no task %s", id)
	return model.Task{}
}

func loadDecision(t *testing.T, root, id string) model.Decision {
	t.Helper()
	var d model.Decisions
	readJSONFile(t, filepath.Join(root, project.Dir, "decisions.json"), &d)
	for _, it := range d.Items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("no decision %s", id)
	return model.Decision{}
}

func loadState(t *testing.T, root string) model.State {
	t.Helper()
	var s model.State
	readJSONFile(t, filepath.Join(root, project.Dir, "state.json"), &s)
	return s
}

func managerFor(t *testing.T, root string) *session.Manager {
	t.Helper()
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return session.NewManager(st, func(string) string { return "" })
}
