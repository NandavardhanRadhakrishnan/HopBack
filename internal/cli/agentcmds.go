package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/store"
)

// addAgentCommands registers the commands agents call during a session.
// Their syntax is documented in the HOPBACK.md template.
func addAgentCommands(root *cobra.Command, a *App) {
	root.AddCommand(a.contextCmd())

	task := &cobra.Command{Use: "task", Short: "Add, update, list and reorder backlog items"}
	task.AddCommand(a.taskAddCmd(), a.taskUpdateCmd(), a.taskListCmd(), a.taskReorderCmd())
	decision := &cobra.Command{Use: "decision", Short: "Record and resolve decisions"}
	decision.AddCommand(a.decisionAddCmd(), a.decisionResolveCmd())
	arch := &cobra.Command{Use: "architecture", Short: "Record architectural changes"}
	arch.AddCommand(a.architectureChangeCmd())
	review := &cobra.Command{Use: "review", Short: "Clear review flags"}
	review.AddCommand(a.reviewClearCmd())
	state := &cobra.Command{Use: "state", Short: "Read or write the project state"}
	state.AddCommand(a.stateGetCmd(), a.stateSetCmd())
	next := &cobra.Command{Use: "next", Short: "Record the recommended next task"}
	next.AddCommand(a.nextSetCmd())
	sess := &cobra.Command{Use: "session", Short: "The current agent session"}
	sess.AddCommand(a.sessionParkCmd(), a.sessionCurrentCmd())
	root.AddCommand(task, decision, arch, review, state, next, a.batchCmd(), sess)
}

// apply runs ops attributed to the current session and reports success.
func (a *App) apply(pc *projectCtx, ops ...store.Op) (store.Result, error) {
	attr, _, err := pc.Sessions.Current()
	if err != nil {
		return store.Result{}, err
	}
	return pc.Store.Apply(attr, ops...)
}

func (a *App) done(res store.Result, text string) error {
	if a.jsonOut {
		created := res.Created
		if created == nil {
			created = []string{}
		}
		return a.printJSON(map[string]any{"ok": true, "created": created})
	}
	_, err := fmt.Fprintln(a.Out, text)
	return err
}

// write is the common shape of a write command: open the project, build the
// ops, apply them, print the result.
func (a *App) write(build func() ([]store.Op, error), text func(store.Result) string) error {
	pc, err := a.open("")
	if err != nil {
		return err
	}
	ops, err := build()
	if err != nil {
		return err
	}
	res, err := a.apply(pc, ops...)
	if err != nil {
		return err
	}
	return a.done(res, text(res))
}

func ok(store.Result) string { return "ok" }

func firstCreated(res store.Result) string {
	if len(res.Created) > 0 {
		return res.Created[0]
	}
	return "ok"
}

func byRank(items []model.Task) []model.Task {
	out := append([]model.Task(nil), items...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out
}

func (a *App) contextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context",
		Short: "Print everything an agent needs at session start, as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := a.open("")
			if err != nil {
				return err
			}
			snap, err := pc.Store.Load()
			if err != nil {
				return err
			}
			_, cur, err := pc.Sessions.Current()
			if err != nil {
				return err
			}
			open, review := []model.Task{}, []model.Task{}
			for _, t := range byRank(snap.Backlog.Items) {
				if t.IsOpen() {
					open = append(open, t)
				}
				if t.NeedsReview != nil {
					review = append(review, t)
				}
			}
			decisions := []model.Decision{}
			for _, d := range snap.Decisions.Items {
				if d.Status == "open" {
					decisions = append(decisions, d)
				}
			}
			handoff := ""
			if last, err := pc.Sessions.LatestParkedBuild(); err != nil {
				return err
			} else if last != nil && last.Handoff != "" {
				b, err := os.ReadFile(pc.Store.Path(last.Handoff))
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				handoff = string(b)
			}
			return a.printJSON(map[string]any{
				"project": pc.Store.Project, "state": snap.State, "open_tasks": open, "needs_review": review,
				"open_decisions": decisions, "latest_handoff": handoff, "current_session": cur,
			})
		},
	}
}

func (a *App) taskAddCmd() *cobra.Command {
	var title, desc, typ, prio, status, deps, origin string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a backlog item",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.write(func() ([]store.Op, error) {
				if origin == "" {
					return nil, &store.ValidationError{Msg: "--origin is required: user (the developer asked) or agent (your own idea, approved by the developer)"}
				}
				o, err := model.ParseOrigin(origin)
				if err != nil {
					return nil, &store.ValidationError{Msg: err.Error()}
				}
				return []store.Op{store.TaskAdd{Title: title, Description: desc, Type: model.TaskType(typ), Priority: model.Priority(prio),
					Status: model.TaskStatus(status), DependsOn: splitList(deps), Origin: o}}, nil
			}, firstCreated)
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "short title")
	f.StringVar(&desc, "description", "", "longer description")
	f.StringVar(&typ, "type", "", "idea|feature|bug|improvement|tech-debt|research|experiment (default feature)")
	f.StringVar(&prio, "priority", "", "high|medium|low (default medium)")
	f.StringVar(&status, "status", "", "idea|planned|in_progress|blocked (default planned)")
	f.StringVar(&deps, "depends-on", "", "comma-separated task IDs")
	f.StringVar(&origin, "origin", "", "user|agent")
	return cmd
}

func (a *App) taskUpdateCmd() *cobra.Command {
	var title, desc, typ, prio, status, deps string
	cmd := &cobra.Command{
		Use:   "update T-n",
		Short: "Change fields of a backlog item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.write(func() ([]store.Op, error) {
				op := store.TaskUpdate{ID: args[0]}
				f := cmd.Flags()
				if f.Changed("title") {
					op.Title = &title
				}
				if f.Changed("description") {
					op.Description = &desc
				}
				if f.Changed("type") {
					v := model.TaskType(typ)
					op.Type = &v
				}
				if f.Changed("priority") {
					v := model.Priority(prio)
					op.Priority = &v
				}
				if f.Changed("status") {
					v := model.TaskStatus(status)
					op.Status = &v
				}
				if f.Changed("depends-on") {
					v := splitList(deps)
					op.DependsOn = &v
				}
				return []store.Op{op}, nil
			}, ok)
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "new title")
	f.StringVar(&desc, "description", "", "new description")
	f.StringVar(&typ, "type", "", "new type")
	f.StringVar(&prio, "priority", "", "new priority")
	f.StringVar(&status, "status", "", "idea|planned|in_progress|blocked|completed|cancelled")
	f.StringVar(&deps, "depends-on", "", "comma-separated task IDs (empty clears)")
	return cmd
}

func (a *App) taskListCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List backlog items in rank order",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := a.open("")
			if err != nil {
				return err
			}
			snap, err := pc.Store.Load()
			if err != nil {
				return err
			}
			tasks := []model.Task{}
			for _, t := range byRank(snap.Backlog.Items) {
				if all || t.IsOpen() {
					tasks = append(tasks, t)
				}
			}
			if a.jsonOut {
				return a.printJSON(tasks)
			}
			for _, t := range tasks {
				fmt.Fprintf(a.Out, "%-6s%-12s%-7s%s\n", t.ID, t.Status, t.Priority, t.Title)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include completed and cancelled items")
	return cmd
}

func (a *App) taskReorderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reorder T-n...",
		Short: "Move the listed tasks to the top of the backlog, in that order",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.write(func() ([]store.Op, error) { return []store.Op{store.TaskReorder{IDs: args}}, nil }, ok)
		},
	}
}

func (a *App) decisionAddCmd() *cobra.Command {
	var title, ctx, decision, rationale, supersedes string
	var options []string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Record an open decision (with options) or a decision made",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.write(func() ([]store.Op, error) {
				op := store.DecisionAdd{Title: title, Context: ctx, Decision: decision, Rationale: rationale, Supersedes: supersedes}
				for _, o := range options {
					name, trade := splitPair(o)
					op.Options = append(op.Options, model.Option{Name: name, Tradeoffs: trade})
				}
				return []store.Op{op}, nil
			}, firstCreated)
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "the question or decision")
	f.StringVar(&ctx, "context", "", "background")
	f.StringArrayVar(&options, "option", nil, `"name: tradeoffs" (repeatable)`)
	f.StringVar(&decision, "decision", "", "what was decided (omit for an open decision)")
	f.StringVar(&rationale, "rationale", "", "why")
	f.StringVar(&supersedes, "supersedes", "", "decision ID this replaces")
	return cmd
}

func (a *App) decisionResolveCmd() *cobra.Command {
	var decision, rationale string
	cmd := &cobra.Command{
		Use:   "resolve D-n",
		Short: "Close an open decision",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.write(func() ([]store.Op, error) {
				return []store.Op{store.DecisionResolve{ID: args[0], Decision: decision, Rationale: rationale}}, nil
			}, ok)
		},
	}
	cmd.Flags().StringVar(&decision, "decision", "", "what was decided")
	cmd.Flags().StringVar(&rationale, "rationale", "", "why")
	return cmd
}

func (a *App) architectureChangeCmd() *cobra.Command {
	var summary, affects string
	cmd := &cobra.Command{
		Use:   "change",
		Short: "Record a change and flag the tasks it may affect for review",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.write(func() ([]store.Op, error) {
				return []store.Op{store.ArchitectureChange{Summary: summary, Affects: splitList(affects)}}, nil
			}, ok)
		},
	}
	cmd.Flags().StringVar(&summary, "summary", "", "what changed")
	cmd.Flags().StringVar(&affects, "affects", "", "comma-separated task IDs that may be affected")
	return cmd
}

func (a *App) reviewClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear T-n",
		Short: "Clear a task's review flag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.write(func() ([]store.Op, error) { return []store.Op{store.ReviewClear{TaskID: args[0]}}, nil }, ok)
		},
	}
}

func (a *App) stateGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Print the project state as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := a.open("")
			if err != nil {
				return err
			}
			snap, err := pc.Store.Load()
			if err != nil {
				return err
			}
			return a.printJSON(snap.State)
		},
	}
}

func decodeStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return &store.ValidationError{Msg: "invalid JSON input: " + err.Error()}
	}
	return nil
}

func (a *App) stateSetCmd() *cobra.Command {
	var rev int
	var file string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Replace the project state (requires the revision you read)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.write(func() ([]store.Op, error) {
				b, err := a.readInput(file)
				if err != nil {
					return nil, err
				}
				var in store.StateInput
				if err := decodeStrict(b, &in); err != nil {
					return nil, err
				}
				return []store.Op{store.StateSet{ExpectRevision: rev, State: in}}, nil
			}, ok)
		},
	}
	cmd.Flags().IntVar(&rev, "expect-revision", 0, "the revision from `hopback state get`")
	cmd.Flags().StringVar(&file, "file", "", "state JSON file (- for stdin)")
	cmd.MarkFlagRequired("expect-revision")
	cmd.MarkFlagRequired("file")
	return cmd
}

func (a *App) nextSetCmd() *cobra.Command {
	var reason string
	var alts []string
	cmd := &cobra.Command{
		Use:   "set T-n",
		Short: "Record the recommended next task and alternatives",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.write(func() ([]store.Op, error) {
				op := store.NextSet{TaskID: args[0], Reason: reason}
				for _, s := range alts {
					id, why := splitPair(s)
					op.Alternatives = append(op.Alternatives, model.Pick{TaskID: id, Reason: why})
				}
				return []store.Op{op}, nil
			}, ok)
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why this task next")
	cmd.Flags().StringArrayVar(&alts, "alt", nil, `"T-m: reason" (repeatable)`)
	return cmd
}

func (a *App) batchCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "batch",
		Short: "Apply a JSON array of operations, all or nothing",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.write(func() ([]store.Op, error) {
				b, err := a.readInput(file)
				if err != nil {
					return nil, err
				}
				return store.DecodeBatch(bytes.NewReader(b))
			}, ok)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "operations JSON file (- for stdin)")
	cmd.MarkFlagRequired("file")
	return cmd
}

func (a *App) sessionParkCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "park",
		Short: "Validate the handoff and park the current session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := a.open("")
			if err != nil {
				return err
			}
			b, err := a.readInput(file)
			if err != nil {
				return err
			}
			_, cur, err := pc.Sessions.Current()
			if err != nil {
				return err
			}
			if cur == nil {
				return &store.ValidationError{Msg: "parking incomplete: Hopback can't tell which session this is (no HOPBACK_SESSION and no agent session ID in the environment)"}
			}
			if err := pc.Sessions.Park(cur.ID, b); err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(map[string]any{"ok": true, "session": cur.ID})
			}
			_, err = fmt.Fprintf(a.Out, "Parked session %s.\n", cur.ID)
			return err
		},
	}
	cmd.Flags().StringVar(&file, "handoff-file", "", "handoff Markdown file (- for stdin)")
	cmd.MarkFlagRequired("handoff-file")
	return cmd
}

func (a *App) sessionCurrentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Print the session this command runs in, as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := a.open("")
			if err != nil {
				return err
			}
			attr, cur, err := pc.Sessions.Current()
			if err != nil {
				return err
			}
			if cur == nil {
				return a.printJSON(attr)
			}
			return a.printJSON(cur)
		},
	}
}
