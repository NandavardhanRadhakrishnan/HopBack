// Package protocol holds the static, versioned text Hopback ships to agents:
// the HOPBACK.md protocol, the park skill, the pointer block and the role
// instructions and opening prompts. None of it is generated.
package protocol

import (
	_ "embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
)

// TemplateVersion is bumped whenever HOPBACK.md or the park skill changes, so
// installed copies are refreshed.
const TemplateVersion = 1

//go:embed templates/HOPBACK.md
var hopbackMD []byte

//go:embed templates/park-skill.md
var parkSkill []byte

var markerRe = regexp.MustCompile(`<!-- hopback-template v(\d+) -->`)

func Marker() string { return fmt.Sprintf("<!-- hopback-template v%d -->", TemplateVersion) }

// ParseMarkerVersion finds a Hopback template marker anywhere in b.
func ParseMarkerVersion(b []byte) (int, bool) {
	m := markerRe.FindSubmatch(b)
	if m == nil {
		return 0, false
	}
	v, err := strconv.Atoi(string(m[1]))
	return v, err == nil
}

func HopbackMD() []byte { return hopbackMD }
func ParkSkill() []byte { return parkSkill }

const (
	PointerBegin = "<!-- hopback:begin -->"
	PointerEnd   = "<!-- hopback:end -->"
)

// PointerBlock is the marked block added to CLAUDE.md and AGENTS.md.
func PointerBlock() string {
	return PointerBegin + "\n## Hopback\n\n" +
		"This project uses Hopback for project continuity. Read `.hopback/HOPBACK.md` at the start of a session and follow it: record project-level changes with the `hopback` command, and when the developer asks to park, follow its parking procedure.\n" +
		PointerEnd + "\n"
}

func BuildInstructions() string {
	return "You are a build session in a project managed by Hopback. Follow .hopback/HOPBACK.md: record meaningful project-level changes with the `hopback` command as you work (ask the developer before recording your own ideas), and when the developer types /park or asks to park, follow its parking procedure. Never claim the session is parked unless `hopback session park` succeeded."
}

func ConciergeInstructions() string {
	return "You are the Hopback concierge for this project: a planning session. Do not change project code. Help the developer decide what to build next using the backlog, decisions and latest handoff. Change project state only through the `hopback` command (use `hopback batch` for bulk changes) and never edit .hopback JSON by hand. Write down anything worth keeping (priorities, decisions, reasons for postponing) as it happens, because the next concierge starts fresh and only sees the files. Record the chosen next task with `hopback next set`."
}

const readFirst = "Read .hopback/HOPBACK.md and run `hopback context`."

// BuildPrompt is the opening prompt of a fresh build session.
func BuildPrompt(objective *model.Task) string {
	p := readFirst + " Give me a short brief of where things stand, bottom line first. "
	if objective == nil {
		return p + "Then ask me what to work on."
	}
	return p + fmt.Sprintf("Then work on %s: %s.", objective.ID, objective.Title)
}

// ReopenPrompt is the opening prompt when a build conversation is resumed.
func ReopenPrompt() string {
	return "Welcome back. Things may have changed since this conversation: run `hopback context`, then give me a short brief of where we were and what has changed."
}

// ConciergePrompt is the opening prompt of a concierge session. changes is the
// list of what happened since the last concierge session, computed by Hopback.
func ConciergePrompt(changes []string, firstTime bool) string {
	var b strings.Builder
	b.WriteString(readFirst + "\n\n")
	switch {
	case firstTime:
		b.WriteString("This is the first concierge session for this project.\n")
	case len(changes) > 0:
		b.WriteString("Changes since the last concierge session:\n")
		for _, c := range changes {
			b.WriteString("- " + c + "\n")
		}
	default:
		b.WriteString("No changes since the last concierge session.\n")
	}
	b.WriteString("\nBrief me like a consultant briefing an executive: bottom line, what needs my decision, and what you recommend building next with a reason and a couple of alternatives. Then help me decide, and record the outcome with `hopback next set`.")
	return b.String()
}
