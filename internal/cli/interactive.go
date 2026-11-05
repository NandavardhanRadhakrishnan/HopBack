package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/brief"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/launcher"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/project"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/registry"
)

// prompt prints question and reads one line. ok is false at end of input.
func (a *App) prompt(question string) (answer string, ok bool) {
	fmt.Fprint(a.Out, question)
	line, err := a.in.ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		fmt.Fprintln(a.Out)
		return "", false
	}
	return strings.TrimSpace(line), true
}

// confirm asks a [Y/n] question. Empty input means yes; end of input means no.
func (a *App) confirm(question string) bool {
	ans, ok := a.prompt(question)
	if !ok {
		return false
	}
	switch strings.ToLower(ans) {
	case "", "y", "yes":
		return true
	}
	return false
}

// interactive is `hopback` with no arguments.
func (a *App) interactive() error {
	if _, err := project.Find(a.WorkDir); errors.Is(err, project.ErrNotProject) {
		return a.pickProject()
	} else if err != nil {
		return err
	}
	pc, err := a.openHuman("")
	if err != nil {
		return err
	}
	return a.projectMenu(pc)
}

func (a *App) pickProject() error {
	entries, err := registry.Load(a.Home)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(a.Out, "No projects yet. Run `hopback init` in a repo.")
		return nil
	}
	for i, e := range entries {
		headline := "(missing)"
		if !missing(e) {
			if s, err := a.summarize(e); err == nil {
				headline = s.Headline
			} else {
				headline = "(error: " + err.Error() + ")"
			}
		}
		fmt.Fprintf(a.Out, "  %d) %s  %s\n", i+1, e.Name, headline)
	}
	for {
		ans, ok := a.prompt("Project number (q to quit): ")
		if !ok || ans == "" || strings.EqualFold(ans, "q") {
			return nil
		}
		n, err := strconv.Atoi(ans)
		if err != nil || n < 1 || n > len(entries) {
			fmt.Fprintln(a.Out, "Not a project number.")
			continue
		}
		pc, err := a.openHuman(entries[n-1].ID)
		if err != nil {
			return err
		}
		return a.projectMenu(pc)
	}
}

type menuItem struct {
	label string
	run   func() (launcher.Outcome, error)
}

// projectMenu shows the brief, then offers the next session.
func (a *App) projectMenu(pc *projectCtx) error {
	in, err := a.briefInput(pc)
	if err != nil {
		return err
	}
	b := brief.Build(in)
	brief.Render(a.Out, b)
	fmt.Fprintln(a.Out)

	l := a.launcherFor(pc)
	obj := objective(in.Snapshot)
	build := menuItem{label: "Start building", run: func() (launcher.Outcome, error) { return l.Build(obj) }}
	if obj != nil {
		build.label = "Continue building: " + obj.ID + " " + obj.Title
	}
	items := []menuItem{build}
	if last, err := pc.Sessions.LatestBuild(); err != nil {
		return err
	} else if last != nil && last.NativeSessionID != "" {
		prev := *last
		items = append(items, menuItem{
			label: fmt.Sprintf("Reopen last build conversation (%s, %s)", a.Adapter.DisplayName(), brief.Ago(a.Now(), prev.LastActivity())),
			run:   func() (launcher.Outcome, error) { return l.Reopen(prev) },
		})
	}
	items = append(items, menuItem{label: "Talk to the concierge", run: l.Concierge})
	def := len(items) // the concierge
	if b.Mode == "mid_feature" && !b.Empty {
		def = 1
	}
	for i, it := range items {
		fmt.Fprintf(a.Out, "  %d) %s\n", i+1, it.label)
	}
	fmt.Fprintln(a.Out, "  q) Quit")
	for {
		ans, ok := a.prompt(fmt.Sprintf("Choose [%d]: ", def))
		if !ok || strings.EqualFold(ans, "q") {
			return nil
		}
		n := def
		if ans != "" {
			if n, err = strconv.Atoi(ans); err != nil || n < 1 || n > len(items) {
				fmt.Fprintln(a.Out, "Choose one of the numbers above, or q.")
				continue
			}
		}
		out, err := items[n-1].run()
		return a.afterSession(pc, out, err)
	}
}

// afterSession is the post-session loop: a parked build offers the
// concierge; a concierge that set a next task offers to build it.
func (a *App) afterSession(pc *projectCtx, out launcher.Outcome, err error) error {
	l := a.launcherFor(pc)
	for {
		if err != nil {
			return err
		}
		switch {
		case out.Session.Role == "build" && out.Parked:
			if !a.confirm("Parked. Talk to the concierge about what's next? [Y/n] ") {
				return nil
			}
			out, err = l.Concierge()
		case out.Session.Role == "build":
			fmt.Fprintln(a.Out, "Session ended without a handoff. Next time, type /park before closing so the next session knows where you stopped.")
			return nil
		case out.NextSet != nil:
			snap, lerr := pc.Store.Load()
			if lerr != nil {
				return lerr
			}
			t := findTask(snap.Backlog.Items, out.NextSet.TaskID)
			if t == nil {
				return nil
			}
			if !a.confirm(fmt.Sprintf("Start building %s %s? [Y/n] ", t.ID, t.Title)) {
				return nil
			}
			out, err = l.Build(t)
		default:
			return nil
		}
	}
}
