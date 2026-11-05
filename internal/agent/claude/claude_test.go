package claude

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/agent"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/protocol"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newClaude() *Claude {
	return &Claude{LookPath: func(string) (string, error) { return "claude", nil }}
}

func TestCommand_Fresh(t *testing.T) {
	cmd := newClaude().Command(agent.LaunchSpec{NativeID: "u1", SessionName: "hopback build: T-12 Diff view", RoleInstructions: "R", OpeningPrompt: "P", ProjectDir: "/p", Env: []string{"HOPBACK_SESSION=s1"}})
	if !reflect.DeepEqual(cmd.Args[1:], []string{"--session-id", "u1", "--name", "hopback build: T-12 Diff view", "--append-system-prompt", "R", "P"}) {
		t.Fatal(cmd.Args)
	}
	if cmd.Dir != "/p" || !slices.Contains(cmd.Env, "HOPBACK_SESSION=s1") {
		t.Fatal(cmd.Dir)
	}
}

func TestCommand_ResumeNoPrompt(t *testing.T) {
	cmd := newClaude().Command(agent.LaunchSpec{ResumeNativeID: "u0", SessionName: "n"})
	if !reflect.DeepEqual(cmd.Args[1:], []string{"--resume", "u0", "--name", "n"}) {
		t.Fatal(cmd.Args)
	}
}

func TestDetectSession(t *testing.T) {
	c := newClaude()
	if id, ok := c.DetectSession(func(k string) string {
		if k == "CLAUDE_CODE_SESSION_ID" {
			return "abc"
		}
		return ""
	}); id != "abc" || !ok {
		t.Fatal(id)
	}
	if _, ok := c.DetectSession(func(string) string { return "" }); ok {
		t.Fatal("detected from empty env")
	}
}

func TestInstall(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, ".claude", "skills", "park", "SKILL.md")
	c := newClaude()
	must(t, c.Install(root))
	if got, _ := os.ReadFile(p); !bytes.Equal(got, protocol.ParkSkill()) {
		t.Fatal("not written")
	}
	os.WriteFile(p, []byte("---\nname: park\n---\n<!-- hopback-template v0 -->"), 0o644)
	must(t, c.Install(root))
	if got, _ := os.ReadFile(p); !bytes.Equal(got, protocol.ParkSkill()) {
		t.Fatal("older owned file not refreshed")
	}
	os.WriteFile(p, []byte("my own park skill"), 0o644)
	if err := c.Install(root); !errors.Is(err, agent.ErrNotOwned) {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); string(got) != "my own park skill" {
		t.Fatal("foreign file overwritten")
	}
}

func TestAvailable_Missing(t *testing.T) {
	c := &Claude{LookPath: func(string) (string, error) { return "", exec.ErrNotFound }}
	if err := c.Available(); err == nil || !strings.Contains(err.Error(), "Claude Code (`claude`) was not found on your PATH") {
		t.Fatal(err)
	}
}
