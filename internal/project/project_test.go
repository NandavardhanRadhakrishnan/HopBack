package project

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/protocol"
)

var fixed = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	must(t, err)
	return string(b)
}

func readJSON(t *testing.T, p string, v any) {
	t.Helper()
	must(t, json.Unmarshal([]byte(readFile(t, p)), v))
}

func readLines(t *testing.T, p string) []string {
	t.Helper()
	return strings.Split(strings.TrimRight(readFile(t, p), "\n"), "\n")
}

func TestInit_CreatesLayout(t *testing.T) {
	root := t.TempDir()
	p, created, err := Init(root, InitOptions{Name: "Docslet"}, fixed)
	if err != nil || !created || p.Name != "Docslet" || p.SchemaVersion != 1 {
		t.Fatal(p, created, err)
	}
	for _, f := range []string{"project.json", "state.json", "backlog.json", "decisions.json", "events.jsonl", "HOPBACK.md", ".gitignore", "sessions", "run"} {
		if _, err := os.Stat(filepath.Join(root, Dir, f)); err != nil {
			t.Fatal(f, err)
		}
	}
	var st model.State
	readJSON(t, filepath.Join(root, Dir, "state.json"), &st)
	if st.Revision != 0 || st.ProjectStatus != "active" {
		t.Fatal(st)
	}
	lines := readLines(t, filepath.Join(root, Dir, "events.jsonl"))
	if len(lines) != 1 || !strings.Contains(lines[0], `"project_initialized"`) {
		t.Fatal(lines)
	}
}

func TestInit_IdempotentAndPointerBlocks(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("hello\n"), 0o644)
	p1, _, _ := Init(root, InitOptions{}, fixed)
	p2, created, _ := Init(root, InitOptions{}, fixed)
	if created || p1.ID != p2.ID {
		t.Fatal("second init must reuse project")
	}
	claude := readFile(t, filepath.Join(root, "CLAUDE.md"))
	agents := readFile(t, filepath.Join(root, "AGENTS.md"))
	if strings.Count(claude, "<!-- hopback:begin -->") != 1 || strings.Count(agents, "<!-- hopback:begin -->") != 1 || !strings.HasPrefix(agents, "hello") {
		t.Fatal(claude, agents)
	}
}

func TestInit_NoAgentsMdCreated(t *testing.T) {
	root := t.TempDir()
	Init(root, InitOptions{}, fixed)
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("AGENTS.md must not be created")
	}
}

func TestUpsertPointerBlock_ReplacesExisting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "CLAUDE.md")
	os.WriteFile(p, []byte("top\n<!-- hopback:begin -->\nOLD\n<!-- hopback:end -->\nbottom\n"), 0o644)
	must(t, UpsertPointerBlock(p, false))
	s := readFile(t, p)
	if strings.Contains(s, "OLD") || !strings.HasPrefix(s, "top\n") || !strings.HasSuffix(s, "bottom\n") {
		t.Fatal(s)
	}
}

func TestInit_RunsInstallers(t *testing.T) {
	var got []string
	root := t.TempDir()
	Init(root, InitOptions{Installers: []func(string) error{func(r string) error { got = append(got, r); return nil }}}, fixed)
	if len(got) != 1 || got[0] != root {
		t.Fatal(got)
	}
}

func TestFind_WalksUp(t *testing.T) {
	root := t.TempDir()
	Init(root, InitOptions{}, fixed)
	sub := filepath.Join(root, "a", "b")
	os.MkdirAll(sub, 0o755)
	if got, err := Find(sub); err != nil || got != root {
		t.Fatal(got, err)
	}
}

func TestFind_NotProject(t *testing.T) {
	_, err := Find(t.TempDir())
	if !errors.Is(err, ErrNotProject) || !strings.Contains(err.Error(), "hopback init") {
		t.Fatal(err)
	}
}

func TestEnsureTemplates(t *testing.T) {
	root := t.TempDir()
	Init(root, InitOptions{}, fixed)
	md := filepath.Join(root, Dir, "HOPBACK.md")
	os.WriteFile(md, []byte("<!-- hopback-template v0 -->"), 0o644)
	must(t, EnsureTemplates(root))
	if !strings.Contains(readFile(t, md), protocol.Marker()) {
		t.Fatal("not refreshed")
	}
	os.WriteFile(md, []byte("<!-- hopback-template v999 -->"), 0o644)
	must(t, EnsureTemplates(root))
	if readFile(t, md) != "<!-- hopback-template v999 -->" {
		t.Fatal("downgraded")
	}
}
