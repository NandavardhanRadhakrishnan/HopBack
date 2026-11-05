// Package brief renders the status brief from a project's files. It is plain
// formatting of agent-written content; nothing here generates text.
package brief

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/store"
)

type Input struct {
	Project    model.Project
	Snapshot   store.Snapshot
	Sessions   []model.Session
	Now        time.Time
	AgentNames map[string]string
}

type Brief struct {
	ProjectName, LastWorked, LastAgent, Warning string
	Empty                                       bool
	Mode                                        model.Mode
	BottomLine                                  string
	NeedsDecision                               []string
	RecommendedNext                             string
	Why, StoppedAt, NextStep                    string
	RecentlyDone                                []string
	StillLeft                                   string
}

type ProjectSummary struct {
	Status            model.ProjectStatus
	LastWorked        string
	Headline          string
	Open, NeedsReview int
}

// LatestBuild returns the build session with the most recent activity, or nil.
func LatestBuild(sessions []model.Session) *model.Session {
	var last *model.Session
	for i := range sessions {
		s := &sessions[i]
		if s.Role == "build" && (last == nil || s.LastActivity().After(last.LastActivity())) {
			last = s
		}
	}
	return last
}

func byRank(items []model.Task) []model.Task {
	out := append([]model.Task(nil), items...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out
}

func title(items []model.Task, id string) string {
	for _, t := range items {
		if t.ID == id {
			return t.Title
		}
	}
	return ""
}

func Build(in Input) Brief {
	snap := in.Snapshot
	b := Brief{ProjectName: in.Project.Name, Empty: snap.State.Revision == 0, Mode: snap.State.Mode, BottomLine: snap.State.BottomLine}
	if last := LatestBuild(in.Sessions); last != nil {
		b.LastWorked = Ago(in.Now, last.LastActivity())
		b.LastAgent = last.Agent
		if n, ok := in.AgentNames[last.Agent]; ok {
			b.LastAgent = n
		}
		if last.Status == "interrupted" {
			b.Warning = fmt.Sprintf("! Last session ended without a handoff (%s, %s)", b.LastAgent, b.LastWorked)
		}
	}
	for _, d := range snap.Decisions.Items {
		if d.Status == "open" {
			b.NeedsDecision = append(b.NeedsDecision, fmt.Sprintf("%-6s%s", d.ID, d.Title))
		}
	}
	tasks := byRank(snap.Backlog.Items)
	for _, t := range tasks {
		if t.NeedsReview != nil {
			b.NeedsDecision = append(b.NeedsDecision, fmt.Sprintf("%-6s%s (review: %s)", t.ID, t.Title, t.NeedsReview.Reason))
		}
	}
	for _, bl := range snap.State.Blockers {
		b.NeedsDecision = append(b.NeedsDecision, "Blocked: "+bl)
	}
	if p := snap.State.RecommendedNext; p != nil {
		b.RecommendedNext = fmt.Sprintf("%-6s%s: %s", p.TaskID, title(tasks, p.TaskID), p.Reason)
	}
	if cf := snap.State.CurrentFeature; cf != nil {
		b.Why, b.StoppedAt, b.NextStep = cf.Why, cf.StoppedAt, cf.NextStep
	}
	var done []model.Task
	var open []string
	high := 0
	for _, t := range tasks {
		switch {
		case t.Status == "completed" && t.CompletedAt != nil:
			done = append(done, t)
		case t.IsOpen():
			open = append(open, t.Title)
			if t.Priority == "high" {
				high++
			}
		}
	}
	sort.SliceStable(done, func(i, j int) bool { return done[i].CompletedAt.After(*done[j].CompletedAt) })
	for i := 0; i < len(done) && i < 3; i++ {
		b.RecentlyDone = append(b.RecentlyDone, done[i].Title)
	}
	if len(open) > 0 {
		top := open[:min(3, len(open))]
		b.StillLeft = fmt.Sprintf("%d open (%d high): %s", len(open), high, strings.Join(top, ", "))
		if len(open) > 3 {
			b.StillLeft += " ..."
		}
	}
	return b
}

func section(w io.Writer, heading string, lines ...string) {
	var kept []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			kept = append(kept, l)
		}
	}
	if len(kept) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\n", heading)
	for _, l := range kept {
		fmt.Fprintf(w, "  %s\n", l)
	}
}

func labeled(width int, label, value string) string {
	if value == "" {
		return ""
	}
	return fmt.Sprintf("%-*s%s", width, label, value)
}

func Render(w io.Writer, b Brief) {
	if b.Warning != "" {
		fmt.Fprintln(w, b.Warning)
	}
	header := b.ProjectName + " · status brief"
	if b.LastWorked != "" {
		header += " · last worked " + b.LastWorked + " · " + b.LastAgent
	}
	fmt.Fprintln(w, header)
	if b.Empty {
		fmt.Fprintln(w, "\nNo brief yet. Talk to the concierge to plan, or start building.")
		section(w, "NEEDS YOUR DECISION", b.NeedsDecision...)
		return
	}
	section(w, "BOTTOM LINE", b.BottomLine)
	section(w, "NEEDS YOUR DECISION", b.NeedsDecision...)
	section(w, "RECOMMENDED NEXT", b.RecommendedNext)
	if b.Mode == "mid_feature" {
		section(w, "BACKGROUND", labeled(13, "Why:", b.Why), labeled(13, "Stopped at:", b.StoppedAt), labeled(13, "Next step:", b.NextStep))
	} else {
		section(w, "BACKGROUND", labeled(16, "Recently done:", strings.Join(b.RecentlyDone, " · ")), labeled(16, "Still left:", b.StillLeft))
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

// Summary is the one-row view of a project used by `hopback projects`.
func Summary(in Input) ProjectSummary {
	b := Build(in)
	s := ProjectSummary{Status: in.Snapshot.State.ProjectStatus, LastWorked: b.LastWorked}
	switch {
	case b.BottomLine != "":
		s.Headline = truncate(b.BottomLine, 60)
	case b.RecommendedNext != "":
		s.Headline = truncate("next: "+b.RecommendedNext, 60)
	}
	for _, t := range in.Snapshot.Backlog.Items {
		if t.IsOpen() {
			s.Open++
		}
		if t.NeedsReview != nil {
			s.NeedsReview++
		}
	}
	return s
}

// Ago describes how long ago t was, neutrally.
func Ago(now, t time.Time) string {
	d := int(now.Sub(t).Hours() / 24)
	switch {
	case d < 1:
		return "today"
	case d == 1:
		return "yesterday"
	case d < 14:
		return fmt.Sprintf("%d days ago", d)
	case d < 60:
		return fmt.Sprintf("%d weeks ago", d/7)
	case d < 365:
		return fmt.Sprintf("%d months ago", d/30)
	case d < 730:
		return "1 year ago"
	}
	return fmt.Sprintf("%d years ago", d/365)
}
