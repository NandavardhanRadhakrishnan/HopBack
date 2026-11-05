package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/agent"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/brief"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/launcher"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/project"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/registry"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/session"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/store"
)

func addHumanCommands(root *cobra.Command, a *App) {
	root.RunE = func(cmd *cobra.Command, args []string) error { return a.interactive() }
	root.AddCommand(a.initCmd(), a.projectsCmd(), a.statusCmd(), a.sessionsCmd(), a.resumeCmd(), a.conciergeCmd())
}

// openHuman opens a project for a human-facing command: it first ends
// sessions whose launcher is gone, so what follows reads current statuses.
func (a *App) openHuman(projectArg string) (*projectCtx, error) {
	pc, err := a.open(projectArg)
	if err != nil {
		return nil, err
	}
	if _, err := pc.Sessions.Reconcile(); err != nil {
		return nil, err
	}
	return pc, nil
}

func (a *App) briefInput(pc *projectCtx) (brief.Input, error) {
	snap, err := pc.Store.Load()
	if err != nil {
		return brief.Input{}, err
	}
	sessions, err := pc.Sessions.List()
	if err != nil {
		return brief.Input{}, err
	}
	return brief.Input{Project: pc.Store.Project, Snapshot: snap, Sessions: sessions, Now: a.Now(),
		AgentNames: map[string]string{a.Adapter.Name(): a.Adapter.DisplayName()}}, nil
}

func (a *App) launcherFor(pc *projectCtx) *launcher.Launcher {
	l := &launcher.Launcher{Store: pc.Store, Sessions: pc.Sessions, Adapter: a.Adapter, Stdout: a.Out, Stderr: a.Err}
	// Hand the agent the real terminal. Any other reader is shared with
	// Hopback's own prompts and must not be drained by the child process.
	if f, ok := a.In.(*os.File); ok {
		l.Stdin = f
	}
	return l
}

func (a *App) initCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up Hopback in this directory (safe to run again)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := filepath.Abs(a.WorkDir)
			if err != nil {
				return err
			}
			p, created, err := project.Init(root, project.InitOptions{Name: name, Installers: []func(string) error{a.Adapter.Install}}, a.Now())
			if err != nil && !errors.Is(err, agent.ErrNotOwned) {
				return err
			}
			if err != nil {
				fmt.Fprintf(a.Err, "warning: %v\n", err)
			}
			if err := registry.Register(a.Home, registry.Entry{ID: p.ID, Name: p.Name, Path: root, RegisteredAt: a.Now().UTC()}); err != nil {
				return err
			}
			if created {
				fmt.Fprintf(a.Out, "Initialized Hopback in %s\n", root)
			} else {
				fmt.Fprintf(a.Out, "Hopback already initialized in %s\n", root)
			}
			fmt.Fprintf(a.Out, "Registered as %q.\n", p.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "project name (default: directory name)")
	return cmd
}

// summarize opens a registered project directly (without refreshing its
// templates) and returns its one-row summary.
func (a *App) summarize(e registry.Entry) (brief.ProjectSummary, error) {
	st, err := store.Open(e.Path)
	if err != nil {
		return brief.ProjectSummary{}, err
	}
	st.Now = a.Now
	pc := &projectCtx{Root: e.Path, Store: st, Sessions: session.NewManager(st, a.Env)}
	if _, err := pc.Sessions.Reconcile(); err != nil {
		return brief.ProjectSummary{}, err
	}
	in, err := a.briefInput(pc)
	if err != nil {
		return brief.ProjectSummary{}, err
	}
	return brief.Summary(in), nil
}

func missing(e registry.Entry) bool {
	_, err := os.Stat(filepath.Join(e.Path, project.Dir, "project.json"))
	return err != nil
}

func (a *App) projectsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "projects",
		Short: "One line per registered project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := registry.Load(a.Home)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintln(a.Out, "No projects yet. Run `hopback init` in a repo.")
				return nil
			}
			for _, e := range entries {
				if missing(e) {
					fmt.Fprintf(a.Out, "%-20s(missing: %s)\n", e.Name, e.Path)
					continue
				}
				s, err := a.summarize(e)
				if err != nil {
					fmt.Fprintf(a.Out, "%-20s(error: %v)\n", e.Name, err)
					continue
				}
				last := s.LastWorked
				if last == "" {
					last = "-"
				}
				fmt.Fprintf(a.Out, "%-20s%-10s%-14s%d open  %d to review  %s\n", e.Name, s.Status, last, s.Open, s.NeedsReview, s.Headline)
			}
			return nil
		},
	}
}

func (a *App) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [project]",
		Short: "Print the status brief",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := a.openHuman(argOr(args))
			if err != nil {
				return err
			}
			in, err := a.briefInput(pc)
			if err != nil {
				return err
			}
			brief.Render(a.Out, brief.Build(in))
			return nil
		},
	}
}

func argOr(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

func (a *App) sessionsCmd() *cobra.Command {
	var concierge bool
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List build sessions (or concierge sessions), newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := a.openHuman("")
			if err != nil {
				return err
			}
			all, err := pc.Sessions.List()
			if err != nil {
				return err
			}
			snap, err := pc.Store.Load()
			if err != nil {
				return err
			}
			role := model.Role("build")
			if concierge {
				role = "concierge"
			}
			list := []model.Session{}
			for _, s := range all {
				if s.Role == role {
					list = append(list, s)
				}
			}
			sort.SliceStable(list, func(i, j int) bool { return list[i].StartedAt.After(list[j].StartedAt) })
			if a.jsonOut {
				return a.printJSON(list)
			}
			for _, s := range list {
				what := s.Name
				if t := findTask(snap.Backlog.Items, s.ObjectiveTaskID); t != nil {
					what = t.ID + " " + t.Title
				}
				fmt.Fprintf(a.Out, "%s  %-12s%-12s%s\n", s.StartedAt.Local().Format("2006-01-02 15:04"), s.Status, s.Agent, what)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&concierge, "concierge", false, "list concierge sessions instead")
	return cmd
}

func findTask(items []model.Task, id string) *model.Task {
	if id == "" {
		return nil
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

// objective is the task a fresh build session should work on: the current
// feature when mid-feature, else the recommended next task, else none.
func objective(snap store.Snapshot) *model.Task {
	st := snap.State
	if st.Mode == "mid_feature" && st.CurrentFeature != nil {
		if t := findTask(snap.Backlog.Items, st.CurrentFeature.TaskID); t != nil {
			return t
		}
	}
	if st.RecommendedNext != nil {
		return findTask(snap.Backlog.Items, st.RecommendedNext.TaskID)
	}
	return nil
}

func (a *App) resumeCmd() *cobra.Command {
	var fresh bool
	cmd := &cobra.Command{
		Use:   "resume [project]",
		Short: "Reopen the last build conversation, or start a fresh build session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := a.openHuman(argOr(args))
			if err != nil {
				return err
			}
			if !fresh {
				last, err := pc.Sessions.LatestBuild()
				if err != nil {
					return err
				}
				if last != nil && last.NativeSessionID != "" {
					out, err := a.launcherFor(pc).Reopen(*last)
					return a.afterSession(pc, out, err)
				}
				fmt.Fprintln(a.Out, "The last build session can't be reopened; starting a fresh build session.")
			}
			return a.startBuild(pc)
		},
	}
	cmd.Flags().BoolVar(&fresh, "fresh", false, "start a fresh build session seeded from the latest handoff")
	return cmd
}

func (a *App) startBuild(pc *projectCtx) error {
	snap, err := pc.Store.Load()
	if err != nil {
		return err
	}
	out, err := a.launcherFor(pc).Build(objective(snap))
	return a.afterSession(pc, out, err)
}

func (a *App) conciergeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "concierge [project]",
		Short: "Start a concierge (planning) session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := a.openHuman(argOr(args))
			if err != nil {
				return err
			}
			out, err := a.launcherFor(pc).Concierge()
			return a.afterSession(pc, out, err)
		},
	}
}
