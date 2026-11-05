package brief

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
	"github.com/NandavardhanRadhakrishnan/HopBack/internal/store"
)

var fixed = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func tp(t time.Time) *time.Time { return &t }

func midFeatureInput() Input {
	parked := fixed.AddDate(0, 0, -12)
	return Input{
		Project: model.Project{Name: "Docslet"},
		Now:     fixed,
		Sessions: []model.Session{
			{ID: "b1", Role: "build", Agent: "claude-code", Status: "parked", StartedAt: parked, ParkedAt: tp(parked)},
			{ID: "c1", Role: "concierge", Agent: "claude-code", Status: "completed", StartedAt: fixed.AddDate(0, 0, -1)},
		},
		AgentNames: map[string]string{"claude-code": "Claude Code"},
		Snapshot: store.Snapshot{
			Backlog: model.Backlog{Items: []model.Task{
				{ID: "T-9", Title: "Sharing API", Status: "planned", Priority: "medium", Rank: 1, NeedsReview: &model.Review{Reason: "Auth moved to sessions"}},
				{ID: "T-12", Title: "Build the diff view", Status: "in_progress", Priority: "high", Rank: 2},
			}},
			Decisions: model.Decisions{Items: []model.Decision{{ID: "D-4", Title: "Store full revisions or only diffs?", Status: "open"}}},
			State: model.State{
				Revision: 3, Mode: "mid_feature",
				BottomLine: "Version history is half built. Storage works; diff view not started.",
				CurrentFeature: &model.CurrentFeature{TaskID: "T-12", Why: "Users need to restore old drafts after bad edits.",
					StoppedAt: "Revision table + save hook done.", NextStep: "Diff endpoint."},
				RecommendedNext: &model.Pick{TaskID: "T-12", Reason: "compare before restoring"},
			},
		},
	}
}

func betweenFeaturesInput() Input {
	var items []model.Task
	for i := 1; i <= 4; i++ {
		items = append(items, model.Task{ID: fmt.Sprintf("T-%d", i), Title: fmt.Sprintf("C%d", i), Status: "completed", Rank: i,
			CompletedAt: tp(fixed.Add(time.Duration(i-5) * time.Hour))})
	}
	for i := 1; i <= 7; i++ {
		p := model.Priority("low")
		if i%2 == 0 {
			p = "high"
		}
		items = append(items, model.Task{ID: fmt.Sprintf("T-%d", 4+i), Title: fmt.Sprintf("O%d", i), Status: "planned", Priority: p, Rank: 4 + i})
	}
	// Shuffle storage order so rank, not slice order, drives "Still left".
	items[4], items[10] = items[10], items[4]
	return Input{
		Project:    model.Project{Name: "Docslet"},
		Now:        fixed,
		Sessions:   []model.Session{{ID: "b1", Role: "build", Agent: "claude-code", Status: "parked", StartedAt: fixed.Add(-time.Hour), ParkedAt: tp(fixed.Add(-time.Hour))}},
		AgentNames: map[string]string{"claude-code": "Claude Code"},
		Snapshot: store.Snapshot{
			Backlog: model.Backlog{Items: items},
			State:   model.State{Revision: 1, Mode: "between_features", BottomLine: "Auth, upload and sharing are done."},
		},
	}
}

func TestRender_MidFeatureGolden(t *testing.T) {
	var buf bytes.Buffer
	Render(&buf, Build(midFeatureInput()))
	want := "Docslet · status brief · last worked 12 days ago · Claude Code\n\n" +
		"BOTTOM LINE\n  Version history is half built. Storage works; diff view not started.\n\n" +
		"NEEDS YOUR DECISION\n  D-4   Store full revisions or only diffs?\n  T-9   Sharing API (review: Auth moved to sessions)\n\n" +
		"RECOMMENDED NEXT\n  T-12  Build the diff view: compare before restoring\n\n" +
		"BACKGROUND\n  Why:         Users need to restore old drafts after bad edits.\n  Stopped at:  Revision table + save hook done.\n  Next step:   Diff endpoint.\n"
	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestBuild_BetweenFeatures(t *testing.T) {
	in := betweenFeaturesInput()
	b := Build(in)
	if !reflect.DeepEqual(b.RecentlyDone, []string{"C4", "C3", "C2"}) || b.StillLeft != "7 open (3 high): O1, O2, O3 ..." {
		t.Fatal(b)
	}
	var buf bytes.Buffer
	Render(&buf, b)
	if !strings.Contains(buf.String(), "  Recently done:  C4 · C3 · C2\n  Still left:     7 open (3 high): O1, O2, O3 ...\n") {
		t.Fatal(buf.String())
	}
}

func TestBuild_InterruptedWarning(t *testing.T) {
	in := midFeatureInput()
	in.Sessions[0].Status = "interrupted"
	in.Sessions[0].StartedAt = fixed.AddDate(0, 0, -3)
	in.Sessions[0].ParkedAt = nil
	var buf bytes.Buffer
	Render(&buf, Build(in))
	if !strings.HasPrefix(buf.String(), "! Last session ended without a handoff (Claude Code, 3 days ago)\n") {
		t.Fatal(buf.String())
	}
}

func TestBuild_Empty(t *testing.T) {
	in := Input{Project: model.Project{Name: "New"}, Now: fixed}
	var buf bytes.Buffer
	Render(&buf, Build(in))
	if !strings.Contains(buf.String(), "No brief yet") || !strings.Contains(buf.String(), "concierge") {
		t.Fatal(buf.String())
	}
}

func TestAgo(t *testing.T) {
	for days, want := range map[int]string{0: "today", 1: "yesterday", 5: "5 days ago", 13: "13 days ago", 14: "2 weeks ago", 60: "2 months ago", 400: "1 year ago", 800: "2 years ago"} {
		if got := Ago(fixed, fixed.AddDate(0, 0, -days)); got != want {
			t.Fatalf("%d: %s", days, got)
		}
	}
}

func TestChangesSince(t *testing.T) {
	evs := []model.Event{{TS: fixed.Add(-time.Hour), Type: "task_created", Agent: "a", Data: map[string]any{"task_id": "T-1", "title": "Old"}},
		{TS: fixed.Add(time.Hour), Type: "task_created", Agent: "claude-code", Data: map[string]any{"task_id": "T-7", "title": "Auth"}},
		{TS: fixed.Add(2 * time.Hour), Type: "decision_recorded", Agent: "a", Data: map[string]any{"decision_id": "D-1"}},
		{TS: fixed.Add(3 * time.Hour), Type: "next_set", Agent: "a", Data: map[string]any{"task_id": "T-7"}}}
	got := ChangesSince(evs, fixed, 2)
	want := []string{`2026-11-05  task_created T-7 "Auth" (claude-code)`, `2026-11-05  decision_recorded D-1 (a)`, "... and 1 more"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}

func TestSummary(t *testing.T) {
	s := Summary(midFeatureInput())
	if s.LastWorked != "12 days ago" || s.Open != 2 || s.NeedsReview != 1 || !strings.HasPrefix(s.Headline, "Version history is half built.") || !strings.HasSuffix(s.Headline, "...") {
		t.Fatal(s)
	}
}
