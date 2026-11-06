package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
)

func TestTaskAddAndList(t *testing.T) {
	app, _, _ := newApp(t, nil)
	if out, e, code := run(app, "", "task", "add", "--title", "Auth", "--origin", "user"); code != 0 || out != "T-1\n" {
		t.Fatal(out, e)
	}
	out, _, _ := run(app, "", "task", "list", "--json")
	var ts []model.Task
	json.Unmarshal([]byte(out), &ts)
	if len(ts) != 1 || ts[0].Title != "Auth" || ts[0].Origin != "user_requested" {
		t.Fatal(out)
	}
}

func TestTaskAdd_InvalidType(t *testing.T) {
	app, _, _ := newApp(t, nil)
	out, errOut, code := run(app, "", "task", "add", "--title", "X", "--origin", "user", "--type", "chore")
	if code != 1 || out != "" || !strings.Contains(errOut, "chore") || !strings.Contains(errOut, "feature") {
		t.Fatal(code, errOut)
	}
}

func TestTaskUpdate_OnlyChangedFlags(t *testing.T) {
	app, _, root := newApp(t, nil)
	run(app, "", "task", "add", "--title", "Auth", "--origin", "user")
	if _, e, code := run(app, "", "task", "update", "T-1", "--status", "completed"); code != 0 {
		t.Fatal(e)
	}
	it := loadTask(t, root, "T-1")
	if it.Status != "completed" || it.Title != "Auth" || it.Priority != "medium" {
		t.Fatal(it)
	}
}

func TestDecisionAddOptions(t *testing.T) {
	app, _, root := newApp(t, nil)
	if _, e, code := run(app, "", "decision", "add", "--title", "Store diffs?", "--option", "full: simple, more disk", "--option", "diffs: compact"); code != 0 {
		t.Fatal(e)
	}
	d := loadDecision(t, root, "D-1")
	if d.Status != "open" || d.Options[0] != (model.Option{Name: "full", Tradeoffs: "simple, more disk"}) || len(d.Options) != 2 {
		t.Fatal(d)
	}
}

func TestArchitectureChangeCLI(t *testing.T) {
	app, _, root := newApp(t, nil)
	run(app, "", "task", "add", "--title", "API", "--origin", "user")
	if _, e, code := run(app, "", "architecture", "change", "--summary", "Auth moved", "--affects", "T-1"); code != 0 {
		t.Fatal(e)
	}
	if loadTask(t, root, "T-1").NeedsReview == nil {
		t.Fatal("not flagged")
	}
}

func TestStateSetStdinAndStale(t *testing.T) {
	app, _, _ := newApp(t, nil)
	in := `{"mode":"between_features","bottom_line":"Auth done."}`
	if _, e, code := run(app, in, "state", "set", "--expect-revision", "0", "--file", "-"); code != 0 {
		t.Fatal(e)
	}
	out, _, _ := run(app, "", "state", "get")
	if !strings.Contains(out, `"revision": 1`) {
		t.Fatal(out)
	}
	if _, e, code := run(app, in, "state", "set", "--expect-revision", "0", "--file", "-"); code != 1 || !strings.Contains(e, "revision 1") {
		t.Fatal(e)
	}
}

func TestNextSetCLI(t *testing.T) {
	app, _, root := newApp(t, nil)
	run(app, "", "task", "add", "--title", "A", "--origin", "user")
	run(app, "", "task", "add", "--title", "B", "--origin", "user")
	if _, e, code := run(app, "", "next", "set", "T-1", "--reason", "R", "--alt", "T-2: maybe"); code != 0 {
		t.Fatal(e)
	}
	st := loadState(t, root)
	if st.RecommendedNext.TaskID != "T-1" || st.Alternatives[0] != (model.Pick{TaskID: "T-2", Reason: "maybe"}) {
		t.Fatal(st)
	}
}

func TestBatchCLI(t *testing.T) {
	app, _, root := newApp(t, nil)
	if _, e, code := run(app, `[{"op":"task_add","title":"A","origin":"user_requested"},{"op":"task_add","title":"B","origin":"agent_suggested"}]`, "batch", "--file", "-"); code != 0 {
		t.Fatal(e)
	}
	if loadTask(t, root, "T-2").Origin != "agent_suggested" {
		t.Fatal("batch not applied")
	}
}

func TestSessionParkCLI(t *testing.T) {
	app, _, _ := newApp(t, map[string]string{"FAKE_AGENT_SESSION_ID": "n1"})
	cur, _, _ := run(app, "", "session", "current")
	if !strings.Contains(cur, `"native_session_id": "n1"`) {
		t.Fatal(cur)
	}
	if _, e, code := run(app, "## Objective\nx\n", "session", "park", "--handoff-file", "-"); code != 1 || !strings.Contains(e, "handoff section: Done") {
		t.Fatal(e)
	}
	run(app, `{"mode":"between_features","bottom_line":"x"}`, "state", "set", "--expect-revision", "0", "--file", "-")
	if out, e, code := run(app, goodHandoff, "session", "park", "--handoff-file", "-"); code != 0 || !strings.HasPrefix(out, "Parked session ") {
		t.Fatal(e)
	}
}

func TestContextJSON(t *testing.T) {
	app, _, _ := newApp(t, nil)
	out, _, _ := run(app, "", "context")
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"project", "state", "open_tasks", "needs_review", "open_decisions", "latest_handoff", "current_session"} {
		if _, ok := m[k]; !ok {
			t.Fatal(k)
		}
	}
}

func TestWritesAttributedToDetectedSession(t *testing.T) {
	app, _, root := newApp(t, map[string]string{"FAKE_AGENT_SESSION_ID": "n1"})
	run(app, "", "task", "add", "--title", "A", "--origin", "user")
	it := loadTask(t, root, "T-1")
	if it.SourceSession == "" || it.RecordedBy != "fake-agent" {
		t.Fatal(it)
	}
}

func TestOutsideProject(t *testing.T) {
	app, _, _ := newApp(t, nil)
	app.WorkDir = t.TempDir()
	if _, e, code := run(app, "", "task", "list"); code != 1 || !strings.Contains(e, "hopback init") {
		t.Fatal(e)
	}
}

func TestStateSet_AcceptsStateGetOutput(t *testing.T) {
	app, _, root := newApp(t, nil)
	if _, e, code := run(app, `{"mode":"between_features","bottom_line":"Auth done."}`, "state", "set", "--expect-revision", "0", "--file", "-"); code != 0 {
		t.Fatal(e)
	}
	got, _, _ := run(app, "", "state", "get")
	edited := strings.Replace(got, "Auth done.", "Auth shipped.", 1)
	if _, e, code := run(app, edited, "state", "set", "--expect-revision", "1", "--file", "-"); code != 0 {
		t.Fatal(e)
	}
	if st := loadState(t, root); st.BottomLine != "Auth shipped." || st.Revision != 2 {
		t.Fatal(st)
	}
}
