// Package cli is the hopback command line. Commands are thin: they parse
// input, call the core packages and print results.
package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/agent"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/agent/claude"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/project"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/registry"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/session"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/store"
)

type App struct {
	WorkDir, Home string
	Env           func(string) string
	In            io.Reader
	Out, Err      io.Writer
	Adapter       agent.Adapter
	Now           func() time.Time

	in      *bufio.Reader // shared reader over In for prompts and stdin input
	jsonOut bool
}

// Execute runs hopback with the real environment and returns the exit code.
func Execute() int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	home, err := registry.Home(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	app := &App{WorkDir: wd, Home: home, Env: os.Getenv, In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Adapter: claude.New(), Now: time.Now}
	return app.Run(os.Args[1:])
}

// Run executes one command line. Errors print as "error: <msg>" with exit code 1.
func (a *App) Run(args []string) int {
	cmd := NewRootCmd(a)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(a.Err, "error: %v\n", err)
		return 1
	}
	return 0
}

func NewRootCmd(app *App) *cobra.Command {
	app.in = bufio.NewReader(app.In)
	app.jsonOut = false
	root := &cobra.Command{
		Use:           "hopback",
		Short:         "Keep track of where your projects stand across agent sessions",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}
	root.SetIn(app.In)
	root.SetOut(app.Out)
	root.SetErr(app.Err)
	root.PersistentFlags().BoolVar(&app.jsonOut, "json", false, "print machine-readable JSON")
	addAgentCommands(root, app)
	addHumanCommands(root, app)
	return root
}

type projectCtx struct {
	Root     string
	Store    *store.Store
	Sessions *session.Manager
}

// open resolves a project (the current one, or a registered one by name or
// ID), refreshes Hopback-owned files in it, and returns its store and sessions.
func (a *App) open(projectArg string) (*projectCtx, error) {
	var root string
	if projectArg == "" {
		r, err := project.Find(a.WorkDir)
		if err != nil {
			return nil, err
		}
		root = r
	} else {
		e, err := registry.Lookup(a.Home, projectArg)
		if err != nil {
			return nil, err
		}
		root = e.Path
	}
	if err := project.EnsureTemplates(root); err != nil {
		return nil, err
	}
	if err := a.Adapter.Install(root); err != nil && !errors.Is(err, agent.ErrNotOwned) {
		return nil, err
	}
	st, err := store.Open(root)
	if err != nil {
		return nil, err
	}
	st.Now = a.Now
	m := session.NewManager(st, a.Env, session.Detector{Agent: a.Adapter.Name(), Detect: a.Adapter.DetectSession})
	return &projectCtx{Root: root, Store: st, Sessions: m}, nil
}

func (a *App) printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Out, "%s\n", b)
	return err
}

// readInput reads a file, or the command's stdin when path is "-".
func (a *App) readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(a.in)
	}
	return os.ReadFile(path)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitPair splits "name: rest" on the first colon, trimming both halves.
func splitPair(s string) (string, string) {
	k, v, _ := strings.Cut(s, ":")
	return strings.TrimSpace(k), strings.TrimSpace(v)
}
