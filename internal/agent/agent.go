// Package agent defines the interface between Hopback's core and a specific
// coding agent. Everything agent-specific lives behind Adapter.
package agent

import (
	"errors"
	"os/exec"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
)

var ErrNotOwned = errors.New("file exists and is not managed by Hopback; left untouched")

// LaunchSpec describes one agent session to start or resume.
type LaunchSpec struct {
	Role             model.Role
	ProjectDir       string
	SessionName      string
	NativeID         string // conversation ID to assign to a fresh session
	ResumeNativeID   string // conversation ID to resume instead
	RoleInstructions string
	OpeningPrompt    string
	Env              []string
}

type Adapter interface {
	Name() string
	DisplayName() string
	// Available reports, in plain words, why the agent can't be launched.
	Available() error
	Command(spec LaunchSpec) *exec.Cmd
	// DetectSession returns the agent's native session ID when a hopback
	// command runs inside one of its sessions.
	DetectSession(env func(string) string) (string, bool)
	// Install writes agent-specific project files, such as the /park command.
	Install(projectRoot string) error
}
