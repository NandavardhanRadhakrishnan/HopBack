// Package claude is the Claude Code adapter.
package claude

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/agent"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/fsutil"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/protocol"
)

type Claude struct {
	LookPath func(string) (string, error)
}

func New() *Claude { return &Claude{LookPath: exec.LookPath} }

func (c *Claude) Name() string        { return "claude-code" }
func (c *Claude) DisplayName() string { return "Claude Code" }

func (c *Claude) Available() error {
	if _, err := c.LookPath("claude"); err != nil {
		return fmt.Errorf("Claude Code (`claude`) was not found on your PATH. Install it or add it to PATH, then try again. (%v)", err)
	}
	return nil
}

// Command builds the claude invocation. It passes only session identity,
// name, role instructions and the opening prompt; the user's own defaults
// (model, permission mode, effort, tools) are left alone.
func (c *Claude) Command(spec agent.LaunchSpec) *exec.Cmd {
	path, err := c.LookPath("claude")
	if err != nil {
		path = "claude"
	}
	var args []string
	if spec.ResumeNativeID != "" {
		args = append(args, "--resume", spec.ResumeNativeID)
	} else {
		args = append(args, "--session-id", spec.NativeID)
	}
	if spec.SessionName != "" {
		args = append(args, "--name", spec.SessionName)
	}
	if spec.RoleInstructions != "" {
		args = append(args, "--append-system-prompt", spec.RoleInstructions)
	}
	if spec.OpeningPrompt != "" {
		args = append(args, spec.OpeningPrompt)
	}
	cmd := exec.Command(path, args...)
	cmd.Dir = spec.ProjectDir
	cmd.Env = append(os.Environ(), spec.Env...)
	return cmd
}

// DetectSession reads CLAUDE_CODE_SESSION_ID, which Claude Code exposes to
// commands its agent runs (observed in 2.1.286; undocumented, best-effort).
func (c *Claude) DetectSession(env func(string) string) (string, bool) {
	id := env("CLAUDE_CODE_SESSION_ID")
	return id, id != ""
}

// Install writes the project-level /park skill, refreshing an older
// Hopback-owned copy and never touching a file Hopback didn't write.
func (c *Claude) Install(projectRoot string) error {
	p := filepath.Join(projectRoot, ".claude", "skills", "park", "SKILL.md")
	b, err := os.ReadFile(p)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		v, ok := protocol.ParseMarkerVersion(b)
		if !ok {
			return fmt.Errorf("%s: %w", p, agent.ErrNotOwned)
		}
		if v >= protocol.TemplateVersion {
			return nil
		}
	}
	return fsutil.WriteFileAtomic(p, protocol.ParkSkill(), 0o644)
}
