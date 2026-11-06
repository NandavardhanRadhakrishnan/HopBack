package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/NandavardhanRadhakrishnan/HopBack/internal/model"
)

func addTasks(t *testing.T, st *Store, titles ...string) {
	t.Helper()
	for _, n := range titles {
		ok(t)(st.Apply(attr, TaskAdd{Title: n, Origin: "user_requested"}))
	}
}

func TestDecisionAdd_OpenDecidedSupersedes(t *testing.T) {
	st := newStore(t)
	ok(t)(st.Apply(attr, DecisionAdd{Title: "Store diffs?", Options: []model.Option{{Name: "full", Tradeoffs: "simple"}, {Name: "diffs", Tradeoffs: "compact"}}}))
	ok(t)(st.Apply(attr, DecisionAdd{Title: "Use diffs", Decision: "diffs", Rationale: "disk", Supersedes: "D-1"}))
	s, _ := st.Load()
	d1, d2 := s.Decisions.Items[0], s.Decisions.Items[1]
	if d1.Status != "superseded" || len(d1.Options) != 2 || d2.ID != "D-2" || d2.Status != "decided" || d2.DecidedAt == nil {
		t.Fatal(d1, d2)
	}
}

func TestDecisionResolve(t *testing.T) {
	st := newStore(t)
	ok(t)(st.Apply(attr, DecisionAdd{Title: "Q"}))
	ok(t)(st.Apply(attr, DecisionResolve{ID: "D-1", Decision: "yes"}))
	if _, err := st.Apply(attr, DecisionResolve{ID: "D-1", Decision: "again"}); err == nil {
		t.Fatal("resolved twice")
	}
}

func TestArchitectureChange_FlagsOnlyAffected(t *testing.T) {
	st := newStore(t)
	addTasks(t, st, "A", "B")
	res, _ := st.Apply(attr, ArchitectureChange{Summary: "Auth moved to sessions", Affects: []string{"T-2"}})
	s, _ := st.Load()
	a, b := s.Backlog.Items[0], s.Backlog.Items[1]
	if a.NeedsReview != nil || b.NeedsReview == nil || b.NeedsReview.Reason != "Auth moved to sessions" ||
		b.NeedsReview.EventID != res.Events[0].ID || b.Status != "planned" {
		t.Fatal(a, b)
	}
}

func TestReviewClear(t *testing.T) {
	st := newStore(t)
	addTasks(t, st, "A")
	ok(t)(st.Apply(attr, ArchitectureChange{Summary: "x", Affects: []string{"T-1"}}))
	ok(t)(st.Apply(attr, ReviewClear{TaskID: "T-1"}))
	if _, err := st.Apply(attr, ReviewClear{TaskID: "T-1"}); err == nil {
		t.Fatal("cleared unflagged task")
	}
}

func TestStateSet_StaleRevision(t *testing.T) {
	st := newStore(t)
	in := StateInput{Mode: "between_features", BottomLine: "Auth done."}
	ok(t)(st.Apply(attr, StateSet{ExpectRevision: 0, State: in}))
	s, _ := st.Load()
	if s.State.Revision != 1 || s.State.UpdatedBy.Session != "s1" {
		t.Fatal(s.State)
	}
	_, err := st.Apply(attr, StateSet{ExpectRevision: 0, State: in})
	var se *StaleError
	if !errors.As(err, &se) || se.Current != 1 || !strings.Contains(err.Error(), "revision 1") || !strings.Contains(err.Error(), "hopback state get") {
		t.Fatal(err)
	}
}

func TestStateSet_Validation(t *testing.T) {
	st := newStore(t)
	for _, in := range []StateInput{{BottomLine: "x"}, {Mode: "mid_feature", BottomLine: "x"}, {Mode: "between_features"},
		{Mode: "between_features", BottomLine: "x", RecommendedNext: &model.Pick{TaskID: "T-9", Reason: "r"}}} {
		if _, err := st.Apply(attr, StateSet{State: in}); err == nil {
			t.Fatalf("accepted %+v", in)
		}
	}
}

func TestNextSet(t *testing.T) {
	st := newStore(t)
	addTasks(t, st, "A", "B")
	ok(t)(st.Apply(attr, NextSet{TaskID: "T-1", Reason: "unblocks B", Alternatives: []model.Pick{{TaskID: "T-2", Reason: "smaller"}}}))
	s, _ := st.Load()
	evs, _ := st.Events()
	if s.State.RecommendedNext.TaskID != "T-1" || len(s.State.Alternatives) != 1 || s.State.Revision != 1 || evs[len(evs)-1].Type != "next_set" {
		t.Fatal(s.State)
	}
}

func TestDecodeBatch(t *testing.T) {
	ops, err := DecodeBatch(strings.NewReader(`[{"op":"task_add","title":"A","origin":"user_requested"},{"op":"task_reorder","ids":["T-1"]}]`))
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	if _, ok := ops[0].(TaskAdd); !ok {
		t.Fatalf("%T", ops[0])
	}
	if _, err := DecodeBatch(strings.NewReader(`[{"op":"task_add","title":"A","origin":"user_requested"},{"op":"task_delete"}]`)); err == nil ||
		!strings.Contains(err.Error(), `batch item 1: unknown op "task_delete"`) {
		t.Fatal(err)
	}
}

func TestBatch_AllOrNothing(t *testing.T) {
	st := newStore(t)
	ops, _ := DecodeBatch(strings.NewReader(`[{"op":"task_add","title":"A","origin":"user_requested"},{"op":"review_clear","task_id":"T-1"}]`))
	if _, err := st.Apply(attr, ops...); err == nil {
		t.Fatal("want error")
	}
	if s, _ := st.Load(); len(s.Backlog.Items) != 0 {
		t.Fatal("partial batch applied")
	}
}

func TestDecodeBatch_StateSetAcceptsStateGetFields(t *testing.T) {
	st := newStore(t)
	ops, err := DecodeBatch(strings.NewReader(`[{"op":"state_set","expect_revision":0,"state":{"schema_version":1,"revision":0,"updated_at":"2026-11-05T12:00:00Z","updated_by":{"session":"x","agent":"y"},"project_status":"active","mode":"between_features","bottom_line":"x"}}]`))
	if err != nil {
		t.Fatal(err)
	}
	ok(t)(st.Apply(attr, ops...))
	if s, _ := st.Load(); s.State.Revision != 1 || s.State.UpdatedBy.Session != "s1" {
		t.Fatal(s.State)
	}
}
